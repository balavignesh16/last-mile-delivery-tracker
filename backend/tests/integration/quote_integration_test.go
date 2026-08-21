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
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// setupQuoteTest builds the exact same router main.go builds — real
// Postgres, real migrations, real middleware, auth.Mount, zones.Mount,
// and rates.Mount (which is what actually registers POST
// /orders/quote) — so these tests exercise the full M04 -> M05 -> M06
// stack end to end, the same way setupRatesTest/setupZonesTest do for
// their own modules.
func setupQuoteTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, ratesRepo rates.Repository, pool *pgxpool.Pool) {
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
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, zRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, zRepo, rRepo, p
}

func createZoneAndArea(t *testing.T, zRepo zones.Repository, zoneName, areaName string) (zoneID, areaID string) {
	t.Helper()
	ctx := context.Background()
	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: zoneName})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	area, err := zRepo.CreateArea(ctx, zone.ID, zones.CreateAreaInput{Name: areaName})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	return zone.ID, area.ID
}

// resetCombination clears any active card for (orderType, zoneRelationship)
// — the same precondition-reset pattern rates_integration_test.go uses,
// necessary because only 4 combinations exist total and TEST_DATABASE_URL
// persists across test runs.
func resetCombination(t *testing.T, pool *pgxpool.Pool, orderType, zoneRelationship string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE rate_cards SET active = false WHERE order_type = $1 AND zone_relationship = $2`, orderType, zoneRelationship); err != nil {
		t.Fatalf("reset precondition failed: %v", err)
	}
}

// setupActiveRateCard resets the combination, creates a fresh rate card,
// and activates it with the given COD surcharge, returning its id.
func setupActiveRateCard(t *testing.T, router http.Handler, pool *pgxpool.Pool, admin, orderType, zoneRelationship string, codSurcharge float64) string {
	t.Helper()
	resetCombination(t, pool, orderType, zoneRelationship)

	createRec := doJSON(router, http.MethodPost, "/api/v1/rates", admin,
		fmt.Sprintf(`{"order_type":%q,"zone_relationship":%q,"cod_surcharge":%v}`, orderType, zoneRelationship, codSurcharge))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("rate card create failed: status = %d, body: %s", createRec.Code, createRec.Body.String())
	}
	var card map[string]any
	if err := json.NewDecoder(createRec.Body).Decode(&card); err != nil {
		t.Fatalf("decode rate card: %v", err)
	}
	cardID, _ := card["id"].(string)

	activateRec := doJSON(router, http.MethodPut, "/api/v1/rates/"+cardID, admin,
		fmt.Sprintf(`{"cod_surcharge":%v,"active":true}`, codSurcharge))
	if activateRec.Code != http.StatusOK {
		t.Fatalf("rate card activate failed: status = %d, body: %s", activateRec.Code, activateRec.Body.String())
	}
	return cardID
}

func addSlab(t *testing.T, router http.Handler, admin, cardID string, minWeight float64, maxWeight *float64, price float64) {
	t.Helper()
	maxJSON := "null"
	if maxWeight != nil {
		maxJSON = fmt.Sprintf("%v", *maxWeight)
	}
	rec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardID+"/slabs", admin,
		fmt.Sprintf(`{"min_weight":%v,"max_weight":%s,"price":%v}`, minWeight, maxJSON, price))
	if rec.Code != http.StatusCreated {
		t.Fatalf("slab create failed: status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func f(v float64) *float64 { return &v }

func decodeQuote(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode quote response: %v, body: %s", err, body)
	}
	return out
}

// --- Golden scenarios ---

func TestQuote_GoldenB2BIntraPrepaid(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	pickupZoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-intra-zone"), "Pickup Point")
	// Second area in the SAME zone as pickup, for a genuine INTRA pair.
	dropArea, err := zRepo.CreateArea(context.Background(), pickupZoneID, zones.CreateAreaInput{Name: "Drop Point"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 20)
	addSlab(t, router, admin, cardID, 0, f(5), 50)
	addSlab(t, router, admin, cardID, 5, f(10), 80)
	addSlab(t, router, admin, cardID, 10, f(15), 110)
	addSlab(t, router, admin, cardID, 15, f(20), 140)
	addSlab(t, router, admin, cardID, 20, nil, 200)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":7}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", customer, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	q := decodeQuote(t, rec.Body.Bytes())

	if q["zone_relationship"] != "INTRA" {
		t.Errorf("zone_relationship = %v, want INTRA", q["zone_relationship"])
	}
	// volumetric = 10*10*10/5000 = 0.2; actual = 7 -> chargeable = max(7,0.2) = 7 -> slab [5,10) -> 80
	if q["volumetric_weight_kg"] != float64(0.2) {
		t.Errorf("volumetric_weight_kg = %v, want 0.2", q["volumetric_weight_kg"])
	}
	if q["chargeable_weight_kg"] != float64(7) {
		t.Errorf("chargeable_weight_kg = %v, want 7", q["chargeable_weight_kg"])
	}
	if q["base_rate"] != float64(80) {
		t.Errorf("base_rate = %v, want 80", q["base_rate"])
	}
	if q["cod_surcharge"] != float64(0) {
		t.Errorf("cod_surcharge = %v, want 0 (PREPAID)", q["cod_surcharge"])
	}
	if q["final_amount"] != float64(80) {
		t.Errorf("final_amount = %v, want 80", q["final_amount"])
	}
	if q["order_type"] != "B2B" || q["payment_type"] != "PREPAID" {
		t.Errorf("unexpected echoed order_type/payment_type: %v / %v", q["order_type"], q["payment_type"])
	}
}

func TestQuote_GoldenB2BIntraCOD(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-cod-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 20)
	addSlab(t, router, admin, cardID, 0, f(5), 50)
	addSlab(t, router, admin, cardID, 5, f(10), 80)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":7}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	q := decodeQuote(t, rec.Body.Bytes())

	if q["cod_surcharge"] != float64(20) {
		t.Errorf("cod_surcharge = %v, want 20 (COD)", q["cod_surcharge"])
	}
	if q["final_amount"] != float64(100) {
		t.Errorf("final_amount = %v, want 100 (80 base + 20 COD)", q["final_amount"])
	}
}

func TestQuote_GoldenB2CInter(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	_, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-inter-zone-a"), "Pickup")
	_, dropID := createZoneAndArea(t, zRepo, uniqueEmail("quote-inter-zone-b"), "Drop")

	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTER", 15)
	addSlab(t, router, admin, cardID, 0, f(20), 60)
	addSlab(t, router, admin, cardID, 20, nil, 250)

	// volumetric = 50*40*30/5000 = 12; actual = 5 -> chargeable = 12 -> slab [0,20) -> 60
	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":50,"breadth_cm":40,"height_cm":30,"actual_weight_kg":5}`,
		pickupID, dropID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	q := decodeQuote(t, rec.Body.Bytes())

	if q["zone_relationship"] != "INTER" {
		t.Errorf("zone_relationship = %v, want INTER", q["zone_relationship"])
	}
	if q["volumetric_weight_kg"] != float64(12) {
		t.Errorf("volumetric_weight_kg = %v, want 12", q["volumetric_weight_kg"])
	}
	if q["chargeable_weight_kg"] != float64(12) {
		t.Errorf("chargeable_weight_kg = %v, want 12 (volumetric > actual)", q["chargeable_weight_kg"])
	}
	if q["base_rate"] != float64(60) {
		t.Errorf("base_rate = %v, want 60", q["base_rate"])
	}
}

