//go:build e2e

// Package e2e implements M12's full-stack HTTP flow tests — the same
// two flows tests/e2e/doc.go always named (register/quote/order/assign/
// status-updates/delivered, and the failed/reschedule/reassign variant).
// This package builds the exact router main.go builds (every module
// mounted, log-based notification providers wired in exactly as
// production does) and drives it entirely through real HTTP requests —
// no direct repository seeding for anything an HTTP endpoint can do
// (zones/areas/rate cards/orders/agents/registration all go through
// their real handlers here, unlike tests/integration's lighter-weight
// per-module setup helpers). The one unavoidable exception is
// delivery_agents.current_zone_id, which — as every other test suite in
// this project already documents — no application code path writes at
// all; it is set directly via SQL, the same documented exception
// tests/integration/assignment_integration_test.go's own seedAgent
// helper uses.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
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

const e2eJWTSecret = "e2e-test-secret"

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupE2E builds the exact same router, module wiring, and demo-user
// seed main.go builds at real startup — the only difference is the JWT
// secret (test-local) and using log-based notification providers with a
// call counter so a test can assert a notification was actually
// attempted, without depending on parsing log output.
func setupE2E(t *testing.T) (router chi.Router, pool *pgxpool.Pool, notifyCount *int) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping e2e test")
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

	usersRepo := users.NewPostgresRepository(p)
	agentsRepo := agents.NewPostgresRepository(p)
	zonesRepo := zones.NewPostgresRepository(p)
	ratesRepo := rates.NewPostgresRepository(p)
	ordersRepo := orders.NewPostgresRepository(p)
	trackingRepo := tracking.NewPostgresRepository(p)

	count := 0
	notificationsRepo := notifications.NewPostgresRepository(p)
	notificationsService := notifications.NewService(
		notificationsRepo, ordersRepo, usersRepo, trackingRepo,
		countingEmailProvider{count: &count}, notifications.NewLogSmsProvider(),
	)

	assignmentRepo := assignment.NewPostgresRepository(p, agentsRepo, ordersRepo, trackingRepo, notificationsService.NotifyTransition)
	reschedulingRepo := rescheduling.NewPostgresRepository(p, trackingRepo, notificationsService.NotifyTransition)

	logger := testLogger()
	if err := auth.SeedDemoUsers(ctx, usersRepo, logger); err != nil {
		t.Fatalf("SeedDemoUsers() failed: %v", err)
	}
	demoAgentUser, err := usersRepo.FindByEmail(ctx, auth.SeedAgentEmail)
	if err != nil {
		t.Fatalf("FindByEmail(demo agent) failed: %v", err)
	}
	if err := agentsRepo.EnsureDemoAgentRecord(ctx, demoAgentUser.ID, logger); err != nil {
		t.Fatalf("EnsureDemoAgentRecord() failed: %v", err)
	}

	r := server.NewRouter(p, logger,
		auth.Mount(usersRepo, e2eJWTSecret),
		agents.Mount(agentsRepo, zonesRepo, e2eJWTSecret),
		zones.Mount(zonesRepo, e2eJWTSecret),
		rates.Mount(ratesRepo, zonesRepo, e2eJWTSecret),
		orders.Mount(ordersRepo, usersRepo, zonesRepo, ratesRepo, agentsRepo, e2eJWTSecret, notificationsService.NotifyOrderCreated),
		tracking.Mount(trackingRepo, e2eJWTSecret, notificationsService.NotifyTransition),
		assignment.Mount(assignmentRepo, e2eJWTSecret),
		rescheduling.Mount(reschedulingRepo, ordersRepo, e2eJWTSecret),
	)
	return r, p, &count
}

// countingEmailProvider is the log-based provider (same content and
// behavior as notifications.LogEmailProvider) plus a shared counter, so
// a test can assert "a notification was attempted" without depending on
// captured log output.
type countingEmailProvider struct {
	count *int
}

