package vk

import (
	"context"
	"fmt"
	"strconv"

	"github.com/SevereCloud/vksdk/v3/api"

	"plantcare/internal/sender"
)

type Adapter struct{ api *api.VK }

func New(groupToken string) *Adapter { return &Adapter{api: api.NewVK(groupToken)} }

func (a *Adapter) Send(ctx context.Context, externalID string, msg sender.Message) error {
	uid, err := strconv.Atoi(externalID)
	if err != nil {
		return fmt.Errorf("некорректный VK user_id %q: %w", externalID, err)
	}
	_, err = a.api.MessagesSend(api.Params{"user_id": uid, "random_id": 0, "message": msg.Text()})
	return err
}
