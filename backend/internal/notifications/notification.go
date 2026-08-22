// Package notifications implements M11 — Notification Service: an
// internal-only service that reacts to the eight order-lifecycle events
// by emailing (always) and texting (when a phone number is on file) the
// order's own customer. It owns exactly one new table (notifications)
// and no REST endpoints — see docs/notifications.md.
//
// This package owns notification dispatch and its own idempotency/audit
// record only. It never computes pricing, never writes orders.status,
// never validates a status transition, never ranks or selects a
// delivery agent, and never decides whether a reschedule is authorized
// — it only observes an event that one of those other, unmodified
// modules already produced and committed. See "Post-commit integration"
// in docs/notifications.md for exactly how it is invoked.
package notifications

import "encoding/json"

// EventType is one of the eight events the blueprint's own M11 section
// names, exactly — no more, no fewer. ORDER_CREATED has no
// corresponding tracking.Status (order creation doesn't go through
// TransitionTx at all); the other seven map 1:1 onto tracking.Status
// values, with ASSIGNED renamed to AGENT_ASSIGNED to match the
// blueprint's own event name.
type EventType string

const (
	EventOrderCreated   EventType = "ORDER_CREATED"
	EventAgentAssigned  EventType = "AGENT_ASSIGNED"
	EventPickedUp       EventType = "PICKED_UP"
	EventInTransit      EventType = "IN_TRANSIT"
	EventOutForDelivery EventType = "OUT_FOR_DELIVERY"
	EventDelivered      EventType = "DELIVERED"
	EventFailed         EventType = "FAILED"
	EventRescheduled    EventType = "RESCHEDULED"
)

// Channel is one of the two provider types the blueprint's own
// "Provider abstraction" names.
type Channel string

const (
	ChannelEmail Channel = "EMAIL"
	ChannelSMS   Channel = "SMS"
)

// Status describes the outcome of one provider attempt for one
// (tracking_event_id, channel) pair. PENDING exists only for the brief
// window between claiming the slot (before the provider is called) and
// resolving it (after) — no row is ever left at PENDING once a
// dispatch attempt has actually run to completion, since the resolve
// step always follows the provider call synchronously in the same
// request.
type Status string

const (
	StatusPending Status = "PENDING"
	StatusSent    Status = "SENT"
	StatusFailed  Status = "FAILED"
)

// content is the minimal, data-only payload every notification
// attempt carries — no exact wording is mandated by either source
// document (see docs/notifications.md); this is deliberately plain
// text, never HTML, never templated, never localized.
type content struct {
	Subject string
	Body    string
}

// buildContent generates the one, shared message used for both the
// email and SMS attempt of a given event occurrence. Pure, deterministic,
// independently testable — no I/O, no provider dependency. metadata is
// the tracking event's own raw JSON metadata (nil/empty is valid and
// produces the generic message with no extra detail line).
func buildContent(event EventType, orderID string, metadata json.RawMessage) content {
	body := "Your order " + orderID + " status changed to " + string(event) + "."

	var m map[string]any
	if len(metadata) > 0 {
		// Malformed/unexpected metadata is not an error here — the
		// notification still fires with the generic message; only the
		// optional extra detail line is skipped.
		_ = json.Unmarshal(metadata, &m)
	}

	switch event {
	case EventFailed:
		if reason, ok := m["failure_reason"].(string); ok && reason != "" {
			body += " Reason: " + reason + "."
		}
	case EventRescheduled:
		if date, ok := m["requested_date"].(string); ok && date != "" {
			body += " New delivery date: " + date + "."
		}
		if reason, ok := m["reason"].(string); ok && reason != "" {
			body += " Reason: " + reason + "."
		}
	case EventAgentAssigned:
		if agentID, ok := m["assigned_agent_id"].(string); ok && agentID != "" {
			body += " Assigned agent: " + agentID + "."
		}
	}

	return content{
		Subject: "Order " + orderID + ": " + string(event),
		Body:    body,
	}
}
