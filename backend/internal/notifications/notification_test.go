package notifications

import (
	"strings"
	"testing"
)

func TestBuildContent_GenericEvent(t *testing.T) {
	c := buildContent(EventPickedUp, "order-1", nil)
	if !strings.Contains(c.Body, "order-1") || !strings.Contains(c.Body, "PICKED_UP") {
		t.Errorf("body = %q, want it to mention the order id and event", c.Body)
	}
	if !strings.Contains(c.Subject, "order-1") {
		t.Errorf("subject = %q, want it to mention the order id", c.Subject)
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
	if !strings.Contains(c.Body, "FAILED") {
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
	if !strings.Contains(c.Body, "DELIVERED") {
		t.Errorf("body = %q, want the generic DELIVERED message", c.Body)
	}
}
