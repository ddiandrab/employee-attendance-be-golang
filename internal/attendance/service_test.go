package attendance

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingRepository struct {
	Repository
	date       string
	now        time.Time
	userID     int64
	profileErr error
}

func (r *recordingRepository) EmployeeID(_ context.Context, id int64) (int64, error) {
	r.userID = id
	return 17, r.profileErr
}
func (r *recordingRepository) CheckIn(_ context.Context, id int64, date string, now time.Time) (Record, error) {
	r.date = date
	r.now = now
	return Record{EmployeeID: id}, nil
}
func (r *recordingRepository) CheckOut(ctx context.Context, id int64, date string, now time.Time) (Record, error) {
	return r.CheckIn(ctx, id, date, now)
}

func TestBusinessDate(t *testing.T) {
	for _, tc := range []struct{ utc, date string }{
		{"2026-09-30T16:59:59Z", "2026-09-30"},
		{"2026-09-30T17:00:00Z", "2026-10-01"},
		{"2026-12-31T17:00:00Z", "2027-01-01"},
	} {
		t.Run(tc.utc, func(t *testing.T) {
			instant, _ := time.Parse(time.RFC3339, tc.utc)
			repo := &recordingRepository{}
			service := NewService(repo, func() time.Time { return instant })
			for _, operation := range []func(context.Context, int64) (Record, error){service.CheckIn, service.CheckOut} {
				record, err := operation(context.Background(), 9)
				if err != nil || repo.date != tc.date || !repo.now.Equal(instant) || repo.userID != 9 || record.EmployeeID != 17 {
					t.Fatalf("wrong identity/date: %+v %+v %v", record, repo, err)
				}
			}
		})
	}
}

func TestDateRanges(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-09-28T00:00:00Z")
	service := NewService(nil, func() time.Time { return now })
	for _, tc := range []struct {
		name, from, to, wantFrom, wantTo string
		wantErr                          error
	}{
		{"defaults", "", "", "2026-09-01", "2026-09-28", nil},
		{"blank", " ", " ", "2026-09-01", "2026-09-28", nil},
		{"from only", "2026-08-01", "", "2026-08-01", "2026-09-28", nil},
		{"to only", "", "2026-09-30", "2026-09-01", "2026-09-30", nil},
		{"same day", "2026-09-01", "2026-09-01", "2026-09-01", "2026-09-01", nil},
		{"invalid", "2026-02-30", "", "", "", ErrInvalidDate},
		{"format", "28-09-2026", "", "", "", ErrInvalidDate},
		{"reverse", "2026-09-29", "2026-09-01", "", "", ErrInvalidRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := service.dateRange(tc.from, tc.to)
			if !errors.Is(err, tc.wantErr) || got.From != tc.wantFrom || got.To != tc.wantTo {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}

func TestMissingEmployee(t *testing.T) {
	service := NewService(&recordingRepository{profileErr: ErrProfileMissing}, time.Now)
	for _, operation := range []func(context.Context, int64) (Record, error){service.CheckIn, service.CheckOut} {
		if _, err := operation(context.Background(), 1); !errors.Is(err, ErrProfileMissing) {
			t.Fatal(err)
		}
	}
	if _, err := service.Mine(context.Background(), 1, "", ""); !errors.Is(err, ErrProfileMissing) {
		t.Fatal(err)
	}
}
