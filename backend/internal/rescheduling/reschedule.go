// Package rescheduling implements M10 — Failed Delivery & Rescheduling:
// POST /api/v1/orders/:id/reschedule and GET /api/v1/orders/:id/reschedules.
// It owns exactly one new table (reschedule_requests) and no schema
// change to orders/order_tracking_events/delivery_agents beyond what
// M08/M09 already added.
//
// This package owns reschedule-request capture only. Order persistence
// stays in internal/orders (M07), the status state machine stays in
// internal/tracking (M08), and agent ranking/assignment stays in
// internal/assignment (M09) — rescheduling never recomputes pricing,
// never writes orders.status directly, never inserts a tracking event
// directly, and never ranks or selects a delivery agent. The
// FAILED->RESCHEDULED transition goes through tracking.Repository's own
// TransitionTx, M08's unmodified, authoritative state-machine write —
// see docs/failed-delivery.md.
package rescheduling

import (
	"errors"
	"time"
)

// dateLayout is the only accepted requested_date wire format
// (YYYY-MM-DD) — a calendar date, not a timestamp: the blueprint's own
// field name is requested_date, not requested_at, and nothing in either
// source document ties a reschedule to a specific time of day.
const dateLayout = "2006-01-02"

// ErrMissingRequestedDate and ErrInvalidRequestedDate are the two
// distinct 422 causes the finalized M10 contract calls out separately
// ("missing requested_date" vs "invalid requested_date") — kept as
// separate sentinels so the handler can report a specific message for
// each, the same "one message per failure mode" convention every prior
// module's validation uses (see rates.ValidateQuoteFields).
var (
	ErrMissingRequestedDate = errors.New("requested_date is required")
	ErrInvalidRequestedDate = errors.New("requested_date must be a valid date (YYYY-MM-DD)")
	// ErrPastRequestedDate is the one, minimal date-validation rule the
	// finalized M10 decision actually approves: reject a requested_date
	// before the server's own current calendar date. Same-day is
	// explicitly allowed. No minimum-notice period, no weekend/holiday
	// rule, no timezone handling, no "later than original delivery
	// date" rule — none of those are approved, so none are implemented.
	ErrPastRequestedDate = errors.New("requested_date must not be before today")
)

// ParseRequestedDate parses the wire-format requested_date string into a
// date-only time.Time (midnight, UTC) — pure, no I/O, independently
// testable. Returns ErrMissingRequestedDate for an empty string and
// ErrInvalidRequestedDate for anything that isn't a valid YYYY-MM-DD
// date (wrong format, non-existent calendar date, extra characters).
func ParseRequestedDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, ErrMissingRequestedDate
	}
	t, err := time.Parse(dateLayout, raw)
	if err != nil {
		return time.Time{}, ErrInvalidRequestedDate
	}
	return t, nil
}

// ValidateRescheduleDate reports whether requestedDate is on or after
// today's calendar date — comparing dates only, never times, so a
// requestedDate/today pair with different time-of-day components (e.g.
// today = now() at 23:59, requestedDate = today at midnight) is still
// correctly treated as "the same day," matching "same-day rescheduling
// is allowed" from the finalized M10 decision. Pure and deterministic:
// callers supply "today" rather than this function calling time.Now()
// itself, so it never touches the server's own clock and is trivially
// unit-testable against a fixed reference date.
func ValidateRescheduleDate(requestedDate, today time.Time) error {
	ry, rm, rd := requestedDate.Date()
	ty, tm, td := today.Date()
	req := time.Date(ry, rm, rd, 0, 0, 0, 0, time.UTC)
	tod := time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC)
	if req.Before(tod) {
		return ErrPastRequestedDate
	}
	return nil
}

// Reschedule is a persisted row in reschedule_requests — one customer's
// (or admin's) request to reschedule a specific FAILED order, captured
// alongside the FAILED->RESCHEDULED tracking event M08 writes for the
// same action. Deliberately has no status field: a reschedule request
// is immediately effective once created, per the finalized M10 decision
// that there is no approval workflow.
type Reschedule struct {
	ID          string
	OrderID     string
	RequestedBy string
	// RequestedByRole is nil only for a row written before migration
	// 0015 added the column — every request recorded since then always
	// has one (the real caller's role, captured by RescheduleHandler
	// from their own verified JWT — see RescheduleInput.ActorRole).
	RequestedByRole *string
	RequestedDate   time.Time
	Reason          *string
	CreatedAt       time.Time
}
