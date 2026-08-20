package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubPinger lets tests control database reachability without a real
// PostgreSQL connection.
type stubPinger struct {
	err error
}

func (s stubPinger) Ping(ctx context.Context) error {
	return s.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthHandler_DatabaseUp(t *testing.T) {
	router := NewRouter(stubPinger{}, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != "ok" || body.Database != "ok" {
		t.Errorf("body = %+v, want status=ok database=ok", body)
	}
}

func TestHealthHandler_DatabaseDown(t *testing.T) {
	router := NewRouter(stubPinger{err: errors.New("dial tcp 10.0.0.5:5432: connect: connection refused, password=hunter2")}, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != "degraded" || body.Database != "unavailable" {
		t.Errorf("body = %+v, want status=degraded database=unavailable", body)
	}

	// The health endpoint must never leak internal connection details
	// (hosts, ports, credentials) into the response, even when the
	// underlying error contains them.
	raw := rec.Body.String()
	for _, secret := range []string{"10.0.0.5", "5432", "hunter2", "connection refused"} {
		if strings.Contains(raw, secret) {
			t.Errorf("response body leaked internal detail %q: %s", secret, raw)
		}
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	router := NewRouter(stubPinger{}, testLogger())

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
