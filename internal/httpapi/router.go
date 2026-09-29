package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/ddiandrab/employee-attendance-be-golang/internal/attendance"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/auth"
)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}
type API struct {
	Attendance    *attendance.Service
	Auth          Authenticator
	Logger        *slog.Logger
	AllowedOrigin string
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/attendance/check-in", a.protect(http.MethodPost, false, func(w http.ResponseWriter, r *http.Request, u auth.User) {
		data, err := a.Attendance.CheckIn(r.Context(), u.ID)
		a.respond(w, r, data, err)
	}))
	mux.HandleFunc("/attendance/check-out", a.protect(http.MethodPost, false, func(w http.ResponseWriter, r *http.Request, u auth.User) {
		data, err := a.Attendance.CheckOut(r.Context(), u.ID)
		a.respond(w, r, data, err)
	}))
	mux.HandleFunc("/attendance/me", a.protect(http.MethodGet, false, func(w http.ResponseWriter, r *http.Request, u auth.User) {
		data, err := a.Attendance.Mine(r.Context(), u.ID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		a.respond(w, r, data, err)
	}))
	mux.HandleFunc("/attendance", a.protect(http.MethodGet, true, func(w http.ResponseWriter, r *http.Request, _ auth.User) {
		data, err := a.Attendance.All(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		a.respond(w, r, data, err)
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { a.fail(w, r, http.StatusNotFound, "Resource not found") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			if origin == a.AllowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			} else if r.Method == http.MethodOptions {
				a.fail(w, r, http.StatusForbidden, "Origin not allowed")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) protect(method string, staffOnly bool, next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			a.respond(w, r, nil, err)
			return
		}
		if u.Role != "ADMIN" && u.Role != "HR" && (u.Role != "EMPLOYEE" || staffOnly) {
			a.fail(w, r, http.StatusForbidden, "Access denied")
			return
		}
		if r.Method != method {
			w.Header().Set("Allow", method)
			a.fail(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		next(w, r, u)
	}
}

func (a *API) respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, data)
		return
	}
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		a.fail(w, r, http.StatusUnauthorized, auth.ErrUnauthorized.Error())
	case errors.Is(err, attendance.ErrProfileMissing):
		a.fail(w, r, http.StatusNotFound, attendance.ErrProfileMissing.Error())
	case errors.Is(err, attendance.ErrAlreadyCheckedIn), errors.Is(err, attendance.ErrNotCheckedIn), errors.Is(err, attendance.ErrAlreadyCheckedOut), errors.Is(err, attendance.ErrInvalidDate), errors.Is(err, attendance.ErrInvalidRange):
		a.fail(w, r, http.StatusBadRequest, err.Error())
	default:
		a.Logger.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
		a.fail(w, r, http.StatusInternalServerError, "An unexpected error occurred")
	}
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, status int, message string) {
	writeJSON(w, status, struct {
		Timestamp time.Time `json:"timestamp"`
		Status    int       `json:"status"`
		Error     string    `json:"error"`
		Message   string    `json:"message"`
		Path      string    `json:"path"`
	}{time.Now().UTC(), status, http.StatusText(status), message, r.URL.Path})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
