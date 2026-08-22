//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// setupOrdersTest builds the exact same router main.go builds — real
// Postgres, real migrations, real middleware, auth.Mount, zones.Mount,
// rates.Mount, and orders.Mount — so these tests exercise the full
// M04 -> M05 -> M06 -> M07 stack end to end. Reuses
// createZoneAndArea/setupActiveRateCard/addSlab/f (quote_integration_test.go)
// and adminToken/customerToken/deliveryAgentToken/doJSON/uniqueEmail
// (agents_integration_test.go) — all in this same package already.
func setupOrdersTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, ratesRepo rates.Repository, ordersRepo orders.Repository, pool *pgxpool.Pool) {
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
	aRepo := agents.NewPostgresRepository(p)
	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, zRepo, agentsIntegrationJWTSecret),
		orders.Mount(oRepo, uRepo, zRepo, rRepo, aRepo, agentsIntegrationJWTSecret, nil),
	)
	return r, uRepo, zRepo, rRepo, oRepo, p
}

func decodeOrder(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode order response: %v, body: %s", err, body)
	}
	return out
}

// --- Create ---

func TestOrderFlow_CustomerCreatesOwnOrder(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	customerEmail := uniqueEmail("order-customer")
	customer := userToken(t, uRepo, customerEmail, users.RoleCustomer)
	customerUser, err := uRepo.FindByEmail(context.Background(), customerEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-create-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 20)
	addSlab(t, router, admin, cardID, 0, f(5), 50)
	addSlab(t, router, admin, cardID, 5, nil, 90)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`,
		pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", customer, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	order := decodeOrder(t, rec.Body.Bytes())

	if order["customer_id"] != customerUser.ID {
		t.Errorf("customer_id = %v, want %v (from JWT)", order["customer_id"], customerUser.ID)
	}
	if order["created_by"] != customerUser.ID {
		t.Errorf("created_by = %v, want %v", order["created_by"], customerUser.ID)
	}
	if order["status"] != "CREATED" {
		t.Errorf("status = %v, want CREATED", order["status"])
	}
	if order["zone_relationship"] != "INTRA" {
		t.Errorf("zone_relationship = %v, want INTRA", order["zone_relationship"])
	}
	if order["base_rate"] != float64(50) || order["cod_surcharge"] != float64(20) || order["final_amount"] != float64(70) {
		t.Errorf("pricing = base %v cod %v final %v, want 50/20/70", order["base_rate"], order["cod_surcharge"], order["final_amount"])
	}

	// Verify the row actually persisted in Postgres, not just the response.
	orderID, _ := order["id"].(string)
	var dbCustomerID, dbStatus string
	var dbFinalAmount float64
	err = pool.QueryRow(context.Background(), `SELECT customer_id, status, final_amount::float8 FROM orders WHERE id = $1`, orderID).
		Scan(&dbCustomerID, &dbStatus, &dbFinalAmount)
	if err != nil {
		t.Fatalf("query persisted order: %v", err)
	}
	if dbCustomerID != customerUser.ID || dbStatus != "CREATED" || dbFinalAmount != 70 {
		t.Errorf("persisted row = customer %v status %v final %v, want %v/CREATED/70", dbCustomerID, dbStatus, dbFinalAmount, customerUser.ID)
	}
}

func TestOrderFlow_AdminCreatesForCustomer(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	adminEmail := uniqueEmail("order-admin-actor")
	admin := userToken(t, uRepo, adminEmail, users.RoleAdmin)
	adminUser, err := uRepo.FindByEmail(context.Background(), adminEmail)
	if err != nil {
		t.Fatalf("FindByEmail(admin) error: %v", err)
	}
	customerEmail := uniqueEmail("order-admin-target")
	userToken(t, uRepo, customerEmail, users.RoleCustomer) // seed the user
	customerUser, err := uRepo.FindByEmail(context.Background(), customerEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-admin-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2B", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 100)

	body := fmt.Sprintf(`{"customer_id":%q,"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2B","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		customerUser.ID, pickupID, dropArea.ID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", admin, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	order := decodeOrder(t, rec.Body.Bytes())

	if order["customer_id"] != customerUser.ID {
		t.Errorf("customer_id = %v, want %v", order["customer_id"], customerUser.ID)
	}
	if order["created_by"] != adminUser.ID {
		t.Errorf("created_by = %v, want the acting admin's id %v", order["created_by"], adminUser.ID)
	}
}

func TestOrderCreate_CustomerCannotSpecifyCustomerID(t *testing.T) {
	router, uRepo, zRepo, _, _, _ := setupOrdersTest(t)
	customer := customerToken(t, uRepo)
	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("order-privesc-zone"), "Point")

	body := fmt.Sprintf(`{"customer_id":"11111111-1111-1111-1111-111111111111","pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		areaID, areaID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", customer, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestOrderCreate_AdminCannotTargetNonCustomer(t *testing.T) {
	router, uRepo, zRepo, _, _, _ := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	agentEmail := uniqueEmail("order-target-agent")
	userToken(t, uRepo, agentEmail, users.RoleDeliveryAgent)
	agentUser, err := uRepo.FindByEmail(context.Background(), agentEmail)
	if err != nil {
		t.Fatalf("FindByEmail() error: %v", err)
	}
	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("order-target-zone"), "Point")

	body := fmt.Sprintf(`{"customer_id":%q,"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		agentUser.ID, areaID, areaID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestOrderCreate_AdminNonexistentCustomerRejected(t *testing.T) {
	router, uRepo, zRepo, _, _, _ := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("order-nonexistent-zone"), "Point")

	body := fmt.Sprintf(`{"customer_id":"00000000-0000-0000-0000-000000000000","pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		areaID, areaID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", admin, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// TestOrderCreate_PricingSnapshotMatchesM06Calculation is the explicit
// "M06 CalculateQuote is actually used, never re-implemented" check at
// the integration level: request a quote and a real order for the exact
// same input, and assert every pricing field matches exactly.
func TestOrderCreate_PricingSnapshotMatchesM06Calculation(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-snapshot-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 25)
	addSlab(t, router, admin, cardID, 0, f(10), 65)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":4}`,
		pickupID, dropArea.ID)

	quoteRec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", customer, body)
	if quoteRec.Code != http.StatusOK {
		t.Fatalf("quote status = %d, body: %s", quoteRec.Code, quoteRec.Body.String())
	}
	quote := decodeQuote(t, quoteRec.Body.Bytes())

	orderRec := doJSON(router, http.MethodPost, "/api/v1/orders", customer, body)
	if orderRec.Code != http.StatusCreated {
		t.Fatalf("order status = %d, body: %s", orderRec.Code, orderRec.Body.String())
	}
	order := decodeOrder(t, orderRec.Body.Bytes())

	fields := []string{"zone_relationship", "volumetric_weight_kg", "chargeable_weight_kg", "rate_card_id", "base_rate", "cod_surcharge", "final_amount"}
	for _, field := range fields {
		if quote[field] != order[field] {
			t.Errorf("field %s: quote=%v order=%v, want equal (both must come from the same rates.CalculateQuote path)", field, quote[field], order[field])
		}
	}
}

// --- Mass assignment ---

func TestOrderCreate_MassAssignmentRejected(t *testing.T) {
	router, uRepo, zRepo, _, _, _ := setupOrdersTest(t)
	customer := customerToken(t, uRepo)
	_, areaID := createZoneAndArea(t, zRepo, uniqueEmail("order-mass-assignment-zone"), "Point")

	forbidden := []string{
		`"id":"11111111-1111-1111-1111-111111111111"`,
		`"created_by":"11111111-1111-1111-1111-111111111111"`,
		`"pickup_zone_id":"11111111-1111-1111-1111-111111111111"`,
		`"drop_zone_id":"11111111-1111-1111-1111-111111111111"`,
		`"zone_relationship":"INTRA"`,
		`"rate_card_id":"11111111-1111-1111-1111-111111111111"`,
		`"volumetric_weight_kg":1`,
		`"chargeable_weight_kg":1`,
		`"base_rate":1`,
		`"cod_surcharge":1`,
		`"final_amount":1`,
		`"status":"DELIVERED"`,
		`"created_at":"2026-01-01T00:00:00Z"`,
	}
	for _, field := range forbidden {
		t.Run(field, func(t *testing.T) {
			body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1,%s}`,
				areaID, areaID, field)
			rec := doJSON(router, http.MethodPost, "/api/v1/orders", customer, body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected), body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		})
	}
}

// --- List / Get: ownership & RBAC ---

func TestOrderList_CustomerIsolation(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	customerAEmail := uniqueEmail("order-isolation-a")
	customerBEmail := uniqueEmail("order-isolation-b")
	customerA := userToken(t, uRepo, customerAEmail, users.RoleCustomer)
	customerB := userToken(t, uRepo, customerBEmail, users.RoleCustomer)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-isolation-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 40)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)
	if rec := doJSON(router, http.MethodPost, "/api/v1/orders", customerA, body); rec.Code != http.StatusCreated {
		t.Fatalf("customer A order create failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(router, http.MethodPost, "/api/v1/orders", customerB, body); rec.Code != http.StatusCreated {
		t.Fatalf("customer B order create failed: %d %s", rec.Code, rec.Body.String())
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/orders", customerA, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body: %s", listRec.Code, listRec.Body.String())
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, o := range list {
		if o["customer_id"] == nil {
			t.Fatalf("order missing customer_id: %v", o)
		}
	}
	customerAUser, _ := uRepo.FindByEmail(context.Background(), customerAEmail)
	for _, o := range list {
		if o["customer_id"] != customerAUser.ID {
			t.Errorf("customer A's order list contained another customer's order: %v", o)
		}
	}
	if len(list) == 0 {
		t.Error("customer A's order list is empty, want at least the one they created")
	}
}

func TestOrderGet_CustomerOwnershipIDOR(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	ownerToken := customerToken(t, uRepo)
	otherToken := customerToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-idor-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 40)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)
	createRec := doJSON(router, http.MethodPost, "/api/v1/orders", ownerToken, body)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", createRec.Code, createRec.Body.String())
	}
	order := decodeOrder(t, createRec.Body.Bytes())
	orderID, _ := order["id"].(string)

	getOwn := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, ownerToken, "")
	if getOwn.Code != http.StatusOK {
		t.Errorf("owner GET status = %d, want %d", getOwn.Code, http.StatusOK)
	}

	getOther := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, otherToken, "")
	if getOther.Code != http.StatusNotFound {
		t.Errorf("non-owner GET status = %d, want %d (must not reveal existence)", getOther.Code, http.StatusNotFound)
	}
}

func TestOrderList_AdminSeesAll(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-admin-list-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 40)

	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)
	if rec := doJSON(router, http.MethodPost, "/api/v1/orders", customer, body); rec.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/orders", admin, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, body: %s", listRec.Code, listRec.Body.String())
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) == 0 {
		t.Error("admin order list is empty, want at least the order just created")
	}
}

func TestOrderList_DeliveryAgentSeesOnlyAssignedOrders(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	aRepo := agents.NewPostgresRepository(pool)

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	myAgentUser, err := uRepo.Create(context.Background(), users.NewUser{Email: uniqueEmail("order-agent-mine"), PasswordHash: hash, FullName: "Mine", Role: users.RoleDeliveryAgent})
	if err != nil {
		t.Fatalf("seed agent user failed: %v", err)
	}
	myAgent, err := aRepo.Create(context.Background(), agents.CreateAgentInput{Email: uniqueEmail("order-agent-mine-record"), PasswordHash: hash, FullName: "Mine Record"})
	if err != nil {
		t.Fatalf("Create agent record failed: %v", err)
	}
	_ = myAgentUser
	otherAgent, err := aRepo.Create(context.Background(), agents.CreateAgentInput{Email: uniqueEmail("order-agent-other-record"), PasswordHash: hash, FullName: "Other Record"})
	if err != nil {
		t.Fatalf("Create other agent record failed: %v", err)
	}
	myAgentToken, err := auth.GenerateToken(agentsIntegrationJWTSecret, myAgent.UserID, users.RoleDeliveryAgent, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-agent-list-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 40)
	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)

	createOrder := func() string {
		rec := doJSON(router, http.MethodPost, "/api/v1/orders", customer, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("order create failed: %d %s", rec.Code, rec.Body.String())
		}
		return decodeOrder(t, rec.Body.Bytes())["id"].(string)
	}
	assignedToMe := createOrder()
	assignedToOther := createOrder()
	createOrder() // unassigned, must not appear for either agent

	if _, err := pool.Exec(context.Background(), `UPDATE orders SET assigned_agent_id = $1 WHERE id = $2`, myAgent.ID, assignedToMe); err != nil {
		t.Fatalf("seed assignment failed: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE orders SET assigned_agent_id = $1 WHERE id = $2`, otherAgent.ID, assignedToOther); err != nil {
		t.Fatalf("seed assignment failed: %v", err)
	}

	listRec := doJSON(router, http.MethodGet, "/api/v1/orders", myAgentToken, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("agent list status = %d, body: %s", listRec.Code, listRec.Body.String())
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != assignedToMe {
		t.Errorf("agent's order list = %v, want exactly the one order assigned to them", list)
	}

	getOwn := doJSON(router, http.MethodGet, "/api/v1/orders/"+assignedToMe, myAgentToken, "")
	if getOwn.Code != http.StatusOK {
		t.Errorf("agent GET own assigned order status = %d, want %d", getOwn.Code, http.StatusOK)
	}
	getOther := doJSON(router, http.MethodGet, "/api/v1/orders/"+assignedToOther, myAgentToken, "")
	if getOther.Code != http.StatusNotFound {
		t.Errorf("agent GET another agent's assigned order status = %d, want %d (must not reveal existence)", getOther.Code, http.StatusNotFound)
	}
}

// --- RBAC ---

// TestOrderEndpoints_DeliveryAgentAndUnauthenticatedForbidden covers
// what's still forbidden after M09 widened GET /orders and GET
// /orders/:id to DELIVERY_AGENT: creating an order remains ADMIN/
// CUSTOMER only, and every route still rejects an unauthenticated
// caller outright. DELIVERY_AGENT read-scoping itself (only their own
// assigned orders) is covered by TestOrderList_DeliveryAgentSeesOnlyAssignedOrders.
func TestOrderEndpoints_DeliveryAgentAndUnauthenticatedForbidden(t *testing.T) {
	router, uRepo, zRepo, _, _, pool := setupOrdersTest(t)
	admin := adminToken(t, uRepo)
	agent := deliveryAgentToken(t, uRepo)

	zoneID, pickupID := createZoneAndArea(t, zRepo, uniqueEmail("order-rbac-zone"), "Pickup")
	dropArea, err := zRepo.CreateArea(context.Background(), zoneID, zones.CreateAreaInput{Name: "Drop"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	cardID := setupActiveRateCard(t, router, pool, admin, "B2C", "INTRA", 0)
	addSlab(t, router, admin, cardID, 0, nil, 40)
	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":1}`,
		pickupID, dropArea.ID)

	cases := []struct {
		label  string
		method string
		path   string
		token  string
		body   string
		want   int
	}{
		{"agent POST forbidden", http.MethodPost, "/api/v1/orders", agent, body, http.StatusForbidden},
		{"agent GET list allowed (scoped, empty without an agent record)", http.MethodGet, "/api/v1/orders", agent, "", http.StatusOK},
		{"agent GET nonexistent order not found", http.MethodGet, "/api/v1/orders/00000000-0000-0000-0000-000000000000", agent, "", http.StatusNotFound},
		{"unauthenticated POST rejected", http.MethodPost, "/api/v1/orders", "", body, http.StatusUnauthorized},
		{"unauthenticated GET list rejected", http.MethodGet, "/api/v1/orders", "", "", http.StatusUnauthorized},
		{"unauthenticated GET one rejected", http.MethodGet, "/api/v1/orders/00000000-0000-0000-0000-000000000000", "", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			rec := doJSON(router, tc.method, tc.path, tc.token, tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// --- Database constraints ---

func TestOrdersTable_ForeignKeyConstraints(t *testing.T) {
	_, uRepo, zRepo, rRepo, _, pool := setupOrdersTest(t)
	ctx := context.Background()

	customerEmail := uniqueEmail("order-fk-customer")
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	customerUser, err := uRepo.Create(ctx, users.NewUser{Email: customerEmail, PasswordHash: hash, FullName: "FK Customer", Role: users.RoleCustomer})
	if err != nil {
		t.Fatalf("seed customer create failed: %v", err)
	}

	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: uniqueEmail("order-fk-zone")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	area, err := zRepo.CreateArea(ctx, zone.ID, zones.CreateAreaInput{Name: "FK Area"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	card, err := rRepo.CreateRateCard(ctx, rates.CreateRateCardInput{OrderType: rates.OrderTypeB2C, ZoneRelationship: zones.RelationshipIntra})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}

	validInsert := `INSERT INTO orders (
		customer_id, created_by, order_type, payment_type,
		pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship,
		length_cm, breadth_cm, height_cm, actual_weight_kg,
		volumetric_weight_kg, chargeable_weight_kg,
		rate_card_id, base_rate, cod_surcharge, final_amount
	) VALUES ($1,$2,'B2C','PREPAID',$3,$3,$4,$4,'INTRA',1,1,1,1,1,1,$5,50,0,50)`

	cases := []struct {
		label      string
		customerID string
		createdBy  string
		areaID     string
		zoneIDArg  string
		rateCardID string
	}{
		{"unknown customer_id", "00000000-0000-0000-0000-000000000000", customerUser.ID, area.ID, zone.ID, card.ID},
		{"unknown created_by", customerUser.ID, "00000000-0000-0000-0000-000000000000", area.ID, zone.ID, card.ID},
		{"unknown rate_card_id", customerUser.ID, customerUser.ID, area.ID, zone.ID, "00000000-0000-0000-0000-000000000000"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			_, err := pool.Exec(ctx,
				`INSERT INTO orders (customer_id, created_by, order_type, payment_type, pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship, length_cm, breadth_cm, height_cm, actual_weight_kg, volumetric_weight_kg, chargeable_weight_kg, rate_card_id, base_rate, cod_surcharge, final_amount)
				 VALUES ($1,$2,'B2C','PREPAID',$3,$3,$4,$4,'INTRA',1,1,1,1,1,1,$5,50,0,50)`,
				tc.customerID, tc.createdBy, tc.areaID, tc.zoneIDArg, tc.rateCardID)
			if err == nil {
				t.Errorf("expected a foreign key violation for %s, got none", tc.label)
			}
		})
	}

	// Sanity: the same shape with all-valid ids succeeds (proves the
	// table/column list above is actually correct, not just that FKs
	// reject garbage).
	if _, err := pool.Exec(ctx, validInsert, customerUser.ID, customerUser.ID, area.ID, zone.ID, card.ID); err != nil {
		t.Errorf("valid insert with all real ids failed: %v", err)
	}
}

func TestOrdersTable_CheckConstraints(t *testing.T) {
	_, uRepo, zRepo, rRepo, _, pool := setupOrdersTest(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	customerUser, err := uRepo.Create(ctx, users.NewUser{Email: uniqueEmail("order-check-customer"), PasswordHash: hash, FullName: "Check Customer", Role: users.RoleCustomer})
	if err != nil {
		t.Fatalf("seed customer create failed: %v", err)
	}
	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: uniqueEmail("order-check-zone")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	area, err := zRepo.CreateArea(ctx, zone.ID, zones.CreateAreaInput{Name: "Check Area"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	card, err := rRepo.CreateRateCard(ctx, rates.CreateRateCardInput{OrderType: rates.OrderTypeB2C, ZoneRelationship: zones.RelationshipIntra})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}

	base := func(orderType, paymentType, zoneRel string, length float64) (string, []any) {
		return `INSERT INTO orders (customer_id, created_by, order_type, payment_type, pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship, length_cm, breadth_cm, height_cm, actual_weight_kg, volumetric_weight_kg, chargeable_weight_kg, rate_card_id, base_rate, cod_surcharge, final_amount)
			 VALUES ($1,$1,$2,$3,$4,$4,$5,$5,$6,$7,1,1,1,1,$8,50,0,50)`,
			[]any{customerUser.ID, orderType, paymentType, area.ID, zone.ID, zoneRel, length, card.ID}
	}

	cases := []struct {
		label                           string
		orderType, paymentType, zoneRel string
		length                          float64
	}{
		{"invalid order_type", "B2Z", "PREPAID", "INTRA", 1},
		{"invalid payment_type", "B2C", "CASH", "INTRA", 1},
		{"invalid zone_relationship", "B2C", "PREPAID", "SIDEWAYS", 1},
		{"non-positive length_cm", "B2C", "PREPAID", "INTRA", 0},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			stmt, args := base(tc.orderType, tc.paymentType, tc.zoneRel, tc.length)
			_, err := pool.Exec(ctx, stmt, args...)
			if err == nil {
				t.Errorf("expected a CHECK constraint violation for %s, got none", tc.label)
			}
		})
	}
}

func TestOrdersTable_StatusDefaultsToCreated(t *testing.T) {
	_, uRepo, zRepo, rRepo, _, pool := setupOrdersTest(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	customerUser, err := uRepo.Create(ctx, users.NewUser{Email: uniqueEmail("order-status-customer"), PasswordHash: hash, FullName: "Status Customer", Role: users.RoleCustomer})
	if err != nil {
		t.Fatalf("seed customer create failed: %v", err)
	}
	zone, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: uniqueEmail("order-status-zone")})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	area, err := zRepo.CreateArea(ctx, zone.ID, zones.CreateAreaInput{Name: "Status Area"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	card, err := rRepo.CreateRateCard(ctx, rates.CreateRateCardInput{OrderType: rates.OrderTypeB2C, ZoneRelationship: zones.RelationshipIntra})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}

	var status string
	err = pool.QueryRow(ctx,
		`INSERT INTO orders (customer_id, created_by, order_type, payment_type, pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship, length_cm, breadth_cm, height_cm, actual_weight_kg, volumetric_weight_kg, chargeable_weight_kg, rate_card_id, base_rate, cod_surcharge, final_amount)
		 VALUES ($1,$1,'B2C','PREPAID',$2,$2,$3,$3,'INTRA',1,1,1,1,1,1,$4,50,0,50)
		 RETURNING status`,
		customerUser.ID, area.ID, zone.ID, card.ID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("insert without explicit status failed: %v", err)
	}
	if status != "CREATED" {
		t.Errorf("default status = %q, want CREATED", status)
	}
}
