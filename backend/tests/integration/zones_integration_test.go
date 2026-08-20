//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// setupZonesTest builds the exact same router main.go builds — real
// Postgres, real migrations, real middleware, both auth.Mount and
// zones.Mount — so these tests exercise the full stack. It reuses
// agentsIntegrationJWTSecret (and therefore adminToken/customerToken/
// userToken/doJSON from agents_integration_test.go) rather than minting
// a separate secret and helper set for no reason.
func setupZonesTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, pool *pgxpool.Pool) {
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
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, zRepo, p
}

func deliveryAgentToken(t *testing.T, uRepo users.Repository) string {
	t.Helper()
	return userToken(t, uRepo, uniqueEmail("delivery-agent"), users.RoleDeliveryAgent)
}

// --- Zone CRUD ---

func TestZoneFlow_AdminCreatesGetsAndUpdatesZone(t *testing.T) {
	router, uRepo, _, _ := setupZonesTest(t)
	admin := adminToken(t, uRepo)
	name := uniqueEmail("zone-flow") // reuse the unique-suffix helper for a unique zone name too

	createRec := doJSON(router, http.MethodPost, "/api/v1/zones", admin, fmt.Sprintf(`{"name":%q}`, name))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	zoneID, _ := created["id"].(string)
	if zoneID == "" {
		t.Fatalf("expected non-empty id, got: %v", created)
	}
	if created["active"] != true {
		t.Errorf("active = %v, want true", created["active"])
	}

	getRec := doJSON(router, http.MethodGet, "/api/v1/zones/"+zoneID, admin, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/zones", admin, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	found := false
	for _, z := range list {
		if z["id"] == zoneID {
			found = true
		}
	}
	if !found {
		t.Error("created zone not present in list response")
	}

	updateRec := doJSON(router, http.MethodPut, "/api/v1/zones/"+zoneID, admin,
		fmt.Sprintf(`{"name":%q,"active":false}`, name+"-renamed"))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	var updated map[string]any
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated["name"] != name+"-renamed" {
		t.Errorf("name = %v, want %v", updated["name"], name+"-renamed")
	}
	if updated["active"] != false {
		t.Errorf("active = %v, want false", updated["active"])
	}
}

func TestZoneEndpoints_RoleGating(t *testing.T) {
	router, uRepo, _, _ := setupZonesTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	cases := []struct {
		label string
		token string
		want  int
	}{
		{"admin allowed", admin, http.StatusCreated},
		{"customer forbidden", customer, http.StatusForbidden},
		{"delivery agent forbidden", agent, http.StatusForbidden},
		{"unauthenticated rejected", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doJSON(router, http.MethodPost, "/api/v1/zones", tc.token,
				fmt.Sprintf(`{"name":%q}`, uniqueEmail("gating-"+tc.label)))
			if rec.Code != tc.want {
				t.Errorf("POST /zones status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}

	// GET /zones must also be admin-only.
	getCases := []struct {
		label string
		token string
		want  int
	}{
		{"admin allowed", admin, http.StatusOK},
		{"customer forbidden", customer, http.StatusForbidden},
		{"delivery agent forbidden", agent, http.StatusForbidden},
		{"unauthenticated rejected", "", http.StatusUnauthorized},
	}
	for _, tc := range getCases {
		t.Run("GET "+tc.label, func(t *testing.T) {
			rec := doJSON(router, http.MethodGet, "/api/v1/zones", tc.token, "")
			if rec.Code != tc.want {
				t.Errorf("GET /zones status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// --- Area CRUD ---

func TestAreaFlow_CreateUnderZoneAndList(t *testing.T) {
	router, uRepo, _, _ := setupZonesTest(t)
	admin := adminToken(t, uRepo)

	zoneRec := doJSON(router, http.MethodPost, "/api/v1/zones", admin, fmt.Sprintf(`{"name":%q}`, uniqueEmail("area-flow-zone")))
	if zoneRec.Code != http.StatusCreated {
		t.Fatalf("zone create status = %d, want %d", zoneRec.Code, http.StatusCreated)
	}
	var zone map[string]any
	if err := json.NewDecoder(zoneRec.Body).Decode(&zone); err != nil {
		t.Fatalf("decode zone response: %v", err)
	}
	zoneID, _ := zone["id"].(string)

	areaRec := doJSON(router, http.MethodPost, "/api/v1/zones/"+zoneID+"/areas", admin, `{"name":"Area One"}`)
	if areaRec.Code != http.StatusCreated {
		t.Fatalf("area create status = %d, want %d, body: %s", areaRec.Code, http.StatusCreated, areaRec.Body.String())
	}
	var area map[string]any
	if err := json.NewDecoder(areaRec.Body).Decode(&area); err != nil {
		t.Fatalf("decode area response: %v", err)
	}
	if area["zone_id"] != zoneID {
		t.Errorf("zone_id = %v, want %v", area["zone_id"], zoneID)
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/zones/"+zoneID+"/areas", admin, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("area list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode area list response: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != area["id"] {
		t.Errorf("area list = %v, want exactly the created area", list)
	}
}

func TestAreaFlow_NonexistentZoneRejected(t *testing.T) {
	router, uRepo, _, _ := setupZonesTest(t)
	admin := adminToken(t, uRepo)

	rec := doJSON(router, http.MethodPost, "/api/v1/zones/00000000-0000-0000-0000-000000000000/areas", admin, `{"name":"Orphan"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestAreaCreate_ZoneIDBodyTamperCannotOverridePath is STEP 25's
// integration-level check: even against the real router and real
// database, a client-supplied zone_id in the body cannot redirect an
// area to a different zone than the URL specifies.
func TestAreaCreate_ZoneIDBodyTamperCannotOverridePath(t *testing.T) {
	router, uRepo, _, _ := setupZonesTest(t)
	admin := adminToken(t, uRepo)

	zoneARec := doJSON(router, http.MethodPost, "/api/v1/zones", admin, fmt.Sprintf(`{"name":%q}`, uniqueEmail("tamper-zone-a")))
	zoneBRec := doJSON(router, http.MethodPost, "/api/v1/zones", admin, fmt.Sprintf(`{"name":%q}`, uniqueEmail("tamper-zone-b")))
	var zoneA, zoneB map[string]any
	if err := json.NewDecoder(zoneARec.Body).Decode(&zoneA); err != nil {
		t.Fatalf("decode zoneA: %v", err)
	}
	if err := json.NewDecoder(zoneBRec.Body).Decode(&zoneB); err != nil {
		t.Fatalf("decode zoneB: %v", err)
	}
	zoneAID, _ := zoneA["id"].(string)
	zoneBID, _ := zoneB["id"].(string)

	rec := doJSON(router, http.MethodPost, "/api/v1/zones/"+zoneAID+"/areas", admin,
		fmt.Sprintf(`{"name":"Sneaky","zone_id":%q}`, zoneBID))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (unknown field zone_id must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestAreaEndpoints_RoleGating(t *testing.T) {
	router, uRepo, _, _ := setupZonesTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	zoneRec := doJSON(router, http.MethodPost, "/api/v1/zones", admin, fmt.Sprintf(`{"name":%q}`, uniqueEmail("area-gating-zone")))
	var zone map[string]any
	if err := json.NewDecoder(zoneRec.Body).Decode(&zone); err != nil {
		t.Fatalf("decode zone: %v", err)
	}
	zoneID, _ := zone["id"].(string)

	cases := []struct {
		label string
		token string
		want  int
	}{
		{"customer forbidden", customer, http.StatusForbidden},
		{"delivery agent forbidden", agent, http.StatusForbidden},
		{"unauthenticated rejected", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doJSON(router, http.MethodPost, "/api/v1/zones/"+zoneID+"/areas", tc.token, `{"name":"Blocked"}`)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// --- Database constraints ---

func TestZonesTable_NameUniqueConstraint(t *testing.T) {
	_, _, _, pool := setupZonesTest(t)
	name := uniqueEmail("unique-zone-name")

	if _, err := pool.Exec(context.Background(), `INSERT INTO zones (name) VALUES ($1)`, name); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO zones (name) VALUES ($1)`, name); err == nil {
		t.Error("expected a unique constraint violation on duplicate zone name, got none")
	}
}

func TestZonesTable_NameCheckConstraintRejectsBlank(t *testing.T) {
	_, _, _, pool := setupZonesTest(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO zones (name) VALUES ($1)`, "   "); err == nil {
		t.Error("expected a CHECK constraint violation on a whitespace-only zone name, got none")
	}
}

func TestAreasTable_ZoneForeignKeyRejectsUnknownZone(t *testing.T) {
	_, _, _, pool := setupZonesTest(t)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO areas (name, zone_id) VALUES ($1, $2)`, "Orphan Area", "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("expected a foreign key violation inserting an area under a nonexistent zone, got none")
	}
}

func TestAreasTable_UniqueConstraintPerZone(t *testing.T) {
	_, _, zRepo, pool := setupZonesTest(t)
	zone, err := zRepo.CreateZone(context.Background(), zones.CreateZoneInput{Name: uniqueEmail("area-unique-zone")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}

	if _, err := pool.Exec(context.Background(), `INSERT INTO areas (name, zone_id) VALUES ($1, $2)`, "Repeat", zone.ID); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO areas (name, zone_id) VALUES ($1, $2)`, "Repeat", zone.ID); err == nil {
		t.Error("expected a unique constraint violation on duplicate (zone_id, name), got none")
	}
}

// TestDeliveryAgentsCurrentZoneFK is STEP 23's explicit requirement:
// verify, against real PostgreSQL, that current_zone_id now really is
// constrained by a foreign key to zones.id — the constraint M03
// deliberately deferred until this module's zones table existed.
func TestDeliveryAgentsCurrentZoneFK(t *testing.T) {
	_, uRepo, zRepo, pool := setupZonesTest(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	user, err := uRepo.Create(ctx, users.NewUser{
		Email: uniqueEmail("fk-agent"), PasswordHash: hash, FullName: "FK Agent", Role: users.RoleDeliveryAgent,
	})
	if err != nil {
		t.Fatalf("seed user create failed: %v", err)
	}
	var agentID string
	if err := pool.QueryRow(ctx, `INSERT INTO delivery_agents (user_id) VALUES ($1) RETURNING id`, user.ID).Scan(&agentID); err != nil {
		t.Fatalf("seed delivery_agents row failed: %v", err)
	}

	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: uniqueEmail("fk-zone")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE delivery_agents SET current_zone_id = $1 WHERE id = $2`, zone.ID, agentID); err != nil {
		t.Errorf("valid zone id was rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE delivery_agents SET current_zone_id = $1 WHERE id = $2`,
		"00000000-0000-0000-0000-000000000000", agentID); err == nil {
		t.Error("expected a foreign key violation setting current_zone_id to a nonexistent zone, got none")
	}
	if _, err := pool.Exec(ctx, `UPDATE delivery_agents SET current_zone_id = NULL WHERE id = $1`, agentID); err != nil {
		t.Errorf("NULL current_zone_id was rejected: %v", err)
	}
}

// --- Resolution, against the real Postgres-backed repository ---

func TestResolution_IntraAndInterAgainstRealDatabase(t *testing.T) {
	_, _, zRepo, _ := setupZonesTest(t)
	ctx := context.Background()

	zoneA, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: uniqueEmail("resolve-zone-a")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	zoneB, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: uniqueEmail("resolve-zone-b")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	areaA, err := zRepo.CreateArea(ctx, zoneA.ID, zones.CreateAreaInput{Name: "A1"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	areaB, err := zRepo.CreateArea(ctx, zoneB.ID, zones.CreateAreaInput{Name: "B1"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	intra, err := zones.ResolvePickupDrop(ctx, zRepo, areaA.ID, areaA.ID)
	if err != nil {
		t.Fatalf("ResolvePickupDrop() error: %v", err)
	}
	if intra.Relationship != zones.RelationshipIntra {
		t.Errorf("A+A relationship = %v, want INTRA", intra.Relationship)
	}

	inter, err := zones.ResolvePickupDrop(ctx, zRepo, areaA.ID, areaB.ID)
	if err != nil {
		t.Fatalf("ResolvePickupDrop() error: %v", err)
	}
	if inter.Relationship != zones.RelationshipInter {
		t.Errorf("A+B relationship = %v, want INTER", inter.Relationship)
	}

	if _, err := zones.ResolvePickupDrop(ctx, zRepo, areaA.ID, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("expected an explicit error resolving a nonexistent area, got none")
	}
}
