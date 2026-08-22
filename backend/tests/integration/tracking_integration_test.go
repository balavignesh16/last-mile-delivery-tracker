//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// setupTrackingTest builds the exact same router main.go builds — real
// Postgres, real migrations, real middleware, auth.Mount, zones.Mount,
// rates.Mount, orders.Mount, and tracking.Mount — so these tests
// exercise the full M04 -> M05 -> M06 -> M07 -> M08 stack end to end.
func setupTrackingTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, pool *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx := context.Background()
	p, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed: %v", err)
	}
	t.Cleanup(p.Close)

	if err := database.Migrate(ctx, p); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	uRepo := users.NewPostgresRepository(p)
	zRepo := zones.NewPostgresRepository(p)
	rRepo := rates.NewPostgresRepository(p)
	oRepo := orders.NewPostgresRepository(p)
	tRepo := tracking.NewPostgresRepository(p)
	aRepo := agents.NewPostgresRepository(p)
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, zRepo, agentsIntegrationJWTSecret),
		orders.Mount(oRepo, uRepo, zRepo, rRepo, aRepo, agentsIntegrationJWTSecret, nil),
		tracking.Mount(tRepo, agentsIntegrationJWTSecret, nil),
	)
	return r, uRepo, zRepo, p
}

// createTestOrder places one real, priced order via the full HTTP
// stack (zone/area/rate-card setup + POST /orders as customerToken)
// and returns its full decoded response, including "id".
func createTestOrder(t *testing.T, router http.Handler, zRepo zones.Repository, pool *pgxpool.Pool, admin, customerToken, orderType, zoneRelationship string) map[string]any {
	t.Helper()
	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("tracking-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, orderType, zoneRelationship, 0)
	addSlab(t, router, admin, cardID, 0, nil, 42)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":%q,"payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID, orderType)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", customerToken, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("order create failed: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	return decodeOrder(t, rec.Body.Bytes())
}

func transitionStatus(router http.Handler, token, orderID, status string) *httptest.ResponseRecorder {
	return doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", token, fmt.Sprintf(`{"status":%q}`, status))
}

func decodeEvent(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode event response: %v, body: %s", err, body)
	}
	return out
}

func decodeEventList(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode event list: %v, body: %s", err, body)
	}
	return out
}

// --- Initial tracking event on order creation (M07 integration) ---

func TestOrderCreate_InitialTrackingEventRecorded(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customerEmail := uniqueEmail("tracking-initial-customer")
	customer := userToken(t, uRepo, customerEmail, users.RoleCustomer)
	customerUser, err := uRepo.FindByEmail(context.Background(), customerEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", customer, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tracking status = %d, body: %s", rec.Code, rec.Body.String())
	}
	events := decodeEventList(t, rec.Body.Bytes())
	if len(events) != 1 {
		t.Fatalf("events = %v, want exactly 1 (the initial CREATED event)", events)
	}
	if events[0]["previous_status"] != nil {
		t.Errorf("previous_status = %v, want null (the initial event has no predecessor)", events[0]["previous_status"])
	}
	if events[0]["new_status"] != "CREATED" {
		t.Errorf("new_status = %v, want CREATED", events[0]["new_status"])
	}
	if events[0]["actor_id"] != customerUser.ID {
		t.Errorf("actor_id = %v, want the creating customer's id %v", events[0]["actor_id"], customerUser.ID)
	}
}

