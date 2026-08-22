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
	"lastmiletracker/internal/assignment"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/config"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/notifications"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/rescheduling"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/tracking"
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
	ordersRepo := orders.NewPostgresRepository(pool)
	trackingRepo := tracking.NewPostgresRepository(pool)

	// M11/post-M12: EmailProvider defaults to the log-based provider (no
	// external credentials, no new dependency — see docs/notifications.md)
	// unless EMAIL_PROVIDER=resend is explicitly set, in which case a real
	// Resend account is required. SmsProvider stays log-only either way —
	// no real SMS provider was added (see docs/notifications.md's own
	// reasoning). notificationsService.NotifyTransition/NotifyOrderCreated
	// are wired below as post-commit hooks into
	// tracking/assignment/rescheduling/orders — none of those packages
	// import internal/notifications themselves.
	var emailProvider notifications.EmailProvider
	switch cfg.Notifications.EmailProvider {
	case "resend":
		if cfg.Notifications.ResendAPIKey == "" || cfg.Notifications.ResendFromAddr == "" {
			logger.Error("EMAIL_PROVIDER=resend requires RESEND_API_KEY and RESEND_FROM_EMAIL to both be set")
			os.Exit(1)
		}
		emailProvider = notifications.NewResendEmailProvider(cfg.Notifications.ResendAPIKey, cfg.Notifications.ResendFromAddr)
		logger.Info("email notifications will be sent via Resend", "from", cfg.Notifications.ResendFromAddr)
	case "log", "":
		emailProvider = notifications.NewLogEmailProvider()
	default:
		logger.Error("unrecognized EMAIL_PROVIDER value", "value", cfg.Notifications.EmailProvider, "want", "log or resend")
		os.Exit(1)
	}

	notificationsRepo := notifications.NewPostgresRepository(pool)
	notificationsService := notifications.NewService(
		notificationsRepo, ordersRepo, usersRepo, trackingRepo,
		emailProvider, notifications.NewLogSmsProvider(),
	)

	assignmentRepo := assignment.NewPostgresRepository(pool, agentsRepo, ordersRepo, trackingRepo, notificationsService.NotifyTransition)
	reschedulingRepo := rescheduling.NewPostgresRepository(pool, trackingRepo, notificationsService.NotifyTransition)

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
		agents.Mount(agentsRepo, zonesRepo, cfg.JWTSecret),
		zones.Mount(zonesRepo, cfg.JWTSecret),
		rates.Mount(ratesRepo, zonesRepo, cfg.JWTSecret),
		orders.Mount(ordersRepo, usersRepo, zonesRepo, ratesRepo, agentsRepo, cfg.JWTSecret, notificationsService.NotifyOrderCreated),
		tracking.Mount(trackingRepo, cfg.JWTSecret, notificationsService.NotifyTransition),
		assignment.Mount(assignmentRepo, cfg.JWTSecret),
		rescheduling.Mount(reschedulingRepo, ordersRepo, cfg.JWTSecret),
	)
	// CORS is a wrapper around the fully-built router, not a parameter of
	// server.NewRouter itself — see server.CORS's own doc comment for
	// why. An empty/unset CORS_ALLOWED_ORIGINS adds no header at all,
	// exactly today's behavior; it only needs to be set once the
	// frontend is hosted on a different origin than this backend.
	handler := server.CORS(cfg.CORSAllowedOrigins)(router)

	addr := net.JoinHostPort(cfg.ServerHost, cfg.ServerPort)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
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
