package agents

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

// agentResponse is the only shape an agent is ever allowed to leave this
// package as — no password_hash (it isn't even a field on AgentWithUser),
// no JWT, no internal detail. Every handler below maps through
// toAgentResponse rather than marshaling a repository type directly.
type agentResponse struct {
	ID                string   `json:"id"`
	UserID            string   `json:"user_id"`
	FullName          string   `json:"full_name"`
	Email             string   `json:"email"`
	Phone             *string  `json:"phone"`
	Availability      string   `json:"availability"`
	CurrentLat        *float64 `json:"current_lat"`
	CurrentLng        *float64 `json:"current_lng"`
	CurrentZoneID     *string  `json:"current_zone_id"`
	LocationUpdatedAt *string  `json:"location_updated_at"`
	LastAssignedAt    *string  `json:"last_assigned_at"`
	Active            bool     `json:"active"`
	CreatedAt         string   `json:"created_at"`
}

func toAgentResponse(a AgentWithUser) agentResponse {
	return agentResponse{
		ID:                a.ID,
		UserID:            a.UserID,
		FullName:          a.FullName,
		Email:             a.Email,
		Phone:             a.Phone,
		Availability:      string(a.Availability),
		CurrentLat:        a.CurrentLat,
		CurrentLng:        a.CurrentLng,
		CurrentZoneID:     a.CurrentZoneID,
		LocationUpdatedAt: formatTimePtr(a.LocationUpdatedAt),
		LastAssignedAt:    formatTimePtr(a.LastAssignedAt),
		Active:            a.Active,
		CreatedAt:         a.CreatedAt.Format(timeLayout),
	}
}

const timeLayout = "2006-01-02T15:04:05Z07:00" // time.RFC3339

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format(timeLayout)
	return &formatted
}

// createAgentRequest has no Role field — the same structural pattern as
// auth's registerRequest. This endpoint's entire purpose is admin
// provisioning a DELIVERY_AGENT; the role is never read from the request.
type createAgentRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

// CreateAgentHandler handles POST /api/v1/agents (admin-only — enforced
// by the RBAC middleware in routes.go, not by this handler). Validation
// and password hashing reuse auth's exported helpers rather than
// duplicating them.
func CreateAgentHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createAgentRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		email := auth.NormalizeEmail(req.Email)
		if problems := auth.ValidateRegistration(email, req.Password, req.FullName); len(problems) > 0 {
			server.WriteError(w, http.StatusUnprocessableEntity, strings.Join(problems, "; "))
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			slog.Error("password hashing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create agent")
			return
		}

		var phone *string
		if trimmed := strings.TrimSpace(req.Phone); trimmed != "" {
			phone = &trimmed
		}

		created, err := repo.Create(r.Context(), CreateAgentInput{
			Email:        email,
			PasswordHash: hash,
			FullName:     strings.TrimSpace(req.FullName),
			Phone:        phone,
		})
		if err != nil {
			if errors.Is(err, users.ErrEmailTaken) {
				server.WriteError(w, http.StatusConflict, "an account with this email already exists")
				return
			}
			slog.Error("agent creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create agent")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toAgentResponse(created))
	}
}

// ListAgentsHandler handles GET /api/v1/agents (admin-only). No filters —
// STEP 8 explicitly says not to invent a search/filter system in M03.
func ListAgentsHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := repo.List(r.Context())
		if err != nil {
			slog.Error("agent listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list agents")
			return
		}

		responses := make([]agentResponse, len(list))
		for i, a := range list {
			responses[i] = toAgentResponse(a)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

// GetMyAgentHandler handles GET /api/v1/agents/me — the delivery-agent
// equivalent of GET /users/me. Without it, a logged-in agent's JWT gives
// them their own user ID but no way to discover their own agent.id, which
// every other agent endpoint is keyed by; this is what the frontend calls
// once after login to learn which agent record is "theirs".
func GetMyAgentHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		agent, err := repo.FindByUserID(r.Context(), identity.UserID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				server.WriteError(w, http.StatusNotFound, "no agent record for this account")
				return
			}
			slog.Error("agent self-lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load agent profile")
			return
		}

		server.WriteJSON(w, http.StatusOK, toAgentResponse(agent))
	}
}

type updateAvailabilityRequest struct {
	Availability string `json:"availability"`
}

// UpdateAvailabilityHandler handles PUT /api/v1/agents/{id}/availability.
// Route-level RBAC (routes.go) already restricts this to ADMIN and
// DELIVERY_AGENT; the ownership check below closes the remaining IDOR gap
// — a DELIVERY_AGENT may only ever manage their own record, never
// another agent's, by comparing the URL's agent ID against the caller's
// own user ID rather than trusting anything else in the request.
func UpdateAvailabilityHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "id")

		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		agent, err := repo.FindByID(r.Context(), agentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				server.WriteError(w, http.StatusNotFound, "agent not found")
				return
			}
			slog.Error("agent lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update availability")
			return
		}

		if identity.Role != users.RoleAdmin && identity.UserID != agent.UserID {
			server.WriteError(w, http.StatusForbidden, "cannot manage another agent's operational state")
			return
		}

		var req updateAvailabilityRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body")
			return
		}

		availability, ok := ParseAvailability(req.Availability)
		if !ok {
			server.WriteError(w, http.StatusUnprocessableEntity, "availability must be one of AVAILABLE, BUSY, OFFLINE")
			return
		}

		updated, err := repo.UpdateAvailability(r.Context(), agentID, availability)
		if err != nil {
			slog.Error("availability update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update availability")
			return
		}

		server.WriteJSON(w, http.StatusOK, toAgentResponse(updated))
	}
}

type updateLocationRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// UpdateLocationHandler handles PUT /api/v1/agents/{id}/location. Unlike
// availability, this is self-only even for ADMIN — the assignment did not
// ask for admin-managed location, and there is no operational reason for
// anyone but the agent themselves to report their own position.
func UpdateLocationHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "id")

		identity, ok := auth.IdentityFromContext(r.Context())
		if !ok {
			server.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		agent, err := repo.FindByID(r.Context(), agentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				server.WriteError(w, http.StatusNotFound, "agent not found")
				return
			}
			slog.Error("agent lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update location")
			return
		}

		if identity.UserID != agent.UserID {
			server.WriteError(w, http.StatusForbidden, "cannot update another agent's location")
			return
		}

		var req updateLocationRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body")
			return
		}

		if problem := validateCoordinates(req.Latitude, req.Longitude); problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		updated, err := repo.UpdateLocation(r.Context(), agentID, *req.Latitude, *req.Longitude)
		if err != nil {
			slog.Error("location update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update location")
			return
		}

		server.WriteJSON(w, http.StatusOK, toAgentResponse(updated))
	}
}

func validateCoordinates(lat, lng *float64) string {
	if lat == nil || lng == nil {
		return "latitude and longitude are both required"
	}
	// encoding/json rejects literal NaN/Infinity tokens outright (they
	// are not valid JSON numbers), but an extreme value like 1e400
	// decodes to +Inf without a decode error — checked explicitly rather
	// than relying solely on the range comparison below to catch it.
	if math.IsNaN(*lat) || math.IsInf(*lat, 0) || math.IsNaN(*lng) || math.IsInf(*lng, 0) {
		return "latitude and longitude must be finite numbers"
	}
	if *lat < -90 || *lat > 90 {
		return "latitude must be between -90 and 90"
	}
	if *lng < -180 || *lng > 180 {
		return "longitude must be between -180 and 180"
	}
	return ""
}
