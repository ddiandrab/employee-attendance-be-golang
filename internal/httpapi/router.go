package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ddiandrab/employee-attendance-be-golang/internal/attendance"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/auth"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/management"
)

type Authenticator interface {
	Authenticate(context.Context, string) (auth.User, error)
}
type API struct {
	Attendance    *attendance.Service
	Auth          Authenticator
	Logger        *slog.Logger
	AllowedOrigin string
	Management    *management.Service
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", a.login)
	mux.HandleFunc("/auth/me", a.protect(http.MethodGet, false, func(w http.ResponseWriter, r *http.Request, u auth.User) {
		a.respondCall(w, r, func() (any, error) { return a.Management.Me(r.Context(), u.ID) })
	}))
	mux.HandleFunc("/users", a.userCollection)
	mux.HandleFunc("/users/", a.userByID)
	mux.HandleFunc("/employees", a.employeeCollection)
	mux.HandleFunc("/employees/me", a.employeeMe)
	mux.HandleFunc("/employees/", a.employeeByID)
	mux.HandleFunc("/departments", a.departmentCollection)
	mux.HandleFunc("/departments/", a.departmentByID)
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
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
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

func (a *API) protectRoles(method string, roles []string, next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			a.respond(w, r, nil, err)
			return
		}
		allowed := false
		for _, role := range roles {
			if u.Role == role {
				allowed = true
			}
		}
		if !allowed {
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

func decode(r *http.Request, dst any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return management.ErrInvalid
	}
	if d.More() {
		return management.ErrInvalid
	}
	return nil
}
func pathID(r *http.Request, prefix string) (int64, error) {
	raw := strings.TrimPrefix(r.URL.Path, prefix)
	if raw == "" || strings.Contains(raw, "/") {
		return 0, management.ErrInvalid
	}
	id, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || id < 1 {
		return 0, management.ErrInvalid
	}
	return id, nil
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.fail(w, r, 405, "Method not allowed")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if e := decode(r, &in); e != nil {
		a.respond(w, r, nil, e)
		return
	}
	u, e := a.Management.Login(r.Context(), in.Email, in.Password)
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	token, e := a.Auth.(*auth.Authenticator).Token(auth.User{ID: u.ID, Email: u.Email, Role: u.Role})
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": token, "tokenType": "Bearer"})
}
func (a *API) userByID(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r, "/users/")
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.respondCall(w, r, func() (any, error) { return a.Management.User(r.Context(), id) })
	case http.MethodPost:
		a.fail(w, r, 405, "Method not allowed")
	case http.MethodPatch:
		var in management.UserPatch
		if e = decode(r, &in); e == nil {
			a.respondCall(w, r, func() (any, error) { return a.Management.PatchUser(r.Context(), id, in) })
		} else {
			a.respond(w, r, nil, e)
		}
	case http.MethodDelete:
		e = a.Management.DeleteUser(r.Context(), id)
		if e != nil {
			a.respond(w, r, nil, e)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		a.fail(w, r, 405, "Method not allowed")
	}
}
func (a *API) userCollection(w http.ResponseWriter, r *http.Request) {
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	if r.Method == http.MethodGet {
		a.respondCall(w, r, func() (any, error) { return a.Management.Users(r.Context()) })
		return
	}
	if r.Method == http.MethodPost {
		var in management.UserInput
		if e = decode(r, &in); e == nil {
			v, e := a.Management.CreateUser(r.Context(), in)
			if e != nil {
				a.respond(w, r, nil, e)
			} else {
				writeJSON(w, 201, v)
			}
		} else {
			a.respond(w, r, nil, e)
		}
		return
	}
	a.fail(w, r, 405, "Method not allowed")
}
func (a *API) employeeMe(w http.ResponseWriter, r *http.Request) {
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" && u.Role != "HR" && u.Role != "EMPLOYEE" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	if r.Method == http.MethodGet {
		a.respondCall(w, r, func() (any, error) { return a.Management.MyEmployee(r.Context(), u.ID) })
		return
	}
	if r.Method == http.MethodPatch {
		var in management.ProfilePatch
		if e = decode(r, &in); e == nil {
			a.respondCall(w, r, func() (any, error) { return a.Management.PatchProfile(r.Context(), u.ID, in) })
		} else {
			a.respond(w, r, nil, e)
		}
		return
	}
	a.fail(w, r, 405, "Method not allowed")
}
func (a *API) employeeCollection(w http.ResponseWriter, r *http.Request) {
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" && u.Role != "HR" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	if r.Method == http.MethodGet {
		a.respondCall(w, r, func() (any, error) { return a.Management.Employees(r.Context()) })
		return
	}
	if r.Method == http.MethodPost {
		var in management.EmployeeInput
		if e = decode(r, &in); e == nil {
			v, e := a.Management.CreateEmployee(r.Context(), in)
			if e != nil {
				a.respond(w, r, nil, e)
			} else {
				writeJSON(w, 201, v)
			}
		} else {
			a.respond(w, r, nil, e)
		}
		return
	}
	a.fail(w, r, 405, "Method not allowed")
}
func (a *API) employeeByID(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r, "/employees/")
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" && u.Role != "HR" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.respondCall(w, r, func() (any, error) { return a.Management.Employee(r.Context(), id) })
	case http.MethodPatch:
		var in management.EmployeePatch
		if e = decode(r, &in); e == nil {
			a.respondCall(w, r, func() (any, error) { return a.Management.PatchEmployee(r.Context(), id, in) })
		} else {
			a.respond(w, r, nil, e)
		}
	case http.MethodDelete:
		e = a.Management.DeleteEmployee(r.Context(), id)
		if e != nil {
			a.respond(w, r, nil, e)
		} else {
			w.WriteHeader(204)
		}
	default:
		a.fail(w, r, 405, "Method not allowed")
	}
}
func (a *API) departmentCollection(w http.ResponseWriter, r *http.Request) {
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" && u.Role != "HR" && u.Role != "EMPLOYEE" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	if r.Method == http.MethodGet {
		a.respondCall(w, r, func() (any, error) { return a.Management.Departments(r.Context()) })
		return
	}
	if r.Method == http.MethodPost && u.Role == "ADMIN" {
		var in management.DepartmentInput
		if e = decode(r, &in); e == nil {
			v, e := a.Management.CreateDepartment(r.Context(), in)
			if e != nil {
				a.respond(w, r, nil, e)
			} else {
				writeJSON(w, 201, v)
			}
		} else {
			a.respond(w, r, nil, e)
		}
		return
	}
	a.fail(w, r, 405, "Method not allowed")
}
func (a *API) departmentByID(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r, "/departments/")
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	u, e := a.Auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if e != nil {
		a.respond(w, r, nil, e)
		return
	}
	if u.Role != "ADMIN" && u.Role != "HR" && u.Role != "EMPLOYEE" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	if r.Method == http.MethodGet {
		a.respondCall(w, r, func() (any, error) { return a.Management.Department(r.Context(), id) })
		return
	}
	if u.Role != "ADMIN" {
		a.fail(w, r, 403, "Access denied")
		return
	}
	if r.Method == http.MethodPatch {
		var in management.DepartmentPatch
		if e = decode(r, &in); e == nil {
			a.respondCall(w, r, func() (any, error) { return a.Management.PatchDepartment(r.Context(), id, in) })
		} else {
			a.respond(w, r, nil, e)
		}
		return
	}
	if r.Method == http.MethodDelete {
		e = a.Management.DeleteDepartment(r.Context(), id)
		if e != nil {
			a.respond(w, r, nil, e)
		} else {
			w.WriteHeader(204)
		}
		return
	}
	a.fail(w, r, 405, "Method not allowed")
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
	case errors.Is(err, management.ErrCredentials):
		a.fail(w, r, http.StatusUnauthorized, management.ErrCredentials.Error())
	case errors.Is(err, management.ErrNotFound):
		a.fail(w, r, http.StatusNotFound, management.ErrNotFound.Error())
	case errors.Is(err, management.ErrConflict), errors.Is(err, management.ErrReferenced):
		a.fail(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, management.ErrInvalid):
		a.fail(w, r, http.StatusBadRequest, management.ErrInvalid.Error())
	case errors.Is(err, attendance.ErrAlreadyCheckedIn), errors.Is(err, attendance.ErrNotCheckedIn), errors.Is(err, attendance.ErrAlreadyCheckedOut), errors.Is(err, attendance.ErrInvalidDate), errors.Is(err, attendance.ErrInvalidRange):
		a.fail(w, r, http.StatusBadRequest, err.Error())
	default:
		a.Logger.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
		a.fail(w, r, http.StatusInternalServerError, "An unexpected error occurred")
	}
}

func (a *API) respondCall(w http.ResponseWriter, r *http.Request, call func() (any, error)) {
	data, err := call()
	a.respond(w, r, data, err)
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
