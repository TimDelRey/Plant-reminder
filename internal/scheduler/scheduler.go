package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"strings"
	"time"

	"plantcare/internal/plants"
)

type Scheduler struct {
	DB         *sql.DB
	NotifyHour int
	Interval   time.Duration
	Now        func() time.Time
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		if n, err := s.Tick(ctx); err != nil {
			slog.Error("scheduler", "err", err)
		} else if n > 0 {
			slog.Info("scheduler: обработано событий", "n", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Scheduler) Drain(ctx context.Context) (total int, err error) {
	for ctx.Err() == nil {
		n, err := s.Tick(ctx)
		total += n
		if err != nil || n == 0 {
			return total, err
		}
	}
	return total, ctx.Err()
}

type dueEvent struct {
	id          int64
	title, desc string
	due         time.Time
	kind        string
	rule        sql.NullString
	channels    string
	plantName   string
	userID      int64
	tz          string
}

func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	now := s.now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT e.id, e.title, e.description, e.next_due_at, e.recurrence_kind, e.recurrence_rule,
			array_to_string(e.channels, ','), p.name, p.user_id, c.timezone
		FROM care_events e
		JOIN plants p ON p.id = e.plant_id
		JOIN users u ON u.id = p.user_id
		JOIN cities c ON c.id = u.city_id
		WHERE e.active AND e.last_notified_for IS DISTINCT FROM e.next_due_at
		  AND NOT (e.recurrence_kind = 'once' AND e.completed_at IS NOT NULL)
		  AND e.next_due_at <= ($1::timestamptz AT TIME ZONE c.timezone)::date
		ORDER BY e.id LIMIT 500 FOR UPDATE OF e SKIP LOCKED`, now)
	if err != nil {
		return 0, err
	}
	var events []dueEvent
	for rows.Next() {
		var e dueEvent
		if err := rows.Scan(&e.id, &e.title, &e.desc, &e.due, &e.kind, &e.rule, &e.channels, &e.plantName, &e.userID, &e.tz); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, e := range events {
		loc, err := time.LoadLocation(e.tz)
		if err != nil {
			loc = time.UTC
		}

		slot := time.Date(e.due.Year(), e.due.Month(), e.due.Day(), s.NotifyHour, 0, 0, 0, loc).UTC()
		scheduled := slot
		if slot.Before(now) {
			scheduled = now.UTC().Truncate(time.Second)
		}

		today := plants.LocalToday(now, e.tz)
		past := e.due.Before(today)
		if !past {
			recipients, err := connectedRecipients(ctx, tx, e.userID)
			if err != nil {
				return 0, err
			}
			title := "🌿 " + e.plantName + " — " + e.title
			for _, ch := range targetChannels(e.channels, recipients) {
				ids := recipients[ch]
				if len(ids) == 0 {
					if err := insertJob(ctx, tx, e, ch, title, scheduled, sql.NullInt64{}); err != nil {
						return 0, err
					}
					continue
				}
				for _, id := range ids {
					if err := insertJob(ctx, tx, e, ch, title, scheduled, sql.NullInt64{Int64: id, Valid: true}); err != nil {
						return 0, err
					}
				}
			}
		}

		if e.kind == "rrule" && e.rule.Valid {
			base := e.due
			if past {
				base = today.AddDate(0, 0, -1)
			}
			next, ok, err := plants.NextAfter(e.rule.String, e.due, base)
			switch {
			case err != nil:
				slog.Error("scheduler: некорректное правило", "event", e.id, "err", err)
				_, err = tx.ExecContext(ctx, `UPDATE care_events SET last_notified_for = $2 WHERE id = $1`, e.id, e.due)
			case !ok:
				_, err = tx.ExecContext(ctx, `UPDATE care_events SET last_notified_for = $2, active = false WHERE id = $1`, e.id, e.due)
			default:
				_, err = tx.ExecContext(ctx, `UPDATE care_events SET last_notified_for = $2, next_due_at = $3 WHERE id = $1`, e.id, e.due, next)
			}
			if err != nil {
				return 0, err
			}
		} else {

			if _, err := tx.ExecContext(ctx, `UPDATE care_events SET last_notified_for = $2 WHERE id = $1`, e.id, e.due); err != nil {
				return 0, err
			}
		}
	}
	return len(events), tx.Commit()
}

func targetChannels(explicit string, recipients map[string][]int64) []string {
	if explicit != "" {
		return strings.Split(explicit, ",")
	}
	out := make([]string, 0, len(recipients))
	for ch := range recipients {
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
}

func insertJob(ctx context.Context, tx *sql.Tx, e dueEvent, channel, title string, scheduled time.Time, recipient sql.NullInt64) error {
	status, lastErr := "pending", sql.NullString{}
	if !recipient.Valid {
		status, lastErr = "failed", sql.NullString{String: "канал не подключён", Valid: true}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_jobs
		(care_event_id, user_id, channel, recipient_id, title, body, scheduled_at, status, last_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
		e.id, e.userID, channel, recipient, title, e.desc, scheduled, status, lastErr)
	return err
}

func connectedRecipients(ctx context.Context, tx *sql.Tx, userID int64) (map[string][]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT channel, id FROM user_channels WHERE user_id = $1 AND status = 'connected' ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string][]int64{}
	for rows.Next() {
		var c string
		var id int64
		if err := rows.Scan(&c, &id); err != nil {
			return nil, err
		}
		m[c] = append(m[c], id)
	}
	return m, rows.Err()
}
