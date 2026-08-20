//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

const agentsIntegrationJWTSecret = "agents-integration-test-secret"

// setupAgentsTest builds the exact same router main.go builds — real
// Postgres, real migrations, real middleware, both auth.Mount and
// agents.Mount — so these tests exercise the full stack.
func setupAgentsTest(t *testing.T) (router http.Handler, usersRepo users.Repository, agentsRepo agents.Repository) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	uRepo := users.NewPostgresRepository(pool)
	aRepo := agents.NewPostgresRepository(pool)
	r := server.NewRouter(pool, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		agents.Mount(aRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, aRepo
}

func adminToken(t *testing.T, uRepo users.Repository) string {
	t.Helper()
	return userToken(t, uRepo, uniqueEmail("admin"), users.RoleAdmin)
}

func customerToken(t *testing.T, uRepo users.Repository) string {
	t.Helper()
	return userToken(t, uRepo, uniqueEmail("customer"), users.RoleCustomer)
}

func userToken(t *testing.T, uRepo users.Repository, email string, role users.Role) string {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	created, err := uRepo.Create(context.Background(), users.NewUser{
		Email: email, PasswordHash: hash, FullName: "Test User", Role: role,
	})
	if err != nil {
		t.Fatalf("seed user create failed: %v", err)
	}
	token, err := auth.GenerateToken(agentsIntegrationJWTSecret, created.ID, role, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	return token
}

func doJSON(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAgentFlow_AdminCreatesListsAndManagesAgent(t *testing.T) {
	router, uRepo, _ := setupAgentsTest(t)
	admin := adminToken(t, uRepo)
	email := uniqueEmail("agent-flow")

	// 1. ADMIN creates an agent.
	createBody := fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"Flow Agent"}`, email)
	createRec := doJSON(router, http.MethodPost, "/api/v1/agents", admin, createBody)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	agentID, _ := created["id"].(string)
	userID, _ := created["user_id"].(string)
	if agentID == "" || userID == "" {
		t.Fatalf("expected non-empty id and user_id, got: %v", created)
	}
	if _, present := created["password_hash"]; present {
		t.Error("create response leaked password_hash")
	}

	// 2. Confirm the created identity is DELIVERY_AGENT in the database
	// itself — not just in the API response — proving the role a
	// malicious client could never have influenced actually landed
	// correctly at the storage layer.
	storedUser, err := uRepo.FindByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if storedUser.Role != users.RoleDeliveryAgent {
		t.Errorf("stored role = %v, want DELIVERY_AGENT", storedUser.Role)
	}

	// 3. ADMIN lists agents and finds the new one.
	listRec := doJSON(router, http.MethodGet, "/api/v1/agents", admin, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	found := false
	for _, a := range list {
		if a["id"] == agentID {
			found = true
		}
		if _, present := a["password_hash"]; present {
			t.Error("list response leaked password_hash")
		}
	}
	if !found {
		t.Error("created agent not found in listing")
	}

	// 4. The agent themself logs in and updates their own availability.
	agentToken, err := auth.GenerateToken(agentsIntegrationJWTSecret, userID, users.RoleDeliveryAgent, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	availRec := doJSON(router, http.MethodPut, "/api/v1/agents/"+agentID+"/availability", agentToken, `{"availability":"AVAILABLE"}`)
	if availRec.Code != http.StatusOK {
		t.Fatalf("availability update status = %d, want %d, body: %s", availRec.Code, http.StatusOK, availRec.Body.String())
	}

	// 5. The agent updates their own location.
	locRec := doJSON(router, http.MethodPut, "/api/v1/agents/"+agentID+"/location", agentToken, `{"latitude":12.9716,"longitude":77.5946}`)
	if locRec.Code != http.StatusOK {
		t.Fatalf("location update status = %d, want %d, body: %s", locRec.Code, http.StatusOK, locRec.Body.String())
	}
	var locBody map[string]any
	if err := json.NewDecoder(locRec.Body).Decode(&locBody); err != nil {
		t.Fatalf("decode location response: %v", err)
	}
	if locBody["location_updated_at"] == nil {
		t.Error("expected location_updated_at to be set")
	}
}

func TestCreateAgent_RoleGating(t *testing.T) {
	router, uRepo, _ := setupAgentsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	cases := []struct {
		name  string
		label string // email-safe, no spaces — t.Run's name isn't guaranteed to be
		token string
		want  int
	}{
		{"admin allowed", "admin", admin, http.StatusCreated},
		{"customer forbidden", "customer", customer, http.StatusForbidden},
		{"unauthenticated rejected", "unauth", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"Gating Test"}`, uniqueEmail("gating-"+tc.label))
			rec := doJSON(router, http.MethodPost, "/api/v1/agents", tc.token, body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestCreateAgent_DeliveryAgentCannotCreateAnotherAgent(t *testing.T) {
	router, uRepo, _ := setupAgentsTest(t)
	admin := adminToken(t, uRepo)

	// Provision one agent via the real endpoint, then try to use that
	// agent's own token to create a second agent.
	firstEmail := uniqueEmail("first-agent")
	createRec := doJSON(router, http.MethodPost, "/api/v1/agents", admin,
		fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"First Agent"}`, firstEmail))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	var created map[string]any
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	userID := created["user_id"].(string)
	agentToken, err := auth.GenerateToken(agentsIntegrationJWTSecret, userID, users.RoleDeliveryAgent, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	rec := doJSON(router, http.MethodPost, "/api/v1/agents", agentToken,
		fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"Second Agent"}`, uniqueEmail("second-agent")))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestListAgents_RoleGating(t *testing.T) {
	router, uRepo, _ := setupAgentsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	if rec := doJSON(router, http.MethodGet, "/api/v1/agents", admin, ""); rec.Code != http.StatusOK {
		t.Errorf("admin: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := doJSON(router, http.MethodGet, "/api/v1/agents", customer, ""); rec.Code != http.StatusForbidden {
		t.Errorf("customer: status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := doJSON(router, http.MethodGet, "/api/v1/agents", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUpdateProfile_DoesNotAffectRole(t *testing.T) {
	router, uRepo, _ := setupAgentsTest(t)
	customer := customerToken(t, uRepo)

	rec := doJSON(router, http.MethodPut, "/api/v1/users/me", customer, `{"full_name":"Updated Name","phone":"555-0199"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["role"] != string(users.RoleCustomer) {
		t.Errorf("role = %v, want unchanged CUSTOMER", body["role"])
	}
	if body["full_name"] != "Updated Name" {
		t.Errorf("full_name = %v, want Updated Name", body["full_name"])
	}
}

// --- Database-level constraint tests ---

func TestDeliveryAgentsTable_UserIDUniqueConstraint(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed: %v", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	uRepo := users.NewPostgresRepository(pool)
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	u, err := uRepo.Create(ctx, users.NewUser{
		Email: uniqueEmail("orphan-check"), PasswordHash: hash, FullName: "Test", Role: users.RoleDeliveryAgent,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// First delivery_agents row for this user succeeds.
	if _, err := pool.Exec(ctx, `INSERT INTO delivery_agents (user_id) VALUES ($1)`, u.ID); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	// A second row for the SAME user must be rejected — one user, one
	// agent profile, enforced by the database, not just application code.
	_, err = pool.Exec(ctx, `INSERT INTO delivery_agents (user_id) VALUES ($1)`, u.ID)
	if err == nil {
		t.Error("expected the unique user_id constraint to reject a second delivery_agents row for the same user")
	}
}

func TestDeliveryAgentsTable_AvailabilityCheckConstraint(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed: %v", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	uRepo := users.NewPostgresRepository(pool)
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	u, err := uRepo.Create(ctx, users.NewUser{
		Email: uniqueEmail("badavail"), PasswordHash: hash, FullName: "Test", Role: users.RoleDeliveryAgent,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO delivery_agents (user_id, availability) VALUES ($1, 'FREE')`, u.ID)
	if err == nil {
		t.Error("expected the availability CHECK constraint to reject 'FREE'")
	}
}

func TestDeliveryAgentsTable_CoordinateCheckConstraint(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed: %v", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	uRepo := users.NewPostgresRepository(pool)
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	u, err := uRepo.Create(ctx, users.NewUser{
		Email: uniqueEmail("badcoord"), PasswordHash: hash, FullName: "Test", Role: users.RoleDeliveryAgent,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO delivery_agents (user_id, current_lat) VALUES ($1, 999)`, u.ID)
	if err == nil {
		t.Error("expected the latitude CHECK constraint to reject 999")
	}
}

func TestDeliveryAgentsTable_ForeignKeyRejectsUnknownUser(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed: %v", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO delivery_agents (user_id) VALUES ('00000000-0000-0000-0000-000000000000')`)
	if err == nil {
		t.Error("expected the foreign key constraint to reject a non-existent user_id")
	}
}

func TestAgentCreation_TransactionalAtomicity(t *testing.T) {
	// Duplicate email means the users insert inside Create's transaction
	// fails — this proves the transaction actually rolls back (no orphan
	// delivery_agents row left behind) rather than just proving the
	// error propagates.
	router, uRepo, aRepo := setupAgentsTest(t)
	admin := adminToken(t, uRepo)
	email := uniqueEmail("atomic")

	body := fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"Atomic Test"}`, email)
	first := doJSON(router, http.MethodPost, "/api/v1/agents", admin, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", first.Code, http.StatusCreated)
	}

	before, err := aRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	second := doJSON(router, http.MethodPost, "/api/v1/agents", admin, body)
	if second.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want %d", second.Code, http.StatusConflict)
	}

	after, err := aRepo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("agent count changed after a failed creation: before=%d after=%d (orphan row left behind)", len(before), len(after))
	}
}

// TestEnsureDemoAgentRecord_BacksTheSeededDemoAgent is a direct
// regression test for a real gap manual verification found: M02's
// SeedDemoUsers creates a users row with role=DELIVERY_AGENT for the demo
// agent account, but predates delivery_agents and doesn't create the
// matching operational record — so every agent endpoint 404'd for the
// seeded demo agent until main.go started calling EnsureDemoAgentRecord
// right after SeedDemoUsers.
func TestEnsureDemoAgentRecord_BacksTheSeededDemoAgent(t *testing.T) {
	router, uRepo, aRepo := setupAgentsTest(t)

	if err := auth.SeedDemoUsers(context.Background(), uRepo, testLogger()); err != nil {
		t.Fatalf("SeedDemoUsers() failed: %v", err)
	}
	demoAgentUser, err := uRepo.FindByEmail(context.Background(), auth.SeedAgentEmail)
	if err != nil {
		t.Fatalf("could not find seeded demo agent user: %v", err)
	}

	postgresRepo, ok := aRepo.(*agents.PostgresRepository)
	if !ok {
		t.Fatalf("expected *agents.PostgresRepository, got %T", aRepo)
	}

	// First call creates the record...
	if err := postgresRepo.EnsureDemoAgentRecord(context.Background(), demoAgentUser.ID, testLogger()); err != nil {
		t.Fatalf("first EnsureDemoAgentRecord() failed: %v", err)
	}
	// ...second call must be a no-op, not an error or a duplicate.
	if err := postgresRepo.EnsureDemoAgentRecord(context.Background(), demoAgentUser.ID, testLogger()); err != nil {
		t.Fatalf("second EnsureDemoAgentRecord() failed: %v", err)
	}

	// And the demo agent can now actually use its own endpoints — the
	// concrete symptom the gap caused.
	agentToken, err := auth.GenerateToken(agentsIntegrationJWTSecret, demoAgentUser.ID, users.RoleDeliveryAgent, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	rec := doJSON(router, http.MethodGet, "/api/v1/agents/me", agentToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agents/me for the demo agent status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
