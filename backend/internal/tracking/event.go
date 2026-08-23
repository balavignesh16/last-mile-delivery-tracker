// Package tracking implements M08 — Tracking & Order Lifecycle: the
// order status state machine and its immutable event log. It owns
// exactly two endpoints (POST /orders/:id/status, GET
// /orders/:id/tracking) and one new table (order_tracking_events).
//
// This package does not own pricing (internal/rates, M06), order
// persistence/ownership (internal/orders, M07), or agent assignment
// (M09, not yet built) — it only reads/writes the orders.status column
// and this package's own tracking-event table. See docs/order-tracking.md.
package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Status is one of the eight values orders.status's CHECK constraint
// (migration 0008) already permits. Defined here, not in
// internal/orders, because owning the legal state machine is this
// module's explicit responsibility — internal/orders only ever writes
// the literal "CREATED" string once, at creation time, and has no
// reason to import this type.
type Status string

const (
	StatusCreated        Status = "CREATED"
	StatusAssigned       Status = "ASSIGNED"
	StatusPickedUp       Status = "PICKED_UP"
	StatusInTransit      Status = "IN_TRANSIT"
	StatusOutForDelivery Status = "OUT_FOR_DELIVERY"
	StatusDelivered      Status = "DELIVERED"
	StatusFailed         Status = "FAILED"
	StatusRescheduled    Status = "RESCHEDULED"
)

// ParseStatus validates a raw string against the eight permitted
// values — the same "never guess, fail explicit" convention every
// enum-like type in this project follows (rates.ParseOrderType,
// zones' ZoneRelationship, ...).
func ParseStatus(raw string) (Status, bool) {
	switch Status(raw) {
	case StatusCreated, StatusAssigned, StatusPickedUp, StatusInTransit,
		StatusOutForDelivery, StatusDelivered, StatusFailed, StatusRescheduled:
		return Status(raw), true
	default:
		return "", false
	}
}

var (
	// ErrOrderNotFound is returned when the referenced order id does not
	// exist at all.
	ErrOrderNotFound = errors.New("order not found")
	// ErrInvalidTransition is returned when the requested new status is a
	// recognized status but is not a legal transition from the order's
	// current status (including a same-status no-op, which is not a
	// legal edge — see statemachine.go). Maps to 409: the request is
	// well-formed, it conflicts with the order's current state.
	ErrInvalidTransition = errors.New("this status transition is not permitted from the order's current status")
	// ErrForbiddenTransition is returned when the transition itself is
	// legal, but the caller's role is not authorized to perform this
	// specific edge (e.g. a DELIVERY_AGENT attempting CREATED->ASSIGNED,
	// which is ADMIN-only). Distinct from route-level RBAC, which only
	// admits ADMIN/DELIVERY_AGENT onto this endpoint at all — this is
	// the finer, per-edge check, and — like M05's slab-overlap
	// validation — must run inside the same locked transaction that
	// reads the current status, since which edge is being attempted
	// isn't known until then.
	ErrForbiddenTransition = errors.New("this role is not authorized to perform this specific status transition")
)

// Event is one immutable row of order_tracking_events. PreviousStatus
// is nil only for an order's very first event (NULL -> CREATED,
// written by internal/orders.CreateOrder itself). Metadata is raw,
// optional JSON — this package enforces no required shape for it.
// ActorRole is nil only for a row written before migration 0015 added
// the column — every event recorded since then always has one (see
// Repository.Transition's own doc comment for why it's a required,
// not optional, parameter on every write path).
type Event struct {
	ID             string
	OrderID        string
	PreviousStatus *string
	NewStatus      string
	ActorID        string
	ActorRole      *string
	Metadata       json.RawMessage
	CreatedAt      time.Time
}

// TransitionHook is called with the Event a successful transition just
// produced, strictly after that transition's own transaction has
// already committed — a purely additive, post-commit extension point
// for M11's notification service (or any future consumer) to observe a
// transition without this package depending on it (avoiding an import
// cycle, since M11 needs to depend on this package for Event/Repository,
// not the other way around). Nil is always safe — every caller of a
// hook-accepting function/constructor checks for nil before invoking it,
// so nothing here is required to wire. TransitionHandler (this package)
// and internal/assignment's and internal/rescheduling's own repositories
// all accept and invoke this exact same type, since all three already
// produce their own Event via Transition/TransitionTx.
type TransitionHook func(ctx context.Context, event Event)
