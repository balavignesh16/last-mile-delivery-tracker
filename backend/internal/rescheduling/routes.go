package rescheduling

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/users"
)

// Mount registers this module's routes onto the shared "/api/v1"
// sub-router — see internal/server.NewRouter's doc for why every
// module registers directly on that router rather than wrapping its
// own r.Route("/api/v1", ...) call.
//
// Both routes are ADMIN + CUSTOMER only — DELIVERY_AGENT gets 403 on
// either, per the finalized M10 decision (agents have no reschedule
// role at all, the same way M08 never gave CUSTOMER any
// status-transition authority). Ownership scoping for CUSTOMER (own
// order only) happens inside the handlers, not here — the same two-gate
// split every RBAC-scoped route in this project already uses.
func Mount(repo Repository, ordersRepo orders.Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		rescheduleRoles := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer))
		}

		rescheduleRoles(v1).Post("/orders/{id}/reschedule", RescheduleHandler(repo, ordersRepo))
		rescheduleRoles(v1).Get("/orders/{id}/reschedules", ListReschedulesHandler(repo, ordersRepo))
	}
}
