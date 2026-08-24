package zones

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/auth"
	"lastmiletracker/internal/users"
)

const testSecret = "zones-test-secret"

// fakeRepo is an in-memory zones.Repository for handler unit tests — the
// Postgres-backed behavior (unique constraints, FK enforcement) is
// covered separately by tests/integration, the same split
// internal/agents uses.
type fakeRepo struct {
	zonesByID map[string]Zone
	zoneNames map[string]bool
	areasByID map[string]Area
	areaNames map[string]bool // "zoneID|name"
	nextID    int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		zonesByID: map[string]Zone{},
		zoneNames: map[string]bool{},
		areasByID: map[string]Area{},
		areaNames: map[string]bool{},
	}
}

func (f *fakeRepo) CreateZone(_ context.Context, input CreateZoneInput) (Zone, error) {
	if f.zoneNames[input.Name] {
		return Zone{}, ErrZoneNameTaken
	}
	f.nextID++
	z := Zone{ID: fmt.Sprintf("fake-zone-%d", f.nextID), Name: input.Name, Active: true, CreatedAt: time.Now()}
	f.zonesByID[z.ID] = z
	f.zoneNames[input.Name] = true
	return z, nil
}

func (f *fakeRepo) ListZones(_ context.Context) ([]Zone, error) {
	out := make([]Zone, 0, len(f.zonesByID))
	for _, z := range f.zonesByID {
		out = append(out, z)
	}
	return out, nil
}

func (f *fakeRepo) FindZoneByID(_ context.Context, id string) (Zone, error) {
	z, ok := f.zonesByID[id]
	if !ok {
		return Zone{}, ErrZoneNotFound
	}
	return z, nil
}

func (f *fakeRepo) UpdateZone(_ context.Context, id string, update ZoneUpdate) (Zone, error) {
	z, ok := f.zonesByID[id]
	if !ok {
		return Zone{}, ErrZoneNotFound
	}
	if update.Name != z.Name && f.zoneNames[update.Name] {
		return Zone{}, ErrZoneNameTaken
	}
	delete(f.zoneNames, z.Name)
	z.Name = update.Name
	f.zoneNames[z.Name] = true
	if update.Active != nil {
		z.Active = *update.Active
	}
	f.zonesByID[id] = z
	return z, nil
}

func (f *fakeRepo) CreateArea(_ context.Context, zoneID string, input CreateAreaInput) (Area, error) {
	if _, ok := f.zonesByID[zoneID]; !ok {
		return Area{}, ErrZoneNotFound
	}
	key := zoneID + "|" + input.Name
	if f.areaNames[key] {
		return Area{}, ErrAreaNameTaken
	}
	f.nextID++
	a := Area{ID: fmt.Sprintf("fake-area-%d", f.nextID), Name: input.Name, ZoneID: zoneID, Active: true, Latitude: input.Latitude, Longitude: input.Longitude, CreatedAt: time.Now()}
	f.areasByID[a.ID] = a
	f.areaNames[key] = true
	return a, nil
}

func (f *fakeRepo) ListAreasByZone(_ context.Context, zoneID string) ([]Area, error) {
	var out []Area
	for _, a := range f.areasByID {
		if a.ZoneID == zoneID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepo) FindAreaByID(_ context.Context, id string) (Area, error) {
	a, ok := f.areasByID[id]
	if !ok {
		return Area{}, ErrAreaNotFound
	}
	return a, nil
}

func (f *fakeRepo) UpdateArea(_ context.Context, areaID string, update AreaUpdate) (Area, error) {
	a, ok := f.areasByID[areaID]
	if !ok {
		return Area{}, ErrAreaNotFound
	}
	key := a.ZoneID + "|" + update.Name
	if update.Name != a.Name && f.areaNames[key] {
		return Area{}, ErrAreaNameTaken
	}
	delete(f.areaNames, a.ZoneID+"|"+a.Name)
	a.Name = update.Name
	f.areaNames[key] = true
	if update.Active != nil {
		a.Active = *update.Active
	}
	if update.Latitude != nil {
		a.Latitude = update.Latitude
	}
	if update.Longitude != nil {
		a.Longitude = update.Longitude
	}
	f.areasByID[areaID] = a
	return a, nil
}

// doRequest builds a request with optional chi URL params — chi.RouteContext
// must be present even when calling a handler directly, without going
// through a router. Same helper as internal/agents' handler_test.go.
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
// attaches a real bearer token — same helper as internal/agents.
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

// --- CreateZone ---

func TestCreateZoneHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateZoneHandler(repo), http.MethodPost, "/api/v1/zones", `{"name":"North"}`, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["name"] != "North" {
		t.Errorf("name = %v, want North", body["name"])
	}
	if body["active"] != true {
		t.Errorf("active = %v, want true (new zones default active)", body["active"])
	}
}

