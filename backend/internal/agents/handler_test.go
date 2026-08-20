package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/users"
)

const testSecret = "agents-test-secret"

// fakeRepo is an in-memory agents.Repository for handler unit tests —
// the Postgres-backed behavior (unique user_id, role/availability CHECK
// constraints, actual transactional creation) is covered separately by
// tests/integration.
type fakeRepo struct {
	byID       map[string]AgentWithUser
	emailTaken map[string]bool
	nextID     int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]AgentWithUser{}, emailTaken: map[string]bool{}}
}

func (f *fakeRepo) Create(_ context.Context, input CreateAgentInput) (AgentWithUser, error) {
	if f.emailTaken[input.Email] {
		return AgentWithUser{}, users.ErrEmailTaken
	}
	f.nextID++
	agent := AgentWithUser{
		ID:           fmt.Sprintf("fake-agent-%d", f.nextID),
		UserID:       fmt.Sprintf("fake-user-%d", f.nextID),
		FullName:     input.FullName,
		Email:        input.Email,
		Phone:        input.Phone,
		Availability: AvailabilityOffline,
		Active:       true,
		CreatedAt:    time.Now(),
	}
	f.byID[agent.ID] = agent
	f.emailTaken[input.Email] = true
	return agent, nil
}

func (f *fakeRepo) List(_ context.Context) ([]AgentWithUser, error) {
	out := make([]AgentWithUser, 0, len(f.byID))
	for _, a := range f.byID {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeRepo) FindByID(_ context.Context, id string) (AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return AgentWithUser{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeRepo) FindByUserID(_ context.Context, userID string) (AgentWithUser, error) {
	for _, a := range f.byID {
		if a.UserID == userID {
			return a, nil
		}
	}
	return AgentWithUser{}, ErrNotFound
}

func (f *fakeRepo) UpdateAvailability(_ context.Context, id string, availability Availability) (AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return AgentWithUser{}, ErrNotFound
	}
	a.Availability = availability
	f.byID[id] = a
	return a, nil
}

func (f *fakeRepo) UpdateLocation(_ context.Context, id string, lat, lng float64) (AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return AgentWithUser{}, ErrNotFound
	}
	a.CurrentLat = &lat
	a.CurrentLng = &lng
	now := time.Now()
	a.LocationUpdatedAt = &now
	f.byID[id] = a
	return a, nil
}

// doRequest builds a request with optional chi URL params (for handlers
// that read chi.URLParam(r, "id")) — chi.RouteContext must be present
// even when calling a handler directly, without going through a router.
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

// withAuth wraps a handler with the real auth.RequireAuth middleware and
// attaches a real bearer token for (userID, role) — the same middleware
// and token mechanics production uses, not a shortcut that bypasses them.
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

// --- CreateAgent ---

func TestCreateAgentHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateAgentHandler(repo), http.MethodPost, "/api/v1/agents",
		`{"email":"newagent@example.com","password":"password123","full_name":"New Agent"}`, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["email"] != "newagent@example.com" {
		t.Errorf("email = %v, want newagent@example.com", body["email"])
	}
	if body["availability"] != string(AvailabilityOffline) {
		t.Errorf("availability = %v, want OFFLINE", body["availability"])
	}
	if _, present := body["password_hash"]; present {
		t.Error("response contains password_hash — must never be returned")
	}
}

func TestCreateAgentHandler_ClientSuppliedRoleIsRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateAgentHandler(repo), http.MethodPost, "/api/v1/agents",
		`{"email":"sneaky@example.com","password":"password123","full_name":"Sneaky","role":"ADMIN"}`, nil)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (unknown field must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
	if len(repo.byID) != 0 {
		t.Error("an agent was created despite the rejected request")
	}
}

func TestCreateAgentHandler_DuplicateEmailReturns409(t *testing.T) {
	repo := newFakeRepo()
	handler := CreateAgentHandler(repo)
	body := `{"email":"dup-agent@example.com","password":"password123","full_name":"Dup"}`

	first := doRequest(t, handler, http.MethodPost, "/api/v1/agents", body, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first creation status = %d, want %d", first.Code, http.StatusCreated)
	}
	second := doRequest(t, handler, http.MethodPost, "/api/v1/agents", body, nil)
	if second.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", second.Code, http.StatusConflict)
	}
}

