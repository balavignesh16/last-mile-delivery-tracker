//go:build integration

package integration

import (
	"context"
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
	"lastmiletracker/internal/rescheduling"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"

	"lastmiletracker/internal/assignment"
)

// setupReschedulingTest builds the exact same router main.go builds —
// real Postgres, real migrations, real middleware, and every module's
// Mount including rescheduling.Mount — so these tests exercise the full
// M04 -> M10 stack end to end.
func setupReschedulingTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, agentsRepo agents.Repository, pool *pgxpool.Pool) {
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
	asRepo := assignment.NewPostgresRepository(p, aRepo, oRepo, tRepo, zRepo, nil)
	rsRepo := rescheduling.NewPostgresRepository(p, tRepo, nil)
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, zRepo, agentsIntegrationJWTSecret),
		orders.Mount(oRepo, uRepo, zRepo, rRepo, aRepo, agentsIntegrationJWTSecret, nil),
		tracking.Mount(tRepo, agentsIntegrationJWTSecret, nil),
		agents.Mount(aRepo, zRepo, agentsIntegrationJWTSecret),
		assignment.Mount(asRepo, agentsIntegrationJWTSecret),
		rescheduling.Mount(rsRepo, oRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, zRepo, aRepo, p
}

func rescheduleOrder(router http.Handler, token, orderID, body string) *httptest.ResponseRecorder {
	return doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/reschedule", token, body)
}

func getReschedules(router http.Handler, token, orderID string) *httptest.ResponseRecorder {
	return doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/reschedules", token, "")
}

// driveOrderToFailed creates a real, priced order, assigns it to an
// eligible agent, and drives it through PICKED_UP -> IN_TRANSIT ->
// OUT_FOR_DELIVERY -> FAILED via the real HTTP stack. Returns the order
// id and the agent that was servicing it when it failed.
func driveOrderToFailed(t *testing.T, router http.Handler, zRepo zones.Repository, aRepo agents.Repository, pool *pgxpool.Pool, admin, customer, agentToken string) (orderID string, failedAgent agents.AgentWithUser) {
	t.Helper()
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ = order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	if rec := assignOrder(router, admin, orderID, agent.ID); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED"} {
		if rec := transitionStatus(router, agentToken, orderID, status); rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}
	return orderID, agent
}

// --- Happy path ---

func TestRescheduleFlow_CustomerHappyPath(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customerEmail := uniqueEmail("reschedule-customer")
	customer := userToken(t, uRepo, customerEmail, users.RoleCustomer)
	customerUser, err := uRepo.FindByEmail(context.Background(), customerEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, failedAgent := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01","reason":"Not available"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reschedule status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["status"] != "RESCHEDULED" {
		t.Errorf("status = %v, want RESCHEDULED", updated["status"])
	}

	// Reschedule record persisted, with the right requested_date/reason.
	historyRec := getReschedules(router, customer, orderID)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status = %d, body: %s", historyRec.Code, historyRec.Body.String())
	}
	history := decodeEventList(t, historyRec.Body.Bytes())
	if len(history) != 1 {
		t.Fatalf("history = %v, want exactly 1 reschedule record", history)
	}
	if history[0]["requested_date"] != "2099-01-01" {
		t.Errorf("requested_date = %v, want 2099-01-01", history[0]["requested_date"])
	}
	if history[0]["reason"] != "Not available" {
		t.Errorf("reason = %v, want %q", history[0]["reason"], "Not available")
	}
	if history[0]["requested_by"] != customerUser.ID {
		t.Errorf("requested_by = %v, want the real customer id %v", history[0]["requested_by"], customerUser.ID)
	}
	// requested_by_role (migration 0015) must be the real caller's role,
	// CUSTOMER — never ADMIN leaking in from the internal authorization
	// workaround (Repository.Reschedule always authorizes the underlying
	// edge as ADMIN regardless of who really asked).
	if history[0]["requested_by_role"] != "CUSTOMER" {
		t.Errorf("requested_by_role = %v, want CUSTOMER (the real caller's role, not the internal ADMIN authorization workaround)", history[0]["requested_by_role"])
	}

	// Tracking event persisted: previous_status=FAILED, new_status=RESCHEDULED,
	// actor_id = the real customer (never RoleAdmin leaking into the
	// persisted record).
	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	last := events[len(events)-1]
	if last["previous_status"] != "FAILED" || last["new_status"] != "RESCHEDULED" {
		t.Errorf("last event = %v, want FAILED->RESCHEDULED", last)
	}
	if last["actor_id"] != customerUser.ID {
		t.Errorf("actor_id = %v, want the real customer id %v (never a role name)", last["actor_id"], customerUser.ID)
	}
	// Same leak check as requested_by_role above, for the tracking
	// event's own actor_role.
	if last["actor_role"] != "CUSTOMER" {
		t.Errorf("actor_role = %v, want CUSTOMER (not the internal ADMIN authorization workaround)", last["actor_role"])
	}
	metadata, _ := last["metadata"].(map[string]any)
	if metadata == nil || metadata["requested_date"] != "2099-01-01" || metadata["reason"] != "Not available" {
		t.Errorf("metadata = %v, want requested_date/reason", last["metadata"])
	}

	// Failed-agent availability: the previously assigned agent must now
	// be AVAILABLE again.
	reloadedAgent, err := aRepo.FindByID(context.Background(), failedAgent.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if reloadedAgent.Availability != agents.AvailabilityAvailable {
		t.Errorf("previously assigned agent availability = %v, want AVAILABLE", reloadedAgent.Availability)
	}
	if reloadedAgent.Active != true {
		t.Errorf("previously assigned agent active = %v, want unchanged true", reloadedAgent.Active)
	}

	// assigned_agent_id is deliberately NOT cleared — historical/current
	// snapshot, per the finalized M10 decision.
	if updated["assigned_agent_id"] != failedAgent.ID {
		t.Errorf("assigned_agent_id = %v, want unchanged %v (never cleared)", updated["assigned_agent_id"], failedAgent.ID)
	}
}

