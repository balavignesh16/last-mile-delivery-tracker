// Command server is the M01 entry point: it loads configuration, connects
// to PostgreSQL, and serves the foundation HTTP API (currently just
// GET /health) with graceful shutdown. Business routes are mounted by
// later modules inside internal/server — this file does not grow with them.
package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lastmiletracker/internal/config"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/server"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Checks cwd=backend/ (`cd backend && go run ./cmd/server`, per README)
	// and the repository root (`.env` actually lives there, and running
	// `go run ./backend/cmd/server` from root is also reasonable). Neither
	// candidate existing is normal — e.g. under Docker Compose, where env
	// vars are supplied directly — and is not an error.
	if path, err := config.LoadFirstDotEnv(".env", "../.env"); err != nil {
		logger.Warn("could not read .env file", "error", err)
	} else if path != "" {
		logger.Info("loaded environment file", "path", path)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DSN())
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database connection established")

	router := server.NewRouter(pool, logger)
	addr := net.JoinHostPort(cfg.ServerHost, cfg.ServerPort)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", addr, "env", cfg.AppEnv)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped cleanly")
}