func TestCreateAgentHandler_InvalidInputReturns422(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateAgentHandler(repo), http.MethodPost, "/api/v1/agents",
		`{"email":"not-an-email","password":"short","full_name":""}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

// --- GetMyAgent ---

func TestGetMyAgentHandler_ReturnsOwnRecord(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "myagent@example.com", FullName: "My Agent"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withAuth(t, created.UserID, users.RoleDeliveryAgent, GetMyAgentHandler(repo))
	rec := doRequest(t, handler, http.MethodGet, "/api/v1/agents/me", "", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["id"] != created.ID {
		t.Errorf("id = %v, want %v", body["id"], created.ID)
	}
}

func TestGetMyAgentHandler_UnauthenticatedReturns401(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, GetMyAgentHandler(repo), http.MethodGet, "/api/v1/agents/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGetMyAgentHandler_NoAgentRecordReturns404(t *testing.T) {
	repo := newFakeRepo()
	handler := withAuth(t, "user-with-no-agent-record", users.RoleDeliveryAgent, GetMyAgentHandler(repo))
	rec := doRequest(t, handler, http.MethodGet, "/api/v1/agents/me", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- ListAgents ---

func TestListAgentsHandler_ReturnsCreatedAgents(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.Create(context.Background(), CreateAgentInput{Email: "a@example.com", FullName: "A"}); err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, ListAgentsHandler(repo), http.MethodGet, "/api/v1/agents", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if _, present := list[0]["password_hash"]; present {
		t.Error("listing contains password_hash — must never be returned")
	}
}

// --- UpdateAvailability ---

func TestUpdateAvailabilityHandler_OwnAgentSucceeds(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "self@example.com", FullName: "Self"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withAuth(t, created.UserID, users.RoleDeliveryAgent, UpdateAvailabilityHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/availability",
		`{"availability":"AVAILABLE"}`, map[string]string{"id": created.ID})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["availability"] != string(AvailabilityAvailable) {
		t.Errorf("availability = %v, want AVAILABLE", body["availability"])
	}
}

func TestUpdateAvailabilityHandler_AnotherAgentForbidden(t *testing.T) {
	repo := newFakeRepo()
	agentA, err := repo.Create(context.Background(), CreateAgentInput{Email: "a@example.com", FullName: "A"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	agentB, err := repo.Create(context.Background(), CreateAgentInput{Email: "b@example.com", FullName: "B"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	// Agent A tries to update Agent B's availability — the classic IDOR
	// case STEP 28 explicitly calls out.
	handler := withAuth(t, agentA.UserID, users.RoleDeliveryAgent, UpdateAvailabilityHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+agentB.ID+"/availability",
		`{"availability":"BUSY"}`, map[string]string{"id": agentB.ID})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	unchanged, _ := repo.FindByID(context.Background(), agentB.ID)
	if unchanged.Availability != AvailabilityOffline {
		t.Errorf("agent B's availability changed despite the forbidden request: %v", unchanged.Availability)
	}
}

func TestUpdateAvailabilityHandler_AdminCanManageAnyAgent(t *testing.T) {
	repo := newFakeRepo()
	agent, err := repo.Create(context.Background(), CreateAgentInput{Email: "managed@example.com", FullName: "Managed"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withAuth(t, "some-admin-id", users.RoleAdmin, UpdateAvailabilityHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+agent.ID+"/availability",
		`{"availability":"BUSY"}`, map[string]string{"id": agent.ID})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (admin should be able to manage any agent)", rec.Code, http.StatusOK)
	}
}

func TestUpdateAvailabilityHandler_InvalidValueRejected(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "self2@example.com", FullName: "Self2"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withAuth(t, created.UserID, users.RoleDeliveryAgent, UpdateAvailabilityHandler(repo))

	for _, invalid := range []string{"FREE", "ONLINE", "available123", ""} {
		t.Run(invalid, func(t *testing.T) {
			rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/availability",
				fmt.Sprintf(`{"availability":%q}`, invalid), map[string]string{"id": created.ID})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("availability=%q: status = %d, want %d", invalid, rec.Code, http.StatusUnprocessableEntity)
			}
		})
	}
}

func TestUpdateAvailabilityHandler_UnknownAgentReturns404(t *testing.T) {
	repo := newFakeRepo()
	handler := withAuth(t, "whoever", users.RoleAdmin, UpdateAvailabilityHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/does-not-exist/availability",
		`{"availability":"BUSY"}`, map[string]string{"id": "does-not-exist"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateAvailabilityHandler_UnauthenticatedReturns401(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "noauth@example.com", FullName: "NoAuth"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := auth.RequireAuth(testSecret)(UpdateAvailabilityHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/availability",
		`{"availability":"BUSY"}`, map[string]string{"id": created.ID})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- UpdateLocation ---

func TestUpdateLocationHandler_OwnAgentSucceeds(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "loc@example.com", FullName: "Loc"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withAuth(t, created.UserID, users.RoleDeliveryAgent, UpdateLocationHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/location",
		`{"latitude":12.9716,"longitude":77.5946}`, map[string]string{"id": created.ID})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["current_lat"] != 12.9716 {
		t.Errorf("current_lat = %v, want 12.9716", body["current_lat"])
	}
	if body["location_updated_at"] == nil {
		t.Error("expected location_updated_at to be set")
	}
}

func TestUpdateLocationHandler_AnotherAgentForbidden(t *testing.T) {
	repo := newFakeRepo()
	agentA, err := repo.Create(context.Background(), CreateAgentInput{Email: "loca@example.com", FullName: "A"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	agentB, err := repo.Create(context.Background(), CreateAgentInput{Email: "locb@example.com", FullName: "B"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	handler := withAuth(t, agentA.UserID, users.RoleDeliveryAgent, UpdateLocationHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+agentB.ID+"/location",
		`{"latitude":1,"longitude":1}`, map[string]string{"id": agentB.ID})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestUpdateLocationHandler_CustomerForbidden(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "locc@example.com", FullName: "C"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	// A customer authenticated as themselves (not the agent) must still
	// be rejected — the ownership check compares against agent.UserID,
	// so any UserID that isn't the agent's own is forbidden regardless
	// of role. (Route-level RequireRole would also reject a CUSTOMER
	// before reaching the handler in production; this test exercises the
	// handler's own defense-in-depth check directly.)
	handler := withAuth(t, "some-other-user-id", users.RoleCustomer, UpdateLocationHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/location",
		`{"latitude":1,"longitude":1}`, map[string]string{"id": created.ID})

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestUpdateLocationHandler_InvalidCoordinatesRejected(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "loc2@example.com", FullName: "Loc2"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := withAuth(t, created.UserID, users.RoleDeliveryAgent, UpdateLocationHandler(repo))

	cases := []struct {
		name string
		body string
	}{
		{"latitude too high", `{"latitude":91,"longitude":0}`},
		{"latitude too low", `{"latitude":-91,"longitude":0}`},
		{"longitude too high", `{"latitude":0,"longitude":181}`},
		{"longitude too low", `{"latitude":0,"longitude":-181}`},
		{"missing longitude", `{"latitude":0}`},
		{"missing latitude", `{"longitude":0}`},
		{"missing both", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/location",
				tc.body, map[string]string{"id": created.ID})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
		})
	}
}

func TestUpdateLocationHandler_ZeroZeroIsValid(t *testing.T) {
	// (0, 0) is a real, valid coordinate (off the coast of West Africa) —
	// must not be rejected as "missing" just because it's the zero value.
	repo := newFakeRepo()
	created, err := repo.Create(context.Background(), CreateAgentInput{Email: "zero@example.com", FullName: "Zero"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := withAuth(t, created.UserID, users.RoleDeliveryAgent, UpdateLocationHandler(repo))
	rec := doRequest(t, handler, http.MethodPut, "/api/v1/agents/"+created.ID+"/location",
		`{"latitude":0,"longitude":0}`, map[string]string{"id": created.ID})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestValidateCoordinates_RejectsNaNAndInf(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)
	zero := 0.0

	if got := validateCoordinates(&nan, &zero); got == "" {
		t.Error("expected NaN latitude to be rejected")
	}
	if got := validateCoordinates(&zero, &inf); got == "" {
		t.Error("expected +Inf longitude to be rejected")
	}
}

func TestParseAvailability(t *testing.T) {
	valid := []Availability{AvailabilityAvailable, AvailabilityBusy, AvailabilityOffline}
	for _, v := range valid {
		if _, ok := ParseAvailability(string(v)); !ok {
			t.Errorf("ParseAvailability(%q) rejected a valid value", v)
		}
	}
	for _, invalid := range []string{"FREE", "ONLINE", "available123", "", "available"} {
		if _, ok := ParseAvailability(invalid); ok {
			t.Errorf("ParseAvailability(%q) accepted an invalid value", invalid)
		}
	}
}
