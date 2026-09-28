package httpapi_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ddiandrab/employee-attendance-be-golang/internal/attendance"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/auth"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/database"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/httpapi"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed testdata/spring_schema.sql
var schemaSQL string

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration tests skipped")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{fmt.Sprintf("attendance_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatal(err)
	}
	return pool
}

const testSecret = "integration-test-secret-at-least-sixty-four-bytes-long-for-spring-hs512"

func token(t *testing.T, email string) string {
	t.Helper()
	// Spring JJWT chooses HS512 for a >=64-byte key. Deliberately stale role.
	value, err := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub": email, "role": "ADMIN", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func request(handler http.Handler, method, path, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func expect(t *testing.T, r *httptest.ResponseRecorder, status int) {
	t.Helper()
	if r.Code != status {
		t.Fatalf("status=%d want=%d body=%s", r.Code, status, r.Body.String())
	}
	if !strings.HasPrefix(r.Header().Get("Content-Type"), "application/json") {
		t.Fatal("response must be JSON")
	}
}

func TestAttendanceAPIIntegration(t *testing.T) {
	pool := testDB(t)
	now, _ := time.Parse(time.RFC3339, "2026-09-30T17:00:00Z") // Oct 1 in Jakarta.
	api := &httpapi.API{Attendance: attendance.NewService(&attendance.Postgres{Pool: pool}, func() time.Time { return now }), Auth: auth.New(&auth.Postgres{Pool: pool}, testSecret), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigin: "http://localhost:5173"}
	handler := api.Handler()
	employee := token(t, "employee@example.com")
	other := token(t, "other@example.com")
	hr := token(t, "hr@example.com")
	admin := token(t, "admin@example.com")
	noProfile := token(t, "noprofile@example.com")

	for _, path := range []string{"/attendance", "/attendance/me", "/attendance/check-in", "/attendance/check-out"} {
		expect(t, request(handler, "GET", path, ""), 401)
		expect(t, request(handler, "GET", path, "invalid"), 401)
	}
	expect(t, request(handler, "GET", "/attendance", employee), 403)
	expect(t, request(handler, "POST", "/attendance/check-in", noProfile), 404)
	expect(t, request(handler, "POST", "/attendance/check-out", noProfile), 404)
	expect(t, request(handler, "GET", "/attendance/me", noProfile), 404)
	expect(t, request(handler, "POST", "/attendance/check-out", employee), 400)
	empty := request(handler, "GET", "/attendance/me", employee)
	expect(t, empty, 200)
	if strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatal(empty.Body.String())
	}

	checkedIn := request(handler, "POST", "/attendance/check-in", employee)
	expect(t, checkedIn, 200)
	var initial attendance.Record
	if err := json.Unmarshal(checkedIn.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.EmployeeID != 1 || initial.AttendanceDate != "2026-10-01" || initial.CheckIn == nil || !initial.CheckIn.Equal(now) || initial.CheckOut != nil {
		t.Fatalf("%+v", initial)
	}
	expect(t, request(handler, "POST", "/attendance/check-in", employee), 400)
	now = now.Add(8 * time.Hour)
	checkedOut := request(handler, "POST", "/attendance/check-out", employee)
	expect(t, checkedOut, 200)
	var final attendance.Record
	if err := json.Unmarshal(checkedOut.Body.Bytes(), &final); err != nil {
		t.Fatal(err)
	}
	if final.ID != initial.ID || final.CheckOut == nil || !final.CheckOut.Equal(now) || !final.CreatedAt.Equal(initial.CreatedAt) || !final.UpdatedAt.Equal(now) {
		t.Fatalf("%+v", final)
	}
	expect(t, request(handler, "POST", "/attendance/check-out", employee), 400)
	expect(t, request(handler, "POST", "/attendance/check-in", other), 200)

	// Historical rows prove defaults, inclusive endpoints, descending order and ownership.
	_, err := pool.Exec(context.Background(), `INSERT INTO "attendanceRecord"("employeeId","attendanceDate","checkIn") VALUES (1,'2026-09-01','2026-09-01T01:00:00Z'),(1,'2026-09-30','2026-09-30T01:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, bearer string
		count        int
		first        string
	}{
		{"/attendance/me", employee, 1, "2026-10-01"},
		{"/attendance/me?from=2026-09-01&to=2026-09-30", employee, 2, "2026-09-30"},
		{"/attendance/me?from=2026-09-01&to=2026-09-01", employee, 1, "2026-09-01"},
		{"/attendance/me?from=2026-09-01", other, 1, "2026-10-01"},
	} {
		r := request(handler, "GET", tc.path, tc.bearer)
		expect(t, r, 200)
		var records []attendance.Record
		if err := json.Unmarshal(r.Body.Bytes(), &records); err != nil {
			t.Fatal(err)
		}
		if len(records) != tc.count || records[0].AttendanceDate != tc.first {
			t.Fatalf("%s: %+v", tc.path, records)
		}
	}
	for i, bearer := range []string{hr, admin} {
		r := request(handler, "GET", "/attendance", bearer)
		expect(t, r, 200)
		var records []attendance.ListRecord
		if err := json.Unmarshal(r.Body.Bytes(), &records); err != nil {
			t.Fatal(err)
		}
		if len(records) != 2+i {
			t.Fatalf("%+v", records)
		}
		var found bool
		for _, record := range records {
			if record.EmployeeID == 1 {
				found = true
				if record.EmployeeName != "Rani Putri" || record.EmployeeNumber != "EMP001" || record.DepartmentID == nil || *record.DepartmentID != 1 || record.Position == nil || *record.Position != "Engineer" {
					t.Fatalf("%+v", record)
				}
			}
		}
		if !found {
			t.Fatal("missing employee summary")
		}
		expect(t, request(handler, "POST", "/attendance/check-in", bearer), 200)
		expect(t, request(handler, "GET", "/attendance/me", bearer), 200)
		expect(t, request(handler, "POST", "/attendance/check-out", bearer), 200)
	}
	for _, path := range []string{"/attendance/me?from=invalid", "/attendance/me?to=2026-02-30", "/attendance?from=2026-10-02&to=2026-10-01"} {
		r := request(handler, "GET", path, hr)
		expect(t, r, 400)
		var body map[string]any
		_ = json.Unmarshal(r.Body.Bytes(), &body)
		for _, field := range []string{"timestamp", "status", "error", "message", "path"} {
			if body[field] == nil {
				t.Fatalf("missing %s", field)
			}
		}
	}
	expect(t, request(handler, "DELETE", "/attendance/check-in", employee), 405)
	expect(t, request(handler, "GET", "/attendance/unknown", employee), 404)
	// Role changes take effect immediately even with an existing signed token.
	if _, err = pool.Exec(context.Background(), `UPDATE "user" SET role='EMPLOYEE' WHERE email='hr@example.com'`); err != nil {
		t.Fatal(err)
	}
	expect(t, request(handler, "GET", "/attendance", hr), 403)
	if _, err = pool.Exec(context.Background(), `UPDATE "user" SET role='UNKNOWN' WHERE email='hr@example.com'`); err != nil {
		t.Fatal(err)
	}
	expect(t, request(handler, "GET", "/attendance/me", hr), 403)
	expect(t, request(handler, "GET", "/attendance/me", token(t, "deleted@example.com")), 401)
	// A new business date cannot check out yesterday's record.
	now = now.Add(24 * time.Hour)
	expect(t, request(handler, "POST", "/attendance/check-out", employee), 400)
	expect(t, request(handler, "POST", "/attendance/check-in", employee), 200)
}

func TestConcurrentAttendanceIntegration(t *testing.T) {
	pool := testDB(t)
	now, _ := time.Parse(time.RFC3339, "2026-09-28T01:00:00Z")
	repo := &attendance.Postgres{Pool: pool}
	api := &httpapi.API{Attendance: attendance.NewService(repo, func() time.Time { return now }), Auth: auth.New(&auth.Postgres{Pool: pool}, testSecret), Logger: slog.Default()}
	handler := api.Handler()
	bearer := token(t, "employee@example.com")
	for _, path := range []string{"/attendance/check-in", "/attendance/check-out"} {
		statuses := make(chan int, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Go(func() { statuses <- request(handler, "POST", path, bearer).Code })
		}
		wg.Wait()
		close(statuses)
		successes := 0
		for status := range statuses {
			if status == 200 {
				successes++
			} else if status != 400 {
				t.Fatalf("unexpected %d", status)
			}
		}
		if successes != 1 {
			t.Fatalf("%s: %d successes", path, successes)
		}
	}
	records, err := repo.Mine(context.Background(), 1, attendance.DateRange{From: "2026-09-28", To: "2026-09-28"})
	if err != nil || len(records) != 1 || records[0].CheckOut == nil {
		t.Fatalf("%+v %v", records, err)
	}
	// A legacy nullable check-in must not be checked out.
	_, err = pool.Exec(context.Background(), `INSERT INTO "attendanceRecord"("employeeId","attendanceDate") VALUES (2,'2026-09-28')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CheckOut(context.Background(), 2, "2026-09-28", now); err != attendance.ErrNotCheckedIn {
		t.Fatal(err)
	}
}
