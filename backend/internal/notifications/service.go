package notifications

import (
	"context"
	"fmt"
	"log/slog"

	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
)

// Service is the Notification Service the blueprint's own §18 diagram
// describes: order events flow in, EmailProvider/SmsProvider calls flow
// out, and every dependency below is an interface — this package never
// imports a concrete email/SMS SDK. Customer is the only recipient (the
// only one either source document ever names); resolving the recipient
// is this Service's own job, never the caller's, so every one of the
// four trigger call sites only ever needs to pass an order id and event
// details, never contact information.
type Service struct {
	repo          Repository
	ordersRepo    orders.Repository
	usersRepo     users.Repository
	trackingRepo  tracking.Repository
	emailProvider EmailProvider
	smsProvider   SmsProvider
}

func NewService(
	repo Repository,
	ordersRepo orders.Repository,
	usersRepo users.Repository,
	trackingRepo tracking.Repository,
	emailProvider EmailProvider,
	smsProvider SmsProvider,
) *Service {
	return &Service{
		repo:          repo,
		ordersRepo:    ordersRepo,
		usersRepo:     usersRepo,
		trackingRepo:  trackingRepo,
		emailProvider: emailProvider,
		smsProvider:   smsProvider,
	}
}

// eventTypeForStatus maps a tracking.Status (the NewStatus every
// TransitionTx-produced Event carries) onto the blueprint's own event
// name — a plain lookup, not a new state machine; the seven values here
// are exactly M08's own seven non-CREATED statuses (CREATED never
// reaches this function at all, since order creation never goes through
// TransitionTx — see NotifyOrderCreated).
func eventTypeForStatus(status string) (EventType, bool) {
	switch tracking.Status(status) {
	case tracking.StatusAssigned:
		return EventAgentAssigned, true
	case tracking.StatusPickedUp:
		return EventPickedUp, true
	case tracking.StatusInTransit:
		return EventInTransit, true
	case tracking.StatusOutForDelivery:
		return EventOutForDelivery, true
	case tracking.StatusDelivered:
		return EventDelivered, true
	case tracking.StatusFailed:
		return EventFailed, true
	case tracking.StatusRescheduled:
		return EventRescheduled, true
	default:
		return "", false
	}
}

// NotifyTransition matches tracking.TransitionHook's signature exactly
// — it is wired directly as the hook value tracking.TransitionHandler,
// internal/assignment's Assign/AutoAssign, and internal/rescheduling's
// Reschedule all invoke after their own transaction has already
// committed. event.ID (the exact order_tracking_events row this
// specific occurrence produced) is the idempotency anchor passed
// through to Repository.Claim — never (order_id, event type) alone —
// so a second, later occurrence of the same event type on the same
// order (e.g. a second FAILED after a reschedule cycle) is always a
// new, independently-notify-able row.
func (s *Service) NotifyTransition(ctx context.Context, event tracking.Event) {
	eventType, ok := eventTypeForStatus(event.NewStatus)
	if !ok {
		// Defensive: unreachable in practice — every status TransitionTx
		// can ever produce as NewStatus is one of the seven handled
		// above. Never a fatal condition either way; simply nothing to
		// notify.
		return
	}
	s.notify(ctx, event.ID, event.OrderID, eventType, event.Metadata)
}

// NotifyOrderCreated matches orders.OrderCreatedHook's signature
// exactly. Unlike the other seven events, order creation never goes
// through TransitionTx and so never hands this package a tracking.Event
// directly — CreateOrder's own transaction already inserted the
// order's opening tracking event (previous_status NULL, new_status
// CREATED) atomically with the order row itself (see
// docs/order-management.md), so this method re-reads it via the
// already-exported, read-only ListEvents rather than requiring any
// change to internal/orders' own CreateOrder signature or SQL. At the
// moment this fires (synchronously, right after order creation
// committed), that CREATED event is the only event the order has, so
// taking the first entry is safe and correct.
func (s *Service) NotifyOrderCreated(ctx context.Context, orderID string) {
	events, err := s.trackingRepo.ListEvents(ctx, orderID)
	if err != nil || len(events) == 0 {
		slog.Error("notification: could not resolve initial tracking event", "error", err, "order_id", orderID)
		return
	}
	s.notify(ctx, events[0].ID, orderID, EventOrderCreated, events[0].Metadata)
}

