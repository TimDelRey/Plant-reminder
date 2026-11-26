package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"plantcare/internal/apperr"
	"plantcare/internal/cities"
	"plantcare/internal/mailer"
)

const (
	SessionTTL     = 30 * 24 * time.Hour
	resetTokenTTL  = time.Hour
	apiTokenPrefix = "pcr_"
	minPasswordLen = 8
	maxPasswordLen = 128
)

type User struct {
	ID        int64       `json:"id"`
	Email     string      `json:"email"`
	City      cities.City `json:"city"`
	CreatedAt time.Time   `json:"created_at"`
}

type APIToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

type CreatedAPIToken struct {
	APIToken
	Token string `json:"token"`
}

type Service struct {
	DB      *sql.DB
	Mailer  mailer.Mailer
	BaseURL string
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || len(email) > 254 || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return "", errors.New("invalid")
	}
	return strings.ToLower(email), nil
}

func validatePassword(pw string) string {
	switch {
	case len(pw) < minPasswordLen:
		return fmt.Sprintf("Минимум %d символов", minPasswordLen)
	case len(pw) > maxPasswordLen:
		return fmt.Sprintf("Максимум %d символов", maxPasswordLen)
	}
	return ""
}

const userSelect = `SELECT u.id, u.email, u.created_at, c.id, c.name, c.region, c.timezone
FROM users u JOIN cities c ON c.id = u.city_id`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.CreatedAt, &u.City.ID, &u.City.Name, &u.City.Region, &u.City.Timezone)
	return u, err
}

func (s *Service) newSession(ctx context.Context, userID int64) (string, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		userID, hashToken(tok), time.Now().Add(SessionTTL))
	return tok, err
}

func (s *Service) Register(ctx context.Context, email, password string, cityID int64) (User, string, error) {
	fields := map[string]string{}
	em, err := normalizeEmail(email)
	if err != nil {
		fields["email"] = "Введите корректный email"
	}
	if m := validatePassword(password); m != "" {
		fields["password"] = m
	}
	if cityID <= 0 {
		fields["city_id"] = "Выберите город из списка"
	}
	if len(fields) > 0 {
		return User{}, "", apperr.Validation(fields)
	}
	if _, err := cities.ByID(ctx, s.DB, cityID); err != nil {
		return User{}, "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, "", err
	}
	var id int64
	err = s.DB.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, city_id) VALUES ($1,$2,$3)
		ON CONFLICT DO NOTHING RETURNING id`, em, hash, cityID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", apperr.Conflict("Пользователь с таким email уже зарегистрирован")
	}
	if err != nil {
		return User{}, "", err
	}
	tok, err := s.newSession(ctx, id)
	if err != nil {
		return User{}, "", err
	}
	u, err := s.GetMe(ctx, id)
	return u, tok, err
}

var dummyHash, _ = HashPassword("dummy-password-for-timing")

func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	em, err := normalizeEmail(email)
	if err != nil || password == "" {
		return User{}, "", apperr.Validation(map[string]string{"email": "Введите email и пароль"})
	}
	var id int64
	var hash string
	err = s.DB.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE lower(email) = $1`, em).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		VerifyPassword(password, dummyHash)
		return User{}, "", apperr.New(401, "invalid_credentials", "Неверный email или пароль")
	}
	if err != nil {
		return User{}, "", err
	}
	if !VerifyPassword(password, hash) {
		return User{}, "", apperr.New(401, "invalid_credentials", "Неверный email или пароль")
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	tok, err := s.newSession(ctx, id)
	if err != nil {
		return User{}, "", err
	}
	u, err := s.GetMe(ctx, id)
	return u, tok, err
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(sessionToken))
	return err
}

func (s *Service) GetMe(ctx context.Context, userID int64) (User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, userSelect+` WHERE u.id = $1`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return u, apperr.Unauthorized()
	}
	return u, err
}

