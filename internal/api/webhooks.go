package api

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"plantcare/internal/apperr"
	"plantcare/internal/sender"
)

func secretEqual(got, want string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func readBody(w http.ResponseWriter, r *http.Request) []byte {
	b, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	return b
}

func (a *API) webhookTelegram(w http.ResponseWriter, r *http.Request) error {
	if !secretEqual(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), a.d.Cfg.TelegramWebhookSecret) {
		return apperr.Unauthorized()
	}
	var u struct {
		Message struct {
			Text string `json:"text"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	}
	if err := json.Unmarshal(readBody(w, r), &u); err != nil {
		return apperr.BadRequest("Некорректный JSON")
	}
	if u.Message.Chat.ID != 0 {
		if _, err := a.d.Linking.HandleIncoming(r.Context(), sender.ChannelTelegram, strconv.FormatInt(u.Message.Chat.ID, 10), u.Message.Text); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (a *API) webhookVK(w http.ResponseWriter, r *http.Request) error {
	var u struct {
		Type   string `json:"type"`
		Secret string `json:"secret"`
		Object struct {
			Message struct {
				FromID int64  `json:"from_id"`
				Text   string `json:"text"`
			} `json:"message"`
		} `json:"object"`
	}
	if err := json.Unmarshal(readBody(w, r), &u); err != nil {
		return apperr.BadRequest("Некорректный JSON")
	}
	if !secretEqual(u.Secret, a.d.Cfg.VKSecret) {
		return apperr.Unauthorized()
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch u.Type {
	case "confirmation":
		_, _ = io.WriteString(w, a.d.Cfg.VKConfirmation)
		return nil
	case "message_new":
		if u.Object.Message.FromID != 0 {
			if _, err := a.d.Linking.HandleIncoming(r.Context(), sender.ChannelVK, strconv.FormatInt(u.Object.Message.FromID, 10), u.Object.Message.Text); err != nil {
				return err
			}
		}
	}
	_, _ = io.WriteString(w, "ok")
	return nil
}

func (a *API) webhookMAX(w http.ResponseWriter, r *http.Request) error {
	if !secretEqual(r.Header.Get("X-Max-Bot-Api-Secret"), a.d.Cfg.MAXWebhookSecret) {
		return apperr.Unauthorized()
	}
	var u struct {
		UpdateType string `json:"update_type"`
		Payload    string `json:"payload"`
		User       struct {
			UserID int64 `json:"user_id"`
		} `json:"user"`
		Message struct {
			Sender struct {
				UserID int64 `json:"user_id"`
			} `json:"sender"`
			Body struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"message"`
	}
	if err := json.Unmarshal(readBody(w, r), &u); err != nil {
		return apperr.BadRequest("Некорректный JSON")
	}
	var uid int64
	var text string
	switch u.UpdateType {
	case "bot_started":
		uid, text = u.User.UserID, u.Payload
	case "message_created":
		uid, text = u.Message.Sender.UserID, u.Message.Body.Text
	}
	if uid != 0 {
		if _, err := a.d.Linking.HandleIncoming(r.Context(), sender.ChannelMAX, strconv.FormatInt(uid, 10), text); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}
