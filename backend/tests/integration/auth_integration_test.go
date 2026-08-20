//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/database"
	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

const integrationJWTSecret = "integration-test-secret"

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// setupAuthTest builds the exact same router main.go builds (real
// Postgres, real migrations, real middleware, real handlers) via
// auth.Mount — the same function production uses — so these tests
// exercise the full stack, not a hand-assembled substitute.
func setupAuthTest(t *testing.T) (router http.Handler, repo users.Repository) {
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

	usersRepo := users.NewPostgresRepository(pool)
	r := server.NewRouter(pool, testLogger(), auth.Mount(usersRepo, integrationJWTSecret))
	return r, usersRepo
}

// uniqueEmail avoids collisions when this test suite runs repeatedly
// against a persistent development database.
func uniqueEmail(label string) string {
	return fmt.Sprintf("%s-%d@integration.test", label, time.Now().UnixNano())
}

func TestAuthFlow_RegisterThenLoginThenMe(t *testing.T) {
	router, _ := setupAuthTest(t)
	email := uniqueEmail("flow")

	// 1. Register.
	registerBody := fmt.Sprintf(`{"email":%q,"password":"password123","full_name":"Integration Test User"}`, email)
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(registerBody)))

	if registerRec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d, body: %s", registerRec.Code, http.StatusCreated, registerRec.Body.String())
	}
	var registered map[string]any
	if err := json.NewDecoder(registerRec.Body).Decode(&registered); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if registered["role"] != "CUSTOMER" {
		t.Errorf("registered role = %v, want CUSTOMER", registered["role"])
	}
	if _, present := registered["password_hash"]; present {
		t.Error("register response leaked password_hash")
	}

	// 2. Duplicate registration is rejected.
	dupRec := httptest.NewRecorder()
	router.ServeHTTP(dupRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(registerBody)))
	if dupRec.Code != http.StatusConflict {
		t.Errorf("duplicate register status = %d, want %d", dupRec.Code, http.StatusConflict)
	}

	// 3. Login with the wrong password fails.
	wrongLoginBody := fmt.Sprintf(`{"email":%q,"password":"wrong-password"}`, email)
	wrongLoginRec := httptest.NewRecorder()
	router.ServeHTTP(wrongLoginRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(wrongLoginBody)))
	if wrongLoginRec.Code != http.StatusUnauthorized {
		t.Errorf("wrong-password login status = %d, want %d", wrongLoginRec.Code, http.StatusUnauthorized)
	}

	// 4. Login with correct credentials succeeds.
	loginBody := fmt.Sprintf(`{"email":%q,"password":"password123"}`, email)
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody)))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d, body: %s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatal("expected a non-empty token")
	}

	// 5. GET /users/me without a token is rejected.
	noAuthRec := httptest.NewRecorder()
	router.ServeHTTP(noAuthRec, httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil))
	if noAuthRec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /users/me status = %d, want %d", noAuthRec.Code, http.StatusUnauthorized)
	}

	// 6. GET /users/me with a malformed token is rejected.
	malformedReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	malformedReq.Header.Set("Authorization", "Bearer not-a-real-token")
	malformedRec := httptest.NewRecorder()
	router.ServeHTTP(malformedRec, malformedReq)
	if malformedRec.Code != http.StatusUnauthorized {
		t.Errorf("malformed-token /users/me status = %d, want %d", malformedRec.Code, http.StatusUnauthorized)
	}

	// 7. GET /users/me with the real token succeeds and matches the
	// registered account.
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("/users/me status = %d, want %d, body: %s", meRec.Code, http.StatusOK, meRec.Body.String())
	}
	var me map[string]any
	if err := json.NewDecoder(meRec.Body).Decode(&me); err != nil {
		t.Fatalf("decode /users/me response: %v", err)
	}
	if me["email"] != email {
		t.Errorf("/users/me email = %v, want %v", me["email"], email)
	}
	if me["id"] != registered["id"] {
		t.Errorf("/users/me id = %v, want %v (should match the registered user)", me["id"], registered["id"])
	}
}

func TestAuthFlow_ExpiredTokenRejected(t *testing.T) {
	router, _ := setupAuthTest(t)

	expiredToken, err := auth.GenerateToken(integrationJWTSecret, "some-user-id", users.RoleCustomer, -time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthFlow_TokenSignedWithWrongSecretRejected(t *testing.T) {
	router, _ := setupAuthTest(t)

	wrongSecretToken, err := auth.GenerateToken("a-completely-different-secret", "some-user-id", users.RoleCustomer, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+wrongSecretToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestUsersTable_RoleCheckConstraint proves the database itself rejects an
// invalid role, independent of Go-level type safety — defense in depth,
// not just trusting the application layer never to send a bad value.
func TestUsersTable_RoleCheckConstraint(t *testing.T) {
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

	_, err = pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, full_name, role) VALUES ($1, 'x', 'Bad Role', 'SUPERUSER')`,
		uniqueEmail("badrole"),
	)
	if err == nil {
		t.Error("expected the role CHECK constraint to reject an invalid role, but the insert succeeded")
	}
}

// TestUsersTable_EmailUniqueConstraint proves uniqueness is enforced at
// the database level, not only by the repository's pre-check.
func TestUsersTable_EmailUniqueConstraint(t *testing.T) {
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

	repo := users.NewPostgresRepository(pool)
	email := uniqueEmail("uniqueness")
	newUser := users.NewUser{Email: email, PasswordHash: "x", FullName: "First", Role: users.RoleCustomer}

	if _, err := repo.Create(ctx, newUser); err != nil {
		t.Fatalf("first Create() failed: %v", err)
	}
	if _, err := repo.Create(ctx, newUser); err == nil {
		t.Error("expected the second Create() with a duplicate email to fail")
	}
}

func TestSeedDemoUsers_IsIdempotent(t *testing.T) {
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

	repo := users.NewPostgresRepository(pool)
	logger := testLogger()

	if err := auth.SeedDemoUsers(ctx, repo, logger); err != nil {
		t.Fatalf("first SeedDemoUsers() failed: %v", err)
	}
	// Calling it again must not error or create duplicates.
	if err := auth.SeedDemoUsers(ctx, repo, logger); err != nil {
		t.Fatalf("second SeedDemoUsers() failed: %v", err)
	}

	admin, err := repo.FindByEmail(ctx, "admin@lastmile.test")
	if err != nil {
		t.Fatalf("expected seeded admin to exist: %v", err)
	}
	if admin.Role != users.RoleAdmin {
		t.Errorf("seeded admin role = %v, want ADMIN", admin.Role)
	}
}
