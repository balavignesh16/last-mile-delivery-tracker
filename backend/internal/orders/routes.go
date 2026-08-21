package orders

import (
	"github.com/go-chi/chi/v5"

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
// Every route is ADMIN + CUSTOMER; DELIVERY_AGENT gets 403 on all of
// them — nothing in any source document gives an agent order visibility
// before assignment exists (M09). usersRepo/zonesRepo/ratesRepo are
// dependencies of CreateOrderHandler, not this package's own domain —
// customer validation (users), zone/area resolution and rate pricing
// (M06's rates.CalculateQuote) all happen through them.
func Mount(ordersRepo Repository, usersRepo users.Repository, zonesRepo zones.Repository, ratesRepo rates.Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		customerOrAdmin := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer))
		}

		customerOrAdmin(v1).Post("/orders", CreateOrderHandler(ordersRepo, usersRepo, zonesRepo, ratesRepo))
		customerOrAdmin(v1).Get("/orders", ListOrdersHandler(ordersRepo))
		customerOrAdmin(v1).Get("/orders/{id}", GetOrderHandler(ordersRepo))
	}
}
