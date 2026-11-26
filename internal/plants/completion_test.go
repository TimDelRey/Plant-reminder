package plants

import (
	"context"
	"testing"
	"time"

	"plantcare/internal/apperr"
	"plantcare/internal/testutil"
)

func completionFixture(t *testing.T) (*Service, int64, int64) {
	t.Helper()
	d := testutil.DB(t)
	var uid int64
	if err := d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('c@x.ru','x',$1) RETURNING id`, testutil.CityID(t, d, "Москва")).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: d}
	p, err := s.CreatePlant(context.Background(), uid, PlantInput{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, p.ID
}

func newEvent(t *testing.T, s *Service, uid, pid int64, in EventInput) CareEvent {
	t.Helper()
	if in.Category == "" {
		in.Category = "Полив"
	}
	ev, err := s.CreateEvent(context.Background(), uid, pid, in)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestOnceEventStaysVisibleAsDoneAndCanBeUndone(t *testing.T) {
	s, uid, pid := completionFixture(t)
	ctx := context.Background()
	ev := newEvent(t, s, uid, pid, EventInput{Title: "Побелка", RecurrenceKind: "once", RecurrenceNote: "Разово", NextDueAt: "2026-12-10"})

	done, err := s.CompleteEvent(ctx, uid, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.CompletedAt == nil || !done.CanUndo || done.NextDueAt != "2026-12-10" || !done.Active {
		t.Fatalf("разовое событие должно остаться на своей дате и стать выполненным: %+v", done)
	}
	if again, _ := s.CompleteEvent(ctx, uid, ev.ID); again.CompletedAt == nil || !again.CompletedAt.Equal(*done.CompletedAt) {
		t.Errorf("повторное выполнение не должно менять отметку: %+v", again)
	}
	if up, _ := s.ListUpcoming(ctx, uid, 10); len(up) != 0 {
		t.Errorf("выполненное разовое событие не должно попадать в ленту ближайших: %+v", up)
	}
	list, _ := s.ListEvents(ctx, uid, pid)
	if len(list) != 1 || list[0].CompletedAt == nil {
		t.Errorf("выполненное событие должно оставаться в списке растения: %+v", list)
	}

	undone, err := s.UndoCompleteEvent(ctx, uid, ev.ID)
	if err != nil || undone.CompletedAt != nil || undone.CanUndo || undone.NextDueAt != "2026-12-10" {
		t.Fatalf("после отмены событие снова в плане: %+v %v", undone, err)
	}
	if _, err := s.UndoCompleteEvent(ctx, uid, ev.ID); err == nil {
		t.Error("отменять нечего: ожидали ошибку")
	} else if ae, ok := err.(*apperr.Error); !ok || ae.Status != 409 {
		t.Errorf("ожидали 409: %v", err)
	}
}

func TestOnceEventEditKeepsDoneUntilReopened(t *testing.T) {
	s, uid, pid := completionFixture(t)
	ctx := context.Background()
	ev := newEvent(t, s, uid, pid, EventInput{Title: "Сбор", RecurrenceKind: "once", RecurrenceNote: "Разово", NextDueAt: "2026-12-10"})
	_, _ = s.CompleteEvent(ctx, uid, ev.ID)

	renamed, err := s.UpdateEvent(ctx, uid, ev.ID, EventPatch{Title: ptr("Сбор яблок")})
	if err != nil || renamed.CompletedAt == nil {
		t.Fatalf("правка названия не должна возвращать событие в план: %+v %v", renamed, err)
	}
	again, err := s.UpdateEvent(ctx, uid, ev.ID, EventPatch{NextDueAt: ptr("2027-03-01"), Reopen: ptr(true)})
	if err != nil || again.CompletedAt != nil || again.CanUndo || again.NextDueAt != "2027-03-01" {
		t.Fatalf("«запланировать снова» должно вернуть событие в план с новой датой: %+v %v", again, err)
	}
	if _, err := s.CompleteEvent(ctx, uid, ev.ID); err != nil {
		t.Fatal(err)
	}
	if done, _ := s.ListEvents(ctx, uid, pid); done[0].CompletedAt == nil {
		t.Error("после повторного плана событие снова можно выполнить разово")
	}
}

func TestRecurringCompletionCanBeUndone(t *testing.T) {
	s, uid, pid := completionFixture(t)
	ctx := context.Background()
	today := time.Now().UTC().Format(DateLayout)
	weekly := newEvent(t, s, uid, pid, EventInput{Title: "Полив", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=WEEKLY"), NextDueAt: today})
	done, err := s.CompleteEvent(ctx, uid, weekly.ID)
	if err != nil || done.NextDueAt == today || !done.CanUndo {
		t.Fatalf("%+v %v", done, err)
	}
	undone, err := s.UndoCompleteEvent(ctx, uid, weekly.ID)
	if err != nil || undone.NextDueAt != today || undone.CompletedAt != nil {
		t.Fatalf("отмена должна вернуть прежнюю дату: %+v %v", undone, err)
	}

	manual := newEvent(t, s, uid, pid, EventInput{Title: "Обработка", RecurrenceKind: "manual", RecurrenceNote: "до распускания почек", NextDueAt: "2026-03-20"})
	m, _ := s.CompleteEvent(ctx, uid, manual.ID)
	if m.NextDueAt != "2027-03-20" {
		t.Fatalf("%+v", m)
	}
	if u, _ := s.UndoCompleteEvent(ctx, uid, manual.ID); u.NextDueAt != "2026-03-20" {
		t.Errorf("%+v", u)
	}

	last := newEvent(t, s, uid, pid, EventInput{Title: "Последний", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=DAILY;COUNT=1"), NextDueAt: today})
	l, _ := s.CompleteEvent(ctx, uid, last.ID)
	if l.Active {
		t.Fatalf("без будущих повторений событие должно стать неактивным: %+v", l)
	}
	if u, _ := s.UndoCompleteEvent(ctx, uid, last.ID); !u.Active {
		t.Errorf("отмена должна вернуть активность: %+v", u)
	}
}

func TestUndoExpiresAndEditResetsIt(t *testing.T) {
	s, uid, pid := completionFixture(t)
	ctx := context.Background()
	today := time.Now().UTC().Format(DateLayout)
	ev := newEvent(t, s, uid, pid, EventInput{Title: "Полив", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=WEEKLY"), NextDueAt: today})
	_, _ = s.CompleteEvent(ctx, uid, ev.ID)
	if _, err := s.UpdateEvent(ctx, uid, ev.ID, EventPatch{Title: ptr("Полив 2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCompleteEvent(ctx, uid, ev.ID); err == nil {
		t.Error("после правки отмена выполнения не должна откатывать дату")
	}

	_, _ = s.CompleteEvent(ctx, uid, ev.ID)
	if _, err := s.DB.Exec(`UPDATE care_events SET completed_at = now() - interval '8 days' WHERE id = $1`, ev.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCompleteEvent(ctx, uid, ev.ID); err == nil {
		t.Error("через 8 дней отмена недоступна")
	}
}

func TestUsersCannotUndoForeignEvents(t *testing.T) {
	s, uid, pid := completionFixture(t)
	var other int64
	if err := s.DB.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('o@x.ru','x',$1) RETURNING id`, testutil.CityID(t, s.DB, "Москва")).Scan(&other); err != nil {
		t.Fatal(err)
	}
	ev := newEvent(t, s, uid, pid, EventInput{Title: "x", RecurrenceKind: "once", RecurrenceNote: "Разово", NextDueAt: "2026-12-10"})
	_, _ = s.CompleteEvent(context.Background(), uid, ev.ID)
	if _, err := s.UndoCompleteEvent(context.Background(), other, ev.ID); err == nil {
		t.Error("чужое событие отменить нельзя")
	} else if ae, ok := err.(*apperr.Error); !ok || ae.Status != 404 {
		t.Errorf("ожидали 404: %v", err)
	}
}

