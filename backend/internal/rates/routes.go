package rates

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
// Every /rates and /rates/.../slabs route stays ADMIN-only, the same
// default M04 established: nothing in the assignment states otherwise
// for rate configuration, and the customer never needs to see rate
// cards or slabs directly — only the computed quote they produce. M06
// adds exactly one route on top of that, POST /orders/quote, which is
// ADMIN + CUSTOMER (a customer requesting their own quote; an admin
// previewing one on a customer's behalf) — see quote_handler.go's doc.
//
// zonesRepo is new here (M05's Mount only took a rates.Repository) —
// QuoteHandler needs it to resolve pickup/drop areas via
// zones.ResolvePickupDrop, the same Go-level consumption path M04's own
// docs anticipated for this module. This is an additive signature
// change to M05's Mount, not a behavior change to any existing route.
func Mount(repo Repository, zonesRepo zones.Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		adminOnly := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin))
		}
		customerOrAdmin := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin, users.RoleCustomer))
		}

		adminOnly(v1).Post("/rates", CreateRateCardHandler(repo))
		adminOnly(v1).Get("/rates", ListRateCardsHandler(repo))
		adminOnly(v1).Get("/rates/{id}", GetRateCardHandler(repo))
		adminOnly(v1).Put("/rates/{id}", UpdateRateCardHandler(repo))

		adminOnly(v1).Post("/rates/{rateCardID}/slabs", CreateSlabHandler(repo))
		adminOnly(v1).Get("/rates/{rateCardID}/slabs", ListSlabsHandler(repo))
		adminOnly(v1).Put("/rates/{rateCardID}/slabs/{slabID}", UpdateSlabHandler(repo))
		adminOnly(v1).Delete("/rates/{rateCardID}/slabs/{slabID}", DeleteSlabHandler(repo))

		customerOrAdmin(v1).Post("/orders/quote", QuoteHandler(zonesRepo, repo))
	}
}
