package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/users"
	"lastmiletracker/internal/zones"
)

// This file defines in-memory fakes for every cross-package interface
// CreateOrderHandler depends on (users.Repository, zones.Repository,
// rates.Repository), plus this package's own Repository. Each is
// unexported to its owning package's test files, so — same as
// internal/rates writing its own fakeZonesRepo rather than reaching into
// internal/zones' unexported fake — this package writes its own of all
// three rather than importing test-only code across a package boundary.

// --- fakeUsersRepo ---

type fakeUsersRepo struct {
	byID map[string]users.User
}

func newFakeUsersRepo() *fakeUsersRepo {
	return &fakeUsersRepo{byID: map[string]users.User{}}
}

func (f *fakeUsersRepo) seed(id string, role users.Role) users.User {
	u := users.User{ID: id, Email: id + "@example.com", FullName: "Test " + id, Role: role, CreatedAt: time.Now()}
	f.byID[id] = u
	return u
}

func (f *fakeUsersRepo) Create(_ context.Context, u users.NewUser) (users.User, error) {
	return users.User{}, errors.New("not implemented in fake")
}

func (f *fakeUsersRepo) FindByEmail(_ context.Context, email string) (users.User, error) {
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return users.User{}, users.ErrNotFound
}

func (f *fakeUsersRepo) FindByID(_ context.Context, id string) (users.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsersRepo) Update(_ context.Context, id string, update users.ProfileUpdate) (users.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	u.FullName = update.FullName
	u.Phone = update.Phone
	f.byID[id] = u
	return u, nil
}

// --- fakeZonesRepo ---

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
	a := zones.Area{ID: fmt.Sprintf("fake-area-%d", f.nextID), Name: input.Name, ZoneID: zoneID, CreatedAt: time.Now()}
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
	f.areasByID[areaID] = a
	return a, nil
}

// --- fakeRatesRepo ---

type fakeRatesRepo struct {
	cardsByID map[string]rates.RateCard
	slabsByID map[string]rates.Slab
	nextID    int
}

func newFakeRatesRepo() *fakeRatesRepo {
	return &fakeRatesRepo{cardsByID: map[string]rates.RateCard{}, slabsByID: map[string]rates.Slab{}}
}

func (f *fakeRatesRepo) CreateRateCard(_ context.Context, input rates.CreateRateCardInput) (rates.RateCard, error) {
	f.nextID++
	rc := rates.RateCard{ID: fmt.Sprintf("fake-card-%d", f.nextID), OrderType: input.OrderType, ZoneRelationship: input.ZoneRelationship, CODSurcharge: input.CODSurcharge, Active: false, CreatedAt: time.Now()}
	f.cardsByID[rc.ID] = rc
	return rc, nil
}

func (f *fakeRatesRepo) ListRateCards(_ context.Context) ([]rates.RateCard, error) {
	out := make([]rates.RateCard, 0, len(f.cardsByID))
	for _, rc := range f.cardsByID {
		out = append(out, rc)
	}
	return out, nil
}

func (f *fakeRatesRepo) FindRateCardByID(_ context.Context, id string) (rates.RateCard, error) {
	rc, ok := f.cardsByID[id]
	if !ok {
		return rates.RateCard{}, rates.ErrRateCardNotFound
	}
	return rc, nil
}

func (f *fakeRatesRepo) UpdateRateCard(_ context.Context, id string, update rates.RateCardUpdate) (rates.RateCard, error) {
	rc, ok := f.cardsByID[id]
	if !ok {
		return rates.RateCard{}, rates.ErrRateCardNotFound
	}
	rc.CODSurcharge = update.CODSurcharge
	if update.Active != nil {
		rc.Active = *update.Active
	}
	f.cardsByID[id] = rc
	return rc, nil
}

func (f *fakeRatesRepo) FindActiveCard(_ context.Context, orderType rates.OrderType, zoneRelationship rates.ZoneRelationship) (rates.RateCard, error) {
	for _, rc := range f.cardsByID {
		if rc.Active && rc.OrderType == orderType && rc.ZoneRelationship == zoneRelationship {
			return rc, nil
		}
	}
	return rates.RateCard{}, rates.ErrRateCardNotFound
}

