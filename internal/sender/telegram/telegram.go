// Package telegram — адаптер отправки через Telegram Bot API (go-telegram/bot).
package telegram

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-telegram/bot"

	"plantcare/internal/sender"
)

type Adapter struct{ b *bot.Bot }

func New(token string) (*Adapter, error) {
	b, err := bot.New(token, bot.WithSkipGetMe())
	if err != nil {
		return nil, err
	}
	return &Adapter{b: b}, nil
}

func (a *Adapter) Send(ctx context.Context, externalID string, msg sender.Message) error {
	chatID, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil {
		return fmt.Errorf("некорректный chat_id %q: %w", externalID, err)
	}
	_, err = a.b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msg.Text()})
	return err
}
