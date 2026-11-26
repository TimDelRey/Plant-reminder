package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"plantcare/internal/apperr"
)

type JobsResult struct {
	Events int `json:"events"`
	Jobs   int `json:"jobs"`
}

const runJobsTimeout = 2 * time.Minute

func timerSecret(body []byte) string {
	var m struct {
		Messages []struct {
			Details struct {
				Payload string `json:"payload"`
			} `json:"details"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &m) != nil || len(m.Messages) == 0 {
		return ""
	}
	return m.Messages[0].Details.Payload
}

func (a *API) runJobs(w http.ResponseWriter, r *http.Request) error {
	if a.d.RunJobs == nil || a.d.Cfg.InternalSecret == "" {
		return apperr.NotFound()
	}
	secret := r.Header.Get("X-Internal-Secret")
	if secret == "" {
		secret = timerSecret(readBody(w, r))
	}
	if !secretEqual(secret, a.d.Cfg.InternalSecret) {
		return apperr.Unauthorized()
	}
	ctx, cancel := context.WithTimeout(r.Context(), runJobsTimeout)
	defer cancel()
	res, err := a.d.RunJobs(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}
