package management

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

var (
	ErrNotFound    = errors.New("Resource not found")
	ErrConflict    = errors.New("Resource already exists")
	ErrReferenced  = errors.New("Resource is still referenced")
	ErrInvalid     = errors.New("Invalid request")
	ErrCredentials = errors.New("Invalid email or password")
)

type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Department struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type Employee struct {
	ID             int64     `json:"id"`
	EmployeeNumber string    `json:"employeeNumber"`
	FirstName      string    `json:"firstName"`
	LastName       *string   `json:"lastName"`
	Email          *string   `json:"email"`
	Phone          *string   `json:"phone"`
	PhotoURL       *string   `json:"photoUrl"`
	Position       *string   `json:"position"`
	JoinDate       *string   `json:"joinDate"`
	IsActive       bool      `json:"isActive"`
	UserID         int64     `json:"userId"`
	DepartmentID   *int64    `json:"departmentId"`
	DepartmentName *string   `json:"departmentName"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
type UserInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}
type UserPatch struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Role     *string `json:"role"`
}
type DepartmentInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}
type DepartmentPatch struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}
type EmployeeInput struct {
	EmployeeNumber string  `json:"employeeNumber"`
	FirstName      string  `json:"firstName"`
	LastName       *string `json:"lastName"`
	Email          string  `json:"email"`
	Password       string  `json:"password"`
	Phone          *string `json:"phone"`
	PhotoURL       *string `json:"photoUrl"`
	DepartmentID   *int64  `json:"departmentId"`
	Position       *string `json:"position"`
	JoinDate       *string `json:"joinDate"`
}
type EmployeePatch struct {
	EmployeeNumber *string `json:"employeeNumber"`
	FirstName      *string `json:"firstName"`
	LastName       *string `json:"lastName"`
	Email          *string `json:"email"`
	Phone          *string `json:"phone"`
	PhotoURL       *string `json:"photoUrl"`
	DepartmentID   *int64  `json:"departmentId"`
	Position       *string `json:"position"`
	JoinDate       *string `json:"joinDate"`
	IsActive       *bool   `json:"isActive"`
}
type ProfilePatch struct {
	Phone    *string `json:"phone"`
	PhotoURL *string `json:"photoUrl"`
}

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: func() time.Time { return time.Now().UTC() }}
}

func validEmail(v string) bool {
	v = strings.TrimSpace(v)
	return strings.Count(v, "@") == 1 && !strings.HasPrefix(v, "@") && !strings.HasSuffix(v, "@") && !strings.ContainsAny(v, " \t\n")
}
func validRole(v string) bool { return v == "ADMIN" || v == "HR" || v == "EMPLOYEE" }
func required(v string) bool  { return strings.TrimSpace(v) != "" }
func passwordHash(raw string) (string, error) {
	if len(raw) < 8 {
		return "", ErrInvalid
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key := argon2.IDKey([]byte(raw), salt, 3, 65536, 4, 32)
	return "$argon2id$v=19$m=65536,t=3,p=4$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func passwordMatches(raw, encoded string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" {
		return false
	}
	var m uint32
	var t uint32
	var parallel uint8
	if _, e := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &m, &t, &parallel); e != nil || m > 65536 || t > 3 || parallel > 4 || m == 0 || t == 0 || parallel == 0 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[4])
	if e != nil {
		return false
	}
	got, e := base64.RawStdEncoding.DecodeString(p[5])
	if e != nil {
		return false
	}
	return string(argon2.IDKey([]byte(raw), salt, t, m, parallel, uint32(len(got)))) == string(got)
}
func scanUser(row pgx.Row) (User, error) {
	var v User
	e := row.Scan(&v.ID, &v.Email, &v.Role, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanDepartment(row pgx.Row) (Department, error) {
	var v Department
	e := row.Scan(&v.ID, &v.Name, &v.Description, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanEmployee(row pgx.Row) (Employee, error) {
	var v Employee
	e := row.Scan(&v.ID, &v.EmployeeNumber, &v.FirstName, &v.LastName, &v.Email, &v.Phone, &v.PhotoURL, &v.Position, &v.JoinDate, &v.IsActive, &v.UserID, &v.DepartmentID, &v.DepartmentName, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

const userColumns = `id,email,role,"createdAt","updatedAt"`
const deptColumns = `id,name,description,"createdAt","updatedAt"`
const empColumns = `e.id,e."employeeNumber",e."firstName",e."lastName",e.email,e.phone,e."photoUrl",e.position,e."joinDate"::text,e."isActive",e."userId",e."departmentId",d.name,e."createdAt",e."updatedAt"`
const empFrom = ` FROM employee e LEFT JOIN department d ON d.id=e."departmentId" `

func (s *Service) Login(ctx context.Context, email, password string) (User, error) {
	var u User
	var hash string
	e := s.pool.QueryRow(ctx, `SELECT id,email,role,"createdAt","updatedAt","passwordHash" FROM "user" WHERE email=$1`, email).Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt, &hash)
	if e != nil || !passwordMatches(password, hash) {
		return User{}, ErrCredentials
	}
	return u, nil
}
func (s *Service) Me(ctx context.Context, id int64) (User, error) {
	u, e := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM "user" WHERE id=$1`, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, e
}
func (s *Service) Users(ctx context.Context) ([]User, error) {
	rows, e := s.pool.Query(ctx, `SELECT `+userColumns+` FROM "user" ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		v, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) User(ctx context.Context, id int64) (User, error) { return s.Me(ctx, id) }
func (s *Service) CreateUser(ctx context.Context, in UserInput) (User, error) {
	if !validEmail(in.Email) || !validRole(in.Role) {
		return User{}, ErrInvalid
	}
	h, e := passwordHash(in.Password)
	if e != nil {
		return User{}, e
	}
	now := s.now()
	u, e := scanUser(s.pool.QueryRow(ctx, `INSERT INTO "user"(email,"passwordHash",role,"createdAt","updatedAt") VALUES($1,$2,$3,$4,$4) RETURNING `+userColumns, in.Email, h, in.Role, now))
	if isUnique(e) {
		return User{}, ErrConflict
	}
	return u, e
}
func (s *Service) PatchUser(ctx context.Context, id int64, in UserPatch) (User, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return User{}, e
	}
	defer tx.Rollback(ctx)
	var email, role, hash string
	e = tx.QueryRow(ctx, `SELECT email,role,"passwordHash" FROM "user" WHERE id=$1 FOR UPDATE`, id).Scan(&email, &role, &hash)
	if errors.Is(e, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if e != nil {
		return User{}, e
	}
	if in.Email != nil {
		if !validEmail(*in.Email) {
			return User{}, ErrInvalid
		}
		email = *in.Email
	}
	if in.Role != nil {
		if !validRole(*in.Role) {
			return User{}, ErrInvalid
		}
		role = *in.Role
	}
	if in.Password != nil {
		hash, e = passwordHash(*in.Password)
		if e != nil {
			return User{}, e
		}
	}
	now := s.now()
	u, e := scanUser(tx.QueryRow(ctx, `UPDATE "user" SET email=$2,role=$3,"passwordHash"=$4,"updatedAt"=$5 WHERE id=$1 RETURNING `+userColumns, id, email, role, hash, now))
	if isUnique(e) {
		return User{}, ErrConflict
	}
	if e != nil {
		return User{}, e
	}
	_, e = tx.Exec(ctx, `UPDATE employee SET email=$2,"updatedAt"=$3 WHERE "userId"=$1`, id, email, now)
	if e != nil {
		return User{}, e
	}
	return u, tx.Commit(ctx)
}
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	tag, e := s.pool.Exec(ctx, `DELETE FROM "user" WHERE id=$1`, id)
	if isFK(e) {
		return ErrReferenced
	}
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Departments(ctx context.Context) ([]Department, error) {
	rows, e := s.pool.Query(ctx, `SELECT `+deptColumns+` FROM department ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Department{}
	for rows.Next() {
		v, e := scanDepartment(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Department(ctx context.Context, id int64) (Department, error) {
	v, e := scanDepartment(s.pool.QueryRow(ctx, `SELECT `+deptColumns+` FROM department WHERE id=$1`, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return Department{}, ErrNotFound
	}
	return v, e
}
func (s *Service) CreateDepartment(ctx context.Context, in DepartmentInput) (Department, error) {
	if !required(in.Name) || len(in.Name) > 100 || in.Description != nil && len(*in.Description) > 255 {
		return Department{}, ErrInvalid
	}
	now := s.now()
	v, e := scanDepartment(s.pool.QueryRow(ctx, `INSERT INTO department(name,description,"createdAt","updatedAt") VALUES($1,$2,$3,$3) RETURNING `+deptColumns, in.Name, in.Description, now))
	if isUnique(e) {
		return Department{}, ErrConflict
	}
	return v, e
}
func (s *Service) PatchDepartment(ctx context.Context, id int64, in DepartmentPatch) (Department, error) {
	v, e := s.Department(ctx, id)
	if e != nil {
		return v, e
	}
	if in.Name != nil {
		if !required(*in.Name) || len(*in.Name) > 100 {
			return v, ErrInvalid
		}
		v.Name = *in.Name
	}
	if in.Description != nil {
		if len(*in.Description) > 255 {
			return v, ErrInvalid
		}
		v.Description = in.Description
	}
	v, e = scanDepartment(s.pool.QueryRow(ctx, `UPDATE department SET name=$2,description=$3,"updatedAt"=$4 WHERE id=$1 RETURNING `+deptColumns, id, v.Name, v.Description, s.now()))
	if isUnique(e) {
		return Department{}, ErrConflict
	}
	return v, e
}
func (s *Service) DeleteDepartment(ctx context.Context, id int64) error {
	tag, e := s.pool.Exec(ctx, `DELETE FROM department WHERE id=$1`, id)
	if isFK(e) {
		return ErrReferenced
	}
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Employees(ctx context.Context) ([]Employee, error) {
	rows, e := s.pool.Query(ctx, `SELECT `+empColumns+empFrom+` ORDER BY e.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Employee{}
	for rows.Next() {
		v, e := scanEmployee(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Employee(ctx context.Context, id int64) (Employee, error) {
	v, e := scanEmployee(s.pool.QueryRow(ctx, `SELECT `+empColumns+empFrom+` WHERE e.id=$1`, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	return v, e
}
func (s *Service) MyEmployee(ctx context.Context, userID int64) (Employee, error) {
	v, e := scanEmployee(s.pool.QueryRow(ctx, `SELECT `+empColumns+empFrom+` WHERE e."userId"=$1`, userID))
	if errors.Is(e, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	return v, e
}
func (s *Service) CreateEmployee(ctx context.Context, in EmployeeInput) (Employee, error) {
	if !required(in.EmployeeNumber) || !required(in.FirstName) || !validEmail(in.Email) {
		return Employee{}, ErrInvalid
	}
	if in.JoinDate != nil {
		if _, e := time.Parse(time.DateOnly, *in.JoinDate); e != nil {
			return Employee{}, ErrInvalid
		}
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Employee{}, e
	}
	defer tx.Rollback(ctx)
	if in.DepartmentID != nil {
		var ok bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM department WHERE id=$1)`, *in.DepartmentID).Scan(&ok)
		if e != nil {
			return Employee{}, e
		}
		if !ok {
			return Employee{}, ErrNotFound
		}
	}
	h, e := passwordHash(in.Password)
	if e != nil {
		return Employee{}, e
	}
	now := s.now()
	var uid int64
	e = tx.QueryRow(ctx, `INSERT INTO "user"(email,"passwordHash",role,"createdAt","updatedAt") VALUES($1,$2,'EMPLOYEE',$3,$3) RETURNING id`, in.Email, h, now).Scan(&uid)
	if isUnique(e) {
		return Employee{}, ErrConflict
	}
	if e != nil {
		return Employee{}, e
	}
	var id int64
	e = tx.QueryRow(ctx, `INSERT INTO employee("userId","employeeNumber","firstName","lastName",email,phone,"photoUrl","departmentId",position,"joinDate","isActive","createdAt","updatedAt") VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,$11,$11) RETURNING id`, uid, in.EmployeeNumber, in.FirstName, in.LastName, in.Email, in.Phone, in.PhotoURL, in.DepartmentID, in.Position, in.JoinDate, now).Scan(&id)
	if isUnique(e) {
		return Employee{}, ErrConflict
	}
	if e != nil {
		return Employee{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Employee{}, e
	}
	return s.Employee(ctx, id)
}
func (s *Service) PatchEmployee(ctx context.Context, id int64, in EmployeePatch) (Employee, error) {
	v, e := s.Employee(ctx, id)
	if e != nil {
		return v, e
	}
	if in.EmployeeNumber != nil {
		if !required(*in.EmployeeNumber) {
			return v, ErrInvalid
		}
		v.EmployeeNumber = *in.EmployeeNumber
	}
	if in.FirstName != nil {
		if !required(*in.FirstName) {
			return v, ErrInvalid
		}
		v.FirstName = *in.FirstName
	}
	if in.Email != nil {
		if !validEmail(*in.Email) {
			return v, ErrInvalid
		}
		v.Email = in.Email
	}
	if in.Phone != nil {
		v.Phone = in.Phone
	}
	if in.PhotoURL != nil {
		v.PhotoURL = in.PhotoURL
	}
	if in.LastName != nil {
		v.LastName = in.LastName
	}
	if in.Position != nil {
		v.Position = in.Position
	}
	if in.JoinDate != nil {
		if _, e := time.Parse(time.DateOnly, *in.JoinDate); e != nil {
			return v, ErrInvalid
		}
		v.JoinDate = in.JoinDate
	}
	if in.IsActive != nil {
		v.IsActive = *in.IsActive
	}
	if in.DepartmentID != nil {
		if _, e := s.Department(ctx, *in.DepartmentID); e != nil {
			return v, e
		}
		v.DepartmentID = in.DepartmentID
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	now := s.now()
	if in.Email != nil {
		_, e = tx.Exec(ctx, `UPDATE "user" SET email=$2,"updatedAt"=$3 WHERE id=$1`, v.UserID, *in.Email, now)
		if isUnique(e) {
			return v, ErrConflict
		}
		if e != nil {
			return v, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE employee SET "employeeNumber"=$2,"firstName"=$3,"lastName"=$4,email=$5,phone=$6,"photoUrl"=$7,position=$8,"joinDate"=$9,"departmentId"=$10,"isActive"=$11,"updatedAt"=$12 WHERE id=$1`, id, v.EmployeeNumber, v.FirstName, v.LastName, v.Email, v.Phone, v.PhotoURL, v.Position, v.JoinDate, v.DepartmentID, v.IsActive, now)
	if isUnique(e) {
		return v, ErrConflict
	}
	if e != nil {
		return v, e
	}
	if e = tx.Commit(ctx); e != nil {
		return v, e
	}
	return s.Employee(ctx, id)
}
func (s *Service) PatchProfile(ctx context.Context, userID int64, in ProfilePatch) (Employee, error) {
	v, e := s.MyEmployee(ctx, userID)
	if e != nil {
		return v, e
	}
	return s.PatchEmployee(ctx, v.ID, EmployeePatch{Phone: in.Phone, PhotoURL: in.PhotoURL})
}
func (s *Service) DeleteEmployee(ctx context.Context, id int64) error {
	tag, e := s.pool.Exec(ctx, `DELETE FROM employee WHERE id=$1`, id)
	if isFK(e) {
		return ErrReferenced
	}
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func isUnique(e error) bool {
	if p, ok := e.(*pgconn.PgError); ok {
		return p.Code == "23505"
	}
	return false
}
func isFK(e error) bool {
	if p, ok := e.(*pgconn.PgError); ok {
		return p.Code == "23503"
	}
	return false
}
