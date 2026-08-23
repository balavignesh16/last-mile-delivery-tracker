package rates

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"lastmiletracker/internal/zones"
)

// --- Pure calculation functions ---

func TestCalculateVolumetricWeight(t *testing.T) {
	cases := []struct {
		length, breadth, height float64
		want                    float64
	}{
		{50, 40, 30, 12}, // blueprint's own worked example
		{10, 10, 10, 0.2},
		{1, 1, 1, 0.0002},
	}
	for _, tc := range cases {
		got := CalculateVolumetricWeight(tc.length, tc.breadth, tc.height)
		if got != tc.want {
			t.Errorf("CalculateVolumetricWeight(%v,%v,%v) = %v, want %v", tc.length, tc.breadth, tc.height, got, tc.want)
		}
	}
}

func TestCalculateChargeableWeight(t *testing.T) {
	cases := []struct {
		label              string
		actual, volumetric float64
		want               float64
	}{
		{"actual greater than volumetric", 10, 2, 10},
		{"volumetric greater than actual", 2, 10, 10},
		{"actual equals volumetric", 5, 5, 5},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := CalculateChargeableWeight(tc.actual, tc.volumetric)
			if got != tc.want {
				t.Errorf("CalculateChargeableWeight(%v,%v) = %v, want %v", tc.actual, tc.volumetric, got, tc.want)
			}
		})
	}
}

func TestParsePaymentType(t *testing.T) {
	if pt, ok := ParsePaymentType("PREPAID"); !ok || pt != PaymentTypePrepaid {
		t.Errorf("ParsePaymentType(PREPAID) = %v, %v", pt, ok)
	}
	if pt, ok := ParsePaymentType("COD"); !ok || pt != PaymentTypeCOD {
		t.Errorf("ParsePaymentType(COD) = %v, %v", pt, ok)
	}
	if _, ok := ParsePaymentType("CASH"); ok {
		t.Error("ParsePaymentType(CASH) succeeded, want failure")
	}
	if _, ok := ParsePaymentType(""); ok {
		t.Error("ParsePaymentType(\"\") succeeded, want failure")
	}
}

func mustFloat(v float64) *float64 { return &v }

// slabsFixture is the demo configuration used throughout M05's own docs:
// 0-5, 5-10, 10-15, 15-20, then open-ended.
func slabsFixture() []Slab {
	return []Slab{
		{ID: "s1", MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
		{ID: "s2", MinWeight: 5, MaxWeight: mustFloat(10), Price: 80},
		{ID: "s3", MinWeight: 10, MaxWeight: mustFloat(15), Price: 110},
		{ID: "s4", MinWeight: 15, MaxWeight: mustFloat(20), Price: 140},
		{ID: "s5", MinWeight: 20, MaxWeight: nil, Price: 200},
	}
}

func TestSelectSlab_Boundaries(t *testing.T) {
	slabs := slabsFixture()
	cases := []struct {
		weight float64
		wantID string
		label  string
	}{
		{4.999, "s1", "just under the first boundary"},
		{5.000, "s2", "exact boundary belongs to the next slab, not the current one"},
		{9.999, "s2", "just under the second boundary"},
		{10.000, "s3", "exact boundary belongs to the next slab"},
		{0, "s1", "exact minimum of the first slab is inclusive"},
		{19.999, "s4", "just under the last closed boundary"},
		{20.000, "s5", "exact minimum of the open-ended slab is inclusive"},
		{1000, "s5", "far above every closed slab still matches the open-ended one"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got, err := SelectSlab(tc.weight, slabs)
			if err != nil {
				t.Fatalf("SelectSlab(%v) error: %v", tc.weight, err)
			}
			if got.ID != tc.wantID {
				t.Errorf("SelectSlab(%v) = %v, want %v", tc.weight, got.ID, tc.wantID)
			}
		})
	}
}