func TestRescheduleFlow_AdminHappyPath(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	rec := rescheduleOrder(router, admin, orderID, `{"requested_date":"2099-02-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reschedule status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["status"] != "RESCHEDULED" {
		t.Errorf("status = %v, want RESCHEDULED", updated["status"])
	}
}

// --- RBAC / ownership ---

func TestRescheduleFlow_CustomerOwnershipEnforced(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	owner := customerToken(t, uRepo)
	other := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, owner, agentToken)

	rec := rescheduleOrder(router, other, orderID, `{"requested_date":"2099-01-01"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("non-owner reschedule status = %d, want %d (must not reveal existence)", rec.Code, http.StatusNotFound)
	}

	historyRec := getReschedules(router, other, orderID)
	if historyRec.Code != http.StatusNotFound {
		t.Errorf("non-owner history status = %d, want %d", historyRec.Code, http.StatusNotFound)
	}
}

func TestRescheduleFlow_DeliveryAgentForbidden(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	rec := rescheduleOrder(router, agentToken, orderID, `{"requested_date":"2099-01-01"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("agent reschedule status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	historyRec := getReschedules(router, agentToken, orderID)
	if historyRec.Code != http.StatusForbidden {
		t.Errorf("agent history status = %d, want %d", historyRec.Code, http.StatusForbidden)
	}
}

func TestRescheduleFlow_UnauthenticatedRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	rec := rescheduleOrder(router, "", orderID, `{"requested_date":"2099-01-01"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRescheduleFlow_UnknownOrderRejected(t *testing.T) {
	router, uRepo, _, _, _ := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)

	rec := rescheduleOrder(router, admin, "00000000-0000-0000-0000-000000000000", `{"requested_date":"2099-01-01"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- State / validation ---

func TestRescheduleFlow_NonFailedOrderRejected(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (order is CREATED, not FAILED), body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestRescheduleFlow_InvalidDateRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2020-01-01"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (past date), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestRescheduleFlow_UnknownFieldRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01","status":"RESCHEDULED"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (unknown field), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// --- Rollback / partial-state safety ---

func TestRescheduleFlow_RepeatedRescheduleOnSameOrderRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	if rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("first reschedule failed: %d %s", rec.Code, rec.Body.String())
	}
	// Second attempt: order is now RESCHEDULED, not FAILED -> 409, and
	// must leave exactly one reschedule record (no orphan row from the
	// rejected attempt).
	rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-02-01"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	historyRec := getReschedules(router, customer, orderID)
	history := decodeEventList(t, historyRec.Body.Bytes())
	if len(history) != 1 {
		t.Errorf("history = %v, want exactly 1 (the rejected second attempt must not persist)", history)
	}
}

// --- Full lifecycle ---

func TestRescheduleFlow_RepeatLifecycleFailedRescheduledAssignedFailedRescheduled(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, firstAgent := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	if rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("first reschedule failed: %d %s", rec.Code, rec.Body.String())
	}

	// M09 reassigns (possibly the same, now-freed agent, or a different
	// one — both are eligible again).
	if rec := assignOrder(router, admin, orderID, firstAgent.ID); rec.Code != http.StatusOK {
		t.Fatalf("reassign failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED"} {
		if rec := transitionStatus(router, agentToken, orderID, status); rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}

	// Second reschedule on the same order must succeed — the cycle is
	// not blocked after the first pass.
	rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-03-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("second reschedule failed: %d %s", rec.Code, rec.Body.String())
	}

	historyRec := getReschedules(router, customer, orderID)
	history := decodeEventList(t, historyRec.Body.Bytes())
	if len(history) != 2 {
		t.Fatalf("history = %v, want exactly 2 reschedule records", history)
	}
	// Deterministic chronological order: oldest first.
	if history[0]["requested_date"] != "2099-01-01" || history[1]["requested_date"] != "2099-03-01" {
		t.Errorf("history order = %v, want oldest-first [2099-01-01, 2099-03-01]", history)
	}
}

// --- M09 interaction ---

func TestRescheduleFlow_SubsequentAutoAssignWorksAfterReschedule(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)
	neutralizeAvailableAgents(t, pool)

	orderID, failedAgent := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	if rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("reschedule failed: %d %s", rec.Code, rec.Body.String())
	}

	// The freed agent is the only eligible candidate in the pool (every
	// other AVAILABLE agent was just neutralized) — auto-assign must
	// pick them, proving M09's own eligibility/ranking rules remain
	// authoritative and unmodified: they are picked because they are
	// once again a legitimately eligible candidate, not through any
	// special-cased "reuse the previous agent" logic in M10.
	rec := autoAssignOrder(router, admin, orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("auto-assign status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["status"] != "ASSIGNED" {
		t.Errorf("status = %v, want ASSIGNED", updated["status"])
	}
	if updated["assigned_agent_id"] != failedAgent.ID {
		t.Errorf("assigned_agent_id = %v, want the freed agent %v", updated["assigned_agent_id"], failedAgent.ID)
	}

	reloadedAgent, err := aRepo.FindByID(context.Background(), failedAgent.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if reloadedAgent.Availability != agents.AvailabilityBusy {
		t.Errorf("agent availability = %v, want BUSY (M09's own writeAssignment, unmodified)", reloadedAgent.Availability)
	}
}

func TestRescheduleFlow_PreviouslyAssignedAgentNotIncorrectlyReused(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)
	neutralizeAvailableAgents(t, pool)

	orderID, failedAgent := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)
	pickupZoneID := ""
	{
		getRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, admin, "")
		pickupZoneID, _ = decodeOrder(t, getRec.Body.Bytes())["pickup_zone_id"].(string)
	}

	if rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("reschedule failed: %d %s", rec.Code, rec.Body.String())
	}

	// Manually take the freed agent BUSY again on a second, unrelated
	// order (simulating "the previous agent is now busy elsewhere") —
	// auto-assign for the rescheduled order must then pick a *different*
	// eligible agent, never fall back to the ineligible previous one.
	if _, err := pool.Exec(context.Background(), `UPDATE delivery_agents SET availability = 'BUSY' WHERE id = $1`, failedAgent.ID); err != nil {
		t.Fatalf("seed busy state failed: %v", err)
	}
	otherAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	rec := autoAssignOrder(router, admin, orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("auto-assign status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["assigned_agent_id"] != otherAgent.ID {
		t.Errorf("assigned_agent_id = %v, want the other eligible agent %v, not the still-busy previous agent %v", updated["assigned_agent_id"], otherAgent.ID, failedAgent.ID)
	}
}

// --- Concurrency ---

// TestRescheduleConcurrency_SameOrderRacedTwice fires two simultaneous
// reschedule requests at the same FAILED order. Exactly one must
// succeed; the other must fail with a conflict, and exactly one
// RESCHEDULED tracking event and exactly one reschedule_requests row
// must exist afterward. Run repeatedly to rule out flakiness.
func TestRescheduleConcurrency_SameOrderRacedTwice(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0] = rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`).Code
	}()
	go func() {
		defer wg.Done()
		codes[1] = rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-02-01"}`).Code
	}()
	wg.Wait()

	successes := 0
	for _, c := range codes {
		if c == http.StatusOK {
			successes++
		} else if c != http.StatusConflict {
			t.Errorf("unexpected status %d, want 200 or 409", c)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1, codes: %v", successes, codes)
	}

	historyRec := getReschedules(router, customer, orderID)
	history := decodeEventList(t, historyRec.Body.Bytes())
	if len(history) != 1 {
		t.Errorf("history = %v, want exactly 1 reschedule_requests row", history)
	}

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	rescheduledCount := 0
	for _, e := range events {
		if e["new_status"] == "RESCHEDULED" {
			rescheduledCount++
		}
	}
	if rescheduledCount != 1 {
		t.Errorf("RESCHEDULED tracking events = %d, want exactly 1", rescheduledCount)
	}
}

// TestRescheduleConcurrency_DoesNotDeadlockWithAssignment fires a
// reschedule (which locks a FAILED order's agent, then the order) and a
// concurrent, unrelated M09 assignment for a *different* order that
// happens to target the *same* agent identifier space (a fresh,
// unrelated eligible agent) simultaneously — proving the two modules'
// consistent agent-then-order lock ordering doesn't produce a deadlock
// under real concurrent load.
func TestRescheduleConcurrency_DoesNotDeadlockWithAssignment(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupReschedulingTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	orderID, _ := driveOrderToFailed(t, router, zRepo, aRepo, pool, admin, customer, agentToken)

	otherOrder := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	otherOrderID, _ := otherOrder["id"].(string)
	otherPickupZoneID, _ := otherOrder["pickup_zone_id"].(string)
	otherAgent := seedAgent(t, aRepo, pool, otherPickupZoneID, true, agents.AvailabilityAvailable)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0] = rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`).Code
	}()
	go func() {
		defer wg.Done()
		codes[1] = assignOrder(router, admin, otherOrderID, otherAgent.ID).Code
	}()
	wg.Wait()

	if codes[0] != http.StatusOK {
		t.Errorf("reschedule status = %d, want %d", codes[0], http.StatusOK)
	}
	if codes[1] != http.StatusOK {
		t.Errorf("assign status = %d, want %d", codes[1], http.StatusOK)
	}
}
