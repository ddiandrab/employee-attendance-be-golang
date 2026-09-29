package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnauthorized = errors.New("Authentication required")

type User struct {
	ID    int64
	Email string
	Role  string
}
type UserRepository interface {
	FindByEmail(context.Context, string) (User, error)
}
type Postgres struct{ Pool *pgxpool.Pool }

func (p *Postgres) FindByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := p.Pool.QueryRow(ctx, `SELECT id,email,role FROM "user" WHERE email=$1`, email).Scan(&u.ID, &u.Email, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	return u, err
}

type Authenticator struct {
	users  UserRepository
	secret []byte
}

func New(users UserRepository, secret string) *Authenticator {
	return &Authenticator{users: users, secret: []byte(secret)}
}

// Token issues the same HS256 token shape as Spring's JwtService.
func (a *Authenticator) Token(user User) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.Email, "role": user.Role,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(a.secret)
}

// Authenticate accepts Spring's email subject and HMAC signing algorithms.
// Roles come from the database, never from a possibly stale JWT claim.
func (a *Authenticator) Authenticate(ctx context.Context, header string) (User, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return User{}, ErrUnauthorized
	}
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
		minimum := map[string]int{"HS256": 32, "HS384": 48, "HS512": 64}[token.Method.Alg()]
		if minimum == 0 || len(a.secret) < minimum {
			return nil, ErrUnauthorized
		}
		return a.secret, nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || strings.TrimSpace(claims.Subject) == "" {
		return User{}, ErrUnauthorized
	}
	u, err := a.users.FindByEmail(ctx, claims.Subject)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return User{}, err
		}
		return User{}, fmt.Errorf("load authenticated user: %w", err)
	}
	return u, nil
}
