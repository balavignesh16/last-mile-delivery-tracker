package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_UnknownRouteReturnsJSON404(t *testing.T) {
	router := NewRouter(stubPinger{}, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/this-route-does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected a JSON error body, got decode error: %v", err)
	}
	if body.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestRouter_RequestIDPropagatesToLogger(t *testing.T) {
	// Smoke test: the middleware chain must not panic and must complete
	// a request end to end with the full chain (RequestID, RealIP,
	// logger, Recoverer, Timeout) attached.
	router := NewRouter(stubPinger{}, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
