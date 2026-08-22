//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/assignment"
	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/notifications"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/rescheduling"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// --- fake providers (integration-test-local) ---
//
// Real Postgres, real notifications.PostgresRepository, real
// notifications.Service, wired into the real orders/tracking/assignment/
// rescheduling hooks exactly as main.go wires them — but a FAKE
// EmailProvider/SmsProvider, so these tests never attempt a real send and
// can assert on exactly what was attempted.

type recordedMessage struct {
	To      string
	Subject string
	Body    string
}

type fakeIntegrationEmailProvider struct {
	mu      sync.Mutex
	sent    []recordedMessage
	callsTo map[string]int
	failFor map[string]bool
}

func newFakeIntegrationEmailProvider() *fakeIntegrationEmailProvider {
	return &fakeIntegrationEmailProvider{callsTo: map[string]int{}, failFor: map[string]bool{}}
}

func (f *fakeIntegrationEmailProvider) SendEmail(_ context.Context, to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callsTo[to]++
	if f.failFor[to] {
		return errors.New("simulated email provider failure")
	}
	f.sent = append(f.sent, recordedMessage{To: to, Subject: subject, Body: body})
	return nil
}

func (f *fakeIntegrationEmailProvider) messagesTo(to string) []recordedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedMessage
	for _, m := range f.sent {
		if m.To == to {
			out = append(out, m)
		}
	}
	return out
}

func (f *fakeIntegrationEmailProvider) callCount(to string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callsTo[to]
}

func (f *fakeIntegrationEmailProvider) setFailFor(to string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failFor[to] = true
}

type fakeIntegrationSmsProvider struct {
	mu   sync.Mutex
	sent []recordedMessage
}

func (f *fakeIntegrationSmsProvider) SendSMS(_ context.Context, to, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, recordedMessage{To: to, Body: body})
	return nil
}

func (f *fakeIntegrationSmsProvider) messagesTo(to string) []recordedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedMessage
	for _, m := range f.sent {
		if m.To == to {
			out = append(out, m)
		}
	}
	return out
}

// --- setup ---

// setupNotificationsTest builds the exact same router main.go builds —
// real Postgres, real migrations, real middleware, every module's Mount
// including the M11 hooks — except EmailProvider/SmsProvider are fakes,
// so these tests exercise the full M04 -> M11 stack end to end without
// ever attempting a real external send.
func setupNotificationsTest(t *testing.T) (router http.Handler, usersRepo users.Repository, zonesRepo zones.Repository, agentsRepo agents.Repository, pool *pgxpool.Pool, nService *notifications.Service, emailProvider *fakeIntegrationEmailProvider, smsProvider *fakeIntegrationSmsProvider) {
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

	emailP := newFakeIntegrationEmailProvider()
	smsP := &fakeIntegrationSmsProvider{}
	nRepo := notifications.NewPostgresRepository(p)
	nSvc := notifications.NewService(nRepo, oRepo, uRepo, tRepo, emailP, smsP)

	asRepo := assignment.NewPostgresRepository(p, aRepo, oRepo, tRepo, nSvc.NotifyTransition)
	rsRepo := rescheduling.NewPostgresRepository(p, tRepo, nSvc.NotifyTransition)

	r := server.NewRouter(p, testLogger(),
		auth.Mount(uRepo, agentsIntegrationJWTSecret),
		zones.Mount(zRepo, agentsIntegrationJWTSecret),
		rates.Mount(rRepo, zRepo, agentsIntegrationJWTSecret),
		orders.Mount(oRepo, uRepo, zRepo, rRepo, aRepo, agentsIntegrationJWTSecret, nSvc.NotifyOrderCreated),
		tracking.Mount(tRepo, agentsIntegrationJWTSecret, nSvc.NotifyTransition),
		agents.Mount(aRepo, agentsIntegrationJWTSecret),
		assignment.Mount(asRepo, agentsIntegrationJWTSecret),
		rescheduling.Mount(rsRepo, oRepo, agentsIntegrationJWTSecret),
	)
	return r, uRepo, zRepo, aRepo, p, nSvc, emailP, smsP
}

