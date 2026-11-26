package scheduler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"plantcare/internal/testutil"
)

type fixture struct {
	d       *sql.DB
	userID  int64
	plantID int64
}

func newFixture(t *testing.T, city string) fixture {
	t.Helper()
	d := testutil.DB(t)
	f := fixture{d: d}
	if err := d.QueryRow(`INSERT INTO users (email, password_hash, city_id) VALUES ('t@example.com','x',$1) RETURNING id`,
		testutil.CityID(t, d, city)).Scan(&f.userID); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`INSERT INTO plants (user_id, name) VALUES ($1,'Слива') RETURNING id`, f.userID).Scan(&f.plantID); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) connect(t *testing.T, channels ...string) {
	t.Helper()
	for _, c := range channels {
		if _, err := f.d.Exec(`INSERT INTO user_channels (user_id, channel, status, external_id) VALUES ($1,$2,'connected','ext')`, f.userID, c); err != nil {
			t.Fatal(err)
		}
	}
}

func (f fixture) event(t *testing.T, kind, rule, due, channels string) int64 {
	t.Helper()
	var r any
	if rule != "" {
		r = rule
	}
	var id int64
	err := f.d.QueryRow(`INSERT INTO care_events (plant_id,title,category,recurrence_kind,recurrence_rule,next_due_at,channels)
		VALUES ($1,'Полив','Полив',$2,$3,$4,string_to_array($5, ',')) RETURNING id`, f.plantID, kind, r, due, channels).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type jobRow struct {
	channel, status, lastErr string
	scheduled                time.Time
}

func (f fixture) jobs(t *testing.T) []jobRow {
	t.Helper()
	rows, err := f.d.Query(`SELECT channel, status, coalesce(last_error,''), scheduled_at FROM notification_jobs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []jobRow
	for rows.Next() {
		var j jobRow
		if err := rows.Scan(&j.channel, &j.status, &j.lastErr, &j.scheduled); err != nil {
			t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

func (f fixture) due(t *testing.T, id int64) string {
	t.Helper()
	var s string
	if err := f.d.QueryRow(`SELECT to_char(next_due_at,'YYYY-MM-DD') FROM care_events WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func at(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func tick(t *testing.T, f fixture, now string) int {
	t.Helper()
	n, err := (&Scheduler{DB: f.d, NotifyHour: 9, Now: func() time.Time { return at(now) }}).Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBeforeMorningIsScheduledForNineLocal(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "telegram")
	f.event(t, "manual", "", "2026-11-23", "telegram")
	tick(t, f, "2026-11-23T05:00:00Z")
	j := f.jobs(t)
	if len(j) != 1 || j[0].status != "pending" {
		t.Fatalf("jobs: %+v", j)
	}
	if want := at("2026-11-23T06:00:00Z"); !j[0].scheduled.Equal(want) {
		t.Errorf("scheduled_at = %v, want %v", j[0].scheduled, want)
	}
}

func TestAfterMorningIsScheduledNow(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "telegram")
	f.event(t, "manual", "", "2026-11-23", "telegram")
	now := "2026-11-23T10:00:00Z"
	tick(t, f, now)
	if j := f.jobs(t); len(j) != 1 || !j[0].scheduled.Equal(at(now)) {
		t.Fatalf("jobs: %+v", j)
	}
}

func TestTimezoneOfUsersCity(t *testing.T) {
	now := "2026-11-23T15:00:00Z"

	f := newFixture(t, "Владивосток")
	f.connect(t, "telegram")
	f.event(t, "manual", "", "2026-11-24", "telegram")
	if n := tick(t, f, now); n != 1 {
		t.Fatalf("Владивосток: обработано %d, ожидалось 1", n)
	}
	if j := f.jobs(t); len(j) != 1 || !j[0].scheduled.Equal(at("2026-11-23T23:00:00Z")) {
		t.Errorf("Владивосток: %+v", j)
	}

	m := newFixture(t, "Москва")
	m.connect(t, "telegram")
	m.event(t, "manual", "", "2026-11-24", "telegram")
	if n := tick(t, m, now); n != 0 {
		t.Errorf("Москва: событие на завтра не должно созревать, обработано %d", n)
	}
}

func TestDailyEventNextNotificationTomorrowMorning(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "telegram")
	id := f.event(t, "rrule", "FREQ=DAILY", "2026-11-23", "telegram")

	tick(t, f, "2026-11-23T10:00:00Z")
	tick(t, f, "2026-11-23T10:01:00Z")
	if n := len(f.jobs(t)); n != 1 {
		t.Fatalf("после двух проходов за день заданий %d, ожидалось 1", n)
	}
	if got := f.due(t, id); got != "2026-11-24" {
		t.Fatalf("next_due_at = %s, ожидалось 2026-11-24", got)
	}

	tick(t, f, "2026-11-24T04:00:00Z")
	j := f.jobs(t)
	if len(j) != 2 || !j[1].scheduled.Equal(at("2026-11-24T06:00:00Z")) {
		t.Fatalf("jobs: %+v", j)
	}
	if got := f.due(t, id); got != "2026-11-25" {
		t.Errorf("next_due_at = %s, ожидалось 2026-11-25", got)
	}
}

func TestManualEventNotifiesOnceUntilDateChanged(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "telegram")
	id := f.event(t, "manual", "", "2026-11-23", "telegram")

	tick(t, f, "2026-11-23T10:00:00Z")
	tick(t, f, "2026-11-24T10:00:00Z")
	tick(t, f, "2026-11-25T10:00:00Z")
	if n := len(f.jobs(t)); n != 1 {
		t.Fatalf("заданий %d, ожидалось 1", n)
	}
	if got := f.due(t, id); got != "2026-11-23" {
		t.Errorf("next_due_at у ручного события не должен меняться автоматически: %s", got)
	}

	if _, err := f.d.Exec(`UPDATE care_events SET next_due_at = '2026-11-26' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	tick(t, f, "2026-11-26T10:00:00Z")
	if n := len(f.jobs(t)); n != 2 {
		t.Errorf("после смены даты заданий %d, ожидалось 2", n)
	}
}

func TestNoFallbackBetweenChannels(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "vk", "max")
	f.event(t, "manual", "", "2026-11-23", "telegram,vk")
	tick(t, f, "2026-11-23T10:00:00Z")
	got := map[string]jobRow{}
	for _, j := range f.jobs(t) {
		got[j.channel] = j
	}
	if len(got) != 2 {
		t.Fatalf("каналы в заданиях: %v (max не выбран — задания быть не должно)", got)
	}
	if got["telegram"].status != "failed" || got["telegram"].lastErr == "" {
		t.Errorf("telegram: %+v", got["telegram"])
	}
	if got["vk"].status != "pending" {
		t.Errorf("vk: %+v", got["vk"])
	}
}

func TestInactiveAndFutureIgnored(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "telegram")
	id := f.event(t, "manual", "", "2026-11-23", "telegram")
	f.event(t, "manual", "", "2026-12-01", "telegram")
	if _, err := f.d.Exec(`UPDATE care_events SET active=false WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if n := tick(t, f, "2026-11-23T10:00:00Z"); n != 0 {
		t.Errorf("обработано %d, ожидалось 0", n)
	}
}

func TestJobPerConnectedRecipient(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "vk")
	if _, err := f.d.Exec(`INSERT INTO user_channels (user_id, channel, status, external_id) VALUES ($1,'vk','connected','mother')`, f.userID); err != nil {
		t.Fatal(err)
	}
	f.event(t, "manual", "", "2026-11-23", "vk")
	tick(t, f, "2026-11-23T10:00:00Z")
	jobs := f.jobs(t)
	if len(jobs) != 2 {
		t.Fatalf("ожидали по заданию на каждого получателя, получили %d: %+v", len(jobs), jobs)
	}
	for _, j := range jobs {
		if j.status != "pending" {
			t.Errorf("%+v", j)
		}
	}
	var withRecipient int
	_ = f.d.QueryRow(`SELECT count(DISTINCT recipient_id) FROM notification_jobs WHERE recipient_id IS NOT NULL`).Scan(&withRecipient)
	if withRecipient != 2 {
		t.Errorf("разных получателей в заданиях: %d", withRecipient)
	}
}

func TestSecondTickDoesNotDuplicateRecipientJobs(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "vk")
	f.event(t, "manual", "", "2026-11-23", "vk")
	tick(t, f, "2026-11-23T10:00:00Z")
	tick(t, f, "2026-11-23T11:00:00Z")
	if n := len(f.jobs(t)); n != 1 {
		t.Errorf("заданий: %d", n)
	}
}