func TestQuote_ActualEqualsVolumetric(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-equal-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 99)

	// volumetric = 50*10*10/5000 = 1; actual = 1 -> equal.
	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":50,"breadth_cm":10,"height_cm":10,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	q := decodeQuote(t, rec.Body.Bytes())
	if q["volumetric_weight_kg"] != float64(1) || q["chargeable_weight_kg"] != float64(1) {
		t.Errorf("expected volumetric == actual == chargeable == 1, got volumetric=%v chargeable=%v", q["volumetric_weight_kg"], q["chargeable_weight_kg"])
	}
}

func TestQuote_SamePickupAndDropAreaIsIntra(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("quote-same-area-zone"), "Only Point")

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 42)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		areaID, areaID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	q := decodeQuote(t, rec.Body.Bytes())
	if q["zone_relationship"] != "INTRA" {
		t.Errorf("zone_relationship = %v, want INTRA (same pickup/drop area is allowed)", q["zone_relationship"])
	}
}

// --- Slab boundary edge cases ---

func TestQuote_SlabBoundaries(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-boundary-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, f(5), 50)
	addSlab(t, router, admin, cardID, 5, f(10), 80)
	addSlab(t, router, admin, cardID, 10, f(15), 110)

	cases := []struct {
		weight    float64
		wantPrice float64
		label     string
	}{
		{4.999, 50, "4.999 belongs to [0,5)"},
		{5.000, 80, "5.000 belongs to [5,10), not [0,5)"},
		{9.999, 80, "9.999 belongs to [5,10)"},
		{10.000, 110, "10.000 belongs to [10,15)"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":%v}`,
				pickupID, dropArea.ID, tc.weight)
			rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			q := decodeQuote(t, rec.Body.Bytes())
			if q["base_rate"] != tc.wantPrice {
				t.Errorf("weight %v: base_rate = %v, want %v", tc.weight, q["base_rate"], tc.wantPrice)
			}
		})
	}
}

func TestQuote_OpenEndedSlab(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	_, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-openended-zone-a"), "Pickup")
	_, dropID := createZoneAndArea(t, zRepo, uniqueEmail("quote-openended-zone-b"), "Drop")

	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTER", 0)
	addSlab(t, router, admin, cardID, 0, f(20), 60)
	addSlab(t, router, admin, cardID, 20, nil, 200)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":25}`,
		pickupID, dropID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	q := decodeQuote(t, rec.Body.Bytes())
	if q["base_rate"] != float64(200) {
		t.Errorf("base_rate = %v, want 200 (open-ended slab)", q["base_rate"])
	}
}

