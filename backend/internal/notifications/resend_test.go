package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withResendTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	original := resendAPIURL
	resendAPIURL = server.URL
	t.Cleanup(func() { resendAPIURL = original })
}

func TestResendEmailProvider_SendsExpectedRequest(t *testing.T) {
	var gotAuth, gotContentType string
	var gotBody resendEmailRequest
	withResendTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})

	p := NewResendEmailProvider("re_test_key", "orders@example.com")
	err := p.SendEmail(context.Background(), "customer@example.com", "Order X: DELIVERED", "Your order is delivered.")
	if err != nil {
		t.Fatalf("SendEmail() error: %v", err)
	}

	if gotAuth != "Bearer re_test_key" {
		t.Errorf("Authorization header = %q, want Bearer re_test_key", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody.From != "orders@example.com" {
		t.Errorf("From = %q, want orders@example.com", gotBody.From)
	}
	if len(gotBody.To) != 1 || gotBody.To[0] != "customer@example.com" {
		t.Errorf("To = %v, want [customer@example.com]", gotBody.To)
	}
	if gotBody.Subject != "Order X: DELIVERED" {
		t.Errorf("Subject = %q, want %q", gotBody.Subject, "Order X: DELIVERED")
	}
	if gotBody.Text != "Your order is delivered." {
		t.Errorf("Text = %q, want %q", gotBody.Text, "Your order is delivered.")
	}
}

func TestResendEmailProvider_NonSuccessStatusIsError(t *testing.T) {
	withResendTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid API key"}`))
	})

	p := NewResendEmailProvider("bad-key", "orders@example.com")
	err := p.SendEmail(context.Background(), "customer@example.com", "subject", "body")
	if err == nil {
		t.Fatal("expected an error for a non-2xx response, got nil")
	}
}

func TestResendEmailProvider_UnreachableServerIsError(t *testing.T) {
	original := resendAPIURL
	resendAPIURL = "http://127.0.0.1:1" // reserved, nothing ever listens here
	t.Cleanup(func() { resendAPIURL = original })

	p := NewResendEmailProvider("re_test_key", "orders@example.com")
	err := p.SendEmail(context.Background(), "customer@example.com", "subject", "body")
	if err == nil {
		t.Fatal("expected an error when the provider is unreachable, got nil")
	}
}