// customerTokenWithPhone seeds a customer with a real phone number on
// file — the plain customerToken()/userToken() helpers never set one, so
// this is the only way to exercise the SMS-attempted path.
func customerTokenWithPhone(t *testing.T, uRepo users.Repository, phone string) (token, customerID, email string) {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	email = uniqueEmail("notif-customer")
	created, err := uRepo.Create(context.Background(), users.NewUser{
		Email: email, PasswordHash: hash, FullName: "Notif Customer", Role: users.RoleCustomer, Phone: &phone,
	})
	if err != nil {
		t.Fatalf("seed user create failed: %v", err)
	}
	tok, err := auth.GenerateToken(agentsIntegrationJWTSecret, created.ID, users.RoleCustomer, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	return tok, created.ID, email
}

// --- direct DB inspection helpers ---

type notificationRow struct {
	ID              string
	TrackingEventID string
	OrderID         string
	Event           string
	Channel         string
	Recipient       string
	Status          string
}

func queryNotifications(t *testing.T, pool *pgxpool.Pool, orderID string) []notificationRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id, tracking_event_id, order_id, event, channel, recipient, status FROM notifications WHERE order_id = $1 ORDER BY created_at`,
		orderID)
	if err != nil {
		t.Fatalf("query notifications: %v", err)
	}
	defer rows.Close()
	var out []notificationRow
	for rows.Next() {
		var n notificationRow
		if err := rows.Scan(&n.ID, &n.TrackingEventID, &n.OrderID, &n.Event, &n.Channel, &n.Recipient, &n.Status); err != nil {
			t.Fatalf("scan notification row: %v", err)
		}
		out = append(out, n)
	}
	return out
}

func notificationsForEvent(rows []notificationRow, event string) []notificationRow {
	var out []notificationRow
	for _, r := range rows {
		if r.Event == event {
			out = append(out, r)
		}
	}
	return out
}

func notificationsForChannel(rows []notificationRow, channel string) []notificationRow {
	var out []notificationRow
	for _, r := range rows {
		if r.Channel == channel {
			out = append(out, r)
		}
	}
	return out
}

func lastTrackingEventID(t *testing.T, router http.Handler, token, orderID string) string {
	t.Helper()
	rec := doJSON(router, http.MethodGet, "/api/v1/orders/"+orderID+"/tracking", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get tracking failed: %d %s", rec.Code, rec.Body.String())
	}
	events := decodeEventList(t, rec.Body.Bytes())
	last := events[len(events)-1]
	id, _ := last["id"].(string)
	if id == "" {
		t.Fatalf("last tracking event has no id: %v", last)
	}
	return id
}

// --- ORDER_CREATED ---

func TestNotificationFlow_OrderCreatedNotifiesCustomerByEmail(t *testing.T) {
	router, uRepo, zRepo, _, pool, _, emailP, smsP := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	customerEmail := ""
	{
		claims, err := auth.ParseToken(agentsIntegrationJWTSecret, customer)
		if err != nil {
			t.Fatalf("ParseToken() error: %v", err)
		}
		u, err := uRepo.FindByID(context.Background(), claims.Subject)
		if err != nil {
			t.Fatalf("FindByID() error: %v", err)
		}
		customerEmail = u.Email
	}

	rows := queryNotifications(t, pool, orderID)
	created := notificationsForEvent(rows, "ORDER_CREATED")
	if len(created) != 1 {
		t.Fatalf("ORDER_CREATED notifications = %v, want exactly 1 (EMAIL only, no phone on file)", created)
	}
	if created[0].Channel != "EMAIL" || created[0].Recipient != customerEmail || created[0].Status != "SENT" {
		t.Errorf("row = %+v, want EMAIL/%s/SENT", created[0], customerEmail)
	}
	trackingEventID := lastTrackingEventID(t, router, admin, orderID)
	if created[0].TrackingEventID != trackingEventID {
		t.Errorf("tracking_event_id = %v, want the real initial CREATED event id %v", created[0].TrackingEventID, trackingEventID)
	}
	if msgs := emailP.messagesTo(customerEmail); len(msgs) != 1 {
		t.Errorf("provider messages to %s = %v, want exactly 1", customerEmail, msgs)
	}
	if msgs := smsP.messagesTo(customerEmail); len(msgs) != 0 {
		t.Errorf("no phone on file, want zero SMS attempts, got %v", msgs)
	}
}

// --- SMS gating ---

func TestNotificationFlow_SmsAttemptedWhenCustomerHasPhone(t *testing.T) {
	router, uRepo, zRepo, _, pool, _, _, smsP := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer, _, _ := customerTokenWithPhone(t, uRepo, "+15551234567")

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rows := queryNotifications(t, pool, orderID)
	sms := notificationsForChannel(rows, "SMS")
	if len(sms) != 1 {
		t.Fatalf("SMS notifications = %v, want exactly 1 (phone on file)", sms)
	}
	if sms[0].Recipient != "+15551234567" || sms[0].Status != "SENT" {
		t.Errorf("row = %+v, want recipient +15551234567, status SENT", sms[0])
	}
	if msgs := smsP.messagesTo("+15551234567"); len(msgs) != 1 {
		t.Errorf("provider SMS messages = %v, want exactly 1", msgs)
	}
}

func TestNotificationFlow_NoSmsRowWhenPhoneAbsent(t *testing.T) {
	router, uRepo, zRepo, _, pool, _, _, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo) // no phone set

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	rows := queryNotifications(t, pool, orderID)
	if sms := notificationsForChannel(rows, "SMS"); len(sms) != 0 {
		t.Errorf("SMS notifications = %v, want zero (no phone on file)", sms)
	}
}

// --- Full lifecycle: every one of the 8 events notifies ---

func TestNotificationFlow_FullLifecycleEveryEventNotifies(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool, _, _, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)
	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	if rec := assignOrder(router, admin, orderID, agent.ID); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"} {
		if rec := transitionStatus(router, agentToken, orderID, status); rec.Code != http.StatusCreated {
			t.Fatalf("transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}

	rows := queryNotifications(t, pool, orderID)
	wantEvents := []string{"ORDER_CREATED", "AGENT_ASSIGNED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"}
	for _, event := range wantEvents {
		matches := notificationsForEvent(rows, event)
		if len(matches) != 1 {
			t.Errorf("%s notifications = %v, want exactly 1 EMAIL row", event, matches)
			continue
		}
		if matches[0].Channel != "EMAIL" || matches[0].Status != "SENT" {
			t.Errorf("%s row = %+v, want EMAIL/SENT", event, matches[0])
		}
		if matches[0].TrackingEventID == "" {
			t.Errorf("%s row has empty tracking_event_id", event)
		}
	}
	// Every row's tracking_event_id must be distinct (one real
	// order_tracking_events occurrence per event, never shared).
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.TrackingEventID] && r.Channel == "EMAIL" {
			t.Errorf("tracking_event_id %v reused across two EMAIL rows", r.TrackingEventID)
		}
		seen[r.TrackingEventID] = true
	}
}

func TestNotificationFlow_FailedAndRescheduledEventsNotify(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool, _, emailP, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
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
	if rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01","reason":"Not home"}`); rec.Code != http.StatusOK {
		t.Fatalf("reschedule failed: %d %s", rec.Code, rec.Body.String())
	}

	rows := queryNotifications(t, pool, orderID)
	if failed := notificationsForEvent(rows, "FAILED"); len(failed) != 1 {
		t.Errorf("FAILED notifications = %v, want exactly 1", failed)
	}
	if resched := notificationsForEvent(rows, "RESCHEDULED"); len(resched) != 1 {
		t.Errorf("RESCHEDULED notifications = %v, want exactly 1", resched)
	} else if resched[0].Status != "SENT" {
		t.Errorf("RESCHEDULED row = %+v, want SENT", resched[0])
	}

	claims, err := auth.ParseToken(agentsIntegrationJWTSecret, customer)
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	customerUser, err := uRepo.FindByID(context.Background(), claims.Subject)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	msgs := emailP.messagesTo(customerUser.Email)
	foundRescheduleContent := false
	for _, m := range msgs {
		if m.Body != "" && (containsAll(m.Body, "2099-01-01") && containsAll(m.Body, "Not home")) {
			foundRescheduleContent = true
		}
	}
	if !foundRescheduleContent {
		t.Errorf("no recorded email message included the reschedule date/reason; messages: %v", msgs)
	}
}

