package auth

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"plantcare/internal/apperr"
	"plantcare/internal/testutil"
)

type captureMailer struct{ to, subject, body string }

func (c *captureMailer) Send(_ context.Context, to, subject, body string) error {
	c.to, c.subject, c.body = to, subject, body
	return nil
}

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil || !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("hash=%q err=%v", h, err)
	}
	if !VerifyPassword("correct horse", h) || VerifyPassword("wrong", h) {
		t.Error("VerifyPassword работает неверно")
	}
}

func TestRegisterLoginAndDuplicate(t *testing.T) {
	d := testutil.DB(t)
	s := &Service{DB: d, Mailer: &captureMailer{}, BaseURL: "http://x"}
	ctx := context.Background()
	city := testutil.CityID(t, d, "Москва")

	u, tok, err := s.Register(ctx, "  Anna@Example.com ", "password-1", city)
	if err != nil || tok == "" || u.Email != "anna@example.com" || u.City.Timezone != "Europe/Moscow" {
		t.Fatalf("register: %+v %v", u, err)
	}
	if _, _, err := s.Register(ctx, "ANNA@example.com", "password-2", city); err == nil || err.(*apperr.Error).Status != 409 {
		t.Errorf("дубликат email должен давать 409, got %v", err)
	}
	if _, _, err := s.Register(ctx, "bad", "short", 0); err == nil || len(err.(*apperr.Error).Fields) != 3 {
		t.Errorf("валидация должна вернуть ошибки по трём полям: %v", err)
	}
	if _, _, err := s.Login(ctx, "anna@example.com", "nope"); err == nil || err.(*apperr.Error).Status != 401 {
		t.Errorf("неверный пароль → 401, got %v", err)
	}
	uid, viaCookie, err := s.Authenticate(ctx, "", tok)
	if err != nil || uid != u.ID || !viaCookie {
		t.Errorf("authenticate по сессии: %d %v %v", uid, viaCookie, err)
	}
}

func TestAPITokenLifecycle(t *testing.T) {
	d := testutil.DB(t)
	s := &Service{DB: d, Mailer: &captureMailer{}}
	ctx := context.Background()
	u, _, _ := s.Register(ctx, "a@example.com", "password-1", testutil.CityID(t, d, "Москва"))

	ct, err := s.CreateToken(ctx, u.ID, "script", nil)
	if err != nil || !strings.HasPrefix(ct.Token, "pcr_") {
		t.Fatalf("create: %+v %v", ct, err)
	}
	var stored string
	_ = d.QueryRow(`SELECT encode(token_hash,'hex') FROM api_tokens WHERE id=$1`, ct.ID).Scan(&stored)
	if strings.Contains(stored, ct.Token) || len(stored) != 64 {
		t.Error("в БД должен храниться только хэш токена")
	}
	if uid, viaCookie, err := s.Authenticate(ctx, ct.Token, ""); err != nil || uid != u.ID || viaCookie {
		t.Errorf("bearer: %d %v %v", uid, viaCookie, err)
	}
	if l, _ := s.ListTokens(ctx, u.ID); len(l) != 1 || l[0].LastUsedAt == nil {
		t.Errorf("list: %+v", l)
	}
	if err := s.RevokeToken(ctx, u.ID, ct.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Authenticate(ctx, ct.Token, ""); err == nil {
		t.Error("отозванный токен не должен работать")
	}

	other, _, _ := s.Register(ctx, "b@example.com", "password-1", testutil.CityID(t, d, "Москва"))
	c2, _ := s.CreateToken(ctx, u.ID, "second", nil)
	if err := s.RevokeToken(ctx, other.ID, c2.ID); err == nil {
		t.Error("токен другого пользователя отозван")
	}
}

func TestPasswordResetByEmail(t *testing.T) {
	d := testutil.DB(t)
	m := &captureMailer{}
	s := &Service{DB: d, Mailer: m, BaseURL: "http://app.test"}
	ctx := context.Background()
	u, session, _ := s.Register(ctx, "anna@example.com", "old-password", testutil.CityID(t, d, "Москва"))
	api, _ := s.CreateToken(ctx, u.ID, "t", nil)

	if err := s.RequestPasswordReset(ctx, "ghost@example.com"); err != nil || m.to != "" {
		t.Fatalf("ghost: err=%v to=%q", err, m.to)
	}
	if err := s.RequestPasswordReset(ctx, "ANNA@example.com"); err != nil {
		t.Fatal(err)
	}
	if m.to != "anna@example.com" {
		t.Fatalf("письмо ушло на %q", m.to)
	}
	token := regexp.MustCompile(`token=([\w-]+)`).FindStringSubmatch(m.body)[1]

	if err := s.ConfirmPasswordReset(ctx, "bogus", "new-password"); err == nil {
		t.Error("недействительный токен принят")
	}
	if err := s.ConfirmPasswordReset(ctx, token, "short"); err == nil {
		t.Error("короткий пароль принят")
	}
	if err := s.ConfirmPasswordReset(ctx, token, "new-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmPasswordReset(ctx, token, "another-pass1"); err == nil {
		t.Error("токен сброса одноразовый, но принят повторно")
	}
	if _, _, err := s.Login(ctx, "anna@example.com", "old-password"); err == nil {
		t.Error("старый пароль всё ещё работает")
	}
	if _, _, err := s.Login(ctx, "anna@example.com", "new-password"); err != nil {
		t.Errorf("новый пароль не работает: %v", err)
	}
	if _, _, err := s.Authenticate(ctx, "", session); err == nil {
		t.Error("старая сессия должна быть отозвана")
	}
	if _, _, err := s.Authenticate(ctx, api.Token, ""); err == nil {
		t.Error("API-токен должен быть отозван после сброса пароля")
	}
}

func TestDeleteAccountRemovesEverything(t *testing.T) {
	d := testutil.DB(t)
	s := &Service{DB: d, Mailer: &captureMailer{}}
	ctx := context.Background()
	u, _, _ := s.Register(ctx, "a@example.com", "password-1", testutil.CityID(t, d, "Москва"))
	_, _ = d.Exec(`INSERT INTO plants (user_id,name) VALUES ($1,'p')`, u.ID)
	_, _ = d.Exec(`INSERT INTO user_channels (user_id,channel,status) VALUES ($1,'vk','pending')`, u.ID)
	_, _ = s.CreateToken(ctx, u.ID, "t", nil)

	if err := s.DeleteAccount(ctx, u.ID, "wrong"); err == nil {
		t.Fatal("удаление с неверным паролем прошло")
	}
	if err := s.DeleteAccount(ctx, u.ID, "password-1"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.QueryRow(`SELECT (SELECT count(*) FROM users)+(SELECT count(*) FROM plants)+(SELECT count(*) FROM user_channels)+(SELECT count(*) FROM api_tokens)+(SELECT count(*) FROM sessions)`).Scan(&n)
	if n != 0 {
		t.Errorf("после удаления аккаунта осталось строк: %d", n)
	}
}
