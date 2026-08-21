package rescheduling

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

const timeLayout = time.RFC3339

// rescheduleRequest is POST /orders/:id/reschedule's only input — no
// other field is accepted. There is deliberately no field for anything
// server-derived (order id comes from the URL, actor from the JWT,
// status/previous_status/timestamps from tracking.TransitionTx) —
// sending one is rejected outright by DisallowUnknownFields, the same
// fail-closed pattern every prior module in this project uses.
type rescheduleRequest struct {
	RequestedDate string  `json:"requested_date"`
	Reason        *string `json:"reason"`
}

// rescheduleResponse is GET /orders/:id/reschedules' element shape.
// requested_date is rendered as a plain YYYY-MM-DD date — it was never
// a timestamp, and reformatting it as one would silently invent a
// time-of-day this package never captured.
type rescheduleResponse struct {
	ID            string  `json:"id"`
	OrderID       string  `json:"order_id"`
	RequestedBy   string  `json:"requested_by"`
	RequestedDate string  `json:"requested_date"`
	Reason        *string `json:"reason"`
	CreatedAt     string  `json:"created_at"`
}

func toRescheduleResponse(re Reschedule) rescheduleResponse {
	return rescheduleResponse{
		ID:            re.ID,
		OrderID:       re.OrderID,
		RequestedBy:   re.RequestedBy,
		RequestedDate: re.RequestedDate.Format(dateLayout),
		Reason:        re.Reason,
		CreatedAt:     re.CreatedAt.Format(timeLayout),
	}
}

// RescheduleHandler handles POST /api/v1/orders/:id/reschedule (ADMIN +
// CUSTOMER — enforced by the RBAC middleware in routes.go). Ownership
// for a CUSTOMER caller (must own the order) is enforced here, in the
// handler — the same place GetOrderHandler (M07/M09) already enforces
// the identical ownership rule for GET /orders/{id} — before this
// package's own Repository is ever invoked. This is also the layer that
// resolves the intentional architectural mismatch the finalized M10
// decisions call out explicitly: the product requirement is "CUSTOMER
// can request a reschedule," but M08's own, unmodified authorization
// matrix authorizes only ADMIN for the underlying FAILED->RESCHEDULED
// edge. This handler is M10's own, independent authorization gate — a
// CUSTOMER reaching Repository.Reschedule has already been confirmed to
// own the order — so Repository.Reschedule always asserts
// users.RoleAdmin when it calls tracking.TransitionTx, regardless of
// the real caller's role, while still recording the real caller's own
// user id as actor_id (a separate parameter TransitionTx never uses for
// authorization, only for the persisted record). This requires zero
// changes to M08's edgeAuthorizedRoles or route-level RBAC.
func RescheduleHandler(repo Repository, ordersRepo orders.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			// Defensive: unreachable in practice, RequireAuth always runs
			// first on this route.
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		orderID := chi.URLParam(r, "id")

		order, err := ordersRepo.FindOrderByID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, orders.ErrOrderNotFound) {
				server.WriteError(w, http.StatusNotFound, "order not found")
				return
			}
			slog.Error("order lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not reschedule order")
			return
		}
		if identity.Role == users.RoleCustomer && order.CustomerID != identity.UserID {
			// Hide existence, never 403 — the same IDOR convention
			// GetOrderHandler already uses for a non-owning CUSTOMER.
			server.WriteError(w, http.StatusNotFound, "order not found")
			return
		}

		var req rescheduleRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		requestedDate, err := ParseRequestedDate(req.RequestedDate)
		if err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if err := ValidateRescheduleDate(requestedDate, time.Now()); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		updated, err := repo.Reschedule(r.Context(), RescheduleInput{
			OrderID:         orderID,
			ActorID:         identity.UserID,
			RequestedDate:   requestedDate,
			Reason:          req.Reason,
			PreviousAgentID: order.AssignedAgentID,
		})
		if err != nil {
			if status, message, ok := mapRescheduleError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("reschedule failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not reschedule order")
			return
		}

		server.WriteJSON(w, http.StatusOK, orders.ToOrderResponse(updated))
	}
}

// ListReschedulesHandler handles GET /api/v1/orders/:id/reschedules
// (ADMIN + CUSTOMER). Ownership follows the exact same convention as
// RescheduleHandler and GetOrderHandler: a CUSTOMER may only view their
// own order's reschedule history, 404 on mismatch.
func ListReschedulesHandler(repo Repository, ordersRepo orders.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		orderID := chi.URLParam(r, "id")

		order, err := ordersRepo.FindOrderByID(r.Context(), orderID)
		if err != nil {
			if errors.Is(err, orders.ErrOrderNotFound) {
				server.WriteError(w, http.StatusNotFound, "order not found")
				return
			}
			slog.Error("order lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load reschedule history")
			return
		}
		if identity.Role == users.RoleCustomer && order.CustomerID != identity.UserID {
			server.WriteError(w, http.StatusNotFound, "order not found")
			return
		}

		reschedules, err := repo.ListReschedules(r.Context(), orderID)
		if err != nil {
			slog.Error("reschedule listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load reschedule history")
			return
		}

		responses := make([]rescheduleResponse, len(reschedules))
		for i, re := range reschedules {
			responses[i] = toRescheduleResponse(re)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

// mapRescheduleError translates this package's own sentinel errors into
// (status, message) pairs, per the finalized M10 decision: 404 for an
// order that no longer exists (a defensive case — the handler already
// checked existence, but the row could theoretically vanish between
// that read and the write in a system with deletion, which this project
// has none of), 409 for an order that is not currently FAILED.
func mapRescheduleError(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return http.StatusNotFound, "order not found", true
	case errors.Is(err, ErrInvalidTransition):
		return http.StatusConflict, ErrInvalidTransition.Error(), true
	default:
		return 0, "", false
	}
}
