package sender

import (
	"context"
	"testing"

	"plantcare/internal/testutil"
)

func linkFixture(t *testing.T) (*Linking, int64) {
	t.Helper()
	d := testutil.DB(t)
	var uid int64
	if err := d.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('l@example.com','x',$1) RETURNING id`, testutil.CityID(t, d, "Москва")).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	return &Linking{DB: d, Setup: map[Channel]Setup{ChannelVK: {Configured: true, BotUsername: "club1"}}}, uid
}

func connectPerson(t *testing.T, l *Linking, uid int64, external string) ChannelStatus {
	t.Helper()
	ctx := context.Background()
	st, err := l.Connect(ctx, uid, ChannelVK)
	if err != nil || st.Link == nil {
		t.Fatalf("connect: %v %+v", err, st)
	}
	ok, err := l.HandleIncoming(ctx, ChannelVK, external, "привет "+st.Link.Code)
	if err != nil || !ok {
		t.Fatalf("handle: ok=%v err=%v", ok, err)
	}
	st, err = l.Status(ctx, uid, ChannelVK)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSeveralRecipientsPerChannel(t *testing.T) {
	l, uid := linkFixture(t)
	st := connectPerson(t, l, uid, "100")
	if st.Status != "connected" || len(st.Recipients) != 1 {
		t.Fatalf("после первого: %+v", st)
	}
	st = connectPerson(t, l, uid, "200")
	if len(st.Recipients) != 2 || st.Link != nil {
		t.Fatalf("после второго должно быть 2 получателя и без активного кода: %+v", st)
	}
}

func TestSamePersonIsNotDuplicated(t *testing.T) {
	l, uid := linkFixture(t)
	connectPerson(t, l, uid, "100")
	st := connectPerson(t, l, uid, "100")
	if len(st.Recipients) != 1 {
		t.Fatalf("тот же человек продублирован: %+v", st)
	}
}

func TestConnectKeepsExistingRecipientsAndReplacesPendingCode(t *testing.T) {
	l, uid := linkFixture(t)
	connectPerson(t, l, uid, "100")
	ctx := context.Background()
	first, err := l.Connect(ctx, uid, ChannelVK)
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.Connect(ctx, uid, ChannelVK)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Recipients) != 1 || second.Status != "connected" || second.Link == nil {
		t.Fatalf("%+v", second)
	}
	if ok, _ := l.HandleIncoming(ctx, ChannelVK, "300", first.Link.Code); ok {
		t.Error("старый код после повторного Connect должен быть недействителен")
	}
}

func TestRemoveRecipientAndDisconnect(t *testing.T) {
	l, uid := linkFixture(t)
	connectPerson(t, l, uid, "100")
	st := connectPerson(t, l, uid, "200")
	ctx := context.Background()
	st, err := l.RemoveRecipient(ctx, uid, ChannelVK, st.Recipients[0].ID)
	if err != nil || len(st.Recipients) != 1 {
		t.Fatalf("remove: %v %+v", err, st)
	}
	if _, err := l.RemoveRecipient(ctx, uid, ChannelVK, 999999); err == nil {
		t.Error("несуществующий получатель должен давать 404")
	}
	st, err = l.Disconnect(ctx, uid, ChannelVK)
	if err != nil || st.Status != "disconnected" || len(st.Recipients) != 0 {
		t.Fatalf("disconnect: %v %+v", err, st)
	}
}

func TestCannotRemoveForeignRecipient(t *testing.T) {
	l, uid := linkFixture(t)
	st := connectPerson(t, l, uid, "100")
	var other int64
	if err := l.DB.QueryRow(`INSERT INTO users (email,password_hash,city_id) VALUES ('o2@example.com','x',$1) RETURNING id`, testutil.CityID(t, l.DB, "Москва")).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RemoveRecipient(context.Background(), other, ChannelVK, st.Recipients[0].ID); err == nil {
		t.Error("чужого получателя удалить нельзя")
	}
}