func (f *fakeRatesRepo) CreateSlab(_ context.Context, rateCardID string, input rates.CreateSlabInput) (rates.Slab, error) {
	if _, ok := f.cardsByID[rateCardID]; !ok {
		return rates.Slab{}, rates.ErrRateCardNotFound
	}
	f.nextID++
	s := rates.Slab{ID: fmt.Sprintf("fake-slab-%d", f.nextID), RateCardID: rateCardID, MinWeight: input.MinWeight, MaxWeight: input.MaxWeight, Price: input.Price, CreatedAt: time.Now()}
	f.slabsByID[s.ID] = s
	return s, nil
}

func (f *fakeRatesRepo) ListSlabsByRateCard(_ context.Context, rateCardID string) ([]rates.Slab, error) {
	var out []rates.Slab
	for _, s := range f.slabsByID {
		if s.RateCardID == rateCardID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRatesRepo) FindSlabByID(_ context.Context, id string) (rates.Slab, error) {
	s, ok := f.slabsByID[id]
	if !ok {
		return rates.Slab{}, rates.ErrSlabNotFound
	}
	return s, nil
}

func (f *fakeRatesRepo) UpdateSlab(_ context.Context, slabID string, update rates.SlabUpdate) (rates.Slab, error) {
	s, ok := f.slabsByID[slabID]
	if !ok {
		return rates.Slab{}, rates.ErrSlabNotFound
	}
	s.MinWeight = update.MinWeight
	s.MaxWeight = update.MaxWeight
	s.Price = update.Price
	f.slabsByID[slabID] = s
	return s, nil
}

func (f *fakeRatesRepo) DeleteSlab(_ context.Context, slabID string) error {
	if _, ok := f.slabsByID[slabID]; !ok {
		return rates.ErrSlabNotFound
	}
	delete(f.slabsByID, slabID)
	return nil
}

// activateCardWithSlabs is a small test helper shared by handler tests:
// create a rate card, activate it, attach slabs, in one call.
func activateCardWithSlabs(rRepo *fakeRatesRepo, orderType rates.OrderType, relationship rates.ZoneRelationship, codSurcharge float64, slabs []rates.CreateSlabInput) rates.RateCard {
	ctx := context.Background()
	card, _ := rRepo.CreateRateCard(ctx, rates.CreateRateCardInput{OrderType: orderType, ZoneRelationship: relationship, CODSurcharge: codSurcharge})
	active := true
	card, _ = rRepo.UpdateRateCard(ctx, card.ID, rates.RateCardUpdate{CODSurcharge: codSurcharge, Active: &active})
	for _, s := range slabs {
		_, _ = rRepo.CreateSlab(ctx, card.ID, s)
	}
	return card
}

func mustFloat(v float64) *float64 { return &v }

// --- fakeOrdersRepo ---

type fakeOrdersRepo struct {
	byID   map[string]Order
	nextID int
}

func newFakeOrdersRepo() *fakeOrdersRepo {
	return &fakeOrdersRepo{byID: map[string]Order{}}
}

func (f *fakeOrdersRepo) CreateOrder(_ context.Context, input CreateOrderInput) (Order, error) {
	f.nextID++
	q := input.Quote
	o := Order{
		ID:                 fmt.Sprintf("fake-order-%d", f.nextID),
		CustomerID:         input.CustomerID,
		CreatedBy:          input.CreatedBy,
		OrderType:          q.OrderType,
		PaymentType:        q.PaymentType,
		PickupAreaID:       q.PickupArea.ID,
		DropAreaID:         q.DropArea.ID,
		PickupZoneID:       q.PickupZone.ID,
		DropZoneID:         q.DropZone.ID,
		ZoneRelationship:   q.ZoneRelationship,
		LengthCM:           q.LengthCM,
		BreadthCM:          q.BreadthCM,
		HeightCM:           q.HeightCM,
		ActualWeightKG:     q.ActualWeightKG,
		VolumetricWeightKG: q.VolumetricWeightKG,
		ChargeableWeightKG: q.ChargeableWeightKG,
		RateCardID:         q.RateCard.ID,
		BaseRate:           q.BaseRate,
		CODSurcharge:       q.CODSurcharge,
		FinalAmount:        q.FinalAmount,
		Status:             StatusCreated,
		CreatedAt:          time.Now(),
	}
	f.byID[o.ID] = o
	return o, nil
}

func (f *fakeOrdersRepo) ListOrdersForCustomer(_ context.Context, customerID string) ([]Order, error) {
	var out []Order
	for _, o := range f.byID {
		if o.CustomerID == customerID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeOrdersRepo) ListAllOrders(_ context.Context, filter OrderFilter) ([]Order, error) {
	out := make([]Order, 0, len(f.byID))
	for _, o := range f.byID {
		if filter.Status != "" && o.Status != filter.Status {
			continue
		}
		if filter.ZoneID != "" && o.PickupZoneID != filter.ZoneID && o.DropZoneID != filter.ZoneID {
			continue
		}
		if filter.AgentID != "" && (o.AssignedAgentID == nil || *o.AssignedAgentID != filter.AgentID) {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func (f *fakeOrdersRepo) ListOrdersForAgent(_ context.Context, agentID string) ([]Order, error) {
	var out []Order
	for _, o := range f.byID {
		if o.AssignedAgentID != nil && *o.AssignedAgentID == agentID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeOrdersRepo) FindOrderByID(_ context.Context, id string) (Order, error) {
	o, ok := f.byID[id]
	if !ok {
		return Order{}, ErrOrderNotFound
	}
	return o, nil
}

// assign is a test-only helper (not part of the Repository interface)
// letting handler tests seed an already-assigned order directly,
// without going through the real M09 assignment flow.
func (f *fakeOrdersRepo) assign(orderID, agentID string) {
	o := f.byID[orderID]
	o.AssignedAgentID = &agentID
	f.byID[orderID] = o
}

// setStatus is a test-only helper (M12) letting handler tests seed an
// order already sitting at a given status, without going through the
// real M08 transition flow — the same "assign directly" convention
// assign already established above.
func (f *fakeOrdersRepo) setStatus(orderID, status string) {
	o := f.byID[orderID]
	o.Status = status
	f.byID[orderID] = o
}

// setZones is a test-only helper (M12) letting handler tests seed an
// order's pickup/drop zone directly, for zone-filter tests.
func (f *fakeOrdersRepo) setZones(orderID, pickupZoneID, dropZoneID string) {
	o := f.byID[orderID]
	o.PickupZoneID = pickupZoneID
	o.DropZoneID = dropZoneID
	f.byID[orderID] = o
}

// --- fakeAgentsRepo ---

type fakeAgentsRepo struct {
	byID     map[string]agents.AgentWithUser
	byUserID map[string]string // userID -> agentID
}

func newFakeAgentsRepo() *fakeAgentsRepo {
	return &fakeAgentsRepo{byID: map[string]agents.AgentWithUser{}, byUserID: map[string]string{}}
}

func (f *fakeAgentsRepo) seed(agentID, userID string) agents.AgentWithUser {
	a := agents.AgentWithUser{ID: agentID, UserID: userID, Availability: agents.AvailabilityAvailable, Active: true, CreatedAt: time.Now()}
	f.byID[agentID] = a
	f.byUserID[userID] = agentID
	return a
}

func (f *fakeAgentsRepo) Create(_ context.Context, _ agents.CreateAgentInput) (agents.AgentWithUser, error) {
	return agents.AgentWithUser{}, errors.New("not implemented in fake")
}

func (f *fakeAgentsRepo) List(_ context.Context) ([]agents.AgentWithUser, error) {
	out := make([]agents.AgentWithUser, 0, len(f.byID))
	for _, a := range f.byID {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeAgentsRepo) FindByID(_ context.Context, id string) (agents.AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return agents.AgentWithUser{}, agents.ErrNotFound
	}
	return a, nil
}

func (f *fakeAgentsRepo) FindByUserID(_ context.Context, userID string) (agents.AgentWithUser, error) {
	agentID, ok := f.byUserID[userID]
	if !ok {
		return agents.AgentWithUser{}, agents.ErrNotFound
	}
	return f.byID[agentID], nil
}

func (f *fakeAgentsRepo) UpdateAvailability(_ context.Context, id string, availability agents.Availability) (agents.AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return agents.AgentWithUser{}, agents.ErrNotFound
	}
	a.Availability = availability
	f.byID[id] = a
	return a, nil
}

func (f *fakeAgentsRepo) UpdateLocation(_ context.Context, id string, lat, lng float64) (agents.AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return agents.AgentWithUser{}, agents.ErrNotFound
	}
	a.CurrentLat, a.CurrentLng = &lat, &lng
	f.byID[id] = a
	return a, nil
}

func (f *fakeAgentsRepo) UpdateZone(_ context.Context, id, zoneID string) (agents.AgentWithUser, error) {
	a, ok := f.byID[id]
	if !ok {
		return agents.AgentWithUser{}, agents.ErrNotFound
	}
	a.CurrentZoneID = &zoneID
	f.byID[id] = a
	return a, nil
}
