package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"plantcare/internal/auth"
	"plantcare/internal/cities"
	"plantcare/internal/config"
	"plantcare/internal/plants"
	"plantcare/internal/sender"
)

func jobsRouter(secret string, run func(context.Context) (JobsResult, error)) http.Handler {
	return NewRouter(Deps{
		Cfg: &config.Config{InternalSecret: secret}, Auth: &auth.Service{}, Plants: &plants.Service{},
		Cities: &cities.Service{}, Linking: &sender.Linking{}, RunJobs: run,
	})
}

func TestRunJobsSecret(t *testing.T) {
	calls := 0
	run := func(context.Context) (JobsResult, error) { calls++; return JobsResult{Events: 1, Jobs: 2}, nil }
	const secret = "0123456789abcdef-secret"
	timerBody := `{"messages":[{"details":{"payload":"` + secret + `"}}]}`

	cases := []struct {
		name, path, header, body string
		want                     int
	}{
		{"без секрета", "/internal/run-jobs", "", "", http.StatusUnauthorized},
		{"неверный секрет", "/internal/run-jobs", "wrong", "", http.StatusUnauthorized},
		{"секрет в заголовке", "/internal/run-jobs", secret, "", http.StatusOK},
		{"вызов таймера на /", "/", "", timerBody, http.StatusOK},
		{"таймер с чужим payload", "/", "", `{"messages":[{"details":{"payload":"x"}}]}`, http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, c.path, strings.NewReader(c.body))
		if c.header != "" {
			req.Header.Set("X-Internal-Secret", c.header)
		}
		rec := httptest.NewRecorder()
		jobsRouter(secret, run).ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: код %d, ожидали %d", c.name, rec.Code, c.want)
		}
	}
	if calls != 2 {
		t.Errorf("воркеры запускались %d раз, ожидали 2 (только при верном секрете)", calls)
	}
}

func TestRunJobsDisabledWithoutSecret(t *testing.T) {
	rec := httptest.NewRecorder()
	jobsRouter("", func(context.Context) (JobsResult, error) { return JobsResult{}, nil }).
		ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/run-jobs", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("без INTERNAL_SECRET эндпоинт должен отвечать 404, получили %d", rec.Code)
	}
}

func TestRootStillServesSPA(t *testing.T) {
	rec := httptest.NewRecorder()
	jobsRouter("secret-secret-secret", nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("GET / вернул 405")
	}
}
