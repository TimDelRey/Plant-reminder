package plants

import (
	"context"
	"testing"
	"time"

	"plantcare/internal/apperr"
	"plantcare/internal/testutil"
)

func TestUsersAreIsolated(t *testing.T) {
	d := testutil.DB(t)
	ctx := context.Background()
	var a, b int64
	city := testutil.CityID(t, d, "Москва")
	_ = d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('a@x.ru','x',$1) RETURNING id`, city).Scan(&a)
	_ = d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('b@x.ru','x',$1) RETURNING id`, city).Scan(&b)
	s := &Service{DB: d}

	p, err := s.CreatePlant(ctx, a, PlantInput{Name: "Слива"})
	if err != nil || p.Emoji != "🌱" {
		t.Fatalf("%+v %v", p, err)
	}
	ev, err := s.CreateEvent(ctx, a, p.ID, EventInput{Title: "Полив", Category: "Полив", RecurrenceKind: "manual", NextDueAt: "2026-12-01", Channels: []string{"telegram"}})
	if err != nil {
		t.Fatal(err)
	}

	notFound := func(name string, err error) {
		t.Helper()
		if ae, ok := err.(*apperr.Error); !ok || ae.Status != 404 {
			t.Errorf("%s: чужие данные должны давать 404, got %v", name, err)
		}
	}
	_, err = s.GetPlant(ctx, b, p.ID)
	notFound("GetPlant", err)
	_, err = s.UpdatePlant(ctx, b, p.ID, PlantPatch{Name: ptr("x")})
	notFound("UpdatePlant", err)
	notFound("DeletePlant", s.DeletePlant(ctx, b, p.ID))
	_, err = s.ListEvents(ctx, b, p.ID)
	notFound("ListEvents", err)
	_, err = s.CreateEvent(ctx, b, p.ID, EventInput{Title: "x", Category: "Полив", RecurrenceKind: "manual", NextDueAt: "2026-12-01"})
	notFound("CreateEvent", err)
	_, err = s.UpdateEvent(ctx, b, ev.ID, EventPatch{Title: ptr("x")})
	notFound("UpdateEvent", err)
	_, err = s.CompleteEvent(ctx, b, ev.ID)
	notFound("CompleteEvent", err)
	notFound("DeleteEvent", s.DeleteEvent(ctx, b, ev.ID))
	if u, _ := s.ListUpcoming(ctx, b, 10); len(u) != 0 {
		t.Errorf("лента b содержит чужие события: %+v", u)
	}
}

func TestEventValidationReportsAllFields(t *testing.T) {
	d := testutil.DB(t)
	var uid int64
	_ = d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('a@x.ru','x',$1) RETURNING id`, testutil.CityID(t, d, "Москва")).Scan(&uid)
	s := &Service{DB: d}
	p, _ := s.CreatePlant(context.Background(), uid, PlantInput{Name: "p"})
	_, err := s.CreateEvent(context.Background(), uid, p.ID, EventInput{Title: "", Category: "нет такой", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=HOURLY"), NextDueAt: "bad", Channels: []string{"icq"}})
	ae, ok := err.(*apperr.Error)
	if !ok || ae.Status != 422 {
		t.Fatalf("ожидалась 422, got %v", err)
	}
	for _, f := range []string{"title", "category", "recurrence_rule", "next_due_at", "channels"} {
		if ae.Fields[f] == "" {
			t.Errorf("нет ошибки по полю %s: %v", f, ae.Fields)
		}
	}
}

func TestCompleteEventRecalculatesDueDate(t *testing.T) {
	d := testutil.DB(t)
	var uid int64
	_ = d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('a@x.ru','x',$1) RETURNING id`, testutil.CityID(t, d, "Москва")).Scan(&uid)
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	s := &Service{DB: d, Now: func() time.Time { return now }}
	ctx := context.Background()
	p, _ := s.CreatePlant(ctx, uid, PlantInput{Name: "p"})

	weekly, _ := s.CreateEvent(ctx, uid, p.ID, EventInput{Title: "w", Category: "Полив", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=WEEKLY"), NextDueAt: "2026-11-16"})
	if ev, err := s.CompleteEvent(ctx, uid, weekly.ID); err != nil || ev.NextDueAt != "2026-11-30" { // ближайшая по правилу строго после сегодня
		t.Errorf("rrule: %+v %v", ev, err)
	}
	manual, _ := s.CreateEvent(ctx, uid, p.ID, EventInput{Title: "m", Category: "Другое", RecurrenceKind: "manual", NextDueAt: "2026-11-23"})
	if ev, _ := s.CompleteEvent(ctx, uid, manual.ID); ev.NextDueAt != "2027-11-23" {
		t.Errorf("manual: %+v", ev)
	}
	once, _ := s.CreateEvent(ctx, uid, p.ID, EventInput{Title: "o", Category: "Другое", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=DAILY;COUNT=1"), NextDueAt: "2026-11-23"})
	if ev, _ := s.CompleteEvent(ctx, uid, once.ID); ev.Active {
		t.Errorf("событие без будущих повторений должно стать неактивным: %+v", ev)
	}
}

func ptr[T any](v T) *T { return &v }
