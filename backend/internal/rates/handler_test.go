package rates

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

const testSecret = "rates-test-secret"

// fakeRepo is an in-memory rates.Repository for handler unit tests — the
// Postgres-backed behavior (unique indexes, FK, the FOR UPDATE-locked
// concurrency path) is covered separately by tests/integration, the
// same split every prior module in this project uses. It reuses the
// same validateSlabAgainstExisting logic the real repository uses, so
// overlap/open-ended rejection behaves identically here.
type fakeRepo struct {
	cardsByID map[string]RateCard
	slabsByID map[string]Slab
	nextID    int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{cardsByID: map[string]RateCard{}, slabsByID: map[string]Slab{}}
}

func (f *fakeRepo) CreateRateCard(_ context.Context, input CreateRateCardInput) (RateCard, error) {
	f.nextID++
	rc := RateCard{
		ID:               fmt.Sprintf("fake-card-%d", f.nextID),
		OrderType:        input.OrderType,
		ZoneRelationship: input.ZoneRelationship,
		CODSurcharge:     input.CODSurcharge,
		Active:           false,
		CreatedAt:        time.Now(),
	}
	f.cardsByID[rc.ID] = rc
	return rc, nil
}

func (f *fakeRepo) ListRateCards(_ context.Context) ([]RateCard, error) {
	out := make([]RateCard, 0, len(f.cardsByID))
	for _, rc := range f.cardsByID {
		out = append(out, rc)
	}
	return out, nil
}

func (f *fakeRepo) FindRateCardByID(_ context.Context, id string) (RateCard, error) {
	rc, ok := f.cardsByID[id]
	if !ok {
		return RateCard{}, ErrRateCardNotFound
	}
	return rc, nil
}

func (f *fakeRepo) FindActiveCard(_ context.Context, orderType OrderType, zoneRelationship ZoneRelationship) (RateCard, error) {
	for _, rc := range f.cardsByID {
		if rc.Active && rc.OrderType == orderType && rc.ZoneRelationship == zoneRelationship {
			return rc, nil
		}
	}
	return RateCard{}, ErrRateCardNotFound
}

func (f *fakeRepo) UpdateRateCard(_ context.Context, id string, update RateCardUpdate) (RateCard, error) {
	rc, ok := f.cardsByID[id]
	if !ok {
		return RateCard{}, ErrRateCardNotFound
	}
	newActive := rc.Active
	if update.Active != nil {
		newActive = *update.Active
	}
	if newActive {
		for otherID, other := range f.cardsByID {
			if otherID != id && other.Active && other.OrderType == rc.OrderType && other.ZoneRelationship == rc.ZoneRelationship {
				return RateCard{}, ErrActiveCombinationExists
			}
		}
	}
	rc.CODSurcharge = update.CODSurcharge
	rc.Active = newActive
	f.cardsByID[id] = rc
	return rc, nil
}

func (f *fakeRepo) CreateSlab(_ context.Context, rateCardID string, input CreateSlabInput) (Slab, error) {
	if _, ok := f.cardsByID[rateCardID]; !ok {
		return Slab{}, ErrRateCardNotFound
	}
	existing := f.slabsForCard(rateCardID)
	if err := validateSlabAgainstExisting(input.MinWeight, input.MaxWeight, "", existing); err != nil {
		return Slab{}, err
	}
	f.nextID++
	s := Slab{
		ID:         fmt.Sprintf("fake-slab-%d", f.nextID),
		RateCardID: rateCardID,
		MinWeight:  input.MinWeight,
		MaxWeight:  input.MaxWeight,
		Price:      input.Price,
		CreatedAt:  time.Now(),
	}
	f.slabsByID[s.ID] = s
	return s, nil
}

func (f *fakeRepo) ListSlabsByRateCard(_ context.Context, rateCardID string) ([]Slab, error) {
	return f.slabsForCard(rateCardID), nil
}

func (f *fakeRepo) FindSlabByID(_ context.Context, id string) (Slab, error) {
	s, ok := f.slabsByID[id]
	if !ok {
		return Slab{}, ErrSlabNotFound
	}
	return s, nil
}

func (f *fakeRepo) UpdateSlab(_ context.Context, slabID string, update SlabUpdate) (Slab, error) {
	s, ok := f.slabsByID[slabID]
	if !ok {
		return Slab{}, ErrSlabNotFound
	}
	existing := f.slabsForCard(s.RateCardID)
	if err := validateSlabAgainstExisting(update.MinWeight, update.MaxWeight, slabID, existing); err != nil {
		return Slab{}, err
	}
	s.MinWeight = update.MinWeight
	s.MaxWeight = update.MaxWeight
	s.Price = update.Price
	f.slabsByID[slabID] = s
	return s, nil
}