func TestCreateZoneHandler_EmptyNameRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateZoneHandler(repo), http.MethodPost, "/api/v1/zones", `{"name":"   "}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateZoneHandler_TooLongNameRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateZoneHandler(repo), http.MethodPost, "/api/v1/zones",
		fmt.Sprintf(`{"name":%q}`, strings.Repeat("a", maxNameLength+1)), nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateZoneHandler_DuplicateNameReturns409(t *testing.T) {
	repo := newFakeRepo()
	handler := CreateZoneHandler(repo)
	first := doRequest(t, handler, http.MethodPost, "/api/v1/zones", `{"name":"South"}`, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first creation status = %d, want %d", first.Code, http.StatusCreated)
	}
	second := doRequest(t, handler, http.MethodPost, "/api/v1/zones", `{"name":"South"}`, nil)
	if second.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", second.Code, http.StatusConflict)
	}
}

func TestCreateZoneHandler_UnknownFieldRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateZoneHandler(repo), http.MethodPost, "/api/v1/zones", `{"name":"East","id":"sneaky"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (unknown field must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
}

// --- ListZones / GetZone ---

func TestListZonesHandler_ReturnsCreatedZones(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "A"}); err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, ListZonesHandler(repo), http.MethodGet, "/api/v1/zones", "", nil)
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
}