func TestSelectSlab_NoMatch(t *testing.T) {
	cases := []struct {
		label  string
		weight float64
		slabs  []Slab
	}{
		{
			"weight below the first configured slab",
			1,
			[]Slab{{ID: "s1", MinWeight: 2, MaxWeight: mustFloat(5), Price: 50}},
		},
		{
			"weight in a gap between two slabs",
			7,
			[]Slab{
				{ID: "s1", MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
				{ID: "s2", MinWeight: 10, MaxWeight: mustFloat(15), Price: 110},
			},
		},
		{
			"weight above every closed slab, no open-ended slab configured",
			25,
			[]Slab{{ID: "s1", MinWeight: 0, MaxWeight: mustFloat(20), Price: 60}},
		},
		{
			"no slabs configured at all",
			5,
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			_, err := SelectSlab(tc.weight, tc.slabs)
			if !errors.Is(err, ErrNoMatchingSlab) {
				t.Errorf("SelectSlab(%v) error = %v, want ErrNoMatchingSlab", tc.weight, err)
			}
		})
	}
}

// --- fakeZonesRepo: an in-memory zones.Repository, local to this
// package's tests since zones' own fakeRepo is unexported and internal
// to the zones package's test files. ---

type fakeZonesRepo struct {
	zonesByID map[string]zones.Zone
	areasByID map[string]zones.Area
	nextID    int
}

func newFakeZonesRepo() *fakeZonesRepo {
	return &fakeZonesRepo{zonesByID: map[string]zones.Zone{}, areasByID: map[string]zones.Area{}}
}

func (f *fakeZonesRepo) CreateZone(_ context.Context, input zones.CreateZoneInput) (zones.Zone, error) {
	f.nextID++
	z := zones.Zone{ID: fmt.Sprintf("fake-zone-%d", f.nextID), Name: input.Name, Active: true, CreatedAt: time.Now()}
	f.zonesByID[z.ID] = z
	return z, nil
}

func (f *fakeZonesRepo) ListZones(_ context.Context) ([]zones.Zone, error) {
	out := make([]zones.Zone, 0, len(f.zonesByID))
	for _, z := range f.zonesByID {
		out = append(out, z)
	}
	return out, nil
}

func (f *fakeZonesRepo) FindZoneByID(_ context.Context, id string) (zones.Zone, error) {
	z, ok := f.zonesByID[id]
	if !ok {
		return zones.Zone{}, zones.ErrZoneNotFound
	}
	return z, nil
}

func (f *fakeZonesRepo) UpdateZone(_ context.Context, id string, update zones.ZoneUpdate) (zones.Zone, error) {
	z, ok := f.zonesByID[id]
	if !ok {
		return zones.Zone{}, zones.ErrZoneNotFound
	}
	z.Name = update.Name
	if update.Active != nil {
		z.Active = *update.Active
	}
	f.zonesByID[id] = z
	return z, nil
}

func (f *fakeZonesRepo) CreateArea(_ context.Context, zoneID string, input zones.CreateAreaInput) (zones.Area, error) {
	if _, ok := f.zonesByID[zoneID]; !ok {
		return zones.Area{}, zones.ErrZoneNotFound
	}
	f.nextID++
	a := zones.Area{ID: fmt.Sprintf("fake-area-%d", f.nextID), Name: input.Name, ZoneID: zoneID, Active: true, CreatedAt: time.Now()}
	f.areasByID[a.ID] = a
	return a, nil
}

