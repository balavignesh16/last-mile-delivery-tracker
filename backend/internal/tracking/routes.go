package tracking

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
// POST /orders/:id/status is ADMIN + DELIVERY_AGENT only — CUSTOMER
// gets 403 here regardless of which specific transition they'd
// attempt, per the finalized M08 decision that customers have no
// status-transition authority in this module. The finer, per-edge
// authorization (which of ADMIN/DELIVERY_AGENT may perform which
// specific edge) happens inside Repository.Transition, not here — this
// route-level gate only excludes CUSTOMER and unauthenticated callers.
//
// GET /orders/:id/tracking is ADMIN + CUSTOMER — DELIVERY_AGENT gets
// 403, matching M07's blanket "agents have no order visibility yet"
// stance (no assigned-agent relationship exists until M09).
func Mount(repo Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		transitionRoles := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleDeliveryAgent))
		}
		trackingReadRoles := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer))
		}

		transitionRoles(v1).Post("/orders/{id}/status", TransitionHandler(repo))
		trackingReadRoles(v1).Get("/orders/{id}/tracking", GetTrackingHandler(repo))
	}
}
