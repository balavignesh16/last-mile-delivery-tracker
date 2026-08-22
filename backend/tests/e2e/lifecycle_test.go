//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
)

// TestE2E_HappyPath_RegisterQuoteOrderAssignDeliver drives the full,
// real HTTP stack through the primary lifecycle: register/login ->
// quote -> create order -> manual assign -> agent-driven status updates
// -> delivered. Every step goes through its real handler — no direct
// repository seeding except delivery_agents.current_zone_id (see
// helpers_test.go's own doc comment for why that one field has no HTTP
// path in this application at all).
func TestE2E_HappyPath_RegisterQuoteOrderAssignDeliver(t *testing.T) {
	router, pool, notifyCount := setupE2E(t)
	adminToken := loginAsSeededAdmin(t, router)
	customerToken, customerID, _ := registerAndLogin(t, router, "happy-customer")
	zoneID, pickupAreaID, dropAreaID := setupZoneAreasAndRateCard(t, router, pool, adminToken, "happy")
	agentID, agentToken := createReadyAgent(t, router, pool, adminToken, zoneID, "happy")

	// --- Quote ---
	quoteBody := `{"pickup_area_id":"` + pickupAreaID + `","drop_area_id":"` + dropAreaID + `","order_type":"B2C","payment_type":"PREPAID","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`
	quoteRec := doJSON(router, http.MethodPost, "/api/v1/orders/quote", customerToken, quoteBody)
	if quoteRec.Code != http.StatusOK {
		t.Fatalf("quote failed: %d %s", quoteRec.Code, quoteRec.Body.String())
	}
	quote := decodeJSON(t, quoteRec.Body.Bytes())
	if quote["final_amount"] != float64(50) {
		t.Errorf("quote final_amount = %v, want 50 (one slab, no COD surcharge)", quote["final_amount"])
	}

	// --- Create order ---
	beforeCreateCount := *notifyCount
	order := createOrder(t, router, customerToken, pickupAreaID, dropAreaID)
	orderID := order["id"].(string)
	if order["customer_id"] != customerID {
		t.Errorf("customer_id = %v, want %v", order["customer_id"], customerID)
	}
	if order["status"] != "CREATED" {
		t.Errorf("status = %v, want CREATED", order["status"])
	}
	if order["final_amount"] != float64(50) {
		t.Errorf("order final_amount = %v, want 50 (matches the quote)", order["final_amount"])
	}
	if *notifyCount <= beforeCreateCount {
		t.Errorf("notification count did not increase after order creation (ORDER_CREATED notification side effect missing)")
	}

	// --- Role restriction: a CUSTOMER may never assign or transition status ---
	if rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/assign", customerToken, `{"agent_id":"`+agentID+`"}`); rec.Code != http.StatusForbidden {
		t.Errorf("customer assign status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", customerToken, `{"status":"ASSIGNED"}`); rec.Code != http.StatusForbidden {
		t.Errorf("customer status-transition status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	// --- Assignment (manual, ADMIN) ---
	beforeAssignCount := *notifyCount
	assignRec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/assign", adminToken, `{"agent_id":"`+agentID+`"}`)
	if assignRec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", assignRec.Code, assignRec.Body.String())
	}
	assigned := decodeJSON(t, assignRec.Body.Bytes())
	if assigned["status"] != "ASSIGNED" || assigned["assigned_agent_id"] != agentID {
		t.Errorf("assigned order = %v, want status ASSIGNED and assigned_agent_id %v", assigned, agentID)
	}
	if *notifyCount <= beforeAssignCount {
		t.Errorf("notification count did not increase after assignment (AGENT_ASSIGNED notification side effect missing)")
	}

	// Tracking event for the assignment carries the real admin actor,
	// never a role name.
	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", adminToken, "")
	if trackingRec.Code != http.StatusOK {
		t.Fatalf("get tracking failed: %d %s", trackingRec.Code, trackingRec.Body.String())
	}
	events := decodeJSONList(t, trackingRec.Body.Bytes())
	if len(events) != 2 || events[1]["new_status"] != "ASSIGNED" {
		t.Fatalf("tracking events = %v, want [CREATED, ASSIGNED]", events)
	}

	// --- Delivery-agent status updates, real actor id verified ---
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"} {
		before := *notifyCount
		rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", agentToken, `{"status":"`+status+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("agent transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
		event := decodeJSON(t, rec.Body.Bytes())
		if event["new_status"] != status {
			t.Errorf("event new_status = %v, want %v", event["new_status"], status)
		}
		if *notifyCount <= before {
			t.Errorf("notification count did not increase after transitioning to %s", status)
		}
	}

	finalRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, adminToken, "")
	if finalRec.Code != http.StatusOK {
		t.Fatalf("get order failed: %d %s", finalRec.Code, finalRec.Body.String())
	}
	finalOrder := decodeJSON(t, finalRec.Body.Bytes())
	if finalOrder["status"] != "DELIVERED" {
		t.Errorf("final status = %v, want DELIVERED", finalOrder["status"])
	}

	trackingRec = doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", adminToken, "")
	events = decodeJSONList(t, trackingRec.Body.Bytes())
	wantSequence := []string{"CREATED", "ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"}
	if len(events) != len(wantSequence) {
		t.Fatalf("tracking events = %v, want %d entries", events, len(wantSequence))
	}
	for i, want := range wantSequence {
		if events[i]["new_status"] != want {
			t.Errorf("event[%d].new_status = %v, want %v", i, events[i]["new_status"], want)
		}
	}
	// Every agent-performed transition's actor_id must be the agent's
	// own real user id, resolved server-side from their JWT — never a
	// role name or a client-supplied value.
	var agentUserID string
	if err := pool.QueryRow(context.Background(), `SELECT user_id FROM delivery_agents WHERE id = $1`, agentID).Scan(&agentUserID); err != nil {
		t.Fatalf("query agent user_id: %v", err)
	}
	for i := 2; i < len(events); i++ {
		if events[i]["actor_id"] != agentUserID {
			t.Errorf("event[%d].actor_id = %v, want the agent's real user id %v", i, events[i]["actor_id"], agentUserID)
		}
	}
}

// TestE2E_FailedDeliveryRescheduleReassignContinues drives a second
// full lifecycle through the failure/recovery path: assign -> drive to
// FAILED -> verify the notification side effect fired -> customer
// reschedules -> the previously assigned agent is freed -> admin
// auto-assigns -> the lifecycle continues to a second, successful
// DELIVERED.
func TestE2E_FailedDeliveryRescheduleReassignContinues(t *testing.T) {
	router, pool, notifyCount := setupE2E(t)
	adminToken := loginAsSeededAdmin(t, router)
	customerToken, _, _ := registerAndLogin(t, router, "failed-customer")
	zoneID, pickupAreaID, dropAreaID := setupZoneAreasAndRateCard(t, router, pool, adminToken, "failed")
	agentID, agentToken := createReadyAgent(t, router, pool, adminToken, zoneID, "failed")

	order := createOrder(t, router, customerToken, pickupAreaID, dropAreaID)
	orderID := order["id"].(string)

	if rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/assign", adminToken, `{"agent_id":"`+agentID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY"} {
		if rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", agentToken, `{"status":"`+status+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}

	beforeFailedCount := *notifyCount
	failRec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", agentToken, `{"status":"FAILED","metadata":{"failure_reason":"Recipient not available"}}`)
	if failRec.Code != http.StatusCreated {
		t.Fatalf("transition to FAILED failed: %d %s", failRec.Code, failRec.Body.String())
	}
	if *notifyCount <= beforeFailedCount {
		t.Errorf("notification count did not increase after FAILED (notification side effect missing)")
	}

	orderRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, adminToken, "")
	if decodeJSON(t, orderRec.Body.Bytes())["status"] != "FAILED" {
		t.Fatalf("order status = %v, want FAILED", decodeJSON(t, orderRec.Body.Bytes())["status"])
	}

	// --- Customer reschedules ---
	rescheduleRec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/reschedule", customerToken, `{"requested_date":"2099-01-01","reason":"Try again later"}`)
	if rescheduleRec.Code != http.StatusOK {
		t.Fatalf("reschedule failed: %d %s", rescheduleRec.Code, rescheduleRec.Body.String())
	}
	rescheduled := decodeJSON(t, rescheduleRec.Body.Bytes())
	if rescheduled["status"] != "RESCHEDULED" {
		t.Errorf("status = %v, want RESCHEDULED", rescheduled["status"])
	}

	// The previously assigned agent must now be freed back to AVAILABLE.
	var availability string
	if err := pool.QueryRow(context.Background(), `SELECT availability FROM delivery_agents WHERE id = $1`, agentID).Scan(&availability); err != nil {
		t.Fatalf("query agent availability: %v", err)
	}
	if availability != "AVAILABLE" {
		t.Errorf("agent availability = %q, want AVAILABLE (freed by the reschedule transaction)", availability)
	}

	// --- Admin auto-assigns for the second attempt ---
	autoAssignRec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/auto-assign", adminToken, "")
	if autoAssignRec.Code != http.StatusOK {
		t.Fatalf("auto-assign failed: %d %s", autoAssignRec.Code, autoAssignRec.Body.String())
	}
	reassigned := decodeJSON(t, autoAssignRec.Body.Bytes())
	if reassigned["status"] != "ASSIGNED" || reassigned["assigned_agent_id"] != agentID {
		t.Errorf("reassigned order = %v, want status ASSIGNED and assigned_agent_id %v (the only, now-freed eligible agent)", reassigned, agentID)
	}

	// --- Second lifecycle continues all the way to DELIVERED ---
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"} {
		if rec := doJSON(router, http.MethodPost, "/api/v1/orders/"+orderID+"/status", agentToken, `{"status":"`+status+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("second-cycle transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}

	finalRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID, adminToken, "")
	if decodeJSON(t, finalRec.Body.Bytes())["status"] != "DELIVERED" {
		t.Errorf("final status = %v, want DELIVERED (second lifecycle completed)", decodeJSON(t, finalRec.Body.Bytes())["status"])
	}

	trackingRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", adminToken, "")
	events := decodeJSONList(t, trackingRec.Body.Bytes())
	wantSequence := []string{"CREATED", "ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED", "RESCHEDULED", "ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"}
	if len(events) != len(wantSequence) {
		t.Fatalf("tracking history = %v, want %d entries", events, len(wantSequence))
	}
	for i, want := range wantSequence {
		if events[i]["new_status"] != want {
			t.Errorf("event[%d].new_status = %v, want %v", i, events[i]["new_status"], want)
		}
	}

	rescheduleHistoryRec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/reschedules", adminToken, "")
	history := decodeJSONList(t, rescheduleHistoryRec.Body.Bytes())
	if len(history) != 1 || history[0]["requested_date"] != "2099-01-01" {
		t.Errorf("reschedule history = %v, want exactly 1 record for 2099-01-01", history)
	}
}
