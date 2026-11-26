package sender

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sort"
	"time"
)

type OutboxWorker struct {
	DB          *sql.DB
	Dispatcher  *Dispatcher
	Interval    time.Duration
	MaxAttempts int
	Now         func() time.Time
}

var backoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

func (w *OutboxWorker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *OutboxWorker) Run(ctx context.Context) {
	t := time.NewTicker(w.Interval)
	defer t.Stop()
	for {
		if n, err := w.ProcessOnce(ctx); err != nil {
			slog.Error("outbox", "err", err)
		} else if n > 0 {
			slog.Info("outbox: обработано заданий", "n", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type job struct {
	id         int64
	channel    Channel
	title      string
	body       string
	attempts   int
	externalID sql.NullString
}

const (
	batchSize   = 20
	sendTimeout = 15 * time.Second

	lease = batchSize*sendTimeout + time.Minute
)

func (w *OutboxWorker) claim(ctx context.Context) ([]job, error) {
	now := w.now()
	rows, err := w.DB.QueryContext(ctx, `
		WITH due AS (
			SELECT id FROM notification_jobs
			WHERE status = 'pending' AND scheduled_at <= $1
			ORDER BY scheduled_at, id LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE notification_jobs j SET scheduled_at = $3 FROM due WHERE j.id = due.id
		RETURNING j.id, j.channel, j.title, j.body, j.attempts,
			(SELECT uc.external_id FROM user_channels uc
			  WHERE uc.user_id = j.user_id AND uc.channel = j.channel AND uc.status = 'connected')`,
		now, batchSize, now.Add(lease))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.channel, &j.title, &j.body, &j.attempts, &j.externalID); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(a, b int) bool { return jobs[a].id < jobs[b].id })
	return jobs, rows.Err()
}

func (w *OutboxWorker) ProcessOnce(ctx context.Context) (int, error) {
	jobs, err := w.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if err := w.process(ctx, j); err != nil {
			slog.Error("outbox: обновление задания", "job", j.id, "err", err)
		}
	}
	return len(jobs), nil
}

func (w *OutboxWorker) Drain(ctx context.Context) (total int, err error) {
	for ctx.Err() == nil {
		n, err := w.ProcessOnce(ctx)
		total += n
		if err != nil || n == 0 {
			return total, err
		}
	}
	return total, ctx.Err()
}

func (w *OutboxWorker) process(ctx context.Context, j job) error {
	if !j.externalID.Valid {
		return w.markFailed(ctx, j, j.attempts+1, "канал не подключён")
	}
	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	err := w.Dispatcher.Send(sctx, j.channel, j.externalID.String, Message{Title: j.title, Body: j.body})
	cancel()
	switch {
	case err == nil:
		_, err = w.DB.ExecContext(ctx, `UPDATE notification_jobs SET status='sent', attempts=attempts+1, sent_at=$2, last_error=NULL WHERE id=$1`, j.id, w.now())
		return err
	case errors.Is(err, ErrChannelNotConfigured):
		return w.markFailed(ctx, j, j.attempts+1, err.Error())
	}
	attempts := j.attempts + 1
	if attempts >= w.MaxAttempts {
		return w.markFailed(ctx, j, attempts, err.Error())
	}
	retryAt := w.now().Add(backoff[min(attempts-1, len(backoff)-1)])
	_, err2 := w.DB.ExecContext(ctx, `UPDATE notification_jobs SET attempts=$2, last_error=$3, scheduled_at=$4 WHERE id=$1`, j.id, attempts, err.Error(), retryAt)
	return err2
}

func (w *OutboxWorker) markFailed(ctx context.Context, j job, attempts int, reason string) error {
	_, err := w.DB.ExecContext(ctx, `UPDATE notification_jobs SET status='failed', attempts=$2, last_error=$3 WHERE id=$1`, j.id, attempts, reason)
	return err
}
