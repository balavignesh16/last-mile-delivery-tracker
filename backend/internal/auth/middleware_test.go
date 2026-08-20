package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lastmiletracker/internal/users"
)

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// --- RequireAuth ---

func TestRequireAuth_RejectsMissingHeader(t *testing.T) {
	handler := RequireAuth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_RejectsMalformedHeader(t *testing.T) {
	cases := []string{"Bearer", "Bearer ", "Token abc123", "abc123"}
	for _, header := range cases {
		t.Run(header, func(t *testing.T) {
			handler := RequireAuth(testSecret)(okHandler())
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", header)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("header %q: status = %d, want %d", header, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRequireAuth_RejectsInvalidToken(t *testing.T) {
	handler := RequireAuth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_RejectsExpiredToken(t *testing.T) {
	token, err := GenerateToken(testSecret, "user-1", users.RoleCustomer, -time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	handler := RequireAuth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_AcceptsValidTokenAndSetsIdentity(t *testing.T) {
	token, err := GenerateToken(testSecret, "user-42", users.RoleDeliveryAgent, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	var gotIdentity Identity
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, gotOK = IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAuth(testSecret)(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !gotOK {
		t.Fatal("expected an identity in context, found none")
	}
	if gotIdentity.UserID != "user-42" || gotIdentity.Role != users.RoleDeliveryAgent {
		t.Errorf("identity = %+v, want UserID=user-42 Role=DELIVERY_AGENT", gotIdentity)
	}
}

// --- RequireRole ---
//
// M02 has no real role-gated production endpoint yet (that starts in
// M03+), so these tests build a minimal, test-only handler chain to prove
// the middleware itself — not a production route, per the instruction not
// to invent fake business endpoints.

func withIdentity(identity Identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), identityContextKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestRequireRole_AllowsPermittedRole(t *testing.T) {
	identity := Identity{UserID: "u1", Role: users.RoleAdmin}
	handler := withIdentity(identity)(RequireRole(users.RoleAdmin)(okHandler()))

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireRole_AllowsAnyOfMultiplePermittedRoles(t *testing.T) {
	for _, role := range []users.Role{users.RoleCustomer, users.RoleDeliveryAgent} {
		t.Run(string(role), func(t *testing.T) {
			identity := Identity{UserID: "u1", Role: role}
			handler := withIdentity(identity)(RequireRole(users.RoleCustomer, users.RoleDeliveryAgent)(okHandler()))

			req := httptest.NewRequest(http.MethodGet, "/shared", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("role %s: status = %d, want %d", role, rec.Code, http.StatusOK)
			}
		})
	}
}

func TestRequireRole_ForbidsWrongRole(t *testing.T) {
	identity := Identity{UserID: "u1", Role: users.RoleCustomer}
	handler := withIdentity(identity)(RequireRole(users.RoleAdmin)(okHandler()))

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireRole_WithoutPriorAuthReturns401NotForbidden(t *testing.T) {
	// Defensive case: RequireRole used without RequireAuth ahead of it.
	// No identity exists to be forbidden, so this must be 401, not 403.
	handler := RequireRole(users.RoleAdmin)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_RequireRole_ComposeCorrectly(t *testing.T) {
	// End-to-end composition using real JWT generation + real parsing,
	// proving the two middlewares chain correctly (not just each in
	// isolation, as the tests above verify).
	adminToken, err := GenerateToken(testSecret, "admin-1", users.RoleAdmin, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	customerToken, err := GenerateToken(testSecret, "cust-1", users.RoleCustomer, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	handler := RequireAuth(testSecret)(RequireRole(users.RoleAdmin)(okHandler()))

	t.Run("admin allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("customer forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
		req.Header.Set("Authorization", "Bearer "+customerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("unauthenticated rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}