func TestOrderCreate_InitialEventActorIsAdminWhenAdminCreates(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	adminEmail := uniqueEmail("tracking-admin-actor")
	admin := userToken(t, uRepo, adminEmail, users.RoleAdmin)
	adminUser, err := uRepo.FindByEmail(context.Background(), adminEmail)
	if err != nil {
		t.Fatalf("FindByEmail(admin) error: %v", err)
	}
	customerEmail := uniqueEmail("tracking-admin-target")
	userToken(t, uRepo, customerEmail, users.RoleCustomer)
	customerUser, err := uRepo.FindByEmail(context.Background(), customerEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("tracking-admin-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 42)

	body := fmt.Sprintf(`{"customer_id":%q,"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		customerUser.ID, pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", admin, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("order create failed: %d %s", rec.Code, rec.Body.String())
	}
	order := decodeOrder(t, rec.Body.Bytes())
	orderID, _ := order["id"].(string)

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	if len(events) != 1 || events[0]["actor_id"] != adminUser.ID {
		t.Errorf("initial event actor = %v, want the creating admin's id %v", events[0]["actor_id"], adminUser.ID)
	}
}

// --- Full lifecycle ---

func TestTrackingFlow_FullHappyPathLifecycle(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	steps := []struct {
		token  string
		status string
	}{
		{admin, "ASSIGNED"},
		{agent, "PICKED_UP"},
		{agent, "IN_TRANSIT"},
		{agent, "OUT_FOR_DELIVERY"},
		{agent, "DELIVERED"},
	}
	for _, step := range steps {
		rec := transitionStatus(router, step.token, orderID, step.status)
		if rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: status = %d, body: %s", step.status, rec.Code, rec.Body.String())
		}
	}

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	wantSequence := []string{"CREATED", "ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"}
	if len(events) != len(wantSequence) {
		t.Fatalf("events = %v, want %d entries", events, len(wantSequence))
	}
	for i, want := range wantSequence {
		if events[i]["new_status"] != want {
			t.Errorf("event[%d].new_status = %v, want %v", i, events[i]["new_status"], want)
		}
	}

	orderRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, admin, "")
	finalOrder := decodeOrder(t, orderRec.Body.Bytes())
	if finalOrder["status"] != "DELIVERED" {
		t.Errorf("final order status = %v, want DELIVERED", finalOrder["status"])
	}
}

func TestTrackingFlow_FailedRescheduledReassignedPath(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTER")
	orderID, _ := order["id"].(string)

	steps := []struct {
		token  string
		status string
	}{
		{admin, "ASSIGNED"},
		{agent, "PICKED_UP"},
		{agent, "IN_TRANSIT"},
		{agent, "OUT_FOR_DELIVERY"},
		{agent, "FAILED"},
		{admin, "RESCHEDULED"},
		{admin, "ASSIGNED"},
		{agent, "PICKED_UP"},
	}
	for _, step := range steps {
		rec := transitionStatus(router, step.token, orderID, step.status)
		if rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s (token role test) failed: status = %d, body: %s", step.status, rec.Code, rec.Body.String())
		}
	}

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	wantSequence := []string{"CREATED", "ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED", "RESCHEDULED", "ASSIGNED", "PICKED_UP"}
	if len(events) != len(wantSequence) {
		t.Fatalf("events = %v, want %d entries", events, len(wantSequence))
	}
	for i, want := range wantSequence {
		if events[i]["new_status"] != want {
			t.Errorf("event[%d].new_status = %v, want %v", i, events[i]["new_status"], want)
		}
	}
}

// --- Invalid transitions ---

func TestTransition_InvalidJumpRejected(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rec := transitionStatus(router, admin, orderID, "DELIVERED")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (CREATED->DELIVERED must be rejected), body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestTransition_SameStatusRejected(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTRA")
	orderID, _ := order["id"].(string)

	if rec := transitionStatus(router, admin, orderID, "ASSIGNED"); rec.Code != http.StatusCreated {
		t.Fatalf("setup transition failed: %d %s", rec.Code, rec.Body.String())
	}
	rec := transitionStatus(router, admin, orderID, "ASSIGNED")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (same-status transition must be rejected)", rec.Code, http.StatusConflict)
	}
}

func TestTransition_UnknownStatusValueRejected(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTER")
	orderID, _ := order["id"].(string)

	rec := transitionStatus(router, admin, orderID, "SHIPPED")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestTransition_UnknownOrderRejected(t *testing.T) {
	router, uRepo, _, _ := setupTrackingTest(t)
	admin := adminToken(t, uRepo)

	rec := transitionStatus(router, admin, "00000000-0000-0000-0000-000000000000", "ASSIGNED")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- Role restrictions ---

func TestTransition_RoleRestrictions(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	t.Run("agent forbidden from CREATED->ASSIGNED", func(t *testing.T) {
		order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
		orderID, _ := order["id"].(string)
		rec := transitionStatus(router, agent, orderID, "ASSIGNED")
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("customer forbidden from every transition", func(t *testing.T) {
		order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTER")
		orderID, _ := order["id"].(string)
		rec := transitionStatus(router, customer, orderID, "ASSIGNED")
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("unauthenticated rejected", func(t *testing.T) {
		order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTRA")
		orderID, _ := order["id"].(string)
		rec := transitionStatus(router, "", orderID, "ASSIGNED")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("agent forbidden from FAILED->RESCHEDULED", func(t *testing.T) {
		order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTER")
		orderID, _ := order["id"].(string)
		for _, s := range []string{"ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED"} {
			token := admin
			if s != "ASSIGNED" {
				token = agent
			}
			if rec := transitionStatus(router, token, orderID, s); rec.Code != http.StatusCreated {
				t.Fatalf("setup transition to %s failed: %d %s", s, rec.Code, rec.Body.String())
			}
		}
		rec := transitionStatus(router, agent, orderID, "RESCHEDULED")
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}

// --- Tracking read: ownership / IDOR / RBAC ---

func TestGetTracking_CustomerOwnershipIDOR(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	owner := customerToken(t, uRepo)
	other := customerToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, owner, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	ownRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", owner, "")
	if ownRec.Code != http.StatusOK {
		t.Errorf("owner status = %d, want %d", ownRec.Code, http.StatusOK)
	}
	otherRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", other, "")
	if otherRec.Code != http.StatusNotFound {
		t.Errorf("non-owner status = %d, want %d (must not reveal existence)", otherRec.Code, http.StatusNotFound)
	}
}

func TestGetTracking_DeliveryAgentForbidden(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTRA")
	orderID, _ := order["id"].(string)

	rec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", agent, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestGetTracking_UnauthenticatedRejected(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTER")
	orderID, _ := order["id"].(string)

	rec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- Immutability: no update/delete route exists ---

func TestTrackingEvents_NoUpdateOrDeleteRouteExists(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	putRec := doJSON(router, http.MethodPut, "/api/v1/orders/"+orderID+"/status", admin, `{"status":"ASSIGNED"}`)
	if putRec.Code == http.StatusOK || putRec.Code == http.StatusCreated {
		t.Errorf("PUT /orders/%s/status unexpectedly succeeded (status %d) — tracking must be append-only", orderID, putRec.Code)
	}
	deleteRec := doJSON(router, http.MethodDelete, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	if deleteRec.Code == http.StatusOK || deleteRec.Code == http.StatusNoContent {
		t.Errorf("DELETE /orders/%s/tracking unexpectedly succeeded (status %d) — tracking must be append-only", orderID, deleteRec.Code)
	}
}

// --- Mass assignment ---

func TestTransition_MassAssignmentRejected(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	forbidden := []string{
		`"actor_id":"11111111-1111-1111-1111-111111111111"`,
		`"previous_status":"CREATED"`,
		`"id":"11111111-1111-1111-1111-111111111111"`,
		`"created_at":"2026-01-01T00:00:00Z"`,
	}
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			body := `{"status":"ASSIGNED",` + field + `}`
			rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", admin, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// --- Database constraints ---

func TestOrderTrackingEventsTable_ForeignKeyConstraints(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2B", "INTRA")
	orderID, _ := order["id"].(string)
	adminUser, err := uRepo.FindByEmail(context.Background(), auth.SeedAgentEmail) // any real user id works; reuse seeded agent
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}

	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id) VALUES ($1, 'CREATED', 'ASSIGNED', $2)`,
		"00000000-0000-0000-0000-000000000000", adminUser.ID); err == nil {
		t.Error("expected a foreign key violation for an unknown order_id, got none")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id) VALUES ($1, 'CREATED', 'ASSIGNED', $2)`,
		orderID, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("expected a foreign key violation for an unknown actor_id, got none")
	}
}

func TestOrderTrackingEventsTable_StatusCheckConstraints(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTER")
	orderID, _ := order["id"].(string)
	adminUser, err := uRepo.FindByEmail(context.Background(), auth.SeedAgentEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}

	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id) VALUES ($1, 'CREATED', 'SHIPPED', $2)`,
		orderID, adminUser.ID); err == nil {
		t.Error("expected a CHECK constraint violation for an invalid new_status, got none")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id) VALUES ($1, 'SIDEWAYS', 'ASSIGNED', $2)`,
		orderID, adminUser.ID); err == nil {
		t.Error("expected a CHECK constraint violation for an invalid previous_status, got none")
	}
	// NULL previous_status must remain legal (the initial-event case).
	if _, err := pool.Exec(ctx,
		`INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id) VALUES ($1, NULL, 'CREATED', $2)`,
		orderID, adminUser.ID); err != nil {
		t.Errorf("NULL previous_status was rejected: %v", err)
	}
}

// --- Concurrency ---

// TestConcurrentTransition_OnlyOneWins fires simultaneous, conflicting
// transition attempts (OUT_FOR_DELIVERY -> DELIVERED and
// OUT_FOR_DELIVERY -> FAILED) at the same order. The SELECT ... FOR
// UPDATE lock inside Transition is what must serialize these: this
// test proves it does, under real concurrent load against real
// Postgres, not merely by calling the handler twice in sequence.
func TestConcurrentTransition_OnlyOneWins(t *testing.T) {
	router, uRepo, zRepo, pool := setupTrackingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	for _, s := range []string{"ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY"} {
		token := admin
		if s != "ASSIGNED" {
			token = agent
		}
		if rec := transitionStatus(router, token, orderID, s); rec.Code != http.StatusCreated {
			t.Fatalf("setup transition to %s failed: %d %s", s, rec.Code, rec.Body.String())
		}
	}

	const attempts = 8
	results := make([]int, 0, attempts)
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})

	fire := func(target string) {
		defer wg.Done()
		<-start
		rec := transitionStatus(router, agent, orderID, target)
		mu.Lock()
		results = append(results, rec.Code)
		mu.Unlock()
	}

	for i := 0; i < attempts/2; i++ {
		wg.Add(2)
		go fire("DELIVERED")
		go fire("FAILED")
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, code := range results {
		switch code {
		case http.StatusCreated:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("unexpected status code under concurrent transition: %d", code)
		}
	}
	if successes != 1 {
		t.Errorf("successful transitions under the race = %d, want exactly 1", successes)
	}
	if successes+conflicts != attempts {
		t.Errorf("successes(%d) + conflicts(%d) != attempts(%d)", successes, conflicts, attempts)
	}

	var finalStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&finalStatus); err != nil {
		t.Fatalf("query final status: %v", err)
	}
	if finalStatus != "DELIVERED" && finalStatus != "FAILED" {
		t.Errorf("final status = %q, want DELIVERED or FAILED", finalStatus)
	}

	var eventCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM order_tracking_events WHERE order_id = $1 AND new_status IN ('DELIVERED','FAILED')`, orderID).
		Scan(&eventCount); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("terminal tracking events recorded = %d, want exactly 1", eventCount)
	}
}
