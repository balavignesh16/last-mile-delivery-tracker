package orders

import (
	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// Mount registers this module's routes onto the shared "/api/v1"
// sub-router — see internal/server.NewRouter's doc for why every module
// registers directly on that router rather than wrapping its own
// r.Route("/api/v1", ...) call.
//
// POST /orders stays ADMIN + CUSTOMER only — nothing changed there, an
// agent still cannot create an order. The two GET routes now also admit
// DELIVERY_AGENT (M09): once assigned_agent_id exists, an agent needs
// to see the orders assigned to them, scoped server-side in the
// handlers (never by a client-supplied filter) — see handler.go.
// agentsRepo is a new dependency of both GET handlers, needed only to
// resolve a DELIVERY_AGENT caller's own agent id from their JWT's user
// id; usersRepo/zonesRepo/ratesRepo remain CreateOrderHandler's
// dependencies exactly as in M07. onOrderCreated (M11) is nil-safe —
// see OrderCreatedHook's own doc comment.
func Mount(ordersRepo Repository, usersRepo users.Repository, zonesRepo zones.Repository, ratesRepo rates.Repository, agentsRepo agents.Repository, jwtSecret string, onOrderCreated OrderCreatedHook) func(chi.Router) {
	return func(v1 chi.Router) {
		createRoles := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer))
		}
		readRoles := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer, users.RoleDeliveryAgent))
		}

		createRoles(v1).Post("/orders", CreateOrderHandler(ordersRepo, usersRepo, zonesRepo, ratesRepo, onOrderCreated))
		readRoles(v1).Get("/orders", ListOrdersHandler(ordersRepo, agentsRepo))
		readRoles(v1).Get("/orders/{id}", GetOrderHandler(ordersRepo, agentsRepo))
	}
}
