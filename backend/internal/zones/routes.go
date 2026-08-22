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
// Every mutation route is ADMIN-only: the task specification is explicit
// that zone/area management is admin-only "unless the assignment
// explicitly states otherwise", and nothing states otherwise for M04.
//
// The three GET routes are additionally readable by CUSTOMER, a
// narrow, additive M06 change: a customer placing an order needs to
// pick a pickup/drop area from a real list, and M04 had no reason to
// anticipate that read need. The same three were further widened to
// DELIVERY_AGENT: an agent needs the zone list to pick their own
// current_zone_id via PUT /agents/{id}/zone (see internal/agents'
// UpdateZoneHandler) — same reasoning, same narrow, read-only shape. No
// mutation route is widened, and internal/zones' Go code (handlers,
// resolution, repository) is untouched — see docs/zone-management.md's
// RBAC section for the full reasoning.
func Mount(repo Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		adminOnly := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin))
		}
		readOnly := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer, users.RoleDeliveryAgent))
		}

		adminOnly(v1).Post("/zones", CreateZoneHandler(repo))
		readOnly(v1).Get("/zones", ListZonesHandler(repo))
		readOnly(v1).Get("/zones/{id}", GetZoneHandler(repo))
		adminOnly(v1).Put("/zones/{id}", UpdateZoneHandler(repo))

		adminOnly(v1).Post("/zones/{zoneID}/areas", CreateAreaHandler(repo))
		readOnly(v1).Get("/zones/{zoneID}/areas", ListAreasHandler(repo))
		adminOnly(v1).Put("/zones/{zoneID}/areas/{areaID}", UpdateAreaHandler(repo))
	}
}
