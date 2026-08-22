package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withTwilioTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	original := twilioAPIBaseURL
	twilioAPIBaseURL = server.URL
	t.Cleanup(func() { twilioAPIBaseURL = original })
}

func TestTwilioSmsProvider_SendsExpectedRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuthUser, gotAuthPass, gotContentType string
	var gotTo, gotFrom, gotBody string
	withTwilioTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuthUser, gotAuthPass, _ = r.BasicAuth()
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form body: %v", err)
		}
		gotTo = r.PostForm.Get("To")
		gotFrom = r.PostForm.Get("From")
		gotBody = r.PostForm.Get("Body")
		w.WriteHeader(http.StatusCreated)
	})

	p := NewTwilioSmsProvider("AC_test_sid", "test_auth_token", "+15550100")
	err := p.SendSMS(context.Background(), "+15550199", "Your order is delivered.")
	if err != nil {
		t.Fatalf("SendSMS() error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.Contains(gotPath, "/Accounts/AC_test_sid/Messages.json") {
		t.Errorf("path = %q, want it to contain /Accounts/AC_test_sid/Messages.json", gotPath)
	}
	if gotAuthUser != "AC_test_sid" || gotAuthPass != "test_auth_token" {
		t.Errorf("basic auth = (%q, %q), want (AC_test_sid, test_auth_token)", gotAuthUser, gotAuthPass)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotContentType)
	}
	if gotTo != "+15550199" {
		t.Errorf("To = %q, want +15550199", gotTo)
	}
	if gotFrom != "+15550100" {
		t.Errorf("From = %q, want +15550100", gotFrom)
	}
	if gotBody != "Your order is delivered." {
		t.Errorf("Body = %q, want %q", gotBody, "Your order is delivered.")
	}
}

func TestTwilioSmsProvider_NonSuccessStatusIsError(t *testing.T) {
	withTwilioTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":20003,"message":"Authenticate"}`))
	})

	p := NewTwilioSmsProvider("AC_test_sid", "bad_token", "+15550100")
	err := p.SendSMS(context.Background(), "+15550199", "body")
	if err == nil {
		t.Fatal("expected an error for a non-2xx response, got nil")
	}
}

func TestTwilioSmsProvider_UnreachableServerIsError(t *testing.T) {
	original := twilioAPIBaseURL
	twilioAPIBaseURL = "http://127.0.0.1:1" // reserved, nothing ever listens here
	t.Cleanup(func() { twilioAPIBaseURL = original })

	p := NewTwilioSmsProvider("AC_test_sid", "test_auth_token", "+15550100")
	err := p.SendSMS(context.Background(), "+15550199", "body")
	if err == nil {
		t.Fatal("expected an error when the provider is unreachable, got nil")
	}
}

// TestTwilioSmsProvider_ErrorNeverLeaksAuthToken guards secret hygiene:
// SendSMS's error path must never let the auth token escape into a
// returned error string, since Service.dispatch logs provider errors
// verbatim (see service.go's own doc comment on why that's safe only
// if providers uphold this).
func TestTwilioSmsProvider_ErrorNeverLeaksAuthToken(t *testing.T) {
	const secretToken = "super-secret-auth-token-must-not-leak"
	withTwilioTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	p := NewTwilioSmsProvider("AC_test_sid", secretToken, "+15550100")
	err := p.SendSMS(context.Background(), "+15550199", "body")
	if err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Errorf("error message leaked the auth token: %v", err)
	}
}
