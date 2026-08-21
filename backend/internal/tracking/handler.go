package tracking

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

const timeLayout = time.RFC3339

// transitionRequest is POST /orders/:id/status's only input. There is
// deliberately no actor_id, previous_status, event_id, or timestamp
// field — all four are server-derived (actor from the JWT,
// previous_status from the locked current row, event_id/timestamp from
// the database) — sending any of them is rejected outright by
// DisallowUnknownFields, the same fail-closed pattern every prior
// module in this project uses. metadata is optional and free-form —
// this module enforces no required shape for it.
type transitionRequest struct {
	Status   string          `json:"status"`
	Metadata json.RawMessage `json:"metadata"`
}

type eventResponse struct {
	ID             string          `json:"id"`
	OrderID        string          `json:"order_id"`
	PreviousStatus *string         `json:"previous_status"`
	NewStatus      string          `json:"new_status"`
	ActorID        string          `json:"actor_id"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      string          `json:"created_at"`
}

func toEventResponse(e Event) eventResponse {
	return eventResponse{
		ID:             e.ID,
		OrderID:        e.OrderID,
		PreviousStatus: e.PreviousStatus,
		NewStatus:      e.NewStatus,
		ActorID:        e.ActorID,
		Metadata:       e.Metadata,
		CreatedAt:      e.CreatedAt.Format(timeLayout),
	}
}

// TransitionHandler handles POST /api/v1/orders/:id/status. Route-level
// RBAC (routes.go) admits only ADMIN and DELIVERY_AGENT — CUSTOMER gets
// 403 before this handler ever runs, satisfying "customers have no
// status-transition authority in M08." The finer, per-edge check (e.g.
// a DELIVERY_AGENT attempting the ADMIN-only CREATED->ASSIGNED edge)
// happens inside Repository.Transition, under the same lock that reads
// the order's current status — see repository.go's doc comment for why
// that has to be where it happens.
func TransitionHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			// Defensive: unreachable in practice, RequireAuth always runs
			// first on this route.
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		orderID := chi.URLParam(r, "id")

		var req transitionRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		newStatus, ok := ParseStatus(req.Status)
		if !ok {
			server.WriteError(w, http.StatusUnprocessableEntity, "status must be one of CREATED, ASSIGNED, PICKED_UP, IN_TRANSIT, OUT_FOR_DELIVERY, DELIVERED, FAILED, RESCHEDULED")
			return
		}

		event, err := repo.Transition(r.Context(), orderID, identity.UserID, identity.Role, newStatus, req.Metadata)
		if err != nil {
			switch {
			case errors.Is(err, ErrOrderNotFound):
				server.WriteError(w, http.StatusNotFound, "order not found")
				return
			case errors.Is(err, ErrInvalidTransition):
				server.WriteError(w, http.StatusConflict, err.Error())
				return
			case errors.Is(err, ErrForbiddenTransition):
				server.WriteError(w, http.StatusForbidden, err.Error())
				return
			}
			slog.Error("status transition failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update order status")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toEventResponse(event))
	}
}

// GetTrackingHandler handles GET /api/v1/orders/:id/tracking. Route-level
// RBAC (routes.go) admits ADMIN and CUSTOMER; DELIVERY_AGENT gets 403.
// ADMIN may view any order's timeline; CUSTOMER only their own — the
// same ownership/IDOR convention as GET /orders/{id} (M07): a mismatch
// returns 404, never 403, so this endpoint never confirms an order
// exists under an id the caller doesn't own.
func GetTrackingHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		orderID := chi.URLParam(r, "id")

		customerID, err := repo.OrderCustomerID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, ErrOrderNotFound) {
				server.WriteError(w, http.StatusNotFound, "order not found")
				return
			}
			slog.Error("order lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load tracking history")
			return
		}
		if identity.Role == users.RoleCustomer && customerID != identity.UserID {
			server.WriteError(w, http.StatusNotFound, "order not found")
			return
		}

		events, err := repo.ListEvents(r.Context(), orderID)
		if err != nil {
			slog.Error("tracking event listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load tracking history")
			return
		}

		responses := make([]eventResponse, len(events))
		for i, e := range events {
			responses[i] = toEventResponse(e)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}