// notify resolves the order's customer, then attempts email (always)
// and SMS (only when a phone number is on file). Every failure from
// this point on — an unresolvable order/customer, a provider error, a
// persistence error — is logged and contained here; nothing this
// method does ever returns an error to its own caller, because its
// caller is a post-commit hook whose own triggering operation has
// already succeeded and must remain successful regardless of what
// happens to the notification attempt.
func (s *Service) notify(ctx context.Context, trackingEventID, orderID string, eventType EventType, metadata []byte) {
	order, err := s.ordersRepo.FindOrderByID(ctx, orderID)
	if err != nil {
		slog.Error("notification: order lookup failed", "error", err, "order_id", orderID, "event", eventType)
		return
	}
	customer, err := s.usersRepo.FindByID(ctx, order.CustomerID)
	if err != nil {
		slog.Error("notification: customer lookup failed", "error", err, "order_id", orderID, "event", eventType)
		return
	}

	msg := buildContent(eventType, orderID, metadata)

	s.dispatch(ctx, trackingEventID, orderID, eventType, ChannelEmail, customer.Email, func() error {
		return s.emailProvider.SendEmail(ctx, customer.Email, msg.Subject, msg.Body, msg.HTMLBody)
	})

	// SMS is attempted only when the customer has a phone number on
	// file — a missing phone is never an error, simply not applicable
	// (see docs/notifications.md).
	if customer.Phone != nil && *customer.Phone != "" {
		phone := *customer.Phone
		s.dispatch(ctx, trackingEventID, orderID, eventType, ChannelSMS, phone, func() error {
			return s.smsProvider.SendSMS(ctx, phone, msg.Body)
		})
	}
}

// dispatch is the one place idempotency and failure containment both
// happen, shared by every channel. Claim is attempted first, strictly
// before send is ever called — this is what actually prevents a
// duplicate *send*, not merely a duplicate persisted row: a second,
// concurrent (or repeated) call for the same (trackingEventID, channel)
// finds claimed=false and returns immediately, never touching the
// provider at all. safeSend additionally recovers a provider panic,
// converting it to an error, so a badly-behaved provider can never
// crash the caller's already-successful request.
func (s *Service) dispatch(ctx context.Context, trackingEventID, orderID string, eventType EventType, channel Channel, recipient string, send func() error) {
	id, claimed, err := s.repo.Claim(ctx, ClaimInput{
		TrackingEventID: trackingEventID,
		OrderID:         orderID,
		Event:           eventType,
		Channel:         channel,
		Recipient:       recipient,
	})
	if err != nil {
		slog.Error("notification: claim failed", "error", err, "order_id", orderID, "event", eventType, "channel", channel)
		return
	}
	if !claimed {
		// Already handled — a prior call already claimed this exact
		// (tracking_event_id, channel) occurrence.
		return
	}

	status := StatusSent
	if err := safeSend(send); err != nil {
		slog.Error("notification: provider failed", "error", err, "order_id", orderID, "event", eventType, "channel", channel, "recipient", recipient)
		status = StatusFailed
	}

	if err := s.repo.Resolve(ctx, id, status); err != nil {
		slog.Error("notification: resolve failed", "error", err, "notification_id", id)
	}
}

// safeSend recovers a panicking provider call and converts it into a
// plain error, so provider misbehavior can never propagate past this
// package (see dispatch's own doc comment for why that guarantee
// matters here specifically).
func safeSend(send func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("notification provider panicked: %v", r)
		}
	}()
	return send()
}