func TestCompletingNotifiedOccurrenceDoesNotSkipNextOne(t *testing.T) {
	s, uid, pid := completionFixture(t)
	ctx := context.Background()
	weekly := newEvent(t, s, uid, pid, EventInput{Title: "Полив", RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=WEEKLY"), NextDueAt: "2026-12-03"})
	if _, err := s.DB.Exec(`UPDATE care_events SET last_notified_for = '2026-11-26' WHERE id = $1`, weekly.ID); err != nil {
		t.Fatal(err)
	}
	done, err := s.CompleteEvent(ctx, uid, weekly.ID)
	if err != nil || done.NextDueAt != "2026-12-03" || done.CompletedAt == nil || !done.CanUndo {
		t.Fatalf("напоминание за 26.11 уже отправлено и дата сдвинута: выполнение не должно сдвигать её ещё раз: %+v %v", done, err)
	}
	again, _ := s.CompleteEvent(ctx, uid, weekly.ID)
	if again.NextDueAt != "2026-12-10" {
		t.Errorf("повторное нажатие относится уже к следующему вхождению: %+v", again)
	}
	undone, err := s.UndoCompleteEvent(ctx, uid, weekly.ID)
	if err != nil || undone.NextDueAt != "2026-12-03" || undone.CompletedAt != nil {
		t.Errorf("отмена возвращает состояние до последнего нажатия: %+v %v", undone, err)
	}
}
