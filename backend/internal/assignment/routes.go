package assignment

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/users"
)

// Mount registers this module's routes onto the shared "/api/v1"
// sub-router — see internal/server.NewRouter's doc for why every
// module registers directly on that router rather than wrapping its
// own r.Route("/api/v1", ...) call.
//
// Both routes are ADMIN-only — neither CUSTOMER nor DELIVERY_AGENT may
// assign an order to anyone, per the finalized M09 decision; there is
// no per-edge nuance here the way M08's transition endpoint has, since
// assignment has exactly one authorized actor.
func Mount(repo Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		adminOnly := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin))
		}

		adminOnly(v1).Post("/orders/{id}/assign", AssignHandler(repo))
		adminOnly(v1).Post("/orders/{id}/auto-assign", AutoAssignHandler(repo))
	}
}
