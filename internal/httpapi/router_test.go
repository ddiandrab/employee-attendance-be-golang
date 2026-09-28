package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ddiandrab/employee-attendance-be-golang/internal/auth"
)

type failingAuth struct{ err error }

func (a failingAuth) Authenticate(context.Context, string) (auth.User, error) {
	return auth.User{}, a.err
}

func TestErrorsAndCORS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"unauthenticated", auth.ErrUnauthorized, 401, "Authentication required"},
		{"database failure", errors.New("secret connection details"), 500, "An unexpected error occurred"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &API{Auth: failingAuth{tc.err}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			r := httptest.NewRequest("GET", "/attendance/me", nil)
			w := httptest.NewRecorder()
			api.Handler().ServeHTTP(w, r)
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body["message"] != tc.message || body["path"] != "/attendance/me" || body["timestamp"] == nil || body["error"] == nil {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret connection") {
				t.Fatal("internal detail leaked")
			}
		})
	}
	api := &API{Auth: failingAuth{auth.ErrUnauthorized}, AllowedOrigin: "http://localhost:5173"}
	for _, tc := range []struct {
		origin string
		status int
	}{{"http://localhost:5173", 204}, {"https://untrusted.example", 403}} {
		r := httptest.NewRequest(http.MethodOptions, "/attendance/check-in", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
		if tc.status == 204 && w.Header().Get("Access-Control-Allow-Origin") != tc.origin {
			t.Fatal("missing allowed origin")
		}
		if tc.status == 403 && w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin allowed")
		}
	}
}
