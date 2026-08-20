// Package server builds the HTTP router and the handlers that belong to
// the foundation itself (currently just /health). Business-domain routes
// are mounted by the caller via NewRouter's variadic mount functions —
// this package never imports a business package, which keeps it reusable
// from every module without risking an import cycle.
package server

import (
	"context"
	"log/slog"
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
// foundation routes, consistent JSON responses for unmatched routes, and
// whatever business routes the caller mounts. Each mount function receives
// the router after foundation setup, so callers can add whatever routes
// their module owns — e.g. auth.Mount(usersRepo, jwtSecret).
//
// The return type is chi.Router (not http.Handler) specifically so callers
// can keep mounting routes after construction if needed; every existing
// caller that only needs http.Handler's ServeHTTP still works unchanged,
// since chi.Router satisfies http.Handler.
func NewRouter(pinger HealthPinger, logger *slog.Logger, mount ...func(chi.Router)) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", healthHandler(pinger))

	for _, m := range mount {
		m(r)
	}

	r.NotFound(notFoundHandler)
	r.MethodNotAllowed(methodNotAllowedHandler)

	return r
}