func containsAll(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

// --- Second occurrence of the same event type is independently notified ---

func TestNotificationFlow_SecondFailedOccurrenceCreatesNewRow(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool, _, _, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	agentToken := deliveryAgentToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
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
	if rec := rescheduleOrder(router, customer, orderID, `{"requested_date":"2099-01-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("reschedule failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := assignOrder(router, admin, orderID, agent.ID); rec.Code != http.StatusOK {
		t.Fatalf("reassign failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, status := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "FAILED"} {
		if rec := transitionStatus(router, agentToken, orderID, status); rec.Code != http.StatusCreated {
			t.Fatalf("second-cycle transition to %s failed: %d %s", status, rec.Code, rec.Body.String())
		}
	}

	rows := queryNotifications(t, pool, orderID)
	failed := notificationsForEvent(rows, "FAILED")
	if len(failed) != 2 {
		t.Fatalf("FAILED notifications = %v, want exactly 2 (each occurrence independently notify-able)", failed)
	}
	if failed[0].TrackingEventID == failed[1].TrackingEventID {
		t.Errorf("both FAILED rows share tracking_event_id %v, want two distinct occurrences", failed[0].TrackingEventID)
	}
	if failed[0].ID == failed[1].ID {
		t.Errorf("both FAILED rows share the same notification id")
	}
}

// --- Provider failure must not break the underlying lifecycle commit ---

func TestNotificationFlow_ProviderFailureDoesNotBreakLifecycleCommit(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool, _, emailP, smsP := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer, _, customerEmail := customerTokenWithPhone(t, uRepo, "+15559998888")
	agentToken := deliveryAgentToken(t, uRepo)
	emailP.setFailFor(customerEmail)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)
	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)

	rec := assignOrder(router, admin, orderID, agent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign status = %d, want %d despite a forced provider failure — the lifecycle commit must never depend on notification success, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := decodeOrder(t, rec.Body.Bytes())
	if updated["status"] != "ASSIGNED" {
		t.Errorf("status = %v, want ASSIGNED (assignment must commit regardless of notification outcome)", updated["status"])
	}
	if rec := transitionStatus(router, agentToken, orderID, "PICKED_UP"); rec.Code != http.StatusCreated {
		t.Fatalf("transition status = %d, want %d despite forced provider failure, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rows := queryNotifications(t, pool, orderID)
	assignedEmail := notificationsForChannel(notificationsForEvent(rows, "AGENT_ASSIGNED"), "EMAIL")
	if len(assignedEmail) != 1 || assignedEmail[0].Status != "FAILED" {
		t.Errorf("AGENT_ASSIGNED EMAIL notification = %v, want exactly 1 with status FAILED", assignedEmail)
	}
	// SMS is a different channel entirely and must be unaffected by the
	// EMAIL failure.
	if smsRows := notificationsForChannel(notificationsForEvent(rows, "AGENT_ASSIGNED"), "SMS"); len(smsRows) != 1 || smsRows[0].Status != "SENT" {
		t.Errorf("AGENT_ASSIGNED SMS row = %v, want exactly 1 with status SENT (unaffected by the EMAIL failure)", smsRows)
	}
	// The provider recorded 3 SMS messages total by this point (one per
	// event fired so far: ORDER_CREATED, AGENT_ASSIGNED, PICKED_UP) —
	// exactly one of them must be the AGENT_ASSIGNED message, since SMS
	// is a separate, unaffected channel from the forced EMAIL failure.
	msgs := smsP.messagesTo("+15559998888")
	assignedSmsCount := 0
	for _, m := range msgs {
		if containsAll(m.Body, "AGENT_ASSIGNED") {
			assignedSmsCount++
		}
	}
	if assignedSmsCount != 1 {
		t.Errorf("AGENT_ASSIGNED SMS provider messages = %d, want exactly 1; all messages: %v", assignedSmsCount, msgs)
	}
}

// --- Duplicate prevention at the service level ---

func TestNotificationFlow_RepeatedNotifyTransitionCallForSameEventClaimsOnce(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool, nSvc, emailP, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)
	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	if rec := assignOrder(router, admin, orderID, agent.ID); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}

	claims, err := auth.ParseToken(agentsIntegrationJWTSecret, customer)
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	customerUser, err := uRepo.FindByID(context.Background(), claims.Subject)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}

	trackingRepo := tracking.NewPostgresRepository(pool)
	events, err := trackingRepo.ListEvents(context.Background(), orderID)
	if err != nil {
		t.Fatalf("ListEvents() error: %v", err)
	}
	assignedEvent := events[len(events)-1]

	before := emailP.callCount(customerUser.Email)
	// The hook already fired this exact event once (via the real HTTP
	// assign call above); invoke the same handler again directly with the
	// identical Event — this must not attempt a second send.
	nSvc.NotifyTransition(context.Background(), assignedEvent)
	nSvc.NotifyTransition(context.Background(), assignedEvent)
	after := emailP.callCount(customerUser.Email)

	if after != before {
		t.Errorf("email provider call count changed from %d to %d after 2 repeated calls for the same tracking event — must stay unchanged (claim-before-send idempotency)", before, after)
	}

	rows := queryNotifications(t, pool, orderID)
	assigned := notificationsForEvent(rows, "AGENT_ASSIGNED")
	if len(assigned) != 1 {
		t.Errorf("AGENT_ASSIGNED notification rows = %v, want exactly 1 despite 3 total NotifyTransition calls for the same event", assigned)
	}
}

// --- Concurrency: real-Postgres proof no duplicate send is possible ---

// TestNotificationConcurrency_ConcurrentIdenticalAttemptsClaimExactlyOnce
// fires many concurrent NotifyTransition calls for the exact same
// tracking.Event (as could happen if a caller ever invoked the hook
// more than once for the same occurrence) and proves the database's own
// unique index — not application-level locking — is what guarantees at
// most one claim, and therefore at most one provider send, ever
// succeeds. Repeat via `go test -count=N` to rule out flakiness.
func TestNotificationConcurrency_ConcurrentIdenticalAttemptsClaimExactlyOnce(t *testing.T) {
	router, uRepo, zRepo, aRepo, pool, nSvc, emailP, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)

	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)
	pickupZoneID, _ := order["pickup_zone_id"].(string)
	agent := seedAgent(t, aRepo, pool, pickupZoneID, true, agents.AvailabilityAvailable)
	if rec := assignOrder(router, admin, orderID, agent.ID); rec.Code != http.StatusOK {
		t.Fatalf("assign failed: %d %s", rec.Code, rec.Body.String())
	}

	claims, err := auth.ParseToken(agentsIntegrationJWTSecret, customer)
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	customerUser, err := uRepo.FindByID(context.Background(), claims.Subject)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}

	trackingRepo := tracking.NewPostgresRepository(pool)
	events, err := trackingRepo.ListEvents(context.Background(), orderID)
	if err != nil {
		t.Fatalf("ListEvents() error: %v", err)
	}
	assignedEvent := events[len(events)-1]
	before := emailP.callCount(customerUser.Email)

	const attempts = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			nSvc.NotifyTransition(context.Background(), assignedEvent)
		}()
	}
	close(start)
	wg.Wait()

	after := emailP.callCount(customerUser.Email)
	if after != before {
		t.Errorf("email provider call count changed from %d to %d after %d concurrent identical NotifyTransition calls — want unchanged (only the original HTTP-triggered send)", before, after, attempts)
	}

	rows := queryNotifications(t, pool, orderID)
	assigned := notificationsForEvent(rows, "AGENT_ASSIGNED")
	if len(assigned) != 1 {
		t.Errorf("AGENT_ASSIGNED notification rows = %v, want exactly 1 despite %d concurrent attempts", assigned, attempts)
	}

	var emailCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE tracking_event_id = $1 AND channel = 'EMAIL'`, assignedEvent.ID,
	).Scan(&emailCount); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if emailCount != 1 {
		t.Errorf("EMAIL rows for tracking_event_id %v = %d, want exactly 1 (unique index backstop)", assignedEvent.ID, emailCount)
	}
}

// --- Schema / migration ---

func TestNotificationsTable_SchemaAndUniqueConstraint(t *testing.T) {
	_, _, _, _, pool, _, _, _ := setupNotificationsTest(t)

	var appliedMigration string
	err := pool.QueryRow(context.Background(),
		`SELECT version FROM schema_migrations WHERE version LIKE '0012%'`,
	).Scan(&appliedMigration)
	if err != nil {
		t.Fatalf("migration 0012 not recorded as applied: %v", err)
	}

	var indexDef string
	err = pool.QueryRow(context.Background(),
		`SELECT indexdef FROM pg_indexes WHERE tablename = 'notifications' AND indexname = 'idx_notifications_tracking_event_channel'`,
	).Scan(&indexDef)
	if err != nil {
		t.Fatalf("expected unique index idx_notifications_tracking_event_channel to exist: %v", err)
	}
	if !containsAll(indexDef, "UNIQUE") {
		t.Errorf("indexdef = %q, want it to be UNIQUE", indexDef)
	}
}

// --- No REST API surface ---

func TestNotifications_NoPublicEndpointsExist(t *testing.T) {
	router, uRepo, zRepo, _, pool, _, _, _ := setupNotificationsTest(t)
	admin := adminToken(t, uRepo)
	customer := customerToken(t, uRepo)
	order := createTestOrder(t, router, zRepo, pool, admin, customer, "B2C", "INTRA")
	orderID, _ := order["id"].(string)

	paths := []string{
		"/api/v1/notifications",
		"/api/v1/orders/" + orderID + "/notifications",
	}
	for _, p := range paths {
		rec := doJSON(router, http.MethodGet, p, admin, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d (M11 intentionally adds no REST API)", p, rec.Code, http.StatusNotFound)
		}
	}
}