func (c countingEmailProvider) SendEmail(_ context.Context, to, subject, body, htmlBody string) error {
	*c.count++
	slog.Info("email notification", "to", to, "subject", subject, "body", body, "html_included", htmlBody != "")
	return nil
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

func decodeJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode JSON response: %v, body: %s", err, body)
	}
	return out
}

func decodeJSONList(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode JSON list response: %v, body: %s", err, body)
	}
	return out
}

// uniqueEmail must stay unique across process runs, not just within
// one — Postgres here is a single shared, never-reset instance (the
// same convention every prior milestone's suite documents), so a
// counter that resets to 0 each run would collide with a previous run's
// already-persisted row. time.Now().UnixNano() is the same fix
// tests/integration/auth_integration_test.go's own uniqueEmail already
// established.
func uniqueEmail(label string) string {
	return fmt.Sprintf("e2e-%s-%d@example.com", label, time.Now().UnixNano())
}

// registerAndLogin drives the real POST /auth/register + POST
// /auth/login handlers — every e2e CUSTOMER account is created this
// way, never seeded directly into the repository.
func registerAndLogin(t *testing.T, router http.Handler, label string) (token, userID, email string) {
	t.Helper()
	email = uniqueEmail(label)
	body := fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"E2E %s"}`, email, label)
	rec := doJSON(router, http.MethodPost, "/api/v1/auth/register", "", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
	user := decodeJSON(t, rec.Body.Bytes())
	userID = user["id"].(string)

	loginRec := doJSON(router, http.MethodPost, "/api/v1/auth/login", "", fmt.Sprintf(`{"email":%q,"password":"password123"}`, email))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}
	token = decodeJSON(t, loginRec.Body.Bytes())["token"].(string)
	return token, userID, email
}

func loginAsSeededAdmin(t *testing.T, router http.Handler) string {
	t.Helper()
	rec := doJSON(router, http.MethodPost, "/api/v1/auth/login", "", `{"email":"admin@lastmile.test","password":"Admin123!"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login failed: %d %s", rec.Code, rec.Body.String())
	}
	return decodeJSON(t, rec.Body.Bytes())["token"].(string)
}

