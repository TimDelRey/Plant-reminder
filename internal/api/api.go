// Package api — HTTP-роутинг, DTO и склейка доменов для фронтенда. Спецификация: openapi.yaml.
package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"plantcare/internal/apperr"
	"plantcare/internal/auth"
	"plantcare/internal/cities"
	"plantcare/internal/config"
	"plantcare/internal/plants"
	"plantcare/internal/sender"
)

//go:embed openapi.yaml
var OpenAPISpec []byte

//go:embed swaggerui/*
var swaggerFS embed.FS

type Deps struct {
	Cfg     *config.Config
	Auth    *auth.Service
	Plants  *plants.Service
	Cities  *cities.Service
	Linking *sender.Linking
	Static  fs.FS // собранный React (корень с index.html); может быть nil
}

type API struct {
	d          Deps
	authLimit  *limiter // вход/регистрация/сброс — по IP
	resetLimit *limiter // запрос сброса — по email
}

const docsHTML = `<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>API — Уход за растениями</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/api/docs/swagger-ui.css"></head><body><div id="swagger-ui"></div>
<script src="/api/docs/swagger-ui-bundle.js"></script>
<script>window.ui = SwaggerUIBundle({url: '/api/openapi.yaml', dom_id: '#swagger-ui', deepLinking: true, persistAuthorization: true});</script>
</body></html>`

func NewRouter(d Deps) http.Handler {
	a := &API{d: d, authLimit: newLimiter(6*time.Second, 10), resetLimit: newLimiter(5*time.Minute, 3)}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, securityHeaders, accessLog)

	// документация — открыта
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(OpenAPISpec)
	})
	r.Get("/api/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsHTML))
	})
	sub, _ := fs.Sub(swaggerFS, "swaggerui")
	r.Handle("/api/docs/*", http.StripPrefix("/api/docs/", http.FileServer(http.FS(sub))))

	// публичные
	r.Post("/api/auth/register", h(a.register))
	r.Post("/api/auth/login", h(a.login))
	r.Post("/api/auth/logout", h(a.logout))
	r.Post("/api/auth/password-reset/request", h(a.requestPasswordReset))
	r.Post("/api/auth/password-reset/confirm", h(a.confirmPasswordReset))
	r.Get("/api/cities", h(a.searchCities))

	// вебхуки платформ — своя проверка подписи/секрета
	r.Post("/webhooks/telegram", h(a.webhookTelegram))
	r.Post("/webhooks/vk", h(a.webhookVK))
	r.Post("/webhooks/max", h(a.webhookMAX))

	// под авторизацией (cookie-сессия или Bearer-токен)
	r.Group(func(p chi.Router) {
		p.Use(a.authenticate)
		p.Get("/api/me", h(a.getMe))
		p.Patch("/api/me", h(a.updateMe))
		p.Delete("/api/me", h(a.deleteMe))
		p.Patch("/api/me/password", h(a.changePassword))

		p.Get("/api/tokens", h(a.listTokens))
		p.Post("/api/tokens", h(a.createToken))
		p.Delete("/api/tokens/{id}", h(a.revokeToken))

		p.Get("/api/plants", h(a.listPlants))
		p.Post("/api/plants", h(a.createPlant))
		p.Get("/api/plants/{id}", h(a.getPlant))
		p.Patch("/api/plants/{id}", h(a.updatePlant))
		p.Delete("/api/plants/{id}", h(a.deletePlant))
		p.Get("/api/plants/{id}/events", h(a.listPlantEvents))
		p.Post("/api/plants/{id}/events", h(a.createEvent))
		p.Get("/api/plant-reference/search", h(a.searchReference))
		p.Get("/api/plants/{id}/care-suggestions", h(a.careSuggestions))
		p.Post("/api/plants/{id}/events/import", h(a.importCare))

		p.Get("/api/events/upcoming", h(a.listUpcoming))
		p.Patch("/api/events/{id}", h(a.updateEvent))
		p.Delete("/api/events/{id}", h(a.deleteEvent))
		p.Post("/api/events/{id}/complete", h(a.completeEvent))

		p.Get("/api/channels", h(a.listChannels))
		p.Post("/api/channels/{channel}/connect", h(a.channelOp(func(r *http.Request, ch sender.Channel) (sender.ChannelStatus, error) {
			return a.d.Linking.Connect(r.Context(), userID(r), ch)
		})))
		p.Post("/api/channels/{channel}/disconnect", h(a.channelOp(func(r *http.Request, ch sender.Channel) (sender.ChannelStatus, error) {
			return a.d.Linking.Disconnect(r.Context(), userID(r), ch)
		})))
		p.Get("/api/channels/{channel}/status", h(a.channelOp(func(r *http.Request, ch sender.Channel) (sender.ChannelStatus, error) {
			return a.d.Linking.Status(r.Context(), userID(r), ch)
		})))
	})

	spa := a.spaHandler()
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") || strings.HasPrefix(req.URL.Path, "/webhooks/") {
			writeErr(w, req, apperr.NotFound())
			return
		}
		spa.ServeHTTP(w, req)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		writeErr(w, req, apperr.New(http.StatusMethodNotAllowed, "method_not_allowed", "Метод не поддерживается"))
	})
	return r
}

// spaHandler раздаёт собранный React; неизвестные пути отдают index.html (клиентский роутинг).
func (a *API) spaHandler() http.Handler {
	if a.d.Static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "фронтенд не встроен в сборку (web/dist пуст)", http.StatusNotFound)
		})
	}
	files := http.FS(a.d.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if f, err := a.d.Static.Open(p); err == nil {
				st, _ := f.Stat()
				f.Close()
				if st != nil && !st.IsDir() {
					if strings.HasPrefix(p, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					http.FileServer(files).ServeHTTP(w, r)
					return
				}
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		http.FileServer(files).ServeHTTP(w, r2)
	})
}
