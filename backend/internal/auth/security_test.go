package auth

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lastmiletracker/internal/users"
)

// withCapturedLogs redirects the package-level default logger to an
// in-memory buffer for the duration of fn, then restores the previous
// default logger. Handlers in this package log via slog.Error/slog.Info
// (the package-level default), matching the pattern established in M01's
// main.go (slog.SetDefault once at startup) — this test helper exploits
// that same mechanism to observe what gets logged.
func withCapturedLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)

	fn()
	return buf.String()
}

func TestRegisterHandler_NeverLogsPlaintextPassword(t *testing.T) {
	repo := newFakeRepo()
	const secretPassword = "SuperSecretPassword123"

	logs := withCapturedLogs(t, func() {
		doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
			`{"email":"logtest@example.com","password":"`+secretPassword+`","full_name":"Log Test"}`)
	})

	if strings.Contains(logs, secretPassword) {
		t.Errorf("plaintext password leaked into logs: %s", logs)
	}
}

func TestLoginHandler_NeverLogsPasswordOrToken(t *testing.T) {
	repo := newFakeRepo()
	const password = "AnotherSecretPassword456"
	_, err := repo.Create(t.Context(), newUserFor(t, "logintest@example.com", password))
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	var rec *httptest.ResponseRecorder
	logs := withCapturedLogs(t, func() {
		rec = doRequest(t, LoginHandler(repo, testSecret), http.MethodPost, "/api/v1/auth/login",
			`{"email":"logintest@example.com","password":"`+password+`"}`)
	})

	if strings.Contains(logs, password) {
		t.Errorf("plaintext password leaked into logs: %s", logs)
	}

	body := decodeJSON[loginResponse](t, rec)
	if body.Token == "" {
		t.Fatal("expected a token in the response to test against")
	}
	if strings.Contains(logs, body.Token) {
		t.Errorf("JWT leaked into logs: %s", logs)
	}
}

func TestRegisterHandler_NeverReturnsPasswordHashEvenOnRawJSON(t *testing.T) {
	repo := newFakeRepo()
	rec := doRequest(t, RegisterHandler(repo), http.MethodPost, "/api/v1/auth/register",
		`{"email":"rawcheck@example.com","password":"password123","full_name":"Raw Check"}`)

	raw := rec.Body.String()
	if strings.Contains(raw, "password_hash") {
		t.Errorf("raw response body contains the string password_hash: %s", raw)
	}
	if strings.Contains(raw, "$2a$") || strings.Contains(raw, "$2b$") {
		t.Errorf("raw response body appears to contain a bcrypt hash: %s", raw)
	}
}

func newUserFor(t *testing.T, email, password string) users.NewUser {
	t.Helper()
	return users.NewUser{
		Email:        email,
		PasswordHash: mustHashForTest(t, password),
		FullName:     "Security Test User",
		Role:         users.RoleCustomer,
	}
}