func (f *fakeRepo) DeleteSlab(_ context.Context, slabID string) error {
	if _, ok := f.slabsByID[slabID]; !ok {
		return ErrSlabNotFound
	}
	delete(f.slabsByID, slabID)
	return nil
}

func (f *fakeRepo) slabsForCard(rateCardID string) []Slab {
	var out []Slab
	for _, s := range f.slabsByID {
		if s.RateCardID == rateCardID {
			out = append(out, s)
		}
	}
	return out
}

// doRequest builds a request with optional chi URL params — same helper
// as every prior module's handler_test.go.
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

// --- CreateRateCard ---

func TestCreateRateCardHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateRateCardHandler(repo), http.MethodPost, "/api/v1/rates",
		`{"order_type":"B2B","zone_relationship":"INTRA","cod_surcharge":15}`, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["order_type"] != "B2B" || body["zone_relationship"] != "INTRA" {
		t.Errorf("unexpected body: %v", body)
	}
	if body["active"] != false {
		t.Errorf("active = %v, want false (new rate cards must start inactive)", body["active"])
	}
}

func TestCreateRateCardHandler_InvalidOrderTypeRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateRateCardHandler(repo), http.MethodPost, "/api/v1/rates",
		`{"order_type":"B2Z","zone_relationship":"INTRA"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateRateCardHandler_InvalidZoneRelationshipRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateRateCardHandler(repo), http.MethodPost, "/api/v1/rates",
		`{"order_type":"B2B","zone_relationship":"SIDEWAYS"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateRateCardHandler_NegativeSurchargeRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateRateCardHandler(repo), http.MethodPost, "/api/v1/rates",
		`{"order_type":"B2B","zone_relationship":"INTRA","cod_surcharge":-5}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateRateCardHandler_ActiveFieldInBodyIsRejected(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateRateCardHandler(repo), http.MethodPost, "/api/v1/rates",
		`{"order_type":"B2B","zone_relationship":"INTRA","active":true}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (unknown field 'active' must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
	if len(repo.cardsByID) != 0 {
		t.Error("a rate card was created despite the rejected request")
	}
}

// --- UpdateRateCard ---

func TestUpdateRateCardHandler_ActivateSucceeds(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateRateCardHandler(repo), http.MethodPut, "/api/v1/rates/"+created.ID,
		`{"cod_surcharge":25,"active":true}`, map[string]string{"id": created.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["active"] != true || body["cod_surcharge"] != float64(25) {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestUpdateRateCardHandler_OrderTypeInBodyIsRejected(t *testing.T) {
	repo := newFakeRepo()
	created, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateRateCardHandler(repo), http.MethodPut, "/api/v1/rates/"+created.ID,
		`{"cod_surcharge":0,"order_type":"B2C"}`, map[string]string{"id": created.ID})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d (unknown field 'order_type' must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestUpdateRateCardHandler_DuplicateActiveCombinationRejected(t *testing.T) {
	repo := newFakeRepo()
	first, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2C, ZoneRelationship: RelationshipInter})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	second, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2C, ZoneRelationship: RelationshipInter})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	activateFirst := doRequest(t, UpdateRateCardHandler(repo), http.MethodPut, "/api/v1/rates/"+first.ID,
		`{"cod_surcharge":0,"active":true}`, map[string]string{"id": first.ID})
	if activateFirst.Code != http.StatusOK {
		t.Fatalf("activating first card: status = %d, want %d", activateFirst.Code, http.StatusOK)
	}

	activateSecond := doRequest(t, UpdateRateCardHandler(repo), http.MethodPut, "/api/v1/rates/"+second.ID,
		`{"cod_surcharge":0,"active":true}`, map[string]string{"id": second.ID})
	if activateSecond.Code != http.StatusConflict {
		t.Errorf("activating second card for the same combination: status = %d, want %d", activateSecond.Code, http.StatusConflict)
	}
}

func TestUpdateRateCardHandler_UnknownCardReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, UpdateRateCardHandler(repo), http.MethodPut, "/api/v1/rates/missing",
		`{"cod_surcharge":0}`, map[string]string{"id": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- CreateSlab ---

func TestCreateSlabHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateSlabHandler(repo), http.MethodPost, "/api/v1/rates/"+card.ID+"/slabs",
		`{"min_weight":0,"max_weight":5,"price":50}`, map[string]string{"rateCardID": card.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["rate_card_id"] != card.ID {
		t.Errorf("rate_card_id = %v, want %v (must come from the URL, not the body)", body["rate_card_id"], card.ID)
	}
	if body["price"] != float64(50) {
		t.Errorf("price = %v, want 50 (flat price, not multiplied)", body["price"])
	}
}

func TestCreateSlabHandler_OpenEndedSlabHasNullMaxWeight(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateSlabHandler(repo), http.MethodPost, "/api/v1/rates/"+card.ID+"/slabs",
		`{"min_weight":20,"price":140}`, map[string]string{"rateCardID": card.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["max_weight"] != nil {
		t.Errorf("max_weight = %v, want null (omitted max_weight means open-ended)", body["max_weight"])
	}
}

func TestCreateSlabHandler_RateCardIDInBodyIsRejected(t *testing.T) {
	repo := newFakeRepo()
	cardA, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	cardB, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipInter})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	rec := doRequest(t, CreateSlabHandler(repo), http.MethodPost, "/api/v1/rates/"+cardA.ID+"/slabs",
		fmt.Sprintf(`{"min_weight":0,"max_weight":5,"price":50,"rate_card_id":%q}`, cardB.ID),
		map[string]string{"rateCardID": cardA.ID})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (unknown field rate_card_id must be rejected)", rec.Code, http.StatusUnprocessableEntity)
	}
	if len(repo.slabsForCard(cardA.ID)) != 0 || len(repo.slabsForCard(cardB.ID)) != 0 {
		t.Error("a slab was created despite the rejected request")
	}
}

