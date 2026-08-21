// Command server is the composition root: it loads configuration, connects
// to PostgreSQL, applies migrations, seeds demo accounts, and serves the
// HTTP API with graceful shutdown. This file is where each module's
// routes get mounted onto the shared router — internal/server itself
// never imports a business package, so this is the one place that wires
// them together.
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

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/config"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
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

	if err := database.Migrate(ctx, pool); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations applied")

	usersRepo := users.NewPostgresRepository(pool)
	agentsRepo := agents.NewPostgresRepository(pool)
	zonesRepo := zones.NewPostgresRepository(pool)
	ratesRepo := rates.NewPostgresRepository(pool)

	if err := auth.SeedDemoUsers(ctx, usersRepo, logger); err != nil {
		logger.Error("demo user seeding failed", "error", err)
		os.Exit(1)
	}

	demoAgentUser, err := usersRepo.FindByEmail(ctx, auth.SeedAgentEmail)
	if err != nil {
		logger.Error("could not find seeded demo agent user", "error", err)
		os.Exit(1)
	}
	if err := agentsRepo.EnsureDemoAgentRecord(ctx, demoAgentUser.ID, logger); err != nil {
		logger.Error("demo agent record seeding failed", "error", err)
		os.Exit(1)
	}

	router := server.NewRouter(pool, logger,
		auth.Mount(usersRepo, cfg.JWTSecret),
		agents.Mount(agentsRepo, cfg.JWTSecret),
		zones.Mount(zonesRepo, cfg.JWTSecret),
		rates.Mount(ratesRepo, zonesRepo, cfg.JWTSecret),
	)
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
