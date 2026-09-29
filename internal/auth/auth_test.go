package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type usersStub struct {
	role string
	err  error
}

func (s usersStub) FindByEmail(_ context.Context, email string) (User, error) {
	return User{ID: 7, Email: email, Role: s.role}, s.err
}

func TestSpringJWT(t *testing.T) {
	secret := strings.Repeat("s", 64)
	for _, method := range []*jwt.SigningMethodHMAC{jwt.SigningMethodHS256, jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		t.Run(method.Alg(), func(t *testing.T) {
			token, err := jwt.NewWithClaims(method, jwt.MapClaims{"sub": "employee@example.com", "role": "ADMIN", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			u, err := New(usersStub{role: "EMPLOYEE"}, secret).Authenticate(context.Background(), "Bearer "+token)
			if err != nil || u.Email != "employee@example.com" || u.Role != "EMPLOYEE" {
				t.Fatalf("%+v %v", u, err)
			}
		})
	}
}

func TestRejectInvalidJWT(t *testing.T) {
	secret := strings.Repeat("s", 64)
	for _, tc := range []struct {
		name   string
		claims jwt.MapClaims
		key    string
		method jwt.SigningMethod
		dbErr  error
	}{
		{"expired", jwt.MapClaims{"sub": "e", "exp": time.Now().Add(-time.Second).Unix()}, secret, jwt.SigningMethodHS256, nil},
		{"missing expiry", jwt.MapClaims{"sub": "e"}, secret, jwt.SigningMethodHS256, nil},
		{"missing subject", jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}, secret, jwt.SigningMethodHS256, nil},
		{"future nbf", jwt.MapClaims{"sub": "e", "exp": time.Now().Add(time.Hour).Unix(), "nbf": time.Now().Add(time.Minute).Unix()}, secret, jwt.SigningMethodHS256, nil},
		{"wrong key", jwt.MapClaims{"sub": "e", "exp": time.Now().Add(time.Hour).Unix()}, strings.Repeat("x", 64), jwt.SigningMethodHS256, nil},
		{"deleted user", jwt.MapClaims{"sub": "e", "exp": time.Now().Add(time.Hour).Unix()}, secret, jwt.SigningMethodHS256, ErrUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(tc.method, tc.claims).SignedString([]byte(tc.key))
			if err != nil {
				t.Fatal(err)
			}
			_, err = New(usersStub{err: tc.dbErr}, secret).Authenticate(context.Background(), "Bearer "+token)
			if !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("expected unauthorized, got %v", err)
			}
		})
	}
	for _, header := range []string{"", "Bearer", "Basic abc", "Bearer garbage", "Bearer a b"} {
		if _, err := New(usersStub{}, secret).Authenticate(context.Background(), header); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("accepted %q", header)
		}
	}
	unsigned, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "e", "exp": time.Now().Add(time.Hour).Unix()}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := New(usersStub{}, secret).Authenticate(context.Background(), "Bearer "+unsigned); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("accepted unsigned token")
	}
}
