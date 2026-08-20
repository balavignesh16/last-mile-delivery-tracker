// Package server builds the HTTP router and the handlers that belong to
// the foundation itself (currently just /health). Business-domain routes
// are mounted here by later modules — this package does not implement them.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// HealthPinger is the minimal capability the health handler needs. It is
// satisfied by *pgxpool.Pool without this package importing pgx, and lets
// tests inject a stub instead of a real database connection.
type HealthPinger interface {
	Ping(ctx context.Context) error
}

// NewRouter builds the application's HTTP handler: middleware chain,
// foundation routes, and consistent JSON responses for unmatched routes.
func NewRouter(pinger HealthPinger, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", healthHandler(pinger))

	r.NotFound(notFoundHandler)
	r.MethodNotAllowed(methodNotAllowedHandler)

	return r
}