func TestCreateSlabHandler_ZeroMinWeightIsValid(t *testing.T) {
	// 0 is a legitimate min_weight (the first slab) — must not be
	// rejected as "missing" just because it's the zero value.
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2C, ZoneRelationship: RelationshipInter})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, CreateSlabHandler(repo), http.MethodPost, "/api/v1/rates/"+card.ID+"/slabs",
		`{"min_weight":0,"max_weight":5,"price":0}`, map[string]string{"rateCardID": card.ID})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestCreateSlabHandler_MissingFieldsRejected(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := CreateSlabHandler(repo)

	cases := []struct {
		name string
		body string
	}{
		{"missing min_weight", `{"max_weight":5,"price":50}`},
		{"missing price", `{"min_weight":0,"max_weight":5}`},
		{"negative min_weight", `{"min_weight":-1,"max_weight":5,"price":50}`},
		{"negative price", `{"min_weight":0,"max_weight":5,"price":-1}`},
		{"max_weight not greater than min_weight", `{"min_weight":5,"max_weight":5,"price":50}`},
		{"max_weight less than min_weight", `{"min_weight":10,"max_weight":5,"price":50}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, handler, http.MethodPost, "/api/v1/rates/"+card.ID+"/slabs", tc.body, map[string]string{"rateCardID": card.ID})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
		})
	}
}

func TestCreateSlabHandler_OverlapRejected(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	handler := CreateSlabHandler(repo)
	first := doRequest(t, handler, http.MethodPost, "/api/v1/rates/"+card.ID+"/slabs", `{"min_weight":0,"max_weight":5,"price":50}`, map[string]string{"rateCardID": card.ID})
	if first.Code != http.StatusCreated {
		t.Fatalf("first slab creation status = %d, want %d", first.Code, http.StatusCreated)
	}
	second := doRequest(t, handler, http.MethodPost, "/api/v1/rates/"+card.ID+"/slabs", `{"min_weight":3,"max_weight":10,"price":80}`, map[string]string{"rateCardID": card.ID})
	if second.Code != http.StatusConflict {
		t.Errorf("overlapping slab: status = %d, want %d", second.Code, http.StatusConflict)
	}
}

func TestCreateSlabHandler_UnknownRateCardReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, CreateSlabHandler(repo), http.MethodPost, "/api/v1/rates/missing/slabs",
		`{"min_weight":0,"max_weight":5,"price":50}`, map[string]string{"rateCardID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- ListSlabs ---

func TestListSlabsHandler_UnknownRateCardReturns404(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, ListSlabsHandler(repo), http.MethodGet, "/api/v1/rates/missing/slabs", "", map[string]string{"rateCardID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListSlabsHandler_EmptyCardReturnsEmptyList(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, ListSlabsHandler(repo), http.MethodGet, "/api/v1/rates/"+card.ID+"/slabs", "", map[string]string{"rateCardID": card.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (existing card with no slabs is 200, not 404)", rec.Code, http.StatusOK)
	}
	var list []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}

// --- UpdateSlab / DeleteSlab: ownership / path-integrity ---

func TestUpdateSlabHandler_WrongRateCardInPathReturns404(t *testing.T) {
	repo := newFakeRepo()
	cardA, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	cardB, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipInter})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	slab, err := repo.CreateSlab(context.Background(), cardA.ID, CreateSlabInput{MinWeight: 0, MaxWeight: f(5), Price: 50})
	if err != nil {
		t.Fatalf("seed slab create failed: %v", err)
	}

	rec := doRequest(t, UpdateSlabHandler(repo), http.MethodPut, "/api/v1/rates/"+cardB.ID+"/slabs/"+slab.ID,
		`{"min_weight":0,"max_weight":9,"price":999}`, map[string]string{"rateCardID": cardB.ID, "slabID": slab.ID})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	unchanged, _ := repo.FindSlabByID(context.Background(), slab.ID)
	if *unchanged.MaxWeight != 5 || unchanged.Price != 50 {
		t.Errorf("slab was modified despite the wrong-rate-card request: %+v", unchanged)
	}
}

func TestUpdateSlabHandler_UnknownSlabReturns404(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, UpdateSlabHandler(repo), http.MethodPut, "/api/v1/rates/"+card.ID+"/slabs/missing",
		`{"min_weight":0,"max_weight":5,"price":50}`, map[string]string{"rateCardID": card.ID, "slabID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteSlabHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	slab, err := repo.CreateSlab(context.Background(), card.ID, CreateSlabInput{MinWeight: 0, MaxWeight: f(5), Price: 50})
	if err != nil {
		t.Fatalf("seed slab create failed: %v", err)
	}

	rec := doRequest(t, DeleteSlabHandler(repo), http.MethodDelete, "/api/v1/rates/"+card.ID+"/slabs/"+slab.ID,
		"", map[string]string{"rateCardID": card.ID, "slabID": slab.ID})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, err := repo.FindSlabByID(context.Background(), slab.ID); err == nil {
		t.Error("slab still exists after deletion")
	}
}

func TestDeleteSlabHandler_WrongRateCardInPathReturns404(t *testing.T) {
	repo := newFakeRepo()
	cardA, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	cardB, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipInter})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	slab, err := repo.CreateSlab(context.Background(), cardA.ID, CreateSlabInput{MinWeight: 0, MaxWeight: f(5), Price: 50})
	if err != nil {
		t.Fatalf("seed slab create failed: %v", err)
	}

	rec := doRequest(t, DeleteSlabHandler(repo), http.MethodDelete, "/api/v1/rates/"+cardB.ID+"/slabs/"+slab.ID,
		"", map[string]string{"rateCardID": cardB.ID, "slabID": slab.ID})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if _, err := repo.FindSlabByID(context.Background(), slab.ID); err != nil {
		t.Error("slab was deleted despite the wrong-rate-card request")
	}
}

func TestDeleteSlabHandler_UnknownSlabReturns404(t *testing.T) {
	repo := newFakeRepo()
	card, err := repo.CreateRateCard(context.Background(), CreateRateCardInput{OrderType: OrderTypeB2B, ZoneRelationship: RelationshipIntra})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	rec := doRequest(t, DeleteSlabHandler(repo), http.MethodDelete, "/api/v1/rates/"+card.ID+"/slabs/missing",
		"", map[string]string{"rateCardID": card.ID, "slabID": "missing"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- Auth (route-level RBAC is exercised in tests/integration; this
// confirms handlers behave correctly once RequireAuth has run) ---

func TestCreateRateCardHandler_UnauthenticatedReturns401(t *testing.T) {
	repo := newFakeRepo()
	handler := auth.RequireAuth(testSecret)(CreateRateCardHandler(repo))
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/rates", `{"order_type":"B2B","zone_relationship":"INTRA"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCreateRateCardHandler_AuthenticatedAdminSucceeds(t *testing.T) {
	repo := newFakeRepo()
	handler := withAuth(t, "some-admin-id", users.RoleAdmin, CreateRateCardHandler(repo))
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/rates", `{"order_type":"B2B","zone_relationship":"INTRA"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestParseOrderType(t *testing.T) {
	for _, v := range []OrderType{OrderTypeB2B, OrderTypeB2C} {
		if _, ok := ParseOrderType(string(v)); !ok {
			t.Errorf("ParseOrderType(%q) rejected a valid value", v)
		}
	}
	for _, invalid := range []string{"b2b", "C2C", "", "B2B "} {
		if _, ok := ParseOrderType(invalid); ok {
			t.Errorf("ParseOrderType(%q) accepted an invalid value", invalid)
		}
	}
}

func TestParseZoneRelationship(t *testing.T) {
	for _, v := range []ZoneRelationship{RelationshipIntra, RelationshipInter} {
		if _, ok := ParseZoneRelationship(string(v)); !ok {
			t.Errorf("ParseZoneRelationship(%q) rejected a valid value", v)
		}
	}
	for _, invalid := range []string{"intra", "SIDEWAYS", ""} {
		if _, ok := ParseZoneRelationship(invalid); ok {
			t.Errorf("ParseZoneRelationship(%q) accepted an invalid value", invalid)
		}
	}
}
