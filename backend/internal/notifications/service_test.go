package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"

	"lastmiletracker/internal/tracking"
)

func newTestService(t *testing.T) (*Service, *fakeNotificationsRepo, *fakeOrdersRepo, *fakeUsersRepo, *fakeTrackingRepo, *fakeEmailProvider, *fakeSmsProvider) {
	t.Helper()
	repo := newFakeNotificationsRepo()
	ordersRepo := newFakeOrdersRepo()
	usersRepo := newFakeUsersRepo()
	trackingRepo := newFakeTrackingRepo()
	email := &fakeEmailProvider{}
	sms := &fakeSmsProvider{}
	svc := NewService(repo, ordersRepo, usersRepo, trackingRepo, email, sms)
	return svc, repo, ordersRepo, usersRepo, trackingRepo, email, sms
}

func phonePtr(p string) *string { return &p }

// --- NotifyTransition: event recognition + recipient resolution ---

func TestNotifyTransition_EveryLifecycleEventRecognized(t *testing.T) {
	cases := []struct {
		status string
		want   EventType
	}{
		{"ASSIGNED", EventAgentAssigned},
		{"PICKED_UP", EventPickedUp},
		{"IN_TRANSIT", EventInTransit},
		{"OUT_FOR_DELIVERY", EventOutForDelivery},
		{"DELIVERED", EventDelivered},
		{"FAILED", EventFailed},
		{"RESCHEDULED", EventRescheduled},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
			ordersRepo.seed("order-1", "customer-1")
			usersRepo.seed("customer-1", "customer@example.com", nil)

			svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-" + tc.status, OrderID: "order-1", NewStatus: tc.status})

			if email.count() != 1 {
				t.Fatalf("email count = %d, want 1", email.count())
			}
			if !strings.Contains(email.sent[0].Subject, eventHeadline[tc.want]) {
				t.Errorf("subject = %q, want it to mention %q", email.sent[0].Subject, eventHeadline[tc.want])
			}
		})
	}
}

func TestNotifyTransition_UnrecognizedStatusIgnored(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "CREATED"})

	if email.count() != 0 {
		t.Errorf("email count = %d, want 0 (CREATED never reaches NotifyTransition in practice)", email.count())
	}
}

func TestNotifyTransition_CustomerRecipientResolved(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	if email.count() != 1 || email.sent[0].To != "customer@example.com" {
		t.Fatalf("email sent = %+v, want exactly one to customer@example.com", email.sent)
	}
}

func TestNotifyTransition_UnknownOrderSkipsNotification(t *testing.T) {
	svc, _, _, _, _, email, sms := newTestService(t)
	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "does-not-exist", NewStatus: "DELIVERED"})
	if email.count() != 0 || sms.count() != 0 {
		t.Errorf("email/sms counts = %d/%d, want 0/0 (unresolvable order must not panic or partially notify)", email.count(), sms.count())
	}
}

// --- SMS: attempted only when a phone exists ---

func TestNotifyTransition_SmsAttemptedWhenPhonePresent(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, sms := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", phonePtr("+15551234567"))

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	if email.count() != 1 {
		t.Errorf("email count = %d, want 1", email.count())
	}
	if sms.count() != 1 || sms.sent[0].To != "+15551234567" {
		t.Fatalf("sms sent = %+v, want exactly one to +15551234567", sms.sent)
	}
}

func TestNotifyTransition_SmsSkippedWhenPhoneAbsent(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, sms := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	if email.count() != 1 {
		t.Errorf("email count = %d, want 1", email.count())
	}
	if sms.count() != 0 {
		t.Errorf("sms count = %d, want 0 (no phone on file is not an error)", sms.count())
	}
}

func TestNotifyTransition_SmsSkippedWhenPhoneEmptyString(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, _, sms := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", phonePtr(""))

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	if sms.count() != 0 {
		t.Errorf("sms count = %d, want 0 (empty phone treated the same as absent)", sms.count())
	}
}

// --- Content per event ---

func TestNotifyTransition_FailedContentIncludesReason(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{
		ID: "event-1", OrderID: "order-1", NewStatus: "FAILED",
		Metadata: []byte(`{"failure_reason":"Recipient not available"}`),
	})

	if !strings.Contains(email.sent[0].Body, "Recipient not available") {
		t.Errorf("body = %q, want the failure reason", email.sent[0].Body)
	}
}

func TestNotifyTransition_RescheduledContentIncludesDateAndReason(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{
		ID: "event-1", OrderID: "order-1", NewStatus: "RESCHEDULED",
		Metadata: []byte(`{"requested_date":"2099-01-01","reason":"Not home"}`),
	})

	if !strings.Contains(email.sent[0].Body, "2099-01-01") || !strings.Contains(email.sent[0].Body, "Not home") {
		t.Errorf("body = %q, want the requested date and reason", email.sent[0].Body)
	}
}

func TestNotifyTransition_AgentAssignedContentIncludesAgentID(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{
		ID: "event-1", OrderID: "order-1", NewStatus: "ASSIGNED",
		Metadata: []byte(`{"assigned_agent_id":"agent-42"}`),
	})

	if !strings.Contains(email.sent[0].Body, "agent-42") {
		t.Errorf("body = %q, want the assigned agent id", email.sent[0].Body)
	}
}

// --- NotifyOrderCreated ---

