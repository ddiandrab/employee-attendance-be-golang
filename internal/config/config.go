package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL   string
	JWTSecret     string
	Address       string
	AllowedOrigin string
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), JWTSecret: os.Getenv("JWT_SECRET"), Address: os.Getenv("HTTP_ADDR")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	if c.Address == "" {
		c.Address = ":8081"
	}
	c.AllowedOrigin = os.Getenv("CORS_ALLOWED_ORIGIN")
	if c.AllowedOrigin == "" {
		c.AllowedOrigin = "http://localhost:5173"
	}
	return c, nil
}