// setupZoneAreasAndRateCard drives POST /zones, POST /zones/{id}/areas
// (pickup + drop), POST /rates, PUT /rates/{id} (activate), and POST
// /rates/{id}/slabs entirely through their real HTTP handlers — the
// full admin configuration flow an evaluator would actually perform,
// not a direct repository shortcut.
func setupZoneAreasAndRateCard(t *testing.T, router http.Handler, pool *pgxpool.Pool, adminToken, label string) (zoneID, pickupAreaID, dropAreaID string) {
	t.Helper()

	zoneRec := doJSON(router, http.MethodPost, "/api/v1/zones", adminToken, fmt.Sprintf(`{"name":%q}`, uniqueEmail(label+"-zone")))
	if zoneRec.Code != http.StatusCreated {
		t.Fatalf("create zone failed: %d %s", zoneRec.Code, zoneRec.Body.String())
	}
	zone := decodeJSON(t, zoneRec.Body.Bytes())
	zoneID = zone["id"].(string)

	pickupRec := doJSON(router, http.MethodPost, "/api/v1/zones/"+zoneID+"/areas", adminToken, `{"name":"Pickup"}`)
	if pickupRec.Code != http.StatusCreated {
		t.Fatalf("create pickup area failed: %d %s", pickupRec.Code, pickupRec.Body.String())
	}
	pickupAreaID = decodeJSON(t, pickupRec.Body.Bytes())["id"].(string)

	dropRec := doJSON(router, http.MethodPost, "/api/v1/zones/"+zoneID+"/areas", adminToken, `{"name":"Drop"}`)
	if dropRec.Code != http.StatusCreated {
		t.Fatalf("create drop area failed: %d %s", dropRec.Code, dropRec.Body.String())
	}
	dropAreaID = decodeJSON(t, dropRec.Body.Bytes())["id"].(string)

	// Postgres in these tests is a single shared, never-reset instance
	// (the same convention every prior milestone's suite documents) —
	// an earlier run may have left an active B2C/INTRA card behind, and
	// only one active card per (order_type, zone_relationship) is ever
	// allowed. Deactivating any existing one first (the same
	// resetCombination pattern tests/integration/quote_integration_test.go
	// already established) is what makes this test's own pricing
	// assertions deterministic regardless of test run history.
	if _, err := pool.Exec(context.Background(), `UPDATE rate_cards SET active = false WHERE order_type = 'B2C' AND zone_relationship = 'INTRA'`); err != nil {
		t.Fatalf("reset active rate card precondition failed: %v", err)
	}

	cardRec := doJSON(router, http.MethodPost, "/api/v1/rates", adminToken, `{"order_type":"B2C","zone_relationship":"INTRA","cod_surcharge":0}`)
	if cardRec.Code != http.StatusCreated {
		t.Fatalf("create rate card failed: %d %s", cardRec.Code, cardRec.Body.String())
	}
	cardID := decodeJSON(t, cardRec.Body.Bytes())["id"].(string)

	activateRec := doJSON(router, http.MethodPut, "/api/v1/rates/"+cardID, adminToken, `{"cod_surcharge":0,"active":true}`)
	if activateRec.Code != http.StatusOK {
		t.Fatalf("activate rate card failed: %d %s", activateRec.Code, activateRec.Body.String())
	}

	slabRec := doJSON(router, http.MethodPost, "/api/v1/rates/"+cardID+"/slabs", adminToken, `{"min_weight":0,"price":50}`)
	if slabRec.Code != http.StatusCreated {
		t.Fatalf("create slab failed: %d %s", slabRec.Code, slabRec.Body.String())
	}

	return zoneID, pickupAreaID, dropAreaID
}

// createReadyAgent provisions a fresh delivery agent via the real POST
// /agents handler, then seeds only current_zone_id/availability/active
// directly via SQL — the one documented exception every prior
// milestone's test suite already relies on, since no HTTP endpoint in
// this application ever writes delivery_agents.current_zone_id.
func createReadyAgent(t *testing.T, router http.Handler, pool *pgxpool.Pool, adminToken, zoneID, label string) (agentID, agentToken string) {
	t.Helper()
	email := uniqueEmail(label + "-agent")
	createRec := doJSON(router, http.MethodPost, "/api/v1/agents", adminToken, fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"E2E Agent"}`, email))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create agent failed: %d %s", createRec.Code, createRec.Body.String())
	}
	agent := decodeJSON(t, createRec.Body.Bytes())
	agentID = agent["id"].(string)
	agentUserID := agent["user_id"].(string)

	if _, err := pool.Exec(context.Background(),
		`UPDATE delivery_agents SET active = true, availability = 'AVAILABLE', current_zone_id = $1 WHERE id = $2`,
		zoneID, agentID); err != nil {
		t.Fatalf("seed agent zone/availability failed: %v", err)
	}

	loginRec := doJSON(router, http.MethodPost, "/api/v1/auth/login", "", fmt.Sprintf(`{"email":%q,"password":"password123"}`, email))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("agent login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}
	agentToken = decodeJSON(t, loginRec.Body.Bytes())["token"].(string)
	_ = agentUserID
	return agentID, agentToken
}

func createOrder(t *testing.T, router http.Handler, customerToken, pickupAreaID, dropAreaID string) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"pickup_area_id":%q,"drop_area_id":%q,"order_type":"B2C","payment_type":"PREPAID","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":2}`,
		pickupAreaID, dropAreaID)
	rec := doJSON(router, http.MethodPost, "/api/v1/orders", customerToken, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order failed: %d %s", rec.Code, rec.Body.String())
	}
	return decodeJSON(t, rec.Body.Bytes())
}
