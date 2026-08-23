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

// content is the message payload every notification attempt carries —
// no exact wording is mandated by either source document (see
// docs/notifications.md). Body is plain text: it is what SMS sends
// as-is, and what email sends as its plain-text part/fallback. HTMLBody
// is the branded HTML rendering of the identical information, sent as
// email's HTML part (see email_template.go) — SMS has no HTML
// equivalent and never uses it.
type content struct {
	Subject  string
	Body     string
	HTMLBody string
}

// eventHeadline is the one-sentence, human-readable description of each
// event — used as the email subject (see orderReference below) and as
// the plain-text/HTML body's opening sentence. Exhaustive over the same
// eight EventType constants declared above; every event this package
// ever builds content for has an entry.
var eventHeadline = map[EventType]string{
	EventOrderCreated:   "Your order has been placed",
	EventAgentAssigned:  "A delivery agent has been assigned to your order",
	EventPickedUp:       "Your order has been picked up",
	EventInTransit:      "Your order is in transit",
	EventOutForDelivery: "Your order is out for delivery",
	EventDelivered:      "Your order has been delivered",
	EventFailed:         "We couldn't complete your delivery",
	EventRescheduled:    "Your delivery has been rescheduled",
}

// eventStatusLabel is the short noun-phrase shown as the HTML email's
// status pill (email_template.go) — deliberately distinct from
// eventHeadline's full sentence, the same way the frontend's
// STATUS_LABEL (frontend/src/components/order-status.ts) is a short
// label rather than a sentence.
var eventStatusLabel = map[EventType]string{
	EventOrderCreated:   "Order Placed",
	EventAgentAssigned:  "Agent Assigned",
	EventPickedUp:       "Picked Up",
	EventInTransit:      "In Transit",
	EventOutForDelivery: "Out for Delivery",
	EventDelivered:      "Delivered",
	EventFailed:         "Delivery Failed",
	EventRescheduled:    "Rescheduled",
}

// buildContent generates the one, shared message used for both the
// email and SMS attempt of a given event occurrence. Pure, deterministic,
// independently testable — no I/O, no provider dependency. metadata is
// the tracking event's own raw JSON metadata (nil/empty is valid and
// produces the generic message with no extra detail line).
func buildContent(event EventType, orderID string, metadata json.RawMessage) content {
	headline := eventHeadline[event]

	var m map[string]any
	if len(metadata) > 0 {
		// Malformed/unexpected metadata is not an error here — the
		// notification still fires with the generic message; only the
		// optional extra detail line is skipped.
		_ = json.Unmarshal(metadata, &m)
	}

	var extras []string
	switch event {
	case EventFailed:
		if reason, ok := m["failure_reason"].(string); ok && reason != "" {
			extras = append(extras, "Reason: "+reason)
		}
	case EventRescheduled:
		if date, ok := m["requested_date"].(string); ok && date != "" {
			extras = append(extras, "New delivery date: "+date)
		}
		if reason, ok := m["reason"].(string); ok && reason != "" {
			extras = append(extras, "Reason: "+reason)
		}
	case EventAgentAssigned:
		if agentID, ok := m["assigned_agent_id"].(string); ok && agentID != "" {
			extras = append(extras, "Assigned agent: "+agentID)
		}
	}

	body := headline + ". Order " + orderID + "."
	for _, e := range extras {
		body += " " + e + "."
	}

	return content{
		Subject:  headline + " · Order #" + orderReference(orderID),
		Body:     body,
		HTMLBody: buildHTMLBody(event, eventStatusLabel[event], headline, orderID, extras),
	}
}

// orderReference is a short, still-unambiguous prefix of the real order
// id for use in the email subject line — real ecommerce notifications
// don't put a full identifier in the subject, but the subject should
// still be searchable against the order. This is a substring of the
// real id (a UUID's first 8 characters, which is exactly its first
// hyphen-delimited segment), never a separately invented order number.
func orderReference(orderID string) string {
	if len(orderID) > 8 {
		return orderID[:8]
	}
	return orderID
}
