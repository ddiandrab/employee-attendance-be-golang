package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("CORS_ALLOWED_ORIGIN", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database URL accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/example")
	if _, err := Load(); err == nil {
		t.Fatal("missing JWT secret accepted")
	}
	t.Setenv("JWT_SECRET", "test-key-at-least-thirty-two-bytes")
	got, err := Load()
	if err != nil || got.Address != ":8081" || got.AllowedOrigin != "http://localhost:5173" {
		t.Fatalf("%+v %v", got, err)
	}
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("CORS_ALLOWED_ORIGIN", "https://attendance.example.com")
	got, err = Load()
	if err != nil || got.Address != ":9090" || got.AllowedOrigin != "https://attendance.example.com" {
		t.Fatalf("%+v %v", got, err)
	}
}
