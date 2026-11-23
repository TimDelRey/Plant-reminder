package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"plantcare/internal/apperr"
)

type ctxKey int

const (
	userIDKey ctxKey = iota
	sessionKey
)

const cookieName = "session"

func userID(r *http.Request) int64 { id, _ := r.Context().Value(userIDKey).(int64); return id }

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// authenticate — общий middleware: пользователь определяется по Bearer-токену либо по cookie сессии.
func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, cookie := bearerToken(r), sessionToken(r)
		uid, viaCookie, err := a.d.Auth.Authenticate(r.Context(), bearer, cookie)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		if viaCookie && !isSafe(r.Method) && !a.sameOrigin(r) {
			writeErr(w, r, apperr.Forbidden("Запрос с другого источника отклонён"))
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isSafe(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin — CSRF-защита для cookie-запросов с изменением данных (Bearer в ней не нуждается).
func (a *API) sameOrigin(r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil {
			return false
		}
		if u.Host == r.Host {
			return true
		}
		if b, err := url.Parse(a.d.Cfg.BaseURL); err == nil && u.Host == b.Host {
			return true
		}
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}
