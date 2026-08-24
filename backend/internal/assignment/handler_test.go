package assignment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
)

const testSecret = "assignment-test-secret"

func doRequest(t *testing.T, handler http.Handler, method, path, body string, urlParams map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if len(urlParams) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range urlParams {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// withAuthAndRole wraps the same auth.RequireAuth + auth.RequireRole(ADMIN)
// pipeline routes.go actually installs in front of these two handlers —
// unlike M08's TransitionHandler (which does its own role check
// internally), AssignHandler/AutoAssignHandler rely entirely on the
// route-level RBAC middleware, so a unit test asserting "CUSTOMER
// forbidden" has to include that middleware to mean anything.
func withAuthAndRole(t *testing.T, userID string, role users.Role, next http.Handler) http.HandlerFunc {
	t.Helper()
	token, err := auth.GenerateToken(testSecret, userID, role, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	wrapped := auth.RequireAuth(testSecret)(auth.RequireRole(users.RoleAdmin)(next))
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
		wrapped.ServeHTTP(w, r)
	}
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("failed to decode JSON response: %v (body: %s)", err, rec.Body.String())
	}
	return v
}

// --- AssignHandler ---

func TestAssignHandler_AdminHappyPath(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.orders["order-1"] = orders.Order{ID: "order-1", Status: "CREATED"}
	repo.agents["agent-1"] = true

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["assigned_agent_id"] != "agent-1" {
		t.Errorf("assigned_agent_id = %v, want agent-1", body["assigned_agent_id"])
	}
	if body["status"] != "ASSIGNED" {
		t.Errorf("status = %v, want ASSIGNED", body["status"])
	}
	if repo.lastAssignAgentID != "agent-1" {
		t.Errorf("repo received agent_id = %q, want agent-1 (request body passed through unmodified)", repo.lastAssignAgentID)
	}
}

func TestAssignHandler_CustomerForbidden(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAssignHandler_DeliveryAgentForbidden(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "agent-1", users.RoleDeliveryAgent, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAssignHandler_UnauthenticatedRejected(t *testing.T) {
	repo := newFakeAssignmentRepo()
	handler := auth.RequireAuth(testSecret)(auth.RequireRole(users.RoleAdmin)(AssignHandler(repo)))
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAssignHandler_MissingAgentIDRejected(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestAssignHandler_UnknownFieldRejected(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1","status":"ASSIGNED"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestAssignHandler_MalformedBodyRejected(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `not-json`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestAssignHandler_UnknownOrderMapsTo404(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.agents["agent-1"] = true
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/does-not-exist/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "does-not-exist"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAssignHandler_UnknownAgentMapsTo404(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.orders["order-1"] = orders.Order{ID: "order-1", Status: "CREATED"}
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"does-not-exist"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAssignHandler_IneligibleAgentMapsTo409(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.assignErr = ErrAgentNotEligible
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAssignHandler_InvalidTransitionMapsTo409(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.assignErr = tracking.ErrInvalidTransition
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestAssignHandler_UnmappedErrorMapsTo500(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.assignErr = errFakeInternal
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// --- AutoAssignHandler ---

func TestAutoAssignHandler_AdminHappyPath(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.orders["order-1"] = orders.Order{ID: "order-1", Status: "CREATED"}

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["status"] != "ASSIGNED" {
		t.Errorf("status = %v, want ASSIGNED", body["status"])
	}
}

func TestAutoAssignHandler_ResponseIncludesAssignmentOutcome(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.orders["order-1"] = orders.Order{ID: "order-1", Status: "CREATED"}
	distance := 2.4
	repo.autoAssignOutcome = AssignmentOutcome{Method: AssignmentMethodDistance, DistanceKM: &distance}

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	assignment, ok := body["assignment"].(map[string]any)
	if !ok {
		t.Fatalf("response has no \"assignment\" object: %v", body)
	}
	if assignment["method"] != "DISTANCE" {
		t.Errorf("assignment.method = %v, want DISTANCE", assignment["method"])
	}
	if assignment["distance_km"] != 2.4 {
		t.Errorf("assignment.distance_km = %v, want 2.4", assignment["distance_km"])
	}
}

func TestAutoAssignHandler_ZoneFallbackOmitsDistance(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.orders["order-1"] = orders.Order{ID: "order-1", Status: "CREATED"}
	repo.autoAssignOutcome = AssignmentOutcome{Method: AssignmentMethodZone}

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	assignment, ok := body["assignment"].(map[string]any)
	if !ok {
		t.Fatalf("response has no \"assignment\" object: %v", body)
	}
	if assignment["method"] != "ZONE" {
		t.Errorf("assignment.method = %v, want ZONE", assignment["method"])
	}
	if _, present := assignment["distance_km"]; present {
		t.Errorf("assignment.distance_km = %v, want the field omitted entirely for the zone fallback", assignment["distance_km"])
	}
}

func TestAutoAssignHandler_CustomerForbidden(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAutoAssignHandler_DeliveryAgentForbidden(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "agent-1", users.RoleDeliveryAgent, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAutoAssignHandler_UnauthenticatedRejected(t *testing.T) {
	repo := newFakeAssignmentRepo()
	handler := auth.RequireAuth(testSecret)(auth.RequireRole(users.RoleAdmin)(AutoAssignHandler(repo)))
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAutoAssignHandler_UnexpectedFieldRejected(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", `{"agent_id":"agent-1"}`, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (no client-supplied fields are accepted), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestAutoAssignHandler_UnknownOrderMapsTo404(t *testing.T) {
	repo := newFakeAssignmentRepo()
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/does-not-exist/auto-assign", ``, map[string]string{"id": "does-not-exist"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAutoAssignHandler_NoEligibleCandidateMapsTo409(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.autoAssignErr = ErrNoEligibleCandidate
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAutoAssignHandler_UnmappedErrorMapsTo500(t *testing.T) {
	repo := newFakeAssignmentRepo()
	repo.autoAssignErr = errFakeInternal
	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, AutoAssignHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/auto-assign", ``, map[string]string{"id": "order-1"})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
