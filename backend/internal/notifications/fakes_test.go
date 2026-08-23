package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
)

// --- fakeNotificationsRepo ---

// fakeNotificationsRepo is an in-memory Repository, mirroring the exact
// claim-then-resolve semantics PostgresRepository's own unique index
// enforces — a map keyed by (trackingEventID, channel) is sufficient to
// prove Service's idempotency logic without a real database (real
// Postgres concurrency behavior is covered separately by
// tests/integration/notifications_integration_test.go).
type fakeNotificationsRepo struct {
	mu      sync.Mutex
	claimed map[string]string // "trackingEventID|channel" -> notification id
	nextID  int

	resolved map[string]Status // notification id -> final status
	claimErr error
}

func newFakeNotificationsRepo() *fakeNotificationsRepo {
	return &fakeNotificationsRepo{claimed: map[string]string{}, resolved: map[string]Status{}}
}

func (f *fakeNotificationsRepo) Claim(_ context.Context, input ClaimInput) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return "", false, f.claimErr
	}
	key := input.TrackingEventID + "|" + string(input.Channel)
	if _, exists := f.claimed[key]; exists {
		return "", false, nil
	}
	f.nextID++
	id := fmt.Sprintf("fake-notification-%d", f.nextID)
	f.claimed[key] = id
	return id, true, nil
}

func (f *fakeNotificationsRepo) Resolve(_ context.Context, id string, status Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved[id] = status
	return nil
}

func (f *fakeNotificationsRepo) statusOf(id string) (Status, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.resolved[id]
	return s, ok
}

// --- fakeEmailProvider / fakeSmsProvider ---

type sentMessage struct {
	To       string
	Subject  string
	Body     string
	HTMLBody string
}

type fakeEmailProvider struct {
	mu      sync.Mutex
	sent    []sentMessage
	sendErr error
}

func (f *fakeEmailProvider) SendEmail(_ context.Context, to, subject, body, htmlBody string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, sentMessage{To: to, Subject: subject, Body: body, HTMLBody: htmlBody})
	return nil
}

func (f *fakeEmailProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fakeSmsProvider struct {
	mu      sync.Mutex
	sent    []sentMessage
	sendErr error
}

func (f *fakeSmsProvider) SendSMS(_ context.Context, to, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, sentMessage{To: to, Body: body})
	return nil
}

func (f *fakeSmsProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// --- fakeOrdersRepo ---

type fakeOrdersRepo struct {
	byID map[string]orders.Order
}

func newFakeOrdersRepo() *fakeOrdersRepo {
	return &fakeOrdersRepo{byID: map[string]orders.Order{}}
}

func (f *fakeOrdersRepo) seed(id, customerID string) {
	f.byID[id] = orders.Order{ID: id, CustomerID: customerID, Status: "CREATED", CreatedAt: time.Now()}
}

func (f *fakeOrdersRepo) CreateOrder(_ context.Context, _ orders.CreateOrderInput) (orders.Order, error) {
	return orders.Order{}, errors.New("not implemented in fake")
}
func (f *fakeOrdersRepo) ListOrdersForCustomer(_ context.Context, _ string) ([]orders.Order, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeOrdersRepo) ListAllOrders(_ context.Context, _ orders.OrderFilter) ([]orders.Order, error) {
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

// --- fakeUsersRepo ---

type fakeUsersRepo struct {
	byID map[string]users.User
}

func newFakeUsersRepo() *fakeUsersRepo {
	return &fakeUsersRepo{byID: map[string]users.User{}}
}

func (f *fakeUsersRepo) seed(id, email string, phone *string) users.User {
	u := users.User{ID: id, Email: email, Phone: phone, FullName: "Test User", Role: users.RoleCustomer, CreatedAt: time.Now()}
	f.byID[id] = u
	return u
}

func (f *fakeUsersRepo) Create(_ context.Context, _ users.NewUser) (users.User, error) {
	return users.User{}, errors.New("not implemented in fake")
}
func (f *fakeUsersRepo) FindByEmail(_ context.Context, _ string) (users.User, error) {
	return users.User{}, errors.New("not implemented in fake")
}
func (f *fakeUsersRepo) FindByID(_ context.Context, id string) (users.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	return u, nil
}
func (f *fakeUsersRepo) Update(_ context.Context, _ string, _ users.ProfileUpdate) (users.User, error) {
	return users.User{}, errors.New("not implemented in fake")
}

// --- fakeTrackingRepo ---

// fakeTrackingRepo only implements ListEvents (all NotifyOrderCreated
// needs) — every other method is unreachable from this package's own
// code and is left unimplemented defensively.
type fakeTrackingRepo struct {
	events map[string][]tracking.Event
}

func newFakeTrackingRepo() *fakeTrackingRepo {
	return &fakeTrackingRepo{events: map[string][]tracking.Event{}}
}

func (f *fakeTrackingRepo) seedEvent(orderID string, e tracking.Event) {
	f.events[orderID] = append(f.events[orderID], e)
}

func (f *fakeTrackingRepo) Transition(_ context.Context, _, _ string, _, _ users.Role, _ tracking.Status, _ json.RawMessage) (tracking.Event, error) {
	return tracking.Event{}, errors.New("not implemented in fake")
}
func (f *fakeTrackingRepo) TransitionTx(_ context.Context, _ pgx.Tx, _, _ string, _, _ users.Role, _ tracking.Status, _ json.RawMessage) (tracking.Event, error) {
	return tracking.Event{}, errors.New("not implemented in fake")
}
func (f *fakeTrackingRepo) OrderCustomerID(_ context.Context, _ string) (string, error) {
	return "", errors.New("not implemented in fake")
}
func (f *fakeTrackingRepo) ListEvents(_ context.Context, orderID string) ([]tracking.Event, error) {
	return f.events[orderID], nil
}
