package agents

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// Mount registers this module's routes onto the shared "/api/v1"
// sub-router — see internal/server.NewRouter's doc for why every module
// registers directly on that router rather than wrapping its own
// r.Route("/api/v1", ...) call.
//
// Role gating happens here, at the route level (coarse: which roles may
// call this endpoint at all), with a second, finer ownership check inside
// UpdateAvailabilityHandler/UpdateLocationHandler/UpdateZoneHandler (an
// agent may manage only their own record). Two layers, not one — a role
// check alone cannot prevent Agent A from editing Agent B's location,
// since both hold the same DELIVERY_AGENT role.
//
// zonesRepo is UpdateZoneHandler's one dependency outside this package
// (agents -> zones is a new, one-directional edge — zones never depends
// back on agents, so this isn't a cycle; internal/orders already
// established the same agents/zones/rates fan-in pattern).
func Mount(repo Repository, zonesRepo zones.Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		v1.With(
			auth.RequireAuth(jwtSecret),
			auth.RequireRole(users.RoleAdmin),
		).Post("/agents", CreateAgentHandler(repo))

		v1.With(
			auth.RequireAuth(jwtSecret),
			auth.RequireRole(users.RoleAdmin),
		).Get("/agents", ListAgentsHandler(repo))

		v1.With(
			auth.RequireAuth(jwtSecret),
			auth.RequireRole(users.RoleDeliveryAgent),
		).Get("/agents/me", GetMyAgentHandler(repo))

		v1.With(
			auth.RequireAuth(jwtSecret),
			auth.RequireRole(users.RoleAdmin, users.RoleDeliveryAgent),
		).Put("/agents/{id}/availability", UpdateAvailabilityHandler(repo))

		v1.With(
			auth.RequireAuth(jwtSecret),
			auth.RequireRole(users.RoleDeliveryAgent),
		).Put("/agents/{id}/location", UpdateLocationHandler(repo))

		v1.With(
			auth.RequireAuth(jwtSecret),
			auth.RequireRole(users.RoleDeliveryAgent),
		).Put("/agents/{id}/zone", UpdateZoneHandler(repo, zonesRepo))
	}
}
