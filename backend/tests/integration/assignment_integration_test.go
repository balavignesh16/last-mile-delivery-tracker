//go:build integration

package integration

import (
	"context"
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

	"lastmiletracker/internal/assignment"
)

// setupAssignmentTest builds the exact same router main.go builds —
// real Postgres, real migrations, real middleware, and every module's
// Mount including assignment.Mount — so these tests exercise the full
// M04 -> M09 stack end to end.
func setupAssignmentTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, agentsRepo agents.Repository, pool *pgxpool.Pool) {
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
	asRepo := assignment.NewPostgresRepository(p, aRepo, oRepo, tRepo, nil)
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, zRepo, agentsIntegrationJWTSecret),
		orders.Mount(oRepo, uRepo, zRepo, rRepo, aRepo, agentsIntegrationJWTSecret, nil),
		tracking.Mount(tRepo, agentsIntegrationJWTSecret, nil),
		agents.Mount(aRepo, zRepo, agentsIntegrationJWTSecret),
		assignment.Mount(asRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, zRepo, aRepo, p
}

// seedAgent creates a real delivery_agents row via agents.Repository.Create
// (the same code path M03's own endpoint uses) and then sets
// active/availability/current_zone_id directly via SQL — the only way
// to populate current_zone_id at all, since (per the M09 audit) no
// application code path ever writes it; a real deployment would need
// some other, out-of-scope mechanism to keep it current.
func seedAgent(t *testing.T, aRepo agents.Repository, pool *pgxpool.Pool, zoneID string, active bool, availability agents.Availability) agents.AgentWithUser {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	a, err := aRepo.Create(context.Background(), agents.CreateAgentInput{
		Email: uniqueEmail("assignment-agent"), PasswordHash: hash, FullName: "Assignment Agent",
	})
	if err != nil {
		t.Fatalf("agents.Create() error: %v", err)
	}
	var zoneArg any
	if zoneID != "" {
		zoneArg = zoneID
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE delivery_agents SET active = $1, availability = $2, current_zone_id = $3 WHERE id = $4`,
		active, availability, zoneArg, a.ID,
	); err != nil {
		t.Fatalf("seed agent state failed: %v", err)
	}
	updated, err := aRepo.FindByID(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("FindByID() after seeding agent state: %v", err)
	}
	return updated
}

func assignOrder(router http.Handler, token, orderID, agentID string) *httptest.ResponseRecorder {
	return doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/assign", token, fmt.Sprintf(`{"agent_id":%q}`, agentID))
}

func autoAssignOrder(router http.Handler, token, orderID string) *httptest.ResponseRecorder {
	return doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/auto-assign", token, ``)
}

// neutralizeAvailableAgents marks every currently-AVAILABLE agent
// OFFLINE. Postgres in these integration tests is a single shared,
// never-reset instance (the same convention every prior milestone's
// integration suite uses), so a test whose auto-assign candidate pool
// must be exactly {no one} or exactly {one specific agent} needs this
// to neutralize leftover AVAILABLE agents any earlier test in the same
// run left behind — otherwise auto-assign's global agents.List() scan
// would pick up unrelated agents from other tests and make the
// assertion flaky. Every later test still seeds its own agents
// explicitly via seedAgent, which sets availability directly, so this
// has no effect on tests that run afterward.
func neutralizeAvailableAgents(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE delivery_agents SET availability = 'OFFLINE' WHERE availability = 'AVAILABLE'`); err != nil {
		t.Fatalf("neutralizeAvailableAgents() error: %v", err)
	}
}

func userIDFromToken(t *testing.T, token string) string {
	t.Helper()
	claims, err := auth.ParseToken(agentsIntegrationJWTSecret, token)
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	return claims.Subject
}

// --- Manual assignment: happy path ---

