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

const uniqueViolation = "23505"

const (
	linkCodeTTL      = 15 * time.Minute
	linkCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	linkCodeLen      = 6
)

type Setup struct {
	Configured  bool
	BotUsername string
}

type ChannelLink struct {
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expires_at"`
	DeepLink    *string   `json:"deep_link"`
	Instruction string    `json:"instruction"`
}

type Recipient struct {
	ID          int64     `json:"id"`
	ConnectedAt time.Time `json:"connected_at"`
}

type ChannelStatus struct {
	Channel     Channel      `json:"channel"`
	Status      string       `json:"status"`
	Configured  bool         `json:"configured"`
	ConnectedAt *time.Time   `json:"connected_at"`
	Recipients  []Recipient  `json:"recipients"`
	Link        *ChannelLink `json:"link"`
}

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
	id          int64
	status      string
	code        sql.NullString
	exp         sql.NullTime
	connectedAt sql.NullTime
}

func (l *Linking) load(ctx context.Context, userID int64) (map[Channel][]row, error) {
	rows, err := l.DB.QueryContext(ctx, `SELECT id, channel, status, link_code, link_code_expires_at, connected_at
		FROM user_channels WHERE user_id = $1 ORDER BY connected_at NULLS LAST, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[Channel][]row{}
	for rows.Next() {
		var ch string
		var r row
		if err := rows.Scan(&r.id, &ch, &r.status, &r.code, &r.exp, &r.connectedAt); err != nil {
			return nil, err
		}
		m[Channel(ch)] = append(m[Channel(ch)], r)
	}
	return m, rows.Err()
}

func (l *Linking) toStatus(ch Channel, rows []row) ChannelStatus {
	st := ChannelStatus{Channel: ch, Status: "disconnected", Configured: l.Setup[ch].Configured, Recipients: []Recipient{}}
	for _, r := range rows {
		switch r.status {
		case "connected":
			at := r.connectedAt.Time
			st.Recipients = append(st.Recipients, Recipient{ID: r.id, ConnectedAt: at})
			if st.ConnectedAt == nil || at.After(*st.ConnectedAt) {
				st.ConnectedAt = &at
			}
		case "pending":
			if r.code.Valid && r.exp.Valid && r.exp.Time.After(l.now()) {
				st.Link = l.link(ch, r.code.String, r.exp.Time)
			}
		}
	}
	switch {
	case len(st.Recipients) > 0:
		st.Status = "connected"
	case st.Link != nil:
		st.Status = "pending"
	}
	return st
}

func (l *Linking) List(ctx context.Context, userID int64) ([]ChannelStatus, error) {
	m, err := l.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelStatus, 0, len(AllChannels))
	for _, ch := range AllChannels {
		out = append(out, l.toStatus(ch, m[ch]))
	}
	return out, nil
}

func (l *Linking) Status(ctx context.Context, userID int64, ch Channel) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	m, err := l.load(ctx, userID)
	if err != nil {
		return ChannelStatus{}, err
	}
	return l.toStatus(ch, m[ch]), nil
}

func (l *Linking) Connect(ctx context.Context, userID int64, ch Channel) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	if !l.Setup[ch].Configured {
		return ChannelStatus{}, apperr.New(503, "not_configured", "Канал не настроен на сервере: не задан токен бота")
	}
	exp := l.now().Add(linkCodeTTL)
	for attempt := 0; attempt < 5; attempt++ {
		code, err := newCode()
		if err != nil {
			return ChannelStatus{}, err
		}
		_, err = l.DB.ExecContext(ctx, `WITH stale AS (
				DELETE FROM user_channels WHERE user_id = $1 AND channel = $2 AND status <> 'connected')
			INSERT INTO user_channels (user_id, channel, status, link_code, link_code_expires_at)
			VALUES ($1, $2, 'pending', $3, $4)`, userID, string(ch), code, exp)
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

func (l *Linking) Disconnect(ctx context.Context, userID int64, ch Channel) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	if _, err := l.DB.ExecContext(ctx, `DELETE FROM user_channels WHERE user_id = $1 AND channel = $2`, userID, string(ch)); err != nil {
		return ChannelStatus{}, err
	}
	return l.Status(ctx, userID, ch)
}

func (l *Linking) RemoveRecipient(ctx context.Context, userID int64, ch Channel, recipientID int64) (ChannelStatus, error) {
	if !ch.Valid() {
		return ChannelStatus{}, apperr.NotFound()
	}
	res, err := l.DB.ExecContext(ctx, `DELETE FROM user_channels WHERE id = $1 AND user_id = $2 AND channel = $3 AND status = 'connected'`,
		recipientID, userID, string(ch))
	if err != nil {
		return ChannelStatus{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ChannelStatus{}, apperr.NotFound()
	}
	return l.Status(ctx, userID, ch)
}

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
		linked, err := l.consumeCode(ctx, ch, tok, externalID)
		if err != nil {
			return false, err
		}
		if !linked {
			continue
		}
		if l.Dispatcher != nil {
			if err := l.Dispatcher.Send(ctx, ch, externalID, Message{Title: "✅ Канал подключён", Body: "Теперь напоминания об уходе за растениями будут приходить сюда."}); err != nil {
				slog.Warn("не удалось отправить подтверждение подключения", "channel", ch, "err", err)
			}
		}
		return true, nil
	}
	return false, nil
}

func (l *Linking) consumeCode(ctx context.Context, ch Channel, code, externalID string) (bool, error) {
	now := l.now()
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, userID int64
	err = tx.QueryRowContext(ctx, `SELECT id, user_id FROM user_channels
		WHERE channel = $1 AND link_code = $2 AND status = 'pending' AND link_code_expires_at > $3 FOR UPDATE`,
		string(ch), code, now).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM user_channels WHERE user_id = $1 AND channel = $2 AND external_id = $3)`,
		userID, string(ch), externalID).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		_, err = tx.ExecContext(ctx, `DELETE FROM user_channels WHERE id = $1`, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE user_channels SET status = 'connected', external_id = $2, link_code = NULL,
			link_code_expires_at = NULL, connected_at = $3 WHERE id = $1`, id, externalID, now)
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}