func (f *fakeZonesRepo) ListAreasByZone(_ context.Context, zoneID string) ([]zones.Area, error) {
	var out []zones.Area
	for _, a := range f.areasByID {
		if a.ZoneID == zoneID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeZonesRepo) FindAreaByID(_ context.Context, id string) (zones.Area, error) {
	a, ok := f.areasByID[id]
	if !ok {
		return zones.Area{}, zones.ErrAreaNotFound
	}
	return a, nil
}

func (f *fakeZonesRepo) UpdateArea(_ context.Context, areaID string, update zones.AreaUpdate) (zones.Area, error) {
	a, ok := f.areasByID[areaID]
	if !ok {
		return zones.Area{}, zones.ErrAreaNotFound
	}
	a.Name = update.Name
	if update.Active != nil {
		a.Active = *update.Active
	}
	f.areasByID[areaID] = a
	return a, nil
}

// --- CalculateQuote: Go-level end-to-end coverage (M04 -> M05 -> M06),
// independent of the HTTP layer. tests/integration's quote tests cover
// the same scenarios again through the real router and real Postgres —
// this file proves the calculation logic itself is correct in isolation.

func setupQuoteFixture(t *testing.T) (*fakeZonesRepo, *fakeRepo, zones.Area, zones.Area) {
	t.Helper()
	zRepo := newFakeZonesRepo()
	rRepo := newFakeRepo()
	ctx := context.Background()

	zoneA, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: "ZoneA"})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	zoneB, err := zRepo.CreateZone(ctx, zones.CreateZoneInput{Name: "ZoneB"})
	if err != nil {
		t.Fatalf("CreateZone() error: %v", err)
	}
	areaA, err := zRepo.CreateArea(ctx, zoneA.ID, zones.CreateAreaInput{Name: "A1"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	areaB, err := zRepo.CreateArea(ctx, zoneB.ID, zones.CreateAreaInput{Name: "B1"})
	if err != nil {
		t.Fatalf("CreateArea() error: %v", err)
	}
	return zRepo, rRepo, areaA, areaB
}

func activateCardWithSlabs(t *testing.T, rRepo *fakeRepo, orderType OrderType, relationship ZoneRelationship, codSurcharge float64, slabs []CreateSlabInput) RateCard {
	t.Helper()
	ctx := context.Background()
	card, err := rRepo.CreateRateCard(ctx, CreateRateCardInput{OrderType: orderType, ZoneRelationship: relationship, CODSurcharge: codSurcharge})
	if err != nil {
		t.Fatalf("CreateRateCard() error: %v", err)
	}
	active := true
	card, err = rRepo.UpdateRateCard(ctx, card.ID, RateCardUpdate{CODSurcharge: codSurcharge, Active: &active})
	if err != nil {
		t.Fatalf("UpdateRateCard() error: %v", err)
	}
	for _, s := range slabs {
		if _, err := rRepo.CreateSlab(ctx, card.ID, s); err != nil {
			t.Fatalf("CreateSlab() error: %v", err)
		}
	}
	return card
}

func TestCalculateQuote_GoldenIntraPrepaid(t *testing.T) {
	zRepo, rRepo, areaA, _ := setupQuoteFixture(t)
	activateCardWithSlabs(t, rRepo, OrderTypeB2B, RelationshipIntra, 20, []CreateSlabInput{
		{MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
		{MinWeight: 5, MaxWeight: mustFloat(10), Price: 80},
	})

	result, err := CalculateQuote(context.Background(), zRepo, rRepo, QuoteInput{
		PickupAreaID: areaA.ID, DropAreaID: areaA.ID,
		OrderType: OrderTypeB2B, PaymentType: PaymentTypePrepaid,
		LengthCM: 10, BreadthCM: 10, HeightCM: 10, ActualWeightKG: 7,
	})
	if err != nil {
		t.Fatalf("CalculateQuote() error: %v", err)
	}
	if result.ZoneRelationship != RelationshipIntra {
		t.Errorf("ZoneRelationship = %v, want INTRA", result.ZoneRelationship)
	}
	if result.VolumetricWeightKG != 0.2 {
		t.Errorf("VolumetricWeightKG = %v, want 0.2", result.VolumetricWeightKG)
	}
	if result.ChargeableWeightKG != 7 {
		t.Errorf("ChargeableWeightKG = %v, want 7", result.ChargeableWeightKG)
	}
	if result.BaseRate != 80 {
		t.Errorf("BaseRate = %v, want 80", result.BaseRate)
	}
	if result.CODSurcharge != 0 {
		t.Errorf("CODSurcharge = %v, want 0 (PREPAID)", result.CODSurcharge)
	}
	if result.FinalAmount != 80 {
		t.Errorf("FinalAmount = %v, want 80", result.FinalAmount)
	}
}

func TestCalculateQuote_GoldenInterCOD(t *testing.T) {
	zRepo, rRepo, areaA, areaB := setupQuoteFixture(t)
	activateCardWithSlabs(t, rRepo, OrderTypeB2C, RelationshipInter, 25, []CreateSlabInput{
		{MinWeight: 0, MaxWeight: nil, Price: 60},
	})

	result, err := CalculateQuote(context.Background(), zRepo, rRepo, QuoteInput{
		PickupAreaID: areaA.ID, DropAreaID: areaB.ID,
		OrderType: OrderTypeB2C, PaymentType: PaymentTypeCOD,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 3,
	})
	if err != nil {
		t.Fatalf("CalculateQuote() error: %v", err)
	}
	if result.ZoneRelationship != RelationshipInter {
		t.Errorf("ZoneRelationship = %v, want INTER", result.ZoneRelationship)
	}
	if result.CODSurcharge != 25 {
		t.Errorf("CODSurcharge = %v, want 25 (COD)", result.CODSurcharge)
	}
	if result.FinalAmount != 85 {
		t.Errorf("FinalAmount = %v, want 85 (60 base + 25 COD)", result.FinalAmount)
	}
}

func TestCalculateQuote_UnknownAreaFailsExplicitly(t *testing.T) {
	zRepo, rRepo, areaA, _ := setupQuoteFixture(t)
	_, err := CalculateQuote(context.Background(), zRepo, rRepo, QuoteInput{
		PickupAreaID: areaA.ID, DropAreaID: "does-not-exist",
		OrderType: OrderTypeB2B, PaymentType: PaymentTypePrepaid,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
	})
	if !errors.Is(err, zones.ErrAreaNotFound) {
		t.Errorf("error = %v, want wrapped zones.ErrAreaNotFound", err)
	}
}

func TestCalculateQuote_InactiveZoneFailsExplicitly(t *testing.T) {
	zRepo, rRepo, areaA, _ := setupQuoteFixture(t)
	inactive := false
	zoneA := zRepo.zonesByID[areaA.ZoneID]
	if _, err := zRepo.UpdateZone(context.Background(), zoneA.ID, zones.ZoneUpdate{Name: zoneA.Name, Active: &inactive}); err != nil {
		t.Fatalf("UpdateZone() error: %v", err)
	}

	_, err := CalculateQuote(context.Background(), zRepo, rRepo, QuoteInput{
		PickupAreaID: areaA.ID, DropAreaID: areaA.ID,
		OrderType: OrderTypeB2B, PaymentType: PaymentTypePrepaid,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
	})
	if !errors.Is(err, zones.ErrZoneInactive) {
		t.Errorf("error = %v, want wrapped zones.ErrZoneInactive", err)
	}
}

func TestCalculateQuote_NoActiveRateCardFailsExplicitly(t *testing.T) {
	zRepo, rRepo, areaA, _ := setupQuoteFixture(t)
	// No rate card created at all for (B2B, INTRA).
	_, err := CalculateQuote(context.Background(), zRepo, rRepo, QuoteInput{
		PickupAreaID: areaA.ID, DropAreaID: areaA.ID,
		OrderType: OrderTypeB2B, PaymentType: PaymentTypePrepaid,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
	})
	if !errors.Is(err, ErrRateCardNotFound) {
		t.Errorf("error = %v, want ErrRateCardNotFound", err)
	}
}

func TestCalculateQuote_NoMatchingSlabFailsExplicitly(t *testing.T) {
	zRepo, rRepo, areaA, _ := setupQuoteFixture(t)
	activateCardWithSlabs(t, rRepo, OrderTypeB2B, RelationshipIntra, 0, []CreateSlabInput{
		{MinWeight: 0, MaxWeight: mustFloat(5), Price: 50},
	})

	_, err := CalculateQuote(context.Background(), zRepo, rRepo, QuoteInput{
		PickupAreaID: areaA.ID, DropAreaID: areaA.ID,
		OrderType: OrderTypeB2B, PaymentType: PaymentTypePrepaid,
		LengthCM: 1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 50,
	})
	if !errors.Is(err, ErrNoMatchingSlab) {
		t.Errorf("error = %v, want ErrNoMatchingSlab", err)
	}
}
