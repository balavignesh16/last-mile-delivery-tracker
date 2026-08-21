package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"lastmiletracker/internal/users"
)

// fakeOrder is the minimal slice of an orders row this package's fake
// needs — just enough to exercise Transition/OrderCustomerID's logic
// without a real database, the same split every prior module's
// handler_test.go uses (real Postgres behavior covered separately by
// tests/integration).
type fakeOrder struct {
	customerID string
	status     Status
}

type fakeRepo struct {
	orders map[string]*fakeOrder
	events map[string][]Event
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{orders: map[string]*fakeOrder{}, events: map[string][]Event{}}
}

func (f *fakeRepo) seedOrder(id, customerID string, status Status) {
	f.orders[id] = &fakeOrder{customerID: customerID, status: status}
}

func (f *fakeRepo) Transition(_ context.Context, orderID, actorID string, role users.Role, newStatus Status, metadata json.RawMessage) (Event, error) {
	o, ok := f.orders[orderID]
	if !ok {
		return Event{}, ErrOrderNotFound
	}
	if !IsValidTransition(o.status, newStatus) {
		return Event{}, ErrInvalidTransition
	}
	if !IsRoleAuthorized(o.status, newStatus, role) {
		return Event{}, ErrForbiddenTransition
	}

	prev := string(o.status)
	o.status = newStatus

	f.nextID++
	e := Event{
		ID:             fmt.Sprintf("fake-event-%d", f.nextID),
		OrderID:        orderID,
		PreviousStatus: &prev,
		NewStatus:      string(newStatus),
		ActorID:        actorID,
		Metadata:       metadata,
		CreatedAt:      time.Now(),
	}
	f.events[orderID] = append(f.events[orderID], e)
	return e, nil
}

func (f *fakeRepo) OrderCustomerID(_ context.Context, orderID string) (string, error) {
	o, ok := f.orders[orderID]
	if !ok {
		return "", ErrOrderNotFound
	}
	return o.customerID, nil
}

func (f *fakeRepo) ListEvents(_ context.Context, orderID string) ([]Event, error) {
	return f.events[orderID], nil
}
