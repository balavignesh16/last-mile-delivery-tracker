package rates

import (
	"context"
	"net/http"
	"testing"

	"lastmiletracker/internal/zones"
)

func setupQuoteHandlerFixture(t *testing.T) (*fakeZonesRepo, *fakeRepo, string, string) {
	t.Helper()
	zRepo := newFakeZonesRepo()
	rRepo := newFakeRepo()
	ctx := context.Background()

	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: "HandlerTestZone"})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	area, err := zRepo.CreateArea(ctx, zone.ID, zones.CreateAreaInput{Name: "HandlerTestArea"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}

	activateCardWithSlabs(t, rRepo, OrderTypeB2B, RelationshipIntra, 15, []CreateSlabInput{
		{MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
		{MinWeight: 5, MaxWeight: nil, Price: 90},
	})

	return zRepo, rRepo, area.ID, area.ID
}

func TestQuoteHandler_Success(t *testing.T) {
	zRepo, rRepo, pickupID, dropID := setupQuoteHandlerFixture(t)

	body := `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `",` +
		`"order_type":"B2B","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
	rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeJSON[map[string]any](t, rec)
	// volumetric = 10*10*10/5000 = 0.2, actual = 2 -> chargeable = 2 -> slab [0,5) -> 50, COD +15 = 65
	if resp["base_rate"] != float64(50) {
		t.Errorf("base_rate = %v, want 50", resp["base_rate"])
	}
	if resp["cod_surcharge"] != float64(15) {
		t.Errorf("cod_surcharge = %v, want 15", resp["cod_surcharge"])
	}
	if resp["final_amount"] != float64(65) {
		t.Errorf("final_amount = %v, want 65", resp["final_amount"])
	}
	if resp["zone_relationship"] != "INTRA" {
		t.Errorf("zone_relationship = %v, want INTRA", resp["zone_relationship"])
	}
}

func TestQuoteHandler_InvalidJSONBody(t *testing.T) {
	zRepo, rRepo, _, _ := setupQuoteHandlerFixture(t)
	rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", `{not json`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestQuoteHandler_ValidationErrors(t *testing.T) {
	zRepo, rRepo, pickupID, dropID := setupQuoteHandlerFixture(t)

	cases := []struct {
		label string
		body  string
	}{
		{"missing pickup_area_id", `{"drop_area_id":"` + dropID + `","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"missing drop_area_id", `{"pickup_area_id":"` + pickupID + `","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"invalid order_type", `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"XYZ","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"invalid payment_type", `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"B2B","payment_type":"CHEQUE","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"missing length_cm", `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"B2B","payment_type":"COD","breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"zero length_cm", `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"B2B","payment_type":"COD","length_cm":0,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"negative actual_weight_kg", `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":-1}`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", tc.body, nil)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// TestQuoteHandler_ServerDerivedFieldsRejected mirrors
// TestCreateSlab_RateCardIDBodyTamperCannotOverridePath: every field the
// pricing pipeline is supposed to derive itself has structurally nowhere
// to decode into, so DisallowUnknownFields turns an attempt to send one
// into a 422 for the whole request — never a silently-ignored field.
func TestQuoteHandler_ServerDerivedFieldsRejected(t *testing.T) {
	zRepo, rRepo, pickupID, dropID := setupQuoteHandlerFixture(t)

	forbidden := []string{
		`"customer_id":"someone-else"`,
		`"pickup_zone_id":"fake-zone-1"`,
		`"drop_zone_id":"fake-zone-1"`,
		`"zone_relationship":"INTRA"`,
		`"rate_card_id":"fake-card-1"`,
		`"volumetric_weight":1`,
		`"chargeable_weight":1`,
		`"base_rate":1`,
		`"cod_surcharge":1`,
		`"final_amount":1`,
		`"status":"CREATED"`,
	}
	base := `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1,`
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", base+field+`}`, nil)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

func TestQuoteHandler_UnknownAreaMapsTo422(t *testing.T) {
	zRepo, rRepo, pickupID, _ := setupQuoteHandlerFixture(t)
	body := `{"pickup_area_id":"` + pickupID + `","drop_area_id":"does-not-exist","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`
	rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuoteHandler_MissingRateCardMapsTo422(t *testing.T) {
	zRepo, rRepo, pickupID, dropID := setupQuoteHandlerFixture(t)
	// B2C/INTRA has no active card in this fixture (only B2B/INTRA does).
	body := `{"pickup_area_id":"` + pickupID + `","drop_area_id":"` + dropID + `","order_type":"B2C","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`
	rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestQuoteHandler_NoMatchingSlabMapsTo422(t *testing.T) {
	zRepo, rRepo := newFakeZonesRepo(), newFakeRepo()
	ctx := context.Background()
	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: "GapZone"})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	area, err := zRepo.CreateArea(ctx, zone.ID, zones.CreateAreaInput{Name: "GapArea"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	activateCardWithSlabs(t, rRepo, OrderTypeB2B, RelationshipIntra, 0, []CreateSlabInput{
		{MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
	})

	body := `{"pickup_area_id":"` + area.ID + `","drop_area_id":"` + area.ID + `","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":50}`
	rec := doRequest(t, QuoteHandler(zRepo, rRepo), http.MethodPost, "/api/v1/orders/quote", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}