func TestNotifyOrderCreated_ResolvesInitialTrackingEvent(t *testing.T) {
	svc, repo, ordersRepo, usersRepo, trackingRepo, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)
	trackingRepo.seedEvent("order-1", tracking.Event{ID: "created-event-1", OrderID: "order-1", NewStatus: "CREATED"})

	svc.NotifyOrderCreated(context.Background(), "order-1")

	if email.count() != 1 {
		t.Fatalf("email count = %d, want 1", email.count())
	}
	if !strings.Contains(email.sent[0].Subject, eventHeadline[EventOrderCreated]) {
		t.Errorf("subject = %q, want it to mention %q", email.sent[0].Subject, eventHeadline[EventOrderCreated])
	}
	if email.sent[0].HTMLBody == "" {
		t.Errorf("HTMLBody is empty, want a rendered HTML email body")
	}
	// Idempotency anchor must be the real tracking event id, not a
	// synthesized one.
	if _, claimed := repo.claimed["created-event-1|EMAIL"]; !claimed {
		t.Errorf("claimed keys = %v, want created-event-1|EMAIL to be claimed", repo.claimed)
	}
}

func TestNotifyOrderCreated_NoEventsIsSafe(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)
	// trackingRepo has no seeded events for order-1.

	svc.NotifyOrderCreated(context.Background(), "order-1")

	if email.count() != 0 {
		t.Errorf("email count = %d, want 0 (no tracking event to anchor on)", email.count())
	}
}

// --- Provider failure containment ---

func TestNotifyTransition_ProviderFailureRecordsFailedStatus(t *testing.T) {
	svc, repo, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)
	email.sendErr = errors.New("smtp unavailable")

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	id, claimed := repo.claimed["event-1|EMAIL"]
	if !claimed {
		t.Fatalf("expected a claim to exist for event-1|EMAIL")
	}
	status, resolved := repo.statusOf(id)
	if !resolved || status != StatusFailed {
		t.Errorf("status = %v (resolved=%v), want FAILED", status, resolved)
	}
}

func TestNotifyTransition_ProviderFailureDoesNotAffectOtherChannel(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, sms := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", phonePtr("+15551234567"))
	email.sendErr = errors.New("smtp unavailable")

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	if sms.count() != 1 {
		t.Errorf("sms count = %d, want 1 (email failure must not block SMS)", sms.count())
	}
}

func TestNotifyTransition_ProviderPanicContained(t *testing.T) {
	svc, repo, ordersRepo, usersRepo, _, _, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	panicProvider := panicEmailProvider{}
	svc.emailProvider = panicProvider

	// Must not panic the calling goroutine.
	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	id, claimed := repo.claimed["event-1|EMAIL"]
	if !claimed {
		t.Fatalf("expected a claim to exist for event-1|EMAIL")
	}
	status, resolved := repo.statusOf(id)
	if !resolved || status != StatusFailed {
		t.Errorf("status = %v (resolved=%v), want FAILED (a panicking provider must resolve as a failure, not crash)", status, resolved)
	}
}

type panicEmailProvider struct{}

func (panicEmailProvider) SendEmail(context.Context, string, string, string, string) error {
	panic("provider exploded")
}

// --- Idempotency ---

func TestNotifyTransition_SameEventTwiceOnlyNotifiesOnce(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	event := tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "FAILED"}
	svc.NotifyTransition(context.Background(), event)
	svc.NotifyTransition(context.Background(), event)

	if email.count() != 1 {
		t.Errorf("email count = %d, want 1 (second call for the same tracking event must be a no-op)", email.count())
	}
}

func TestNotifyTransition_RepeatedFailedOccurrencesEachNotify(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	// Two genuinely distinct FAILED occurrences on the same order (e.g.
	// after a FAILED -> RESCHEDULED -> ASSIGNED -> ... -> FAILED cycle)
	// have different tracking_event_id values and must each notify.
	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "FAILED"})
	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-2", OrderID: "order-1", NewStatus: "FAILED"})

	if email.count() != 2 {
		t.Errorf("email count = %d, want 2 (each distinct tracking event occurrence must notify independently)", email.count())
	}
}

func TestNotifyTransition_RepeatedRescheduledOccurrencesEachNotify(t *testing.T) {
	svc, _, ordersRepo, usersRepo, _, email, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", nil)

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "RESCHEDULED"})
	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-2", OrderID: "order-1", NewStatus: "RESCHEDULED"})

	if email.count() != 2 {
		t.Errorf("email count = %d, want 2", email.count())
	}
}

func TestNotifyTransition_SameEventDifferentChannelsBothClaim(t *testing.T) {
	svc, repo, ordersRepo, usersRepo, _, _, _ := newTestService(t)
	ordersRepo.seed("order-1", "customer-1")
	usersRepo.seed("customer-1", "customer@example.com", phonePtr("+15551234567"))

	svc.NotifyTransition(context.Background(), tracking.Event{ID: "event-1", OrderID: "order-1", NewStatus: "DELIVERED"})

	if _, ok := repo.claimed["event-1|EMAIL"]; !ok {
		t.Error("expected an EMAIL claim for event-1")
	}
	if _, ok := repo.claimed["event-1|SMS"]; !ok {
		t.Error("expected a separate SMS claim for event-1 (channels are independent idempotency slots)")
	}
}
