package rescheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/rates"
	"lastmiletracker/internal/zones"
)

// --- fakeOrdersRepo ---

// fakeOrdersRepo is the minimal in-memory orders.Repository this
// package's handler tests need — just enough to exercise ownership and
// existence checks without a real database, the same split every prior
// module's handler_test.go uses (real Postgres behavior covered
// separately by tests/integration).
type fakeOrdersRepo struct {
	byID map[string]orders.Order
}

func newFakeOrdersRepo() *fakeOrdersRepo {
	return &fakeOrdersRepo{byID: map[string]orders.Order{}}
}

func (f *fakeOrdersRepo) seed(id, customerID, status string, assignedAgentID *string) orders.Order {
	o := orders.Order{
		ID: id, CustomerID: customerID, CreatedBy: customerID,
		OrderType: rates.OrderTypeB2C, PaymentType: rates.PaymentTypePrepaid,
		PickupAreaID: "area-1", DropAreaID: "area-1", PickupZoneID: "zone-1", DropZoneID: "zone-1",
		ZoneRelationship: zones.RelationshipIntra,
		LengthCM:         1, BreadthCM: 1, HeightCM: 1, ActualWeightKG: 1,
		VolumetricWeightKG: 1, ChargeableWeightKG: 1,
		RateCardID: "card-1", BaseRate: 50, CODSurcharge: 0, FinalAmount: 50,
		AssignedAgentID: assignedAgentID,
		Status:          status,
		CreatedAt:       time.Now(),
	}
	f.byID[id] = o
	return o
}

func (f *fakeOrdersRepo) CreateOrder(_ context.Context, _ orders.CreateOrderInput) (orders.Order, error) {
	return orders.Order{}, errors.New("not implemented in fake")
}

func (f *fakeOrdersRepo) ListOrdersForCustomer(_ context.Context, _ string) ([]orders.Order, error) {
	return nil, errors.New("not implemented in fake")
}

func (f *fakeOrdersRepo) ListAllOrders(_ context.Context) ([]orders.Order, error) {
	return nil, errors.New("not implemented in fake")
}

func (f *fakeOrdersRepo) ListOrdersForAgent(_ context.Context, _ string) ([]orders.Order, error) {
	return nil, errors.New("not implemented in fake")
}

func (f *fakeOrdersRepo) FindOrderByID(_ context.Context, id string) (orders.Order, error) {
	o, ok := f.byID[id]
	if !ok {
		return orders.Order{}, orders.ErrOrderNotFound
	}
	return o, nil
}

// --- fakeReschedulingRepo ---

// fakeReschedulingRepo is an in-memory Repository used only to test
// RescheduleHandler/ListReschedulesHandler's own concerns — request
// validation, RBAC (enforced by routes.go's middleware, not this fake),
// and status-code mapping — never the real transactional/locking logic,
// which is only meaningfully exercised against real Postgres (see
// tests/integration/rescheduling_integration_test.go).
type fakeReschedulingRepo struct {
	orders      map[string]orders.Order
	reschedules map[string][]Reschedule

	rescheduleErr error
	// lastInput records the input RescheduleHandler forwarded, so tests
	// can confirm the handler passed the real caller's identity through
	// unmodified rather than trusting anything from the request body.
	lastInput RescheduleInput
	nextID    int
}

func newFakeReschedulingRepo() *fakeReschedulingRepo {
	return &fakeReschedulingRepo{orders: map[string]orders.Order{}, reschedules: map[string][]Reschedule{}}
}

func (f *fakeReschedulingRepo) Reschedule(_ context.Context, input RescheduleInput) (orders.Order, error) {
	f.lastInput = input
	if f.rescheduleErr != nil {
		return orders.Order{}, f.rescheduleErr
	}
	o, ok := f.orders[input.OrderID]
	if !ok {
		return orders.Order{}, ErrOrderNotFound
	}
	if o.Status != "FAILED" {
		return orders.Order{}, ErrInvalidTransition
	}
	o.Status = "RESCHEDULED"
	f.orders[input.OrderID] = o

	f.nextID++
	f.reschedules[input.OrderID] = append(f.reschedules[input.OrderID], Reschedule{
		ID: fmt.Sprintf("fake-reschedule-%d", f.nextID), OrderID: input.OrderID,
		RequestedBy: input.ActorID, RequestedDate: input.RequestedDate,
		Reason: input.Reason, CreatedAt: time.Now(),
	})
	return o, nil
}

func (f *fakeReschedulingRepo) ListReschedules(_ context.Context, orderID string) ([]Reschedule, error) {
	return f.reschedules[orderID], nil
}

var errFakeInternal = errors.New("fake internal error")