func TestGetZoneHandler_UnknownZoneReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, GetZoneHandler(repo), http.MethodGet, "/api/v1/zones/missing", "", map[string]string{"id": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- UpdateZone ---

func TestUpdateZoneHandler_RenameSucceeds(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "Old"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateZoneHandler(repo), http.MethodPut, "/api/v1/zones/"+created.ID,
		`{"name":"New"}`, map[string]string{"id": created.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["name"] != "New" {
		t.Errorf("name = %v, want New", body["name"])
	}
	if body["active"] != true {
		t.Errorf("active = %v, want true (omitted active must not change it)", body["active"])
	}
}

func TestUpdateZoneHandler_DeactivateSucceeds(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ToDeactivate"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateZoneHandler(repo), http.MethodPut, "/api/v1/zones/"+created.ID,
		`{"name":"ToDeactivate","active":false}`, map[string]string{"id": created.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["active"] != false {
		t.Errorf("active = %v, want false", body["active"])
	}
}

func TestUpdateZoneHandler_UnknownZoneReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, UpdateZoneHandler(repo), http.MethodPut, "/api/v1/zones/missing",
		`{"name":"New"}`, map[string]string{"id": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- CreateArea ---

func TestCreateAreaHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "Z1"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Area 1"}`, map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["zone_id"] != zone.ID {
		t.Errorf("zone_id = %v, want %v (must come from the URL, not the body)", body["zone_id"], zone.ID)
	}
}

// TestCreateAreaHandler_ZoneIDInBodyIsRejected is STEP 25's explicit
// mass-assignment / relationship-tampering test: a client attempting to
// set zone_id via the request body must be rejected outright, not merely
// ignored, and the resulting area (if creation is retried correctly)
// must belong to the zone in the URL, never the one from the tampered
// body.
func TestCreateAreaHandler_ZoneIDInBodyIsRejected(t *testing.T) {
	repo := newFakeRepo()
	zoneA, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZoneA"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	zoneB, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZoneB"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zoneA.ID+"/areas",
		fmt.Sprintf(`{"name":"Sneaky","zone_id":%q}`, zoneB.ID), map[string]string{"zoneID": zoneA.ID})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (unknown field zone_id must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
	areasA, _ := repo.ListAreasByZone(context.Background(), zoneA.ID)
	areasB, _ := repo.ListAreasByZone(context.Background(), zoneB.ID)
	if len(areasA) != 0 || len(areasB) != 0 {
		t.Error("an area was created despite the rejected request")
	}
}

func TestCreateAreaHandler_UnknownZoneReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/missing/areas",
		`{"name":"Area"}`, map[string]string{"zoneID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateAreaHandler_DuplicateNameWithinZoneReturns409(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZDup"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := CreateAreaHandler(repo)
	first := doRequest(t, handler, http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Dup"}`, map[string]string{"zoneID": zone.ID})
	if first.Code != http.StatusCreated {
		t.Fatalf("first creation status = %d, want %d", first.Code, http.StatusCreated)
	}
	second := doRequest(t, handler, http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Dup"}`, map[string]string{"zoneID": zone.ID})
	if second.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", second.Code, http.StatusConflict)
	}
}

func TestCreateAreaHandler_SameNameDifferentZonesAllowed(t *testing.T) {
	repo := newFakeRepo()
	zoneA, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZA"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	zoneB, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZB"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := CreateAreaHandler(repo)
	first := doRequest(t, handler, http.MethodPost, "/api/v1/zones/"+zoneA.ID+"/areas",
		`{"name":"Downtown"}`, map[string]string{"zoneID": zoneA.ID})
	second := doRequest(t, handler, http.MethodPost, "/api/v1/zones/"+zoneB.ID+"/areas",
		`{"name":"Downtown"}`, map[string]string{"zoneID": zoneB.ID})
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Errorf("same area name in two different zones should both succeed: got %d, %d", first.Code, second.Code)
	}
}

// --- ListAreas ---

func TestListAreasHandler_UnknownZoneReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, ListAreasHandler(repo), http.MethodGet, "/api/v1/zones/missing/areas",
		"", map[string]string{"zoneID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListAreasHandler_EmptyZoneReturnsEmptyList(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "Empty"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, ListAreasHandler(repo), http.MethodGet, "/api/v1/zones/"+zone.ID+"/areas",
		"", map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (existing zone with no areas is 200, not 404)", rec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}

// --- UpdateArea ---

func TestUpdateAreaHandler_RenameSucceeds(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZR"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "OldArea"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateAreaHandler(repo), http.MethodPut, "/api/v1/zones/"+zone.ID+"/areas/"+area.ID,
		`{"name":"NewArea"}`, map[string]string{"zoneID": zone.ID, "areaID": area.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["name"] != "NewArea" {
		t.Errorf("name = %v, want NewArea", body["name"])
	}
}

// TestUpdateAreaHandler_ActiveToggleIsReversible verifies deactivating
// and reactivating an area via the same PUT-based rename endpoint —
// mirroring UpdateZoneHandler's own combined rename+active shape.
func TestUpdateAreaHandler_ActiveToggleIsReversible(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZA"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "ToggleArea"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	if !area.Active {
		t.Fatalf("newly created area should start active")
	}

	handler := UpdateAreaHandler(repo)
	path := "/api/v1/zones/" + zone.ID + "/areas/" + area.ID
	params := map[string]string{"zoneID": zone.ID, "areaID": area.ID}

	deactivate := doRequest(t, handler, http.MethodPut, path, `{"name":"ToggleArea","active":false}`, params)
	if deactivate.Code != http.StatusOK {
		t.Fatalf("deactivate status = %d, want %d, body: %s", deactivate.Code, http.StatusOK, deactivate.Body.String())
	}
	if body := decodeJSON[map[string]any](t, deactivate); body["active"] != false {
		t.Errorf("active = %v, want false", body["active"])
	}

	reactivate := doRequest(t, handler, http.MethodPut, path, `{"name":"ToggleArea","active":true}`, params)
	if reactivate.Code != http.StatusOK {
		t.Fatalf("reactivate status = %d, want %d, body: %s", reactivate.Code, http.StatusOK, reactivate.Body.String())
	}
	if body := decodeJSON[map[string]any](t, reactivate); body["active"] != true {
		t.Errorf("active = %v, want true", body["active"])
	}
}

// TestUpdateAreaHandler_OmittedActiveLeavesItUnchanged verifies a plain
// rename (no "active" field in the request body) does not accidentally
// flip an area's active state — the same nil-means-unchanged contract
// zones.ZoneUpdate.Active already guarantees.
func TestUpdateAreaHandler_OmittedActiveLeavesItUnchanged(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZO"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "OmitActive"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, UpdateAreaHandler(repo), http.MethodPut, "/api/v1/zones/"+zone.ID+"/areas/"+area.ID,
		`{"name":"OmitActiveRenamed"}`, map[string]string{"zoneID": zone.ID, "areaID": area.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["active"] != true {
		t.Errorf("active = %v, want true (unchanged)", body["active"])
	}
}

// TestUpdateAreaHandler_WrongZoneInPathReturns404 verifies an area
// cannot be reached (or renamed) through a URL naming a different zone
// than the one it actually belongs to.
func TestUpdateAreaHandler_WrongZoneInPathReturns404(t *testing.T) {
	repo := newFakeRepo()
	zoneA, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZWA"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	zoneB, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZWB"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zoneA.ID, CreateAreaInput{Name: "BelongsToA"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, UpdateAreaHandler(repo), http.MethodPut, "/api/v1/zones/"+zoneB.ID+"/areas/"+area.ID,
		`{"name":"Hijacked"}`, map[string]string{"zoneID": zoneB.ID, "areaID": area.ID})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	unchanged, _ := repo.FindAreaByID(context.Background(), area.ID)
	if unchanged.Name != "BelongsToA" {
		t.Errorf("area was renamed despite the wrong-zone request: %v", unchanged.Name)
	}
}

func TestUpdateAreaHandler_UnknownAreaReturns404(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZU"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateAreaHandler(repo), http.MethodPut, "/api/v1/zones/"+zone.ID+"/areas/missing",
		`{"name":"X"}`, map[string]string{"zoneID": zone.ID, "areaID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- Area coordinates ---

func TestCreateAreaHandler_WithCoordinatesSucceeds(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZCoord"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Area 1","latitude":12.9716,"longitude":77.5946}`, map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["latitude"] != 12.9716 || body["longitude"] != 77.5946 {
		t.Errorf("latitude/longitude = %v/%v, want 12.9716/77.5946", body["latitude"], body["longitude"])
	}
}

func TestCreateAreaHandler_WithoutCoordinatesLeavesThemNull(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZNoCoord"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Area 1"}`, map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["latitude"] != nil || body["longitude"] != nil {
		t.Errorf("latitude/longitude = %v/%v, want both null when not supplied", body["latitude"], body["longitude"])
	}
}

func TestCreateAreaHandler_OnlyLatitudeProvidedRejected(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZPartial"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Area 1","latitude":12.9716}`, map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (latitude without longitude must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateAreaHandler_OutOfRangeLatitudeRejected(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZBadLat"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Area 1","latitude":91,"longitude":0}`, map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (latitude 91 is out of range)", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateAreaHandler_OutOfRangeLongitudeRejected(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZBadLng"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateAreaHandler(repo), http.MethodPost, "/api/v1/zones/"+zone.ID+"/areas",
		`{"name":"Area 1","latitude":0,"longitude":181}`, map[string]string{"zoneID": zone.ID})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (longitude 181 is out of range)", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestUpdateAreaHandler_SetsCoordinates(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZSetCoord"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "A1"})
	if err != nil {
		t.Fatalf("seed area failed: %v", err)
	}
	rec := doRequest(t, UpdateAreaHandler(repo), http.MethodPut, "/api/v1/zones/"+zone.ID+"/areas/"+area.ID,
		`{"name":"A1","latitude":12.9716,"longitude":77.5946}`, map[string]string{"zoneID": zone.ID, "areaID": area.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["latitude"] != 12.9716 || body["longitude"] != 77.5946 {
		t.Errorf("latitude/longitude = %v/%v, want 12.9716/77.5946", body["latitude"], body["longitude"])
	}
}

func TestUpdateAreaHandler_OmittedCoordinatesLeaveThemUnchanged(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZKeepCoord"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	lat, lng := 12.9716, 77.5946
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "A1", Latitude: &lat, Longitude: &lng})
	if err != nil {
		t.Fatalf("seed area failed: %v", err)
	}
	// Rename only — no latitude/longitude field at all in the body.
	rec := doRequest(t, UpdateAreaHandler(repo), http.MethodPut, "/api/v1/zones/"+zone.ID+"/areas/"+area.ID,
		`{"name":"Renamed"}`, map[string]string{"zoneID": zone.ID, "areaID": area.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["latitude"] != 12.9716 || body["longitude"] != 77.5946 {
		t.Errorf("latitude/longitude = %v/%v, want them left unchanged at 12.9716/77.5946", body["latitude"], body["longitude"])
	}
}

// --- RBAC (route-level; Mount itself is exercised in tests/integration,
// this exercises that handlers work correctly once auth middleware has
// run — the same split internal/agents uses) ---

func TestCreateZoneHandler_UnauthenticatedViaRequireAuthReturns401(t *testing.T) {
	repo := newFakeRepo()
	handler := auth.RequireAuth(testSecret)(CreateZoneHandler(repo))
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/zones", `{"name":"NoAuth"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateZoneHandler_AuthenticatedAdminSucceeds(t *testing.T) {
	repo := newFakeRepo()
	handler := withAuth(t, "some-admin-id", users.RoleAdmin, CreateZoneHandler(repo))
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/zones", `{"name":"AdminZone"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestDetermineZoneRelationship(t *testing.T) {
	if got := DetermineZoneRelationship("zone-a", "zone-a"); got != RelationshipIntra {
		t.Errorf("same zone id = %v, want INTRA", got)
	}
	if got := DetermineZoneRelationship("zone-a", "zone-b"); got != RelationshipInter {
		t.Errorf("different zone ids = %v, want INTER", got)
	}
}
