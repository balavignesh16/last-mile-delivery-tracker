package auth

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/users"
)

// Mount registers this module's routes onto the shared "/api/v1"
// sub-router internal/server.NewRouter builds and passes to every mount
// function — see that function's doc for why routes are registered
// directly on it rather than each module wrapping its own
// r.Route("/api/v1", ...) call.
func Mount(usersRepo users.Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		v1.Post("/auth/register", RegisterHandler(usersRepo))
		v1.Post("/auth/login", LoginHandler(usersRepo, jwtSecret))

		v1.With(RequireAuth(jwtSecret)).Get("/users/me", GetMeHandler(usersRepo))
		v1.With(RequireAuth(jwtSecret)).Put("/users/me", UpdateMeHandler(usersRepo))
	}
}
