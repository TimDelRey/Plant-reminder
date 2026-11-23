package sender

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"plantcare/internal/apperr"
)

// pgerrcode — код ошибки Postgres unique_violation.
const uniqueViolation = "23505"

const (
	linkCodeTTL      = 15 * time.Minute
	linkCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // без похожих символов
	linkCodeLen      = 6
)

// Setup — настройки канала на сервере (из окружения).
type Setup struct {
	Configured  bool   // есть ли отправщик (токен бота или dev-заглушка)
	BotUsername string // для диплинка; VK — screen_name сообщества
}

type ChannelLink struct {
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expires_at"`
	DeepLink    *string   `json:"deep_link"`
	Instruction string    `json:"instruction"`
}

type ChannelStatus struct {
	Channel     Channel      `json:"channel"`
	Status      string       `json:"status"`
	Configured  bool         `json:"configured"`
	ConnectedAt *time.Time   `json:"connected_at"`
	Link        *ChannelLink `json:"link"`
}

// Linking — привязка каналов к пользователю (коды подключения, вебхуки).
type Linking struct {
	DB         *sql.DB
	Setup      map[Channel]Setup
	Dispatcher *Dispatcher
	Now        func() time.Time
}

func (l *Linking) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func newCode() (string, error) {
	b := make([]byte, linkCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = linkCodeAlphabet[int(b[i])%len(linkCodeAlphabet)]
	}
	return string(b), nil
}

func (l *Linking) link(ch Channel, code string, exp time.Time) *ChannelLink {
	setup := l.Setup[ch]
	link := &ChannelLink{Code: code, ExpiresAt: exp}
	var dl string
	switch ch {
	case ChannelTelegram:
		if setup.BotUsername != "" {
			dl = "https://t.me/" + setup.BotUsername + "?start=" + code
		}
		link.Instruction = "Нажмите «Открыть бота» и затем Start — подключение подтвердится автоматически. Если бот открылся без кнопки, отправьте ему: /start " + code
	case ChannelVK:
		if setup.BotUsername != "" {
			dl = "https://vk.me/" + setup.BotUsername
		}
		link.Instruction = "Откройте диалог с сообществом (разрешите сообщения) и отправьте любое сообщение с кодом: " + code
	case ChannelMAX:
		if setup.BotUsername != "" {
			dl = "https://max.ru/" + setup.BotUsername + "?start=" + code
		}
		link.Instruction = "Откройте бота в MAX и отправьте ему: /start " + code
	}
	if dl != "" {
		link.DeepLink = &dl
	}
	return link
}

type row struct {
	status      string
	code        sql.NullString
	exp         sql.NullTime
	connectedAt sql.NullTime
}

