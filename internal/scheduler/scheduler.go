package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
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

		connected, err := connectedChannels(ctx, tx, e.userID)
		if err != nil {
			return 0, err
		}
		title := "🌿 " + e.plantName + " — " + e.title
		if e.channels != "" {
			for _, ch := range strings.Split(e.channels, ",") {
				status, lastErr := "pending", sql.NullString{}
				if !connected[ch] {
					status, lastErr = "failed", sql.NullString{String: "канал не подключён", Valid: true}
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO notification_jobs
					(care_event_id, user_id, channel, title, body, scheduled_at, status, last_error)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
					e.id, e.userID, ch, title, e.desc, scheduled, status, lastErr); err != nil {
					return 0, err
				}
			}
		}

		if e.kind == "rrule" && e.rule.Valid {
			today := plants.LocalToday(now, e.tz)
			base := e.due
			if today.After(base) {
				base = today
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

func connectedChannels(ctx context.Context, tx *sql.Tx, userID int64) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT channel FROM user_channels WHERE user_id = $1 AND status = 'connected'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		m[c] = true
	}
	return m, rows.Err()
}
