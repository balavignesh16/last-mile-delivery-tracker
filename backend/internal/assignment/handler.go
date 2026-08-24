package assignment

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/tracking"
)

// assignRequest is POST /orders/:id/assign's only input — no other
// field is accepted. There is deliberately no field for anything
// server-derived (order id comes from the URL, actor from the JWT,
// status/tracking fields from M08's own Transition) — sending one is
// rejected outright by DisallowUnknownFields, the same fail-closed
// pattern every prior module in this project uses.
type assignRequest struct {
	AgentID string `json:"agent_id"`
}

// AssignHandler handles POST /api/v1/orders/:id/assign (ADMIN only —
// enforced by the RBAC middleware in routes.go). The entire workflow
// (lock agent, re-check eligibility, transition to ASSIGNED via M08's
// TransitionTx, write assigned_agent_id, mark the agent BUSY) is one
// atomic call into Repository.Assign; this handler only validates the
// request shape and maps the result to a status code.
func AssignHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			// Defensive: unreachable in practice, RequireAuth always runs
			// first on this route.
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		orderID := chi.URLParam(r, "id")

		var req assignRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}
		if req.AgentID == "" {
			server.WriteError(w, http.StatusUnprocessableEntity, "agent_id is required")
			return
		}

		updated, err := repo.Assign(r.Context(), orderID, req.AgentID, identity.UserID, identity.Role)
		if err != nil {
			if status, message, ok := mapAssignmentError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("manual assignment failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not assign order")
			return
		}

		server.WriteJSON(w, http.StatusOK, orders.ToOrderResponse(updated))
	}
}

// autoAssignResponse embeds the standard order response (unchanged
// shape from every other order-returning endpoint) and adds one
// "assignment" object describing how the winning candidate was picked
// — the useful ranking information section 10 of the feature spec
// calls for, in this project's existing snake_case convention rather
// than inventing a new response shape or a second endpoint.
type autoAssignResponse struct {
	orders.OrderResponse
	Assignment assignmentInfo `json:"assignment"`
}

type assignmentInfo struct {
	// Method is "DISTANCE" (a real Haversine distance to the pickup
	// point decided the winner) or "ZONE" (the zone-based fallback did
	// — no usable coordinate on one or both sides). See
	// AssignmentOutcome's doc.
	Method string `json:"method"`
	// DistanceKM is present only when Method == "DISTANCE".
	DistanceKM *float64 `json:"distance_km,omitempty"`
}

func toAutoAssignResponse(o orders.Order, outcome AssignmentOutcome) autoAssignResponse {
	return autoAssignResponse{
		OrderResponse: orders.ToOrderResponse(o),
		Assignment:    assignmentInfo{Method: outcome.Method, DistanceKM: outcome.DistanceKM},
	}
}

// AutoAssignHandler handles POST /api/v1/orders/:id/auto-assign (ADMIN
// only). No client-supplied assignment parameters exist — every input
// the ranking algorithm needs is already stored on the order itself
// (pickup zone/area) or read fresh from the agents table. An empty body
// is expected; DisallowUnknownFields still applies if one is sent
// anyway, for the same reason every other endpoint in this project
// rejects unexpected fields rather than silently ignoring them.
func AutoAssignHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		orderID := chi.URLParam(r, "id")

		var empty struct{}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&empty); err != nil && !errors.Is(err, io.EOF) {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		updated, outcome, err := repo.AutoAssign(r.Context(), orderID, identity.UserID, identity.Role)
		if err != nil {
			if status, message, ok := mapAssignmentError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("auto-assignment failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not auto-assign order")
			return
		}

		server.WriteJSON(w, http.StatusOK, toAutoAssignResponse(updated, outcome))
	}
}

// mapAssignmentError translates this package's and tracking's domain
// errors into (status, message) pairs, per the finalized M09 decision:
// 404 for unknown order/agent, 409 for every assignment/state conflict
// (ineligible agent, no eligible candidate, an order not currently in a
// state M08 permits ->ASSIGNED from — including "already ASSIGNED",
// since M08 has no ASSIGNED->ASSIGNED edge), 403 only in the
// practically-unreachable case where TransitionTx's own role check
// somehow failed (every caller here is ADMIN, which M08's matrix
// already authorizes for both CREATED->ASSIGNED and
// RESCHEDULED->ASSIGNED).
func mapAssignmentError(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return http.StatusNotFound, "order not found", true
	case errors.Is(err, ErrAgentNotFound):
		return http.StatusNotFound, "delivery agent not found", true
	case errors.Is(err, ErrAgentNotEligible):
		return http.StatusConflict, ErrAgentNotEligible.Error(), true
	case errors.Is(err, ErrNoEligibleCandidate):
		return http.StatusConflict, ErrNoEligibleCandidate.Error(), true
	case errors.Is(err, tracking.ErrInvalidTransition):
		return http.StatusConflict, err.Error(), true
	case errors.Is(err, tracking.ErrForbiddenTransition):
		return http.StatusForbidden, err.Error(), true
	default:
		return 0, "", false
	}
}
