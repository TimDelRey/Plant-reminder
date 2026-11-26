package sender

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"plantcare/internal/testutil"
)

type fakeSender struct {
	err   error
	calls []string
}

func (f *fakeSender) Send(_ context.Context, id string, m Message) error {
	f.calls = append(f.calls, id+": "+m.Title)
	return f.err
}

func setup(t *testing.T, connected bool) (*sql.DB, int64) {
	t.Helper()
	d := testutil.DB(t)
	var uid, pid, eid, jid int64
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('o@example.com','x',$1) RETURNING id`, testutil.CityID(t, d, "Москва")).Scan(&uid))
	must(d.QueryRow(`INSERT INTO plants (user_id,name) VALUES ($1,'p') RETURNING id`, uid).Scan(&pid))
	must(d.QueryRow(`INSERT INTO care_events (plant_id,title,category,recurrence_kind,next_due_at) VALUES ($1,'e','Полив','manual','2026-11-23') RETURNING id`, pid).Scan(&eid))
	var recipient sql.NullInt64
	if connected {
		must(d.QueryRow(`INSERT INTO user_channels (user_id,channel,status,external_id) VALUES ($1,'telegram','connected','42') RETURNING id`, uid).Scan(&recipient.Int64))
		recipient.Valid = true
	}
	must(d.QueryRow(`INSERT INTO notification_jobs (care_event_id,user_id,channel,recipient_id,title,body,scheduled_at) VALUES ($1,$2,'telegram',$3,'Заголовок','Текст','2026-11-23T10:00:00Z') RETURNING id`, eid, uid, recipient).Scan(&jid))
	return d, jid
}

func worker(d *sql.DB, s Sender, now string) *OutboxWorker {
	disp := NewDispatcher()
	if s != nil {
		disp.Register(ChannelTelegram, s)
	}
	t, _ := time.Parse(time.RFC3339, now)
	return &OutboxWorker{DB: d, Dispatcher: disp, MaxAttempts: 3, Now: func() time.Time { return t }}
}

func status(t *testing.T, d *sql.DB, id int64) (st string, attempts int, lastErr string) {
	t.Helper()
	if err := d.QueryRow(`SELECT status, attempts, coalesce(last_error,'') FROM notification_jobs WHERE id=$1`, id).Scan(&st, &attempts, &lastErr); err != nil {
		t.Fatal(err)
	}
	return
}

func TestOutboxSendsDueJob(t *testing.T) {
	d, id := setup(t, true)
	fs := &fakeSender{}
	if n, err := worker(d, fs, "2026-11-23T10:00:01Z").ProcessOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if st, a, _ := status(t, d, id); st != "sent" || a != 1 {
		t.Errorf("status=%s attempts=%d", st, a)
	}
	if len(fs.calls) != 1 || fs.calls[0] != "42: Заголовок" {
		t.Errorf("calls: %v", fs.calls)
	}

	_, _ = worker(d, fs, "2026-11-23T10:05:00Z").ProcessOnce(context.Background())
	if len(fs.calls) != 1 {
		t.Errorf("повторная отправка: %v", fs.calls)
	}
}

func TestOutboxIgnoresFutureJobs(t *testing.T) {
	d, id := setup(t, true)
	fs := &fakeSender{}
	if n, _ := worker(d, fs, "2026-11-23T09:59:59Z").ProcessOnce(context.Background()); n != 0 {
		t.Fatalf("обработано %d", n)
	}
	if st, _, _ := status(t, d, id); st != "pending" || len(fs.calls) != 0 {
		t.Errorf("status=%s calls=%v", st, fs.calls)
	}
}

func TestOutboxRetriesWithBackoffThenFails(t *testing.T) {
	d, id := setup(t, true)
	fs := &fakeSender{err: errors.New("сеть недоступна")}
	now := "2026-11-23T10:00:00Z"
	_, _ = worker(d, fs, now).ProcessOnce(context.Background())
	st, a, le := status(t, d, id)
	if st != "pending" || a != 1 || le != "сеть недоступна" {
		t.Fatalf("после 1-й ошибки: %s %d %q", st, a, le)
	}
	var sched time.Time
	_ = d.QueryRow(`SELECT scheduled_at FROM notification_jobs WHERE id=$1`, id).Scan(&sched)
	if want, _ := time.Parse(time.RFC3339, "2026-11-23T10:01:00Z"); !sched.Equal(want) {
		t.Errorf("backoff: scheduled_at=%v", sched)
	}
	_, _ = worker(d, fs, "2026-11-23T10:01:00Z").ProcessOnce(context.Background())
	_, _ = worker(d, fs, "2026-11-23T10:07:00Z").ProcessOnce(context.Background())
	if st, a, _ := status(t, d, id); st != "failed" || a != 3 {
		t.Errorf("после лимита попыток: %s %d", st, a)
	}
}

func TestOutboxFailsWhenChannelDisconnected(t *testing.T) {
	d, id := setup(t, false)
	fs := &fakeSender{}
	_, _ = worker(d, fs, "2026-11-23T10:00:01Z").ProcessOnce(context.Background())
	if st, _, le := status(t, d, id); st != "failed" || le == "" || len(fs.calls) != 0 {
		t.Errorf("status=%s err=%q calls=%v", st, le, fs.calls)
	}
}

func TestOutboxFailsWhenSenderNotConfigured(t *testing.T) {
	d, id := setup(t, true)
	_, _ = worker(d, nil, "2026-11-23T10:00:01Z").ProcessOnce(context.Background())
	if st, a, _ := status(t, d, id); st != "failed" || a != 1 {
		t.Errorf("status=%s attempts=%d (без токена бота ретраи бессмысленны)", st, a)
	}
}

type selectiveSender struct {
	failFor string
	calls   []string
}

func (f *selectiveSender) Send(_ context.Context, id string, _ Message) error {
	f.calls = append(f.calls, id)
	if id == f.failFor {
		return errors.New("получатель недоступен")
	}
	return nil
}

func TestOutboxRecipientsAreIndependent(t *testing.T) {
	d := testutil.DB(t)
	var uid, pid, eid int64
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('m@example.com','x',$1) RETURNING id`, testutil.CityID(t, d, "Москва")).Scan(&uid))
	must(d.QueryRow(`INSERT INTO plants (user_id,name) VALUES ($1,'p') RETURNING id`, uid).Scan(&pid))
	must(d.QueryRow(`INSERT INTO care_events (plant_id,title,category,recurrence_kind,next_due_at) VALUES ($1,'e','Полив','manual','2026-11-23') RETURNING id`, pid).Scan(&eid))
	jobs := map[string]int64{}
	for _, ext := range []string{"owner", "mother"} {
		var rid int64
		must(d.QueryRow(`INSERT INTO user_channels (user_id,channel,status,external_id) VALUES ($1,'telegram','connected',$2) RETURNING id`, uid, ext).Scan(&rid))
		var jid int64
		must(d.QueryRow(`INSERT INTO notification_jobs (care_event_id,user_id,channel,recipient_id,title,body,scheduled_at) VALUES ($1,$2,'telegram',$3,'T','B','2026-11-23T10:00:00Z') RETURNING id`, eid, uid, rid).Scan(&jid))
		jobs[ext] = jid
	}
	fs := &selectiveSender{failFor: "mother"}
	disp := NewDispatcher()
	disp.Register(ChannelTelegram, fs)
	now, _ := time.Parse(time.RFC3339, "2026-11-23T10:00:01Z")
	w := &OutboxWorker{DB: d, Dispatcher: disp, MaxAttempts: 3, Now: func() time.Time { return now }}
	if n, err := w.ProcessOnce(context.Background()); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if st, _, _ := status(t, d, jobs["owner"]); st != "sent" {
		t.Errorf("owner: %s", st)
	}
	if st, a, _ := status(t, d, jobs["mother"]); st != "pending" || a != 1 {
		t.Errorf("mother: %s attempts=%d (ошибка у одного получателя не должна затрагивать другого)", st, a)
	}
	fs.failFor = ""
	later := now.Add(2 * time.Minute)
	w.Now = func() time.Time { return later }
	if _, err := w.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	owners := 0
	for _, c := range fs.calls {
		if c == "owner" {
			owners++
		}
	}
	if owners != 1 {
		t.Errorf("owner получил %d сообщений, ожидали 1 (повтор только для mother): %v", owners, fs.calls)
	}
	if st, _, _ := status(t, d, jobs["mother"]); st != "sent" {
		t.Errorf("mother после повтора: %s", st)
	}
}

func TestOutboxSkipsRecipientRemovedBeforeSend(t *testing.T) {
	d, id := setup(t, true)
	if _, err := d.Exec(`DELETE FROM user_channels`); err != nil {
		t.Fatal(err)
	}
	fs := &fakeSender{}
	_, _ = worker(d, fs, "2026-11-23T10:00:01Z").ProcessOnce(context.Background())
	if st, _, _ := status(t, d, id); st != "failed" || len(fs.calls) != 0 {
		t.Errorf("status=%s calls=%v", st, fs.calls)
	}
}
