package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lastmiletracker/internal/users"
)

// fakeRepo is an in-memory users.Repository for handler unit tests, so
// they don't need a real PostgreSQL instance. The Postgres-backed
// behavior (unique constraint, role CHECK constraint, actual persistence)
// is covered separately by tests/integration.
type fakeRepo struct {
	byEmail map[string]users.User
	byID    map[string]users.User
	nextID  int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byEmail: map[string]users.User{}, byID: map[string]users.User{}}
}

func (f *fakeRepo) Create(_ context.Context, u users.NewUser) (users.User, error) {
	if _, exists := f.byEmail[u.Email]; exists {
		return users.User{}, users.ErrEmailTaken
	}
	f.nextID++
	created := users.User{
		ID:           fmt.Sprintf("fake-id-%d", f.nextID),
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		FullName:     u.FullName,
		Phone:        u.Phone,
		Role:         u.Role,
		CreatedAt:    time.Now(),
	}
	f.byEmail[u.Email] = created
	f.byID[created.ID] = created
	return created, nil
}

func (f *fakeRepo) FindByEmail(_ context.Context, email string) (users.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) FindByID(_ context.Context, id string) (users.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) Update(_ context.Context, id string, update users.ProfileUpdate) (users.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	u.FullName = update.FullName
	u.Phone = update.Phone
	f.byID[id] = u
	f.byEmail[u.Email] = u
	return u, nil
}

func doRequest(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("failed to decode JSON response: %v (body: %s)", err, rec.Body.String())
	}
	return v
}

// --- Registration ---

func TestRegisterHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"new@example.com","password":"password123","full_name":"New User"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	body := decodeJSON[map[string]any](t, rec)
	if body["role"] != string(users.RoleCustomer) {
		t.Errorf("role = %v, want CUSTOMER", body["role"])
	}
	if body["email"] != "new@example.com" {
		t.Errorf("email = %v, want new@example.com", body["email"])
	}
	if _, present := body["password_hash"]; present {
		t.Error("response contains password_hash — must never be returned")
	}
	if _, present := body["password"]; present {
		t.Error("response contains password — must never be returned")
	}
}

func TestRegisterHandler_NormalizesEmail(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"  MixedCase@Example.COM  ","password":"password123","full_name":"Case Test"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["email"] != "mixedcase@example.com" {
		t.Errorf("email = %v, want normalized mixedcase@example.com", body["email"])
	}
}

func TestRegisterHandler_DuplicateEmailReturns409(t *testing.T) {
	repo := newFakeRepo()
	handler := RegisterHandler(repo)
	first := doRequest(t, handler, http.MethodPost, "/api/v1/auth/register",
		`{"email":"dup@example.com","password":"password123","full_name":"First"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first registration status = %d, want %d", first.Code, http.StatusCreated)
	}

	second := doRequest(t, handler, http.MethodPost, "/api/v1/auth/register",
		`{"email":"dup@example.com","password":"password123","full_name":"Second"}`)
	if second.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", second.Code, http.StatusConflict)
	}
}

func TestRegisterHandler_InvalidEmailReturns422(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"not-an-email","password":"password123","full_name":"Bad Email"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestRegisterHandler_MissingFieldsReturns422(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"","password":"","full_name":""}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestRegisterHandler_WeakPasswordReturns422(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"weak@example.com","password":"short","full_name":"Weak Pass"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestRegisterHandler_ClientSuppliedRoleIsRejectedNotHonored(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"attacker@example.com","password":"password123","full_name":"Attacker","role":"ADMIN"}`)

	// DisallowUnknownFields means this fails closed: the whole request is
	// rejected, not silently downgraded to CUSTOMER.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (client-supplied role must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
	if _, err := repo.FindByEmail(context.Background(), "attacker@example.com"); err == nil {
		t.Error("a user was created despite the rejected request — role-injection attempt must not create any account")
	}
}

// --- Login ---

func mustHashForTest(t *testing.T, password string) string {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	return hash
}

func TestLoginHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.Create(context.Background(), users.NewUser{
		Email: "login@example.com", PasswordHash: mustHashForTest(t, "correct-password"),
		FullName: "Login User", Role: users.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, LoginHandler(repo, testSecret), http.MethodPost, "/api/v1/auth/login",
		`{"email":"login@example.com","password":"correct-password"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[loginResponse](t, rec)
	if body.Token == "" {
		t.Error("expected a non-empty token")
	}
	if _, err := ParseToken(testSecret, body.Token); err != nil {
		t.Errorf("returned token does not parse: %v", err)
	}
}

func TestLoginHandler_WrongPasswordReturns401Generic(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.Create(context.Background(), users.NewUser{
		Email: "wrongpass@example.com", PasswordHash: mustHashForTest(t, "correct-password"),
		FullName: "User", Role: users.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, LoginHandler(repo, testSecret), http.MethodPost, "/api/v1/auth/login",
		`{"email":"wrongpass@example.com","password":"wrong-password"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	assertGenericInvalidCredentials(t, rec)
}

func TestLoginHandler_UnknownEmailReturns401Generic(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, LoginHandler(repo, testSecret), http.MethodPost, "/api/v1/auth/login",
		`{"email":"nobody@example.com","password":"whatever123"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	assertGenericInvalidCredentials(t, rec)
}

// assertGenericInvalidCredentials verifies both failure modes described in
// STEP 6 produce the exact same response body — proving a caller cannot
// distinguish "unknown email" from "wrong password" by inspecting the
// response.
func assertGenericInvalidCredentials(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	const wantMessage = "invalid email or password"
	body := decodeJSON[map[string]any](t, rec)
	if body["error"] != wantMessage {
		t.Errorf("error = %v, want %q", body["error"], wantMessage)
	}
}

func TestLoginHandler_MissingFieldsReturns422(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, LoginHandler(repo, testSecret), http.MethodPost, "/api/v1/auth/login", `{"email":"","password":""}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

// --- GetMe ---

func TestGetMeHandler_ValidIdentity(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), users.NewUser{
		Email: "me@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Me User", Role: users.RoleDeliveryAgent,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withIdentity(Identity{UserID: created.ID, Role: created.Role})(GetMeHandler(repo))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["email"] != "me@example.com" {
		t.Errorf("email = %v, want me@example.com", body["email"])
	}
	if body["role"] != string(users.RoleDeliveryAgent) {
		t.Errorf("role = %v, want DELIVERY_AGENT", body["role"])
	}
	if _, present := body["password_hash"]; present {
		t.Error("response contains password_hash — must never be returned")
	}
}

func TestGetMeHandler_NoIdentityReturns401(t *testing.T) {
	repo := newFakeRepo()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rec := httptest.NewRecorder()
	GetMeHandler(repo).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGetMeHandler_UserNoLongerExistsReturns401(t *testing.T) {
	repo := newFakeRepo()
	handler := withIdentity(Identity{UserID: "does-not-exist", Role: users.RoleCustomer})(GetMeHandler(repo))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- UpdateMe ---

func TestUpdateMeHandler_UpdatesPermittedFields(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), users.NewUser{
		Email: "update@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Old Name", Role: users.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withIdentity(Identity{UserID: created.ID, Role: created.Role})(UpdateMeHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/users/me",
		`{"full_name":"New Name","phone":"555-0100"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["full_name"] != "New Name" {
		t.Errorf("full_name = %v, want New Name", body["full_name"])
	}
	if body["phone"] != "555-0100" {
		t.Errorf("phone = %v, want 555-0100", body["phone"])
	}
	if _, present := body["password_hash"]; present {
		t.Error("response contains password_hash — must never be returned")
	}
}

func TestUpdateMeHandler_UnauthenticatedReturns401(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, UpdateMeHandler(repo), http.MethodPut, "/api/v1/users/me", `{"full_name":"New Name"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUpdateMeHandler_EmptyFullNameReturns422(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), users.NewUser{
		Email: "empty@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Someone", Role: users.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withIdentity(Identity{UserID: created.ID, Role: created.Role})(UpdateMeHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/users/me", `{"full_name":"   ","phone":""}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

// TestUpdateMeHandler_CannotMassAssignProtectedFields is the direct test
// for STEP 28's mass-assignment concern: id, email, role, password_hash,
// and created_at are all absent from updateMeRequest's JSON tags, and
// DisallowUnknownFields means sending any of them fails the whole request
// closed rather than silently dropping the field.
func TestUpdateMeHandler_CannotMassAssignProtectedFields(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), users.NewUser{
		Email: "protected@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Original Name", Role: users.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	cases := []string{
		`{"full_name":"New Name","role":"ADMIN"}`,
		`{"full_name":"New Name","id":"attacker-supplied-id"}`,
		`{"full_name":"New Name","email":"new-email@example.com"}`,
		`{"full_name":"New Name","password_hash":"not-a-real-hash"}`,
		`{"full_name":"New Name","created_at":"2000-01-01T00:00:00Z"}`,
	}

	handler := withIdentity(Identity{UserID: created.ID, Role: created.Role})(UpdateMeHandler(repo))
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			rec := doRequest(t, handler, http.MethodPut, "/api/v1/users/me", body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d (unknown field must be rejected)", rec.Code, http.StatusUnprocessableEntity)
			}
		})
	}

	// The user's actual stored record must be completely unchanged —
	// role especially.
	unchanged, err := repo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if unchanged.Role != users.RoleCustomer {
		t.Errorf("role = %v, want unchanged CUSTOMER", unchanged.Role)
	}
	if unchanged.FullName != "Original Name" {
		t.Errorf("full_name = %v, want unchanged Original Name", unchanged.FullName)
	}
	if unchanged.PasswordHash != created.PasswordHash {
		t.Error("password_hash changed despite being absent from the DTO")
	}
}

// --- LookupCustomer ---

func TestLookupCustomerHandler_FindsCustomerByEmail(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), users.NewUser{
		Email: "shopper@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Shopper Sam", Role: users.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, LookupCustomerHandler(repo), http.MethodGet, "/api/v1/users/lookup?email=shopper@example.com", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["id"] != created.ID {
		t.Errorf("id = %v, want %v", body["id"], created.ID)
	}
	if body["full_name"] != "Shopper Sam" {
		t.Errorf("full_name = %v, want Shopper Sam", body["full_name"])
	}
	if _, present := body["role"]; present {
		t.Error("response contains role — this endpoint only ever returns customers, no need to echo it")
	}
	if _, present := body["password_hash"]; present {
		t.Error("response contains password_hash — must never be returned")
	}
}

func TestLookupCustomerHandler_IsCaseInsensitiveOnEmail(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.Create(context.Background(), users.NewUser{
		Email: "mixedcase@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Mixed Case", Role: users.RoleCustomer,
	}); err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, LookupCustomerHandler(repo), http.MethodGet, "/api/v1/users/lookup?email=MixedCase@Example.com", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestLookupCustomerHandler_UnknownEmailReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, LookupCustomerHandler(repo), http.MethodGet, "/api/v1/users/lookup?email=nobody@example.com", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// A non-customer account (admin/agent) must 404 exactly like an unknown
// email — this endpoint must never let an admin distinguish "this email
// isn't registered" from "this email belongs to staff".
func TestLookupCustomerHandler_NonCustomerAccountReturns404(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.Create(context.Background(), users.NewUser{
		Email: "staff@example.com", PasswordHash: mustHashForTest(t, "password123"),
		FullName: "Staff Member", Role: users.RoleDeliveryAgent,
	}); err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, LookupCustomerHandler(repo), http.MethodGet, "/api/v1/users/lookup?email=staff@example.com", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestLookupCustomerHandler_MissingEmailReturns422(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, LookupCustomerHandler(repo), http.MethodGet, "/api/v1/users/lookup", "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// --- Pure validation helpers ---

func TestValidateRegistration(t *testing.T) {
	cases := []struct {
		name             string
		email, pw, fname string
		wantProblems     bool
	}{
		{"valid input", "person@example.com", "password123", "Person", false},
		{"empty email", "", "password123", "Person", true},
		{"malformed email", "not-an-email", "password123", "Person", true},
		{"short password", "person@example.com", "short1", "Person", true},
		{"empty full name", "person@example.com", "password123", "   ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := ValidateRegistration(tc.email, tc.pw, tc.fname)
			if tc.wantProblems && len(problems) == 0 {
				t.Error("expected validation problems, got none")
			}
			if !tc.wantProblems && len(problems) > 0 {
				t.Errorf("expected no validation problems, got: %v", problems)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Test@Example.COM  "); got != "test@example.com" {
		t.Errorf("NormalizeEmail() = %q, want test@example.com", got)
	}
}
