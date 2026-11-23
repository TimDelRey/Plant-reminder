package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"plantcare/internal/apperr"
	"plantcare/internal/plants"
	"plantcare/internal/sender"
)

func (a *API) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: a.d.Cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(30 * 24 * time.Hour), MaxAge: 30 * 24 * 3600,
	})
}

func (a *API) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: a.d.Cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0)})
}

func (a *API) tooMany(r *http.Request, l *limiter, key string) error {
	if !l.allow(key) {
		return apperr.New(http.StatusTooManyRequests, "too_many_requests", "Слишком много попыток — повторите позже")
	}
	return nil
}

// ---------- auth ----------

func (a *API) register(w http.ResponseWriter, r *http.Request) error {
	if err := a.tooMany(r, a.authLimit, clientIP(r, a.d.Cfg.IsProd())); err != nil {
		return err
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		CityID   int64  `json:"city_id"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	u, tok, err := a.d.Auth.Register(r.Context(), in.Email, in.Password, in.CityID)
	if err != nil {
		return err
	}
	a.setSession(w, tok)
	writeJSON(w, http.StatusCreated, u)
	return nil
}

func (a *API) login(w http.ResponseWriter, r *http.Request) error {
	if err := a.tooMany(r, a.authLimit, clientIP(r, a.d.Cfg.IsProd())); err != nil {
		return err
	}
	var in struct{ Email, Password string }
	if err := decode(w, r, &in); err != nil {
		return err
	}
	u, tok, err := a.d.Auth.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		return err
	}
	a.setSession(w, tok)
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) error {
	if err := a.d.Auth.Logout(r.Context(), sessionToken(r)); err != nil {
		return err
	}
	a.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) requestPasswordReset(w http.ResponseWriter, r *http.Request) error {
	if err := a.tooMany(r, a.authLimit, clientIP(r, a.d.Cfg.IsProd())); err != nil {
		return err
	}
	var in struct{ Email string }
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := a.tooMany(r, a.resetLimit, strings.ToLower(strings.TrimSpace(in.Email))); err != nil {
		return err
	}
	if err := a.d.Auth.RequestPasswordReset(r.Context(), in.Email); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (a *API) confirmPasswordReset(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Token, Password string }
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := a.d.Auth.ConfirmPasswordReset(r.Context(), in.Token, in.Password); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---------- account ----------

func (a *API) getMe(w http.ResponseWriter, r *http.Request) error {
	u, err := a.d.Auth.GetMe(r.Context(), userID(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (a *API) updateMe(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		CityID int64 `json:"city_id"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	u, err := a.d.Auth.UpdateCity(r.Context(), userID(r), in.CityID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (a *API) deleteMe(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Password string }
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.Password == "" {
		return apperr.Validation(map[string]string{"password": "Введите пароль для подтверждения"})
	}
	if err := a.d.Auth.DeleteAccount(r.Context(), userID(r), in.Password); err != nil {
		return err
	}
	a.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := a.d.Auth.ChangePassword(r.Context(), userID(r), in.CurrentPassword, in.NewPassword, sessionToken(r)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) error {
	t, err := a.d.Auth.ListTokens(r.Context(), userID(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

func (a *API) createToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name          string `json:"name"`
		ExpiresInDays *int   `json:"expires_in_days"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	t, err := a.d.Auth.CreateToken(r.Context(), userID(r), in.Name, in.ExpiresInDays)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, t)
	return nil
}

func (a *API) revokeToken(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	if err := a.d.Auth.RevokeToken(r.Context(), userID(r), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) searchCities(w http.ResponseWriter, r *http.Request) error {
	c, err := a.d.Cities.Search(r.Context(), r.URL.Query().Get("q"), intQuery(r, "limit", 15))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, c)
	return nil
}

// ---------- plants & events ----------

func (a *API) listPlants(w http.ResponseWriter, r *http.Request) error {
	p, err := a.d.Plants.ListPlants(r.Context(), userID(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (a *API) createPlant(w http.ResponseWriter, r *http.Request) error {
	var in plants.PlantInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	p, err := a.d.Plants.CreatePlant(r.Context(), userID(r), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, p)
	return nil
}

func (a *API) getPlant(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	p, err := a.d.Plants.GetPlant(r.Context(), userID(r), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (a *API) deletePlant(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r, "id")
	if err != nil {
		return err
	}
	if err := a.d.Plants.DeletePlant(r.Context(), userID(r), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---------- channels ----------

func (a *API) listChannels(w http.ResponseWriter, r *http.Request) error {
	l, err := a.d.Linking.List(r.Context(), userID(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, l)
	return nil
}

func (a *API) channelOp(op func(r *http.Request, ch sender.Channel) (sender.ChannelStatus, error)) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		st, err := op(r, sender.Channel(chi.URLParam(r, "channel")))
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, st)
		return nil
	}
}
