package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ddiandrab/employee-attendance-be-golang/internal/attendance"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/auth"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/config"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/database"
	"github.com/ddiandrab/employee-attendance-be-golang/internal/httpapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := database.Open(startup, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	api := &httpapi.API{
		Attendance: attendance.NewService(&attendance.Postgres{Pool: pool}, time.Now),
		Auth:       auth.New(&auth.Postgres{Pool: pool}, cfg.JWTSecret),
		Logger:     slog.Default(), AllowedOrigin: cfg.AllowedOrigin,
	}
	server := &http.Server{Addr: cfg.Address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	slog.Info("attendance API listening", "address", cfg.Address)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
