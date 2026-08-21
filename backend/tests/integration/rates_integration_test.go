//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// setupRatesTest builds the exact same router main.go builds — real
// Postgres, real migrations, real middleware, auth.Mount and
// rates.Mount — so these tests exercise the full stack. Reuses
// agentsIntegrationJWTSecret (and therefore adminToken/customerToken/
// deliveryAgentToken/doJSON from agents_integration_test.go and
// zones_integration_test.go) rather than minting a separate secret and
// helper set for no reason.
func setupRatesTest(t *testing.T) (router http.Handler, usersRepo users.Repository, ratesRepo rates.Repository, pool *pgxpool.Pool) {
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
	rRepo := rates.NewPostgresRepository(p)
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, rRepo, p
}

// --- Rate card CRUD ---

func TestRateCardFlow_AdminCreatesActivatesAndUpdates(t *testing.T) {
	router, uRepo, _, pool := setupRatesTest(t)
	admin := adminToken(t, uRepo)

	// See TestConcurrentActivation_OnlyOneWins's comment: only 4
	// (order_type, zone_relationship) combinations exist at all, so a
	// prior run's still-active card for this combination (this
	// TEST_DATABASE_URL persists across runs) would make the
	// activation step below fail with a correct-but-unexpected 409.
	if _, err := pool.Exec(context.Background(),
		`UPDATE rate_cards SET active = false WHERE order_type = 'B2B' AND zone_relationship = 'INTRA'`); err != nil {
		t.Fatalf("reset precondition failed: %v", err)
	}

	createRec := doJSON(router, http.MethodPost, "/api/v1/rates", admin, `{"order_type":"B2B","zone_relationship":"INTRA","cod_surcharge":10}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	cardID, _ := created["id"].(string)
	if cardID == "" {
		t.Fatalf("expected non-empty id, got: %v", created)
	}
	if created["active"] != false {
		t.Errorf("active = %v, want false (new rate cards must start inactive)", created["active"])
	}

	getRec := doJSON(router, http.MethodGet, "/api/v1/rates/"+cardID, admin, "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/rates", admin, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	updateRec := doJSON(router, http.MethodPut, "/api/v1/rates/"+cardID, admin, `{"cod_surcharge":20,"active":true}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("activate status = %d, want %d, body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	var updated map[string]any
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated["active"] != true || updated["cod_surcharge"] != float64(20) {
		t.Errorf("unexpected update response: %v", updated)
	}
}

func TestRateCardEndpoints_RoleGating(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
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
			rec := doJSON(router, http.MethodPost, "/api/v1/rates", tc.token, `{"order_type":"B2C","zone_relationship":"INTER"}`)
			if rec.Code != tc.want {
				t.Errorf("POST /rates status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}

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
			rec := doJSON(router, http.MethodGet, "/api/v1/rates", tc.token, "")
			if rec.Code != tc.want {
				t.Errorf("GET /rates status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// --- Slabs ---

func createTestRateCard(t *testing.T, router http.Handler, admin, orderType, zoneRelationship string) string {
	t.Helper()
	rec := doJSON(router, http.MethodPost, "/api/v1/rates", admin, fmt.Sprintf(`{"order_type":%q,"zone_relationship":%q}`, orderType, zoneRelationship))
	if rec.Code != http.StatusCreated {
		t.Fatalf("rate card creation failed: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&card); err != nil {
		t.Fatalf("decode rate card response: %v", err)
	}
	id, _ := card["id"].(string)
	return id
}

func TestSlabFlow_CreateUnderCardAndList(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	cardID := createTestRateCard(t, router, admin, "B2B", "INTRA")

	slabRec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardID+"/slabs", admin, `{"min_weight":0,"max_weight":5,"price":50}`)
	if slabRec.Code != http.StatusCreated {
		t.Fatalf("slab create status = %d, want %d, body: %s", slabRec.Code, http.StatusCreated, slabRec.Body.String())
	}
	var slab map[string]any
	if err := json.NewDecoder(slabRec.Body).Decode(&slab); err != nil {
		t.Fatalf("decode slab response: %v", err)
	}
	if slab["rate_card_id"] != cardID {
		t.Errorf("rate_card_id = %v, want %v", slab["rate_card_id"], cardID)
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/rates/"+cardID+"/slabs", admin, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("slab list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode slab list: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != slab["id"] {
		t.Errorf("slab list = %v, want exactly the created slab", list)
	}
}

func TestCreateSlab_RateCardIDBodyTamperCannotOverridePath(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	cardA := createTestRateCard(t, router, admin, "B2B", "INTRA")
	cardB := createTestRateCard(t, router, admin, "B2B", "INTER")

	rec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardA+"/slabs", admin,
		fmt.Sprintf(`{"min_weight":0,"max_weight":5,"price":50,"rate_card_id":%q}`, cardB))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (unknown field rate_card_id must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestUpdateSlab_WrongRateCardInPathRejected(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	cardA := createTestRateCard(t, router, admin, "B2C", "INTRA")
	cardB := createTestRateCard(t, router, admin, "B2C", "INTER")

	slabRec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardA+"/slabs", admin, `{"min_weight":0,"max_weight":5,"price":50}`)
	var slab map[string]any
	if err := json.NewDecoder(slabRec.Body).Decode(&slab); err != nil {
		t.Fatalf("decode slab: %v", err)
	}
	slabID, _ := slab["id"].(string)

	rec := doJSON(router, http.MethodPut, "/api/v1/rates/"+cardB+"/slabs/"+slabID, admin, `{"min_weight":0,"max_weight":99,"price":999}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteSlab_WrongRateCardInPathRejected(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	cardA := createTestRateCard(t, router, admin, "B2C", "INTRA")
	cardB := createTestRateCard(t, router, admin, "B2C", "INTER")

	slabRec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardA+"/slabs", admin, `{"min_weight":0,"max_weight":5,"price":50}`)
	var slab map[string]any
	if err := json.NewDecoder(slabRec.Body).Decode(&slab); err != nil {
		t.Fatalf("decode slab: %v", err)
	}
	slabID, _ := slab["id"].(string)

	rec := doJSON(router, http.MethodDelete, "/api/v1/rates/"+cardB+"/slabs/"+slabID, admin, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	stillThere := doJSON(router, http.MethodGet, "/api/v1/rates/"+cardA+"/slabs", admin, "")
	var list []map[string]any
	if err := json.NewDecoder(stillThere.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("slab was deleted through the wrong rate card's URL: list = %v", list)
	}
}

func TestDeleteSlab_Success(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	cardID := createTestRateCard(t, router, admin, "B2B", "INTER")

	slabRec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardID+"/slabs", admin, `{"min_weight":0,"max_weight":5,"price":50}`)
	var slab map[string]any
	if err := json.NewDecoder(slabRec.Body).Decode(&slab); err != nil {
		t.Fatalf("decode slab: %v", err)
	}
	slabID, _ := slab["id"].(string)

	deleteRec := doJSON(router, http.MethodDelete, "/api/v1/rates/"+cardID+"/slabs/"+slabID, admin, "")
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/rates/"+cardID+"/slabs", admin, "")
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("slab list after deletion = %v, want empty", list)
	}
}

func TestSlabEndpoints_RoleGating(t *testing.T) {
	router, uRepo, _, _ := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)
	cardID := createTestRateCard(t, router, admin, "B2C", "INTRA")

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
			rec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardID+"/slabs", tc.token, `{"min_weight":0,"max_weight":5,"price":50}`)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// --- Database constraints ---

func TestRateCardsTable_OrderTypeCheckConstraint(t *testing.T) {
	_, _, _, pool := setupRatesTest(t)
	_, err := pool.Exec(context.Background(), `INSERT INTO rate_cards (order_type, zone_relationship) VALUES ('B2Z', 'INTRA')`)
	if err == nil {
		t.Error("expected a CHECK constraint violation on an invalid order_type, got none")
	}
}

func TestRateCardsTable_ZoneRelationshipCheckConstraint(t *testing.T) {
	_, _, _, pool := setupRatesTest(t)
	_, err := pool.Exec(context.Background(), `INSERT INTO rate_cards (order_type, zone_relationship) VALUES ('B2B', 'SIDEWAYS')`)
	if err == nil {
		t.Error("expected a CHECK constraint violation on an invalid zone_relationship, got none")
	}
}

func TestRateCardsTable_CODSurchargeCheckConstraint(t *testing.T) {
	_, _, _, pool := setupRatesTest(t)
	_, err := pool.Exec(context.Background(), `INSERT INTO rate_cards (order_type, zone_relationship, cod_surcharge) VALUES ('B2B', 'INTRA', -1)`)
	if err == nil {
		t.Error("expected a CHECK constraint violation on a negative cod_surcharge, got none")
	}
}

func TestRateCardsTable_OneActivePerCombinationConstraint(t *testing.T) {
	_, _, _, pool := setupRatesTest(t)
	ctx := context.Background()
	// See TestConcurrentActivation_OnlyOneWins's comment: reset this
	// combination first so a prior run's leftover active row can't make
	// the "first insert succeeds" assumption below false.
	if _, err := pool.Exec(ctx, `UPDATE rate_cards SET active = false WHERE order_type = 'B2C' AND zone_relationship = 'INTER'`); err != nil {
		t.Fatalf("reset precondition failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO rate_cards (order_type, zone_relationship, active) VALUES ('B2C', 'INTER', true)`); err != nil {
		t.Fatalf("first active insert failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO rate_cards (order_type, zone_relationship, active) VALUES ('B2C', 'INTER', true)`); err == nil {
		t.Error("expected a unique constraint violation inserting a second active card for the same combination, got none")
	}
	// A second *inactive* card for the same combination must be fine.
	if _, err := pool.Exec(ctx, `INSERT INTO rate_cards (order_type, zone_relationship, active) VALUES ('B2C', 'INTER', false)`); err != nil {
		t.Errorf("a second inactive card for the same combination was rejected: %v", err)
	}
}

func TestRateCardSlabsTable_ForeignKeyRejectsUnknownRateCard(t *testing.T) {
	_, _, _, pool := setupRatesTest(t)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO rate_card_slabs (rate_card_id, min_weight, price) VALUES ($1, 0, 50)`, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("expected a foreign key violation inserting a slab under a nonexistent rate card, got none")
	}
}

func TestRateCardSlabsTable_MinWeightCheckConstraint(t *testing.T) {
	_, _, rRepo, pool := setupRatesTest(t)
	card, err := rRepo.CreateRateCard(context.Background(), rates.CreateRateCardInput{OrderType: rates.OrderTypeB2B, ZoneRelationship: zones.RelationshipIntra})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO rate_card_slabs (rate_card_id, min_weight, price) VALUES ($1, -1, 50)`, card.ID)
	if err == nil {
		t.Error("expected a CHECK constraint violation on a negative min_weight, got none")
	}
}

func TestRateCardSlabsTable_MaxWeightMustExceedMinWeightConstraint(t *testing.T) {
	_, _, rRepo, pool := setupRatesTest(t)
	card, err := rRepo.CreateRateCard(context.Background(), rates.CreateRateCardInput{OrderType: rates.OrderTypeB2B, ZoneRelationship: zones.RelationshipInter})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO rate_card_slabs (rate_card_id, min_weight, max_weight, price) VALUES ($1, 10, 5, 50)`, card.ID)
	if err == nil {
		t.Error("expected a CHECK constraint violation when max_weight <= min_weight, got none")
	}
}

func TestRateCardSlabsTable_PriceCheckConstraint(t *testing.T) {
	_, _, rRepo, pool := setupRatesTest(t)
	card, err := rRepo.CreateRateCard(context.Background(), rates.CreateRateCardInput{OrderType: rates.OrderTypeB2C, ZoneRelationship: zones.RelationshipIntra})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO rate_card_slabs (rate_card_id, min_weight, price) VALUES ($1, 0, -1)`, card.ID)
	if err == nil {
		t.Error("expected a CHECK constraint violation on a negative price, got none")
	}
}

func TestRateCardSlabsTable_OneOpenEndedSlabPerCardConstraint(t *testing.T) {
	_, _, rRepo, pool := setupRatesTest(t)
	card, err := rRepo.CreateRateCard(context.Background(), rates.CreateRateCardInput{OrderType: rates.OrderTypeB2C, ZoneRelationship: zones.RelationshipInter})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO rate_card_slabs (rate_card_id, min_weight, max_weight, price) VALUES ($1, 20, NULL, 140)`, card.ID); err != nil {
		t.Fatalf("first open-ended insert failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO rate_card_slabs (rate_card_id, min_weight, max_weight, price) VALUES ($1, 30, NULL, 200)`, card.ID); err == nil {
		t.Error("expected a unique constraint violation inserting a second open-ended slab for the same rate card, got none")
	}
}

// --- Concurrency ---

// TestConcurrentActivation_OnlyOneWins fires simultaneous activation
// requests for two distinct, pre-created inactive rate cards that share
// the same (order_type, zone_relationship) combination. The partial
// unique index is what must make exactly one of them win — this test
// proves that under an actual concurrent race against real Postgres,
// not merely by asserting on sequential test order.
func TestConcurrentActivation_OnlyOneWins(t *testing.T) {
	router, uRepo, _, pool := setupRatesTest(t)
	admin := adminToken(t, uRepo)

	// (order_type, zone_relationship) only has 4 possible values total,
	// unlike e.g. email addresses, so this test can't reach for
	// uniqueEmail()-style per-run isolation. Instead it resets this
	// specific combination to "no active card" before racing — without
	// this, a prior run's still-active card (this TEST_DATABASE_URL is
	// a persistent shared database, not reset between test runs) would
	// make every activation attempt below correctly fail with 409,
	// which is right production behavior but would make this test
	// non-deterministic across repeated runs.
	if _, err := pool.Exec(context.Background(),
		`UPDATE rate_cards SET active = false WHERE order_type = 'B2B' AND zone_relationship = 'INTER'`); err != nil {
		t.Fatalf("reset precondition failed: %v", err)
	}

	cardA := createTestRateCard(t, router, admin, "B2B", "INTER")
	cardB := createTestRateCard(t, router, admin, "B2B", "INTER")

	const attempts = 8
	results := make([]int, 0, attempts)
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})

	fire := func(cardID string) {
		defer wg.Done()
		<-start
		rec := doJSON(router, http.MethodPut, "/api/v1/rates/"+cardID, admin, `{"cod_surcharge":0,"active":true}`)
		mu.Lock()
		results = append(results, rec.Code)
		mu.Unlock()
	}

	for i := 0; i < attempts/2; i++ {
		wg.Add(2)
		go fire(cardA)
		go fire(cardB)
	}
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, code := range results {
		switch code {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("unexpected status code under concurrent activation: %d", code)
		}
	}
	if successes == 0 {
		t.Error("no activation attempt succeeded at all")
	}
	if successes+conflicts != attempts {
		t.Errorf("successes(%d) + conflicts(%d) != attempts(%d)", successes, conflicts, attempts)
	}

	var activeCount int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM rate_cards WHERE order_type = 'B2B' AND zone_relationship = 'INTER' AND active`,
	).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if activeCount != 1 {
		t.Errorf("active rate cards for the combination after the race = %d, want exactly 1", activeCount)
	}
}

// TestConcurrentSlabCreation_OverlapPreventedUnderRace fires
// simultaneous requests to create the *same* overlapping slab range
// under one rate card. The FOR UPDATE lock on the parent rate card is
// what must serialize these — this test proves it does, under real
// concurrent load against real Postgres, not merely by calling the
// handler twice in sequence.
func TestConcurrentSlabCreation_OverlapPreventedUnderRace(t *testing.T) {
	router, uRepo, _, pool := setupRatesTest(t)
	admin := adminToken(t, uRepo)
	cardID := createTestRateCard(t, router, admin, "B2C", "INTRA")

	const attempts = 8
	results := make([]int, 0, attempts)
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardID+"/slabs", admin, `{"min_weight":0,"max_weight":5,"price":50}`)
			mu.Lock()
			results = append(results, rec.Code)
			mu.Unlock()
		}()
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
			t.Errorf("unexpected status code under concurrent slab creation: %d", code)
		}
	}
	if successes != 1 {
		t.Errorf("successful slab creations under the race = %d, want exactly 1", successes)
	}
	if successes+conflicts != attempts {
		t.Errorf("successes(%d) + conflicts(%d) != attempts(%d)", successes, conflicts, attempts)
	}

	var slabCount int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM rate_card_slabs WHERE rate_card_id = $1`, cardID).Scan(&slabCount)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if slabCount != 1 {
		t.Errorf("slabs persisted for the rate card after the race = %d, want exactly 1", slabCount)
	}
}
