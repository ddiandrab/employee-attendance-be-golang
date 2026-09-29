package attendance

import (
	"context"
	"errors"
	"strings"
	"time"
	_ "time/tzdata"
)

var (
	ErrProfileMissing    = errors.New("Employee profile not found")
	ErrAlreadyCheckedIn  = errors.New("Employee has already checked in today")
	ErrNotCheckedIn      = errors.New("Employee has not checked in today")
	ErrAlreadyCheckedOut = errors.New("Employee has already checked out today")
	ErrInvalidDate       = errors.New("Date must use the ISO-8601 format (YYYY-MM-DD)")
	ErrInvalidRange      = errors.New("from must not be after to")
)

type Record struct {
	ID             int64      `json:"id"`
	EmployeeID     int64      `json:"employeeId"`
	AttendanceDate string     `json:"attendanceDate"`
	CheckIn        *time.Time `json:"checkIn"`
	CheckOut       *time.Time `json:"checkOut"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type ListRecord struct {
	ID             int64      `json:"id"`
	EmployeeID     int64      `json:"employeeId"`
	EmployeeNumber string     `json:"employeeNumber"`
	EmployeeName   string     `json:"employeeName"`
	DepartmentID   *int64     `json:"departmentId"`
	Position       *string    `json:"position"`
	AttendanceDate string     `json:"attendanceDate"`
	CheckIn        *time.Time `json:"checkIn"`
	CheckOut       *time.Time `json:"checkOut"`
}

type DateRange struct{ From, To string }

type Repository interface {
	EmployeeID(context.Context, int64) (int64, error)
	CheckIn(context.Context, int64, string, time.Time) (Record, error)
	CheckOut(context.Context, int64, string, time.Time) (Record, error)
	Mine(context.Context, int64, DateRange) ([]Record, error)
	All(context.Context, DateRange) ([]ListRecord, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
	zone *time.Location
}

func NewService(repo Repository, now func() time.Time) *Service {
	zone, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err)
	} // tzdata is embedded in the binary.
	return &Service{repo: repo, now: now, zone: zone}
}

func (s *Service) CheckIn(ctx context.Context, userID int64) (Record, error) {
	employeeID, err := s.repo.EmployeeID(ctx, userID)
	if err != nil {
		return Record{}, err
	}
	now := s.now().UTC()
	return s.repo.CheckIn(ctx, employeeID, now.In(s.zone).Format(time.DateOnly), now)
}

func (s *Service) CheckOut(ctx context.Context, userID int64) (Record, error) {
	employeeID, err := s.repo.EmployeeID(ctx, userID)
	if err != nil {
		return Record{}, err
	}
	now := s.now().UTC()
	return s.repo.CheckOut(ctx, employeeID, now.In(s.zone).Format(time.DateOnly), now)
}

func (s *Service) Mine(ctx context.Context, userID int64, from, to string) ([]Record, error) {
	dates, err := s.dateRange(from, to)
	if err != nil {
		return nil, err
	}
	employeeID, err := s.repo.EmployeeID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.Mine(ctx, employeeID, dates)
}

func (s *Service) All(ctx context.Context, from, to string) ([]ListRecord, error) {
	dates, err := s.dateRange(from, to)
	if err != nil {
		return nil, err
	}
	return s.repo.All(ctx, dates)
}

func (s *Service) dateRange(from, to string) (DateRange, error) {
	today := s.now().In(s.zone)
	if strings.TrimSpace(from) == "" {
		from = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, s.zone).Format(time.DateOnly)
	}
	if strings.TrimSpace(to) == "" {
		to = today.Format(time.DateOnly)
	}
	for _, date := range []string{from, to} {
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return DateRange{}, ErrInvalidDate
		}
	}
	if from > to {
		return DateRange{}, ErrInvalidRange
	}
	return DateRange{From: from, To: to}, nil
}