func TestPastDatedEventsAreNotNotified(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "vk")
	manual := f.event(t, "manual", "", "2026-11-20", "vk")
	daily := f.event(t, "rrule", "FREQ=DAILY", "2026-11-20", "vk")
	tick(t, f, "2026-11-23T10:00:00Z")
	if jobs := f.jobs(t); len(jobs) != 0 {
		t.Fatalf("события с датой в прошлом не должны давать уведомлений: %+v", jobs)
	}
	if got := f.due(t, daily); got != "2026-11-23" {
		t.Errorf("ежедневное событие должно сдвинуться на сегодня без уведомления: %s", got)
	}
	if got := f.due(t, manual); got != "2026-11-20" {
		t.Errorf("разовое прошлое событие остаётся просроченным: %s", got)
	}
	tick(t, f, "2026-11-23T11:00:00Z")
	if jobs := f.jobs(t); len(jobs) != 1 {
		t.Fatalf("ежедневное событие после сдвига должно уведомить сегодня один раз: %+v", jobs)
	}
}

func TestDoneOnceEventIsNotNotified(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "vk")
	id := f.event(t, "once", "", "2026-11-23", "vk")
	if _, err := f.d.Exec(`UPDATE care_events SET completed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	tick(t, f, "2026-11-23T10:00:00Z")
	if jobs := f.jobs(t); len(jobs) != 0 {
		t.Fatalf("выполненное разовое событие уведомлять не нужно: %+v", jobs)
	}
}

func TestEmptyChannelsMeanAllConnectedChannels(t *testing.T) {
	f := newFixture(t, "Москва")
	f.connect(t, "vk", "telegram")
	f.event(t, "manual", "", "2026-11-23", "")
	tick(t, f, "2026-11-23T10:00:00Z")
	got := map[string]string{}
	for _, j := range f.jobs(t) {
		got[j.channel] = j.status
	}
	if len(got) != 2 || got["vk"] != "pending" || got["telegram"] != "pending" {
		t.Fatalf("без явных каналов напоминание должно уйти во все подключённые: %v", got)
	}
}

func TestEmptyChannelsWithoutConnectedChannelsCreateNoJobs(t *testing.T) {
	f := newFixture(t, "Москва")
	f.event(t, "manual", "", "2026-11-23", "")
	tick(t, f, "2026-11-23T10:00:00Z")
	if jobs := f.jobs(t); len(jobs) != 0 {
		t.Fatalf("нет подключённых каналов: заданий быть не должно: %+v", jobs)
	}
}
