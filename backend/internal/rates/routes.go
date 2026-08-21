package rates

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
// Every route is ADMIN-only, the same default M04 established: nothing
// in the assignment states otherwise for rate configuration, and M06's
// future read access happens through this package's Go-level Repository
// interface, not through these HTTP routes — so there is no reason for
// CUSTOMER or DELIVERY_AGENT to ever reach them.
func Mount(repo Repository, jwtSecret string) func(chi.Router) {
	return func(v1 chi.Router) {
		adminOnly := func(r chi.Router) chi.Router {
			return r.With(auth.RequireAuth(jwtSecret), auth.RequireRole(users.RoleAdmin))
		}

		adminOnly(v1).Post("/rates", CreateRateCardHandler(repo))
		adminOnly(v1).Get("/rates", ListRateCardsHandler(repo))
		adminOnly(v1).Get("/rates/{id}", GetRateCardHandler(repo))
		adminOnly(v1).Put("/rates/{id}", UpdateRateCardHandler(repo))

		adminOnly(v1).Post("/rates/{rateCardID}/slabs", CreateSlabHandler(repo))
		adminOnly(v1).Get("/rates/{rateCardID}/slabs", ListSlabsHandler(repo))
		adminOnly(v1).Put("/rates/{rateCardID}/slabs/{slabID}", UpdateSlabHandler(repo))
		adminOnly(v1).Delete("/rates/{rateCardID}/slabs/{slabID}", DeleteSlabHandler(repo))
	}
}
