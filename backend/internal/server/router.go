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
// whatever business routes the caller mounts.
//
// Each mount function receives one shared "/api/v1" sub-router, not the
// top-level router — every business module registers its own paths onto
// that same sub-router, and it is mounted onto the top-level router
// exactly once, here. This matters: chi panics ("attempting to Mount() a
// handler on an existing path") if two different modules each try to
// independently wrap their own routes in r.Route("/api/v1", ...) on the
// same parent router. Verified empirically before relying on it — see the
// M03 report.
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

	v1 := chi.NewRouter()
	for _, m := range mount {
		m(v1)
	}
	r.Mount("/api/v1", v1)

	r.NotFound(notFoundHandler)
	r.MethodNotAllowed(methodNotAllowedHandler)

	return r
}
