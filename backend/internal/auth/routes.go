package auth

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/users"
)

// Mount returns a function that registers this module's routes on a
// chi.Router. internal/server.NewRouter accepts these as variadic
// arguments, which is what keeps internal/server independent of business
// packages: it only ever sees "func(chi.Router)", never internal/auth or
// internal/users directly.
//
// Both main.go and the integration tests call this the same way, so
// production and tests exercise identical route wiring.
func Mount(usersRepo users.Repository, jwtSecret string) func(chi.Router) {
	return func(r chi.Router) {
		r.Route("/api/v1", func(v1 chi.Router) {
			v1.Post("/auth/register", RegisterHandler(usersRepo))
			v1.Post("/auth/login", LoginHandler(usersRepo, jwtSecret))

			v1.With(RequireAuth(jwtSecret)).Get("/users/me", GetMeHandler(usersRepo))
		})
	}
}
