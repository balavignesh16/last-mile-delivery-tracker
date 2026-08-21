package tracking

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
	"lastmiletracker/internal/users"
)

const testSecret = "tracking-test-secret"

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

func withAuth(t *testing.T, userID string, role users.Role, next http.Handler) http.HandlerFunc {
	t.Helper()
	token, err := auth.GenerateToken(testSecret, userID, role, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	wrapped := auth.RequireAuth(testSecret)(next)
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

// --- TransitionHandler ---

func TestTransitionHandler_ValidTransitionByAdmin(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"ASSIGNED"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["new_status"] != "ASSIGNED" {
		t.Errorf("new_status = %v, want ASSIGNED", body["new_status"])
	}
	if body["previous_status"] != "CREATED" {
		t.Errorf("previous_status = %v, want CREATED", body["previous_status"])
	}
	if body["actor_id"] != "admin-1" {
		t.Errorf("actor_id = %v, want admin-1 (must come from the JWT)", body["actor_id"])
	}
	if body["order_id"] != "order-1" {
		t.Errorf("order_id = %v, want order-1", body["order_id"])
	}
	if body["created_at"] == nil || body["created_at"] == "" {
		t.Error("created_at was not captured")
	}
}

func TestTransitionHandler_ValidTransitionByAgent(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusAssigned)

	rec := doRequest(t, withAuth(t, "agent-1", users.RoleDeliveryAgent, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"PICKED_UP"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["actor_id"] != "agent-1" {
		t.Errorf("actor_id = %v, want agent-1", body["actor_id"])
	}
}

func TestTransitionHandler_InvalidJumpRejected(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"DELIVERED"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (CREATED->DELIVERED must be rejected), body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestTransitionHandler_SameStatusRejected(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusAssigned)

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"ASSIGNED"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (same-status transition must be rejected)", rec.Code, http.StatusConflict)
	}
}

func TestTransitionHandler_UnknownStatusValueRejected(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"SHIPPED"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestTransitionHandler_AgentForbiddenFromAdminOnlyEdge(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "agent-1", users.RoleDeliveryAgent, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"ASSIGNED"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (DELIVERY_AGENT may not perform CREATED->ASSIGNED)", rec.Code, http.StatusForbidden)
	}
}

func TestTransitionHandler_AgentForbiddenFromRescheduleEdge(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusFailed)

	rec := doRequest(t, withAuth(t, "agent-1", users.RoleDeliveryAgent, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"RESCHEDULED"}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (DELIVERY_AGENT may not perform FAILED->RESCHEDULED)", rec.Code, http.StatusForbidden)
	}
}

func TestTransitionHandler_UnknownOrderRejected(t *testing.T) {
	repo := newFakeRepo()

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/does-not-exist/status", `{"status":"ASSIGNED"}`, map[string]string{"id": "does-not-exist"})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestTransitionHandler_MetadataPassedThrough(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusOutForDelivery)

	rec := doRequest(t, withAuth(t, "agent-1", users.RoleDeliveryAgent, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"FAILED","metadata":{"reason":"customer not home"}}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %v, want an object", body["metadata"])
	}
	if metadata["reason"] != "customer not home" {
		t.Errorf("metadata.reason = %v, want %q", metadata["reason"], "customer not home")
	}
}

func TestTransitionHandler_OmittedMetadataIsNull(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
		http.MethodPost, "/api/v1/orders/order-1/status", `{"status":"ASSIGNED"}`, map[string]string{"id": "order-1"})

	body := decodeJSON[map[string]any](t, rec)
	if body["metadata"] != nil {
		t.Errorf("metadata = %v, want nil when omitted", body["metadata"])
	}
}

// TestTransitionHandler_ServerDerivedFieldsRejected mirrors every prior
// module's mass-assignment test: none of these fields exist on
// transitionRequest, so DisallowUnknownFields rejects the whole
// request rather than silently ignoring them.
func TestTransitionHandler_ServerDerivedFieldsRejected(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	forbidden := []string{
		`"id":"11111111-1111-1111-1111-111111111111"`,
		`"event_id":"11111111-1111-1111-1111-111111111111"`,
		`"actor_id":"someone-else"`,
		`"previous_status":"CREATED"`,
		`"order_id":"another-order"`,
		`"created_at":"2026-01-01T00:00:00Z"`,
		`"timestamp":"2026-01-01T00:00:00Z"`,
	}
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			body := `{"status":"ASSIGNED",` + field + `}`
			rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, TransitionHandler(repo)),
				http.MethodPost, "/api/v1/orders/order-1/status", body, map[string]string{"id": "order-1"})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// --- GetTrackingHandler ---

func TestGetTrackingHandler_OwnerCustomerSeesTimeline(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)
	if _, err := repo.Transition(context.Background(), "order-1", "admin-1", users.RoleAdmin, StatusAssigned, nil); err != nil {
		t.Fatalf("seed transition failed: %v", err)
	}

	rec := doRequest(t, withAuth(t, "customer-1", users.RoleCustomer, GetTrackingHandler(repo)),
		http.MethodGet, "/api/v1/orders/order-1/tracking", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	list := decodeJSON[[]map[string]any](t, rec)
	if len(list) != 1 {
		t.Fatalf("events = %v, want exactly 1", list)
	}
}

func TestGetTrackingHandler_NonOwnerCustomerGets404(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "customer-2", users.RoleCustomer, GetTrackingHandler(repo)),
		http.MethodGet, "/api/v1/orders/order-1/tracking", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (must not reveal existence)", rec.Code, http.StatusNotFound)
	}
}

func TestGetTrackingHandler_AdminSeesAnyOrder(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, GetTrackingHandler(repo)),
		http.MethodGet, "/api/v1/orders/order-1/tracking", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetTrackingHandler_UnknownOrderRejected(t *testing.T) {
	repo := newFakeRepo()

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, GetTrackingHandler(repo)),
		http.MethodGet, "/api/v1/orders/does-not-exist/tracking", "", map[string]string{"id": "does-not-exist"})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetTrackingHandler_ChronologicalOrderPreserved(t *testing.T) {
	repo := newFakeRepo()
	repo.seedOrder("order-1", "customer-1", StatusCreated)
	ctx := context.Background()
	transitions := []Status{StatusAssigned, StatusPickedUp, StatusInTransit}
	for _, s := range transitions {
		if _, err := repo.Transition(ctx, "order-1", "admin-1", users.RoleAdmin, s, nil); err != nil {
			t.Fatalf("seed transition to %s failed: %v", s, err)
		}
	}

	rec := doRequest(t, withAuth(t, "admin-1", users.RoleAdmin, GetTrackingHandler(repo)),
		http.MethodGet, "/api/v1/orders/order-1/tracking", "", map[string]string{"id": "order-1"})

	list := decodeJSON[[]map[string]any](t, rec)
	if len(list) != 3 {
		t.Fatalf("events = %v, want exactly 3", list)
	}
	wantOrder := []string{"ASSIGNED", "PICKED_UP", "IN_TRANSIT"}
	for i, want := range wantOrder {
		if list[i]["new_status"] != want {
			t.Errorf("event[%d].new_status = %v, want %v (chronological order not preserved)", i, list[i]["new_status"], want)
		}
	}
}