func (s *Service) UpdateCity(ctx context.Context, userID, cityID int64) (User, error) {
	if _, err := cities.ByID(ctx, s.DB, cityID); err != nil {
		return User{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET city_id = $1 WHERE id = $2`, cityID, userID); err != nil {
		return User{}, err
	}
	return s.GetMe(ctx, userID)
}

func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next, keepSession string) error {
	if m := validatePassword(next); m != "" {
		return apperr.Validation(map[string]string{"new_password": m})
	}
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		return err
	}
	if !VerifyPassword(current, hash) {
		return apperr.Forbidden("Текущий пароль указан неверно")
	}
	nh, err := HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, nh, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, hashToken(keepSession)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) DeleteAccount(ctx context.Context, userID int64, password string) error {
	var hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		return err
	}
	if !VerifyPassword(password, hash) {
		return apperr.Forbidden("Пароль указан неверно")
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	em, err := normalizeEmail(email)
	if err != nil {
		return apperr.Validation(map[string]string{"email": "Введите корректный email"})
	}
	var userID int64
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(email) = $1`, em).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	tok, err := randomToken(32)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`,
		userID, hashToken(tok), time.Now().Add(resetTokenTTL)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	link := s.BaseURL + "/reset-password?token=" + tok
	body := "Вы запросили сброс пароля в сервисе «Уход за растениями».\n\nПерейдите по ссылке, чтобы задать новый пароль (действует 1 час):\n" + link +
		"\n\nЕсли вы не запрашивали сброс — просто проигнорируйте это письмо."
	if err := s.Mailer.Send(ctx, em, "Сброс пароля — Уход за растениями", body); err != nil {

		slog.Error("отправка письма сброса пароля", "err", err)
	}
	return nil
}

func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if m := validatePassword(newPassword); m != "" {
		return apperr.Validation(map[string]string{"password": m})
	}
	invalid := apperr.BadRequest("Ссылка недействительна или устарела — запросите сброс пароля заново")
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, userID int64
	err = tx.QueryRowContext(ctx, `SELECT id, user_id FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`, hashToken(token)).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid
	}
	if err != nil {
		return err
	}
	nh, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, nh); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ListTokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, created_at, last_used_at, expires_at FROM api_tokens
		WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) CreateToken(ctx context.Context, userID int64, name string, expiresInDays *int) (CreatedAPIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return CreatedAPIToken{}, apperr.Validation(map[string]string{"name": "Введите название токена (до 100 символов)"})
	}
	var exp *time.Time
	if expiresInDays != nil {
		if *expiresInDays < 1 || *expiresInDays > 3650 {
			return CreatedAPIToken{}, apperr.Validation(map[string]string{"expires_in_days": "От 1 до 3650 дней"})
		}
		t := time.Now().Add(time.Duration(*expiresInDays) * 24 * time.Hour)
		exp = &t
	}
	raw, err := randomToken(32)
	if err != nil {
		return CreatedAPIToken{}, err
	}
	full := apiTokenPrefix + raw
	var t CreatedAPIToken
	err = s.DB.QueryRowContext(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, expires_at) VALUES ($1,$2,$3,$4)
		RETURNING id, name, created_at, last_used_at, expires_at`, userID, name, hashToken(full), exp).
		Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt)
	t.Token = full
	return t, err
}

func (s *Service) RevokeToken(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.NotFound()
	}
	return nil
}

func (s *Service) Authenticate(ctx context.Context, bearer, sessionCookie string) (userID int64, viaCookie bool, err error) {
	if bearer != "" {
		err = s.DB.QueryRowContext(ctx, `UPDATE api_tokens SET last_used_at = now()
			WHERE token_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
			RETURNING user_id`, hashToken(bearer)).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, apperr.Unauthorized()
		}
		return userID, false, err
	}
	if sessionCookie != "" {
		err = s.DB.QueryRowContext(ctx, `SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > now()`,
			hashToken(sessionCookie)).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, true, apperr.Unauthorized()
		}
		return userID, true, err
	}
	return 0, false, apperr.Unauthorized()
}
