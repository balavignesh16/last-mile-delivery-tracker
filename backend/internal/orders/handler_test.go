package orders

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
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

const testSecret = "orders-test-secret"

// doRequest builds a request with optional chi URL params — same helper
// every prior module's handler_test.go uses.
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

// fixture builds a customer user, an admin user, a pickup/drop area pair
// in one zone (INTRA), and an active B2C rate card with two slabs — the
// common setup nearly every handler test below needs.
type fixture struct {
	usersRepo  *fakeUsersRepo
	zonesRepo  *fakeZonesRepo
	ratesRepo  *fakeRatesRepo
	ordersRepo *fakeOrdersRepo

	customer    users.User
	admin       users.User
	agent       users.User
	areaID      string
	otherAreaID string
}

func newFixture() *fixture {
	uRepo := newFakeUsersRepo()
	zRepo := newFakeZonesRepo()
	rRepo := newFakeRatesRepo()
	oRepo := newFakeOrdersRepo()

	customer := uRepo.seed("customer-1", users.RoleCustomer)
	admin := uRepo.seed("admin-1", users.RoleAdmin)
	agent := uRepo.seed("agent-1", users.RoleDeliveryAgent)

	zone, _ := zRepo.CreateZone(context.Background(), zones.CreateZoneInput{Name: "Zone"})
	area, _ := zRepo.CreateArea(context.Background(), zone.ID, zones.CreateAreaInput{Name: "Area"})
	otherArea, _ := zRepo.CreateArea(context.Background(), zone.ID, zones.CreateAreaInput{Name: "OtherArea"})

	activateCardWithSlabs(rRepo, rates.OrderTypeB2C, rates.RelationshipIntra, 15, []rates.CreateSlabInput{
		{MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
		{MinWeight: 5, MaxWeight: nil, Price: 90},
	})

	return &fixture{
		usersRepo: uRepo, zonesRepo: zRepo, ratesRepo: rRepo, ordersRepo: oRepo,
		customer: customer, admin: admin, agent: agent,
		areaID: area.ID, otherAreaID: otherArea.ID,
	}
}

func (f *fixture) createHandler() http.HandlerFunc {
	return CreateOrderHandler(f.ordersRepo, f.usersRepo, f.zonesRepo, f.ratesRepo)
}

func validCustomerBody(pickup, drop string) string {
	return `{"pickup_area_id":"` + pickup + `","drop_area_id":"` + drop + `","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
}

// --- Create: happy paths ---

func TestCreateOrderHandler_CustomerCreatesOwnOrder(t *testing.T) {
	f := newFixture()
	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
		http.MethodPost, "/api/v1/orders", validCustomerBody(f.areaID, f.areaID), nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["customer_id"] != f.customer.ID {
		t.Errorf("customer_id = %v, want %v (must come from JWT, not the request)", body["customer_id"], f.customer.ID)
	}
	if body["created_by"] != f.customer.ID {
		t.Errorf("created_by = %v, want %v", body["created_by"], f.customer.ID)
	}
	if body["status"] != "CREATED" {
		t.Errorf("status = %v, want CREATED", body["status"])
	}
	// M06 CalculateQuote actually ran: chargeable=2kg -> slab[0,5)=50, COD +15 = 65.
	if body["base_rate"] != float64(50) {
		t.Errorf("base_rate = %v, want 50 (from M06's slab selection)", body["base_rate"])
	}
	if body["cod_surcharge"] != float64(15) {
		t.Errorf("cod_surcharge = %v, want 15 (from M06's COD application)", body["cod_surcharge"])
	}
	if body["final_amount"] != float64(65) {
		t.Errorf("final_amount = %v, want 65", body["final_amount"])
	}
	if body["zone_relationship"] != "INTRA" {
		t.Errorf("zone_relationship = %v, want INTRA (from M06's zone resolution)", body["zone_relationship"])
	}
}

func TestCreateOrderHandler_AdminCreatesForCustomer(t *testing.T) {
	f := newFixture()
	body := `{"customer_id":"` + f.customer.ID + `","pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"PREPAID","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`

	rec := doRequest(t, withAuth(t, f.admin.ID, users.RoleAdmin, f.createHandler()),
		http.MethodPost, "/api/v1/orders", body, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	resp := decodeJSON[map[string]any](t, rec)
	if resp["customer_id"] != f.customer.ID {
		t.Errorf("customer_id = %v, want %v", resp["customer_id"], f.customer.ID)
	}
	if resp["created_by"] != f.admin.ID {
		t.Errorf("created_by = %v, want %v (the admin who submitted it, not the customer)", resp["created_by"], f.admin.ID)
	}
	if resp["cod_surcharge"] != float64(0) {
		t.Errorf("cod_surcharge = %v, want 0 (PREPAID)", resp["cod_surcharge"])
	}
}

func TestCreateOrderHandler_SamePickupAndDropAreaAllowed(t *testing.T) {
	f := newFixture()
	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
		http.MethodPost, "/api/v1/orders", validCustomerBody(f.areaID, f.areaID), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	resp := decodeJSON[map[string]any](t, rec)
	if resp["zone_relationship"] != "INTRA" {
		t.Errorf("zone_relationship = %v, want INTRA", resp["zone_relationship"])
	}
}

// --- Create: identity / privilege escalation ---

func TestCreateOrderHandler_CustomerIdentityComesFromJWTNotBody(t *testing.T) {
	f := newFixture()
	otherCustomer := f.usersRepo.seed("customer-2", users.RoleCustomer)
	_ = otherCustomer

	// customerCreateOrderRequest has no customer_id field at all, so
	// attempting to send one is an unknown field, not a silently
	// ignored one.
	body := `{"customer_id":"customer-2","pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
		http.MethodPost, "/api/v1/orders", body, nil)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (customer_id must be an unknown field for a CUSTOMER caller), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestCreateOrderHandler_AdminMustSpecifyCustomerID(t *testing.T) {
	f := newFixture()
	body := `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
	rec := doRequest(t, withAuth(t, f.admin.ID, users.RoleAdmin, f.createHandler()),
		http.MethodPost, "/api/v1/orders", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateOrderHandler_AdminCannotTargetNonCustomerUser(t *testing.T) {
	f := newFixture()
	cases := []string{f.admin.ID, f.agent.ID}
	for _, targetID := range cases {
		body := `{"customer_id":"` + targetID + `","pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
		rec := doRequest(t, withAuth(t, f.admin.ID, users.RoleAdmin, f.createHandler()),
			http.MethodPost, "/api/v1/orders", body, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("target %s: status = %d, want %d, body: %s", targetID, rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	}
}

func TestCreateOrderHandler_AdminNonexistentCustomerRejected(t *testing.T) {
	f := newFixture()
	body := `{"customer_id":"does-not-exist","pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
	rec := doRequest(t, withAuth(t, f.admin.ID, users.RoleAdmin, f.createHandler()),
		http.MethodPost, "/api/v1/orders", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// --- Create: field validation ---

func TestCreateOrderHandler_ValidationErrors(t *testing.T) {
	f := newFixture()
	cases := []struct {
		label string
		body  string
	}{
		{"invalid order_type", `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"XYZ","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"invalid payment_type", `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"CHEQUE","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"zero length", `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":0,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`},
		{"negative weight", `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":-1}`},
		{"missing dimensions", `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","actual_weight_kg":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
				http.MethodPost, "/api/v1/orders", tc.body, nil)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

func TestCreateOrderHandler_UnknownAreaRejected(t *testing.T) {
	f := newFixture()
	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
		http.MethodPost, "/api/v1/orders", validCustomerBody(f.areaID, "does-not-exist"), nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestCreateOrderHandler_InactiveZoneRejected(t *testing.T) {
	f := newFixture()
	area, _ := f.zonesRepo.FindAreaByID(context.Background(), f.areaID)
	inactive := false
	_, _ = f.zonesRepo.UpdateZone(context.Background(), area.ZoneID, zones.ZoneUpdate{Name: "Zone", Active: &inactive})

	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
		http.MethodPost, "/api/v1/orders", validCustomerBody(f.areaID, f.areaID), nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestCreateOrderHandler_MissingRateCardRejected(t *testing.T) {
	f := newFixture()
	// B2B/INTRA has no active card in this fixture (only B2C/INTRA does).
	body := `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2B","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`
	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
		http.MethodPost, "/api/v1/orders", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestCreateOrderHandler_MissingSlabRejected(t *testing.T) {
	uRepo := newFakeUsersRepo()
	zRepo := newFakeZonesRepo()
	rRepo := newFakeRatesRepo()
	oRepo := newFakeOrdersRepo()
	customer := uRepo.seed("customer-1", users.RoleCustomer)
	zone, _ := zRepo.CreateZone(context.Background(), zones.CreateZoneInput{Name: "Zone"})
	area, _ := zRepo.CreateArea(context.Background(), zone.ID, zones.CreateAreaInput{Name: "Area"})
	activateCardWithSlabs(rRepo, rates.OrderTypeB2C, rates.RelationshipIntra, 0, []rates.CreateSlabInput{
		{MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
	})

	handler := CreateOrderHandler(oRepo, uRepo, zRepo, rRepo)
	body := `{"pickup_area_id":"` + area.ID + `","drop_area_id":"` + area.ID + `","order_type":"B2C","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":50}`
	rec := doRequest(t, withAuth(t, customer.ID, users.RoleCustomer, handler), http.MethodPost, "/api/v1/orders", body, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// --- Mass assignment ---

func TestCreateOrderHandler_ServerDerivedFieldsRejected(t *testing.T) {
	f := newFixture()
	forbidden := []string{
		`"id":"some-id"`,
		`"created_by":"someone-else"`,
		`"pickup_zone_id":"fake-zone-1"`,
		`"drop_zone_id":"fake-zone-1"`,
		`"zone_relationship":"INTRA"`,
		`"volumetric_weight_kg":1`,
		`"chargeable_weight_kg":1`,
		`"rate_card_id":"fake-card-1"`,
		`"base_rate":1`,
		`"cod_surcharge":1`,
		`"final_amount":1`,
		`"status":"DELIVERED"`,
		`"created_at":"2026-01-01T00:00:00Z"`,
	}
	base := `{"pickup_area_id":"` + f.areaID + `","drop_area_id":"` + f.areaID + `","order_type":"B2C","payment_type":"COD","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1,`
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, f.createHandler()),
				http.MethodPost, "/api/v1/orders", base+field+`}`, nil)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// --- List / Get: ownership ---

func TestListOrdersHandler_CustomerSeesOnlyOwnOrders(t *testing.T) {
	f := newFixture()
	otherCustomer := f.usersRepo.seed("customer-2", users.RoleCustomer)

	mustCreate := func(customerID string) {
		quote, err := rates.CalculateQuote(context.Background(), f.zonesRepo, f.ratesRepo, rates.QuoteInput{
			PickupAreaID: f.areaID, DropAreaID: f.areaID, OrderType: rates.OrderTypeB2C, PaymentType: rates.PaymentTypePrepaid,
			LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
		})
		if err != nil {
			t.Fatalf("CalculateQuote() error: %v", err)
		}
		if _, err := f.ordersRepo.CreateOrder(context.Background(), CreateOrderInput{CustomerID: customerID, CreatedBy: customerID, Quote: quote}); err != nil {
			t.Fatalf("CreateOrder() error: %v", err)
		}
	}
	mustCreate(f.customer.ID)
	mustCreate(otherCustomer.ID)

	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, ListOrdersHandler(f.ordersRepo)), http.MethodGet, "/api/v1/orders", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	list := decodeJSON[[]map[string]any](t, rec)
	if len(list) != 1 || list[0]["customer_id"] != f.customer.ID {
		t.Errorf("customer's order list = %v, want exactly their own order", list)
	}
}

func TestGetOrderHandler_CustomerCannotRetrieveAnotherCustomersOrder(t *testing.T) {
	f := newFixture()
	otherCustomer := f.usersRepo.seed("customer-2", users.RoleCustomer)

	quote, err := rates.CalculateQuote(context.Background(), f.zonesRepo, f.ratesRepo, rates.QuoteInput{
		PickupAreaID: f.areaID, DropAreaID: f.areaID, OrderType: rates.OrderTypeB2C, PaymentType: rates.PaymentTypePrepaid,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
	})
	if err != nil {
		t.Fatalf("CalculateQuote() error: %v", err)
	}
	order, err := f.ordersRepo.CreateOrder(context.Background(), CreateOrderInput{CustomerID: otherCustomer.ID, CreatedBy: otherCustomer.ID, Quote: quote})
	if err != nil {
		t.Fatalf("CreateOrder() error: %v", err)
	}

	rec := doRequest(t, withAuth(t, f.customer.ID, users.RoleCustomer, GetOrderHandler(f.ordersRepo)),
		http.MethodGet, "/api/v1/orders/"+order.ID, "", map[string]string{"id": order.ID})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (must not reveal the order exists)", rec.Code, http.StatusNotFound)
	}
}

func TestGetOrderHandler_AdminCanRetrieveAnyOrder(t *testing.T) {
	f := newFixture()
	quote, err := rates.CalculateQuote(context.Background(), f.zonesRepo, f.ratesRepo, rates.QuoteInput{
		PickupAreaID: f.areaID, DropAreaID: f.areaID, OrderType: rates.OrderTypeB2C, PaymentType: rates.PaymentTypePrepaid,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
	})
	if err != nil {
		t.Fatalf("CalculateQuote() error: %v", err)
	}
	order, err := f.ordersRepo.CreateOrder(context.Background(), CreateOrderInput{CustomerID: f.customer.ID, CreatedBy: f.customer.ID, Quote: quote})
	if err != nil {
		t.Fatalf("CreateOrder() error: %v", err)
	}

	rec := doRequest(t, withAuth(t, f.admin.ID, users.RoleAdmin, GetOrderHandler(f.ordersRepo)),
		http.MethodGet, "/api/v1/orders/"+order.ID, "", map[string]string{"id": order.ID})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