func TestQuote_WeightBelowFirstSlab(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-below-first-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 2, f(5), 50) // slab starts at 2, not 0

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuote_WeightInGapBetweenSlabs(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-gap-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, f(5), 50)
	addSlab(t, router, admin, cardID, 10, f(15), 110) // gap between 5 and 10

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":7}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuote_WeightAboveAllSlabsNoOpenEnded(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-above-all-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, f(5), 50)
	addSlab(t, router, admin, cardID, 5, f(10), 80) // nothing open-ended

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":50}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuote_MissingSlabOnActiveCardWithNoSlabs(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-no-slabs-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0) // no slabs added

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":5}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuote_MissingRateCard(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	_, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-no-card-zone-a"), "Pickup")
	_, dropID := createZoneAndArea(t, zRepo, uniqueEmail("quote-no-card-zone-b"), "Drop")

	resetCombination(t, pool, "B2B", "INTER") // ensure no active card at all

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":5}`,
		pickupID, dropID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// --- Zone/area resolution failures ---

func TestQuote_UnknownArea(t *testing.T) {
	router, uRepo, zRepo, _, _ := setupQuoteTest(t)
	admin := adminToken(t, uRepo)
	_, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-unknown-area-zone"), "Pickup")

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":"00000000-0000-0000-0000-000000000000","order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":5}`,
		pickupID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuote_InactiveZoneRejected(t *testing.T) {
	router, uRepo, zRepo, _, _ := setupQuoteTest(t)
	admin := adminToken(t, uRepo)

	zoneName := uniqueEmail("quote-inactive-zone")
	zoneID, pickupID := createZoneAndArea(t, zRepo, zoneName, "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	deactivateRec := doJSON(router, http.MethodPut, "/api/v1/zones/"+zoneID, admin, fmt.Sprintf(`{"name":%q,"active":false}`, zoneName))
	if deactivateRec.Code != http.StatusOK {
		t.Fatalf("zone deactivate failed: status = %d, body: %s", deactivateRec.Code, deactivateRec.Body.String())
	}

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":5}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// --- Input validation ---

func TestQuote_InvalidDimensionsRejected(t *testing.T) {
	router, uRepo, zRepo, _, _ := setupQuoteTest(t)
	admin := adminToken(t, uRepo)
	_, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("quote-invalid-dims-zone"), "Pickup")
	dropID := pickupID

	cases := []struct {
		label string
		body  string
	}{
		{"zero length", fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":0,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`, pickupID, dropID)},
		{"negative weight", fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":-1}`, pickupID, dropID)},
		{"missing height", fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"actual_weight_kg":1}`, pickupID, dropID)},
		{"missing pickup_area_id", fmt.Sprintf(`{"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`, dropID)},
		{"invalid order_type", fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2Z","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`, pickupID, dropID)},
		{"invalid payment_type", fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"CASH","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`, pickupID, dropID)},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, tc.body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// TestQuote_MassAssignmentRejected is the direct analogue of
// TestCreateSlab_RateCardIDBodyTamperCannotOverridePath: none of these
// server-derived fields have anywhere to decode into, so
// DisallowUnknownFields rejects the whole request.
func TestQuote_MassAssignmentRejected(t *testing.T) {
	router, uRepo, zRepo, _, _ := setupQuoteTest(t)
	admin := adminToken(t, uRepo)
	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("quote-mass-assignment-zone"), "Point")

	forbidden := []string{
		`"customer_id":"11111111-1111-1111-1111-111111111111"`,
		`"pickup_zone_id":"11111111-1111-1111-1111-111111111111"`,
		`"drop_zone_id":"11111111-1111-1111-1111-111111111111"`,
		`"zone_relationship":"INTRA"`,
		`"rate_card_id":"11111111-1111-1111-1111-111111111111"`,
		`"volumetric_weight":1`,
		`"chargeable_weight":1`,
		`"base_rate":1`,
		`"cod_surcharge":1`,
		`"final_amount":1`,
		`"status":"CREATED"`,
	}
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1,%s}`,
				areaID, areaID, field)
			rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", admin, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// --- RBAC ---

func TestQuoteEndpoint_RoleGating(t *testing.T) {
	router, uRepo, zRepo, _, pool := setupQuoteTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("quote-rbac-zone"), "Point")
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 42)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		areaID, areaID)

	cases := []struct {
		label string
		token string
		want  int
	}{
		{"admin allowed", admin, http.StatusOK},
		{"customer allowed", customer, http.StatusOK},
		{"delivery agent forbidden", agent, http.StatusForbidden},
		{"unauthenticated rejected", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", tc.token, body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
