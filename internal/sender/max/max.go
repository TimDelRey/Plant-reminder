// Package max — адаптер отправки через MAX Bot API (https://dev.max.ru/docs-api).
// Используется прямой HTTP-вызов POST /messages?user_id=..., без внешнего SDK.
package max

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"plantcare/internal/sender"
)

const defaultBaseURL = "https://platform-api.max.ru"

type Adapter struct {
	Token   string
	BaseURL string
	Client  *http.Client
}

func New(token string) *Adapter {
	return &Adapter{Token: token, BaseURL: defaultBaseURL, Client: &http.Client{Timeout: 15 * time.Second}}
}

func (a *Adapter) Send(ctx context.Context, externalID string, msg sender.Message) error {
	body, _ := json.Marshal(map[string]string{"text": msg.Text()})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/messages?user_id="+url.QueryEscape(externalID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", a.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("MAX API: %s: %s", resp.Status, b)
	}
	return nil
}