func (l *Linking) load(ctx context.Context, userID int64) (map[Channel]row, error) {
	rows, err := l.DB.QueryContext(ctx, `SELECT channel, status, link_code, link_code_expires_at, connected_at FROM user_channels WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[Channel]row{}
	for rows.Next() {
		var ch string
		var r row
		if err := rows.Scan(&ch, &r.status, &r.code, &r.exp, &r.connectedAt); err != nil {
			return nil, err
		}
		m[Channel(ch)] = r
	}
	return m, rows.Err()
}

func (l *Linking) toStatus(ch Channel, r row, ok bool) ChannelStatus {
	st := ChannelStatus{Channel: ch, Status: "disconnected", Configured: l.Setup[ch].Configured}
	if !ok {
		return st
	}
	switch r.status {
	case "connected":
		st.Status = "connected"
		if r.connectedAt.Valid {
			t := r.connectedAt.Time
			st.ConnectedAt = &t
		}
	case "pending":
		if r.code.Valid && r.exp.Valid && r.exp.Time.After(l.now()) {
			st.Status = "pending"
			st.Link = l.link(ch, r.code.String, r.exp.Time)
		}
	}
	return st
}

// List возвращает статус подключения по всем каналам.
func (l *Linking) List(ctx context.Context, userID int64) ([]ChannelStatus, error) {
	m, err := l.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelStatus, 0, len(AllChannels))
	for _, ch := range AllChannels {
		r, ok := m[ch]
		out = append(out, l.toStatus(ch, r, ok))
	}
	return out, nil
}

// Status возвращает статус одного канала (для поллинга).
func (l *Linking) Status(ctx context.Context, userID int64, ch Channel) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	m, err := l.load(ctx, userID)
	if err != nil {
		return ChannelStatus{}, err
	}
	r, ok := m[ch]
	return l.toStatus(ch, r, ok), nil
}

// Connect выдаёт одноразовый код подключения и переводит канал в pending.
func (l *Linking) Connect(ctx context.Context, userID int64, ch Channel) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	if !l.Setup[ch].Configured {
		return ChannelStatus{}, apperr.New(503, "not_configured", "Канал не настроен на сервере: не задан токен бота")
	}
	cur, err := l.load(ctx, userID)
	if err != nil {
		return ChannelStatus{}, err
	}
	if r, ok := cur[ch]; ok && r.status == "connected" {
		return ChannelStatus{}, apperr.Conflict("Канал уже подключён — сначала отключите его")
	}
	exp := l.now().Add(linkCodeTTL)
	for attempt := 0; attempt < 5; attempt++ {
		code, err := newCode()
		if err != nil {
			return ChannelStatus{}, err
		}
		_, err = l.DB.ExecContext(ctx, `INSERT INTO user_channels (user_id, channel, status, link_code, link_code_expires_at)
			VALUES ($1,$2,'pending',$3,$4)
			ON CONFLICT (user_id, channel) DO UPDATE SET status='pending', external_id=NULL, link_code=EXCLUDED.link_code,
				link_code_expires_at=EXCLUDED.link_code_expires_at, connected_at=NULL`, userID, string(ch), code, exp)
		if err == nil {
			return l.Status(ctx, userID, ch)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation || pgErr.ConstraintName != "user_channels_code_uq" {
			return ChannelStatus{}, err
		}
	}
	return ChannelStatus{}, errors.New("не удалось сгенерировать код подключения")
}

// Disconnect сбрасывает привязку канала.
func (l *Linking) Disconnect(ctx context.Context, userID int64, ch Channel) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	_, err := l.DB.ExecContext(ctx, `UPDATE user_channels SET status='disconnected', external_id=NULL, link_code=NULL,
		link_code_expires_at=NULL, connected_at=NULL WHERE user_id=$1 AND channel=$2`, userID, string(ch))
	if err != nil {
		return ChannelStatus{}, err
	}
	return l.Status(ctx, userID, ch)
}

// HandleIncoming вызывается вебхуками платформ: ищет код подключения в тексте сообщения и привязывает external_id.
func (l *Linking) HandleIncoming(ctx context.Context, ch Channel, externalID, text string) (bool, error) {
	if externalID == "" || !ch.Valid() {
		return false, nil
	}
	for _, tok := range strings.FieldsFunc(strings.ToUpper(text), func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		if len(tok) != linkCodeLen {
			continue
		}
		res, err := l.DB.ExecContext(ctx, `UPDATE user_channels SET status='connected', external_id=$3, link_code=NULL,
			link_code_expires_at=NULL, connected_at=$4
			WHERE channel=$1 AND link_code=$2 AND status='pending' AND link_code_expires_at > $4`, string(ch), tok, externalID, l.now())
		if err != nil {
			return false, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			if l.Dispatcher != nil {
				if err := l.Dispatcher.Send(ctx, ch, externalID, Message{Title: "✅ Канал подключён", Body: "Теперь напоминания об уходе за растениями будут приходить сюда."}); err != nil {
					slog.Warn("не удалось отправить подтверждение подключения", "channel", ch, "err", err)
				}
			}
			return true, nil
		}
	}
	return false, nil
}