func TestAssignmentFlow_ManualHappyPath(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	adminID := userIDFromToken(t, admin)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	rec := assignOrder(router, admin, orderID, agent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["assigned_agent_id"] != agent.ID {
		t.Errorf("assigned_agent_id = %v, want %v", updated["assigned_agent_id"], agent.ID)
	}
	if updated["status"] != "ASSIGNED" {
		t.Errorf("status = %v, want ASSIGNED", updated["status"])
	}

	reloaded, err := aRepo.FindByID(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if reloaded.Availability != agents.AvailabilityBusy {
		t.Errorf("agent availability = %v, want BUSY", reloaded.Availability)
	}
	if reloaded.LastAssignedAt == nil {
		t.Error("last_assigned_at was not set")
	}

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	if len(events) != 2 {
		t.Fatalf("events = %v, want 2 (CREATED, ASSIGNED)", events)
	}
	last := events[len(events)-1]
	if last["new_status"] != "ASSIGNED" {
		t.Errorf("last event new_status = %v, want ASSIGNED", last["new_status"])
	}
	if last["actor_id"] != adminID {
		t.Errorf("last event actor_id = %v, want the assigning admin's id %v", last["actor_id"], adminID)
	}
	metadata, _ := last["metadata"].(map[string]any)
	if metadata == nil || metadata["assigned_agent_id"] != agent.ID {
		t.Errorf("last event metadata = %v, want {assigned_agent_id: %v}", last["metadata"], agent.ID)
	}
}

// --- Auto-assignment: ranking ---

func TestAssignmentFlow_AutoAssignSameZoneWins(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)
	otherZoneID, _ := zRepo.CreateZone(context.Background(), zones.CreateZoneInput{Name: uniqueEmail("assignment-other-zone")})

	crossZoneAgent := seedAgent(t, aRepo, pool, otherZoneID.ID, true, agents.AvailabilityAvailable)
	sameZoneAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	_ = crossZoneAgent

	rec := autoAssignOrder(router, admin, orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("auto-assign status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["assigned_agent_id"] != sameZoneAgent.ID {
		t.Errorf("assigned_agent_id = %v, want the same-zone agent %v", updated["assigned_agent_id"], sameZoneAgent.ID)
	}
}

func TestAssignmentFlow_AutoAssignAcceptsCrossZoneWhenNoSameZoneCandidate(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	neutralizeAvailableAgents(t, pool)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	otherZoneID, _ := zRepo.CreateZone(context.Background(), zones.CreateZoneInput{Name: uniqueEmail("assignment-cross-zone")})

	crossZoneAgent := seedAgent(t, aRepo, pool, otherZoneID.ID, true, agents.AvailabilityAvailable)

	rec := autoAssignOrder(router, admin, orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("auto-assign status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["assigned_agent_id"] != crossZoneAgent.ID {
		t.Errorf("assigned_agent_id = %v, want the only (cross-zone) candidate %v", updated["assigned_agent_id"], crossZoneAgent.ID)
	}
}

// --- Manual assignment: eligibility rejections ---

func TestAssignHandler_IneligibleAgentRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	cases := []struct {
		label        string
		active       bool
		availability agents.Availability
		withZone     bool
	}{
		{"inactive", false, agents.AvailabilityAvailable, true},
		{"busy", true, agents.AvailabilityBusy, true},
		{"offline", true, agents.AvailabilityOffline, true},
		{"no zone", true, agents.AvailabilityAvailable, false},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
			orderID, _ := order["id"].(string)
			pickupZoneID, _ := order["pickup_zone_id"].(string)
			zoneArg := pickupZoneID
			if !tc.withZone {
				zoneArg = ""
			}
			agent := seedAgent(t, aRepo, pool, zoneArg, tc.active, tc.availability)

			rec := assignOrder(router, admin, orderID, agent.ID)
			if rec.Code != http.StatusConflict {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusConflict, rec.Body.String())
			}
		})
	}
}

