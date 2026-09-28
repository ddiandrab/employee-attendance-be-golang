package attendance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ Pool *pgxpool.Pool }

func (p *Postgres) EmployeeID(ctx context.Context, userID int64) (int64, error) {
	var id int64
	err := p.Pool.QueryRow(ctx, `SELECT id FROM employee WHERE "userId"=$1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrProfileMissing
	}
	return id, err
}

const recordColumns = `id, "employeeId", "attendanceDate"::text, "checkIn", "checkOut", "createdAt", "updatedAt"`

func scanRecord(row pgx.Row) (Record, error) {
	var r Record
	err := row.Scan(&r.ID, &r.EmployeeID, &r.AttendanceDate, &r.CheckIn, &r.CheckOut, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (p *Postgres) CheckIn(ctx context.Context, id int64, date string, now time.Time) (Record, error) {
	r, err := scanRecord(p.Pool.QueryRow(ctx, `INSERT INTO "attendanceRecord"
  ("employeeId", "attendanceDate", "checkIn", "createdAt", "updatedAt")
  VALUES ($1,$2,$3,$3,$3) ON CONFLICT ("employeeId", "attendanceDate") DO NOTHING
  RETURNING `+recordColumns, id, date, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrAlreadyCheckedIn
	}
	return r, err
}

func (p *Postgres) CheckOut(ctx context.Context, id int64, date string, now time.Time) (Record, error) {
	// The predicate makes concurrent check-outs atomic: only one can succeed.
	r, err := scanRecord(p.Pool.QueryRow(ctx, `UPDATE "attendanceRecord"
  SET "checkOut"=GREATEST($3,"checkIn"), "updatedAt"=GREATEST($3,"checkIn")
  WHERE "employeeId"=$1 AND "attendanceDate"=$2 AND "checkIn" IS NOT NULL AND "checkOut" IS NULL
  RETURNING `+recordColumns, id, date, now))
	if !errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	var checkedIn bool
	err = p.Pool.QueryRow(ctx, `SELECT "checkIn" IS NOT NULL FROM "attendanceRecord"
  WHERE "employeeId"=$1 AND "attendanceDate"=$2`, id, date).Scan(&checkedIn)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !checkedIn) {
		return Record{}, ErrNotCheckedIn
	}
	if err != nil {
		return Record{}, err
	}
	return Record{}, ErrAlreadyCheckedOut
}

func (p *Postgres) Mine(ctx context.Context, id int64, dates DateRange) ([]Record, error) {
	rows, err := p.Pool.Query(ctx, `SELECT `+recordColumns+` FROM "attendanceRecord"
  WHERE "employeeId"=$1 AND "attendanceDate" BETWEEN $2 AND $3 ORDER BY "attendanceDate" DESC, id DESC`, id, dates.From, dates.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Record, 0)
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (p *Postgres) All(ctx context.Context, dates DateRange) ([]ListRecord, error) {
	rows, err := p.Pool.Query(ctx, `SELECT a.id, a."employeeId", e."employeeNumber",
  COALESCE(NULLIF(trim(concat_ws(' ',NULLIF(trim(e."firstName"),''),NULLIF(trim(e."lastName"),''))),''),'-'),
  e."departmentId", e.position, a."attendanceDate"::text, a."checkIn", a."checkOut"
  FROM "attendanceRecord" a JOIN employee e ON e.id=a."employeeId"
  WHERE a."attendanceDate" BETWEEN $1 AND $2 ORDER BY a."attendanceDate" DESC, a.id DESC`, dates.From, dates.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ListRecord, 0)
	for rows.Next() {
		var r ListRecord
		if err := rows.Scan(&r.ID, &r.EmployeeID, &r.EmployeeNumber, &r.EmployeeName, &r.DepartmentID, &r.Position, &r.AttendanceDate, &r.CheckIn, &r.CheckOut); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
