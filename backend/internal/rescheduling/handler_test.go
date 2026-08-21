package rescheduling

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

const testSecret = "rescheduling-test-secret"

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

// withAuthAndRole wraps the same auth.RequireAuth + auth.RequireRole(ADMIN,
// CUSTOMER) pipeline routes.go actually installs in front of both
// handlers — RescheduleHandler/ListReschedulesHandler rely entirely on
// route-level RBAC middleware for excluding DELIVERY_AGENT, so a unit
// test asserting "DELIVERY_AGENT forbidden" has to include that
// middleware to mean anything.
func withAuthAndRole(t *testing.T, userID string, role users.Role, next http.Handler) http.HandlerFunc {
	t.Helper()
	token, err := auth.GenerateToken(testSecret, userID, role, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	wrapped := auth.RequireAuth(testSecret)(auth.RequireRole(users.RoleAdmin, users.RoleCustomer)(next))
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

func validBody() string {
	return `{"requested_date":"2099-01-01","reason":"Not available"}`
}

// --- RescheduleHandler ---

func TestRescheduleHandler_CustomerOwnFailedOrderSucceeds(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", nil)
	repo := newFakeReschedulingRepo()
	repo.orders["order-1"] = ordersRepo.byID["order-1"]

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["status"] != "RESCHEDULED" {
		t.Errorf("status = %v, want RESCHEDULED", body["status"])
	}
	if repo.lastInput.ActorID != "customer-1" {
		t.Errorf("actor_id passed to repo = %q, want customer-1 (the real caller, not a body field)", repo.lastInput.ActorID)
	}
}

func TestRescheduleHandler_CustomerAnotherCustomersOrderRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "other-customer", "FAILED", nil)
	repo := newFakeReschedulingRepo()
	repo.orders["order-1"] = ordersRepo.byID["order-1"]

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (must not reveal the order exists)", rec.Code, http.StatusNotFound)
	}
}

func TestRescheduleHandler_AdminAnyFailedOrderSucceeds(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "some-customer", "FAILED", nil)
	repo := newFakeReschedulingRepo()
	repo.orders["order-1"] = ordersRepo.byID["order-1"]

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if repo.lastInput.ActorID != "admin-1" {
		t.Errorf("actor_id passed to repo = %q, want admin-1", repo.lastInput.ActorID)
	}
}

func TestRescheduleHandler_DeliveryAgentForbidden(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "agent-1", users.RoleDeliveryAgent, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRescheduleHandler_UnauthenticatedRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	repo := newFakeReschedulingRepo()
	handler := auth.RequireAuth(testSecret)(auth.RequireRole(users.RoleAdmin, users.RoleCustomer)(RescheduleHandler(repo, ordersRepo)))

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRescheduleHandler_UnknownOrderRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/does-not-exist/reschedule", validBody(), map[string]string{"id": "does-not-exist"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRescheduleHandler_NonFailedOrderRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "CREATED", nil)
	repo := newFakeReschedulingRepo()
	repo.orders["order-1"] = ordersRepo.byID["order-1"]

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestRescheduleHandler_MissingRequestedDateRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", nil)
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", `{}`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestRescheduleHandler_InvalidRequestedDateRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", nil)
	repo := newFakeReschedulingRepo()

	cases := []struct {
		label string
		body  string
	}{
		{"malformed", `{"requested_date":"not-a-date"}`},
		{"past date", `{"requested_date":"2020-01-01"}`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
				http.MethodPost, "/api/v1/orders/order-1/reschedule", tc.body, map[string]string{"id": "order-1"})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

func TestRescheduleHandler_MassAssignmentRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", nil)
	repo := newFakeReschedulingRepo()

	forbidden := []string{
		`"actor_id":"someone-else"`,
		`"requested_by":"someone-else"`,
		`"order_id":"another-order"`,
		`"status":"RESCHEDULED"`,
		`"created_at":"2026-01-01T00:00:00Z"`,
		`"requested_at":"2026-01-01T00:00:00Z"`,
		`"id":"11111111-1111-1111-1111-111111111111"`,
	}
	base := `{"requested_date":"2099-01-01",`
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
				http.MethodPost, "/api/v1/orders/order-1/reschedule", base+field+`}`, map[string]string{"id": "order-1"})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

func TestRescheduleHandler_MalformedBodyRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", nil)
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", `not-json`, map[string]string{"id": "order-1"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestRescheduleHandler_UnmappedErrorMapsTo500(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", nil)
	repo := newFakeReschedulingRepo()
	repo.rescheduleErr = errFakeInternal

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRescheduleHandler_PreviousAgentIDPassedThrough(t *testing.T) {
	agentID := "agent-1"
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "FAILED", &agentID)
	repo := newFakeReschedulingRepo()
	repo.orders["order-1"] = ordersRepo.byID["order-1"]

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, RescheduleHandler(repo, ordersRepo)),
		http.MethodPost, "/api/v1/orders/order-1/reschedule", validBody(), map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if repo.lastInput.PreviousAgentID == nil || *repo.lastInput.PreviousAgentID != agentID {
		t.Errorf("PreviousAgentID = %v, want %q", repo.lastInput.PreviousAgentID, agentID)
	}
}

// --- ListReschedulesHandler ---

func TestListReschedulesHandler_CustomerOwnOrder(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "customer-1", "RESCHEDULED", nil)
	repo := newFakeReschedulingRepo()
	repo.reschedules["order-1"] = []Reschedule{{ID: "r1", OrderID: "order-1", RequestedBy: "customer-1", RequestedDate: time.Now(), CreatedAt: time.Now()}}

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, ListReschedulesHandler(repo, ordersRepo)),
		http.MethodGet, "/api/v1/orders/order-1/reschedules", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	list := decodeJSON[[]map[string]any](t, rec)
	if len(list) != 1 || list[0]["id"] != "r1" {
		t.Errorf("list = %v, want exactly the one seeded reschedule", list)
	}
}

func TestListReschedulesHandler_CustomerAnotherCustomersOrderRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "other-customer", "RESCHEDULED", nil)
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "customer-1", users.RoleCustomer, ListReschedulesHandler(repo, ordersRepo)),
		http.MethodGet, "/api/v1/orders/order-1/reschedules", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListReschedulesHandler_AdminAnyOrder(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	ordersRepo.seed("order-1", "some-customer", "RESCHEDULED", nil)
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, ListReschedulesHandler(repo, ordersRepo)),
		http.MethodGet, "/api/v1/orders/order-1/reschedules", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestListReschedulesHandler_DeliveryAgentForbidden(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "agent-1", users.RoleDeliveryAgent, ListReschedulesHandler(repo, ordersRepo)),
		http.MethodGet, "/api/v1/orders/order-1/reschedules", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestListReschedulesHandler_UnauthenticatedRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	repo := newFakeReschedulingRepo()
	handler := auth.RequireAuth(testSecret)(auth.RequireRole(users.RoleAdmin, users.RoleCustomer)(ListReschedulesHandler(repo, ordersRepo)))

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/orders/order-1/reschedules", "", map[string]string{"id": "order-1"})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestListReschedulesHandler_UnknownOrderRejected(t *testing.T) {
	ordersRepo := newFakeOrdersRepo()
	repo := newFakeReschedulingRepo()

	rec := doRequest(t, withAuthAndRole(t, "admin-1", users.RoleAdmin, ListReschedulesHandler(repo, ordersRepo)),
		http.MethodGet, "/api/v1/orders/does-not-exist/reschedules", "", map[string]string{"id": "does-not-exist"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
