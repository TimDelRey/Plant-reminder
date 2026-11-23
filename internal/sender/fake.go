package sender

import (
	"context"
	"database/sql"
	"log/slog"
)

// DevSender — заглушка для разработки: «отправляет» сообщение в лог и в таблицу dev_outbox.
// Подключается только при DEV_FAKE_SENDERS=true в dev-окружении для каналов без токена бота.
type DevSender struct {
	DB      *sql.DB
	Channel Channel
}

func (d *DevSender) Send(ctx context.Context, externalID string, msg Message) error {
	slog.Info("[dev-sender] сообщение", "channel", d.Channel, "to", externalID, "title", msg.Title, "body", msg.Body)
	_, err := d.DB.ExecContext(ctx, `INSERT INTO dev_outbox (user_id, channel, external_id, title, body)
		VALUES ((SELECT user_id FROM user_channels WHERE channel = $1 AND external_id = $2 LIMIT 1), $1, $2, $3, $4)`,
		string(d.Channel), externalID, msg.Title, msg.Body)
	return err
}