func TestAssignHandler_NonexistentAgentRejected(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rec := assignOrder(router, admin, orderID, "00000000-0000-0000-0000-000000000000")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAssignHandler_NonexistentOrderRejected(t *testing.T) {
	router, uRepo, _, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	agent := seedAgent(t, aRepo, pool, "", true, agents.AvailabilityAvailable)

	rec := assignOrder(router, admin, "00000000-0000-0000-0000-000000000000", agent.ID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// --- RBAC ---

func TestAssignmentEndpoints_RoleGating(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)
	agentRecord := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	cases := []struct {
		label string
		token string
		want  int
	}{
		{"customer forbidden", customer, http.StatusForbidden},
		{"delivery agent forbidden", agentToken, http.StatusForbidden},
		{"unauthenticated rejected", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run("assign/"+tc.label, func(t *testing.T) {
			rec := assignOrder(router, tc.token, orderID, agentRecord.ID)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
		t.Run("auto-assign/"+tc.label, func(t *testing.T) {
			rec := autoAssignOrder(router, tc.token, orderID)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// --- State-machine boundaries (M08 remains authoritative) ---

func TestAssignHandler_AlreadyAssignedOrderRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	firstAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	if rec := assignOrder(router, admin, orderID, firstAgent.ID); rec.Code != http.StatusOK {
		t.Fatalf("first assign failed: %d %s", rec.Code, rec.Body.String())
	}

	secondAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	rec := assignOrder(router, admin, orderID, secondAgent.ID)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (no ASSIGNED->ASSIGNED edge in M08), body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestAssignHandler_DeliveredOrderRejected(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	deliveryAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	if rec := assignOrder(router, admin, orderID, deliveryAgent.ID); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"} {
		if rec := transitionStatus(router, agentToken, orderID, status); rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}

	anotherAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	rec := assignOrder(router, admin, orderID, anotherAgent.ID)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (DELIVERED is terminal), body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

// TestAssignmentFlow_FailedRescheduledAssignedWorks proves the one
// legal reassignment path the finalized M09 decisions permit: an order
// that failed delivery can be rescheduled and then assigned again
// (potentially to a different agent) via the same shared Assign path —
// no separate "reassignment" endpoint, no ASSIGNED->ASSIGNED edge.
func TestAssignmentFlow_FailedRescheduledAssignedWorks(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	firstAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	if rec := assignOrder(router, admin, orderID, firstAgent.ID); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}
	// PICKED_UP/IN_TRANSIT/OUT_FOR_DELIVERY/FAILED are agent-authorized
	// edges; RESCHEDULED is ADMIN-only in M08's own role matrix (an
	// agent may report a failed delivery, but only an admin decides to
	// reschedule it) — unchanged, unmodified M08 behavior this test must
	// respect, not a new M09 rule.
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED"} {
		if rec := transitionStatus(router, agentToken, orderID, status); rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}
	if rec := transitionStatus(router, admin, orderID, "RESCHEDULED"); rec.Code != http.StatusCreated {
		t.Fatalf("transition to RESCHEDULED failed: %d %s", rec.Code, rec.Body.String())
	}

	secondAgent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	rec := assignOrder(router, admin, orderID, secondAgent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("reassign after RESCHEDULED failed: %d %s", rec.Code, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["assigned_agent_id"] != secondAgent.ID {
		t.Errorf("assigned_agent_id = %v, want the new agent %v", updated["assigned_agent_id"], secondAgent.ID)
	}
	if updated["status"] != "ASSIGNED" {
		t.Errorf("status = %v, want ASSIGNED", updated["status"])
	}
}

func TestAutoAssignHandler_NoEligibleCandidateRejected(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	neutralizeAvailableAgents(t, pool)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rec := autoAssignOrder(router, admin, orderID)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (no eligible agent exists), body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

// --- Rollback / partial-state safety ---

func TestAssignmentFlow_RollbackLeavesNoPartialState(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	// BUSY at the time of the call -> IsEligible fails under lock,
	// nothing should have been written.
	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityBusy)

	rec := assignOrder(router, admin, orderID, agent.ID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	getRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, admin, "")
	reloadedOrder := decodeOrder(t, getRec.Body.Bytes())
	if reloadedOrder["assigned_agent_id"] != nil {
		t.Errorf("assigned_agent_id = %v, want nil (rejected assignment must not persist)", reloadedOrder["assigned_agent_id"])
	}
	if reloadedOrder["status"] != "CREATED" {
		t.Errorf("status = %v, want CREATED (unchanged)", reloadedOrder["status"])
	}

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", admin, "")
	events := decodeEventList(t, trackingRec.Body.Bytes())
	if len(events) != 1 {
		t.Errorf("events = %v, want exactly 1 (only the initial CREATED event; no ASSIGNED event on a rejected assignment)", events)
	}

	reloadedAgent, err := aRepo.FindByID(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if reloadedAgent.Availability != agents.AvailabilityBusy {
		t.Errorf("agent availability = %v, want unchanged BUSY", reloadedAgent.Availability)
	}
}

// --- Concurrency ---

// TestAssignmentConcurrency_SameOrderRacedByTwoAdmins fires two manual
// assignments at the same CREATED order, targeting two different
// eligible agents, from concurrent goroutines. Exactly one must
// succeed (the order can only ever hold one ASSIGNED transition); the
// other must fail with a conflict, never with two orders half-assigned
// or a corrupted state. Run with -count to prove this repeatedly, not
// just once.
func TestAssignmentConcurrency_SameOrderRacedByTwoAdmins(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)

	agentA := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	agentB := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0] = assignOrder(router, admin, orderID, agentA.ID).Code
	}()
	go func() {
		defer wg.Done()
		codes[1] = assignOrder(router, admin, orderID, agentB.ID).Code
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

	getRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, admin, "")
	finalOrder := decodeOrder(t, getRec.Body.Bytes())
	if finalOrder["status"] != "ASSIGNED" {
		t.Errorf("final status = %v, want ASSIGNED", finalOrder["status"])
	}
	assignedTo, _ := finalOrder["assigned_agent_id"].(string)
	if assignedTo != agentA.ID && assignedTo != agentB.ID {
		t.Errorf("assigned_agent_id = %v, want one of %v/%v", assignedTo, agentA.ID, agentB.ID)
	}

	// Whichever agent lost the race must remain AVAILABLE, not BUSY.
	loserID := agentA.ID
	if assignedTo == agentA.ID {
		loserID = agentB.ID
	}
	loser, err := aRepo.FindByID(context.Background(), loserID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if loser.Availability != agents.AvailabilityAvailable {
		t.Errorf("losing agent availability = %v, want unchanged AVAILABLE", loser.Availability)
	}
}

// TestAssignmentConcurrency_SameAgentRacedByTwoOrders fires manual
// assignment of the SAME agent to two different CREATED orders from
// concurrent goroutines. The critical invariant (an agent can never
// end up with two simultaneously active assigned orders) must hold:
// exactly one assignment succeeds, backed by the partial unique index
// as the final database-level guarantee even if the application-level
// lock were somehow bypassed.
func TestAssignmentConcurrency_SameAgentRacedByTwoOrders(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool := setupAssignmentTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	orderA := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderAID, _ := orderA["id"].(string)
	pickupZoneID, _ := orderA["pickup_zone_id"].(string)
	orderB := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderBID, _ := orderB["id"].(string)

	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0] = assignOrder(router, admin, orderAID, agent.ID).Code
	}()
	go func() {
		defer wg.Done()
		codes[1] = assignOrder(router, admin, orderBID, agent.ID).Code
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
		t.Fatalf("successes = %d, want exactly 1 (an agent must never hold two active assignments), codes: %v", successes, codes)
	}

	// Database-level backstop: at most one of these orders may reference
	// this agent while in an active (non-terminal) status.
	var activeCount int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM orders WHERE assigned_agent_id = $1 AND status IN ('ASSIGNED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY')`,
		agent.ID,
	).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active assignments: %v", err)
	}
	if activeCount != 1 {
		t.Errorf("active assignment count for agent = %d, want exactly 1", activeCount)
	}
}
