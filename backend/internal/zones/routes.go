package zones

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/users"
)

// Mount registers this module's routes onto the shared "/api/v1"
// sub-router — see internal/server.NewRouter's doc for why every module
// registers directly on that router rather than wrapping its own
// r.Route("/api/v1", ...) call.
//
// Every route is ADMIN-only: the task specification is explicit that
// zone/area management is admin-only "unless the assignment explicitly
// states otherwise", and nothing states otherwise for M04.
func Mount(repo Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		adminOnly := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin))
		}

		adminOnly(v1).Post("/zones", CreateZoneHandler(repo))
		adminOnly(v1).Get("/zones", ListZonesHandler(repo))
		adminOnly(v1).Get("/zones/{id}", GetZoneHandler(repo))
		adminOnly(v1).Put("/zones/{id}", UpdateZoneHandler(repo))

		adminOnly(v1).Post("/zones/{zoneID}/areas", CreateAreaHandler(repo))
		adminOnly(v1).Get("/zones/{zoneID}/areas", ListAreasHandler(repo))
		adminOnly(v1).Put("/zones/{zoneID}/areas/{areaID}", UpdateAreaHandler(repo))
	}
}
