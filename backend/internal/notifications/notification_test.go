package notifications

import (
	"strings"
	"testing"
)

func TestBuildContent_GenericEvent(t *testing.T) {
	c := buildContent(EventPickedUp, "order-1", nil)
	if !strings.Contains(c.Body, "order-1") || !strings.Contains(c.Body, eventHeadline[EventPickedUp]) {
		t.Errorf("body = %q, want it to mention the order id and the headline", c.Body)
	}
	if !strings.Contains(c.Subject, "order-1") {
		t.Errorf("subject = %q, want it to mention the order id", c.Subject)
	}
	if !strings.Contains(c.Subject, eventHeadline[EventPickedUp]) {
		t.Errorf("subject = %q, want it to mention the headline", c.Subject)
	}
}

func TestBuildContent_FailedWithReason(t *testing.T) {
	c := buildContent(EventFailed, "order-1", []byte(`{"failure_reason":"Recipient not available"}`))
	if !strings.Contains(c.Body, "Recipient not available") {
		t.Errorf("body = %q, want it to include the failure reason", c.Body)
	}
}

func TestBuildContent_FailedWithoutReason(t *testing.T) {
	c := buildContent(EventFailed, "order-1", nil)
	if !strings.Contains(c.Body, eventHeadline[EventFailed]) {
		t.Errorf("body = %q, want the generic FAILED message", c.Body)
	}
	if strings.Contains(c.Body, "Reason:") {
		t.Errorf("body = %q, want no reason line when none was supplied", c.Body)
	}
}

func TestBuildContent_RescheduledWithDateAndReason(t *testing.T) {
	c := buildContent(EventRescheduled, "order-1", []byte(`{"requested_date":"2099-01-01","reason":"Not home"}`))
	if !strings.Contains(c.Body, "2099-01-01") {
		t.Errorf("body = %q, want the requested date", c.Body)
	}
	if !strings.Contains(c.Body, "Not home") {
		t.Errorf("body = %q, want the reason", c.Body)
	}
}

func TestBuildContent_RescheduledWithoutReason(t *testing.T) {
	c := buildContent(EventRescheduled, "order-1", []byte(`{"requested_date":"2099-01-01"}`))
	if !strings.Contains(c.Body, "2099-01-01") {
		t.Errorf("body = %q, want the requested date", c.Body)
	}
	if strings.Contains(c.Body, "Reason:") {
		t.Errorf("body = %q, want no reason line when none was supplied", c.Body)
	}
}

func TestBuildContent_AgentAssignedWithAgentID(t *testing.T) {
	c := buildContent(EventAgentAssigned, "order-1", []byte(`{"assigned_agent_id":"agent-42"}`))
	if !strings.Contains(c.Body, "agent-42") {
		t.Errorf("body = %q, want the assigned agent id", c.Body)
	}
}

func TestBuildContent_MalformedMetadataDoesNotPanic(t *testing.T) {
	c := buildContent(EventFailed, "order-1", []byte(`not-json`))
	if !strings.Contains(c.Body, "order-1") {
		t.Errorf("body = %q, want the generic message despite malformed metadata", c.Body)
	}
}

func TestBuildContent_EmptyMetadata(t *testing.T) {
	c := buildContent(EventDelivered, "order-1", []byte{})
	if !strings.Contains(c.Body, eventHeadline[EventDelivered]) {
		t.Errorf("body = %q, want the generic DELIVERED message", c.Body)
	}
}

func TestOrderReference_TruncatesLongIDs(t *testing.T) {
	got := orderReference("a9073790-53a7-442a-9281-b1d0b56db44c")
	if got != "a9073790" {
		t.Errorf("orderReference() = %q, want the first 8 characters", got)
	}
}

func TestOrderReference_ShortIDsUnchanged(t *testing.T) {
	got := orderReference("order-1")
	if got != "order-1" {
		t.Errorf("orderReference() = %q, want the id unchanged (it is already <= 8 chars)", got)
	}
}

// --- HTML email body ---

func TestBuildContent_HTMLBodyIncludesOrderIDAndStatus(t *testing.T) {
	c := buildContent(EventDelivered, "order-1", nil)
	if !strings.Contains(c.HTMLBody, "order-1") {
		t.Errorf("HTMLBody = %q, want it to include the order id", c.HTMLBody)
	}
	if !strings.Contains(c.HTMLBody, eventStatusLabel[EventDelivered]) {
		t.Errorf("HTMLBody = %q, want it to include the status label", c.HTMLBody)
	}
	if !strings.Contains(c.HTMLBody, eventHeadline[EventDelivered]) {
		t.Errorf("HTMLBody = %q, want it to include the headline", c.HTMLBody)
	}
	if !strings.HasPrefix(c.HTMLBody, "<!doctype html>") {
		t.Errorf("HTMLBody = %q, want a well-formed HTML document", c.HTMLBody)
	}
}

func TestBuildContent_HTMLBodyIncludesExtras(t *testing.T) {
	c := buildContent(EventFailed, "order-1", []byte(`{"failure_reason":"Recipient not available"}`))
	if !strings.Contains(c.HTMLBody, "Recipient not available") {
		t.Errorf("HTMLBody = %q, want the failure reason", c.HTMLBody)
	}
}

func TestBuildContent_HTMLBodyEscapesUntrustedMetadata(t *testing.T) {
	c := buildContent(EventFailed, "order-1", []byte(`{"failure_reason":"<script>alert(1)</script> & \"quoted\""}`))
	if strings.Contains(c.HTMLBody, "<script>") {
		t.Errorf("HTMLBody = %q, want the failure reason HTML-escaped, not injected raw", c.HTMLBody)
	}
	if !strings.Contains(c.HTMLBody, "&lt;script&gt;") {
		t.Errorf("HTMLBody = %q, want the escaped form of the failure reason present", c.HTMLBody)
	}
	// The plain-text Body is not HTML and must not be escaped — SMS and
	// the email's plain-text fallback should show the reason as-is.
	if !strings.Contains(c.Body, "<script>alert(1)</script>") {
		t.Errorf("Body = %q, want the raw, unescaped failure reason in the plain-text body", c.Body)
	}
}
