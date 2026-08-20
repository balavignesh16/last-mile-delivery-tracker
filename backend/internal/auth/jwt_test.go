package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"lastmiletracker/internal/users"
)

const testSecret = "test-signing-secret"

func TestGenerateToken_ParseToken_RoundTrip(t *testing.T) {
	token, err := GenerateToken(testSecret, "user-123", users.RoleAdmin, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	claims, err := ParseToken(testSecret, token)
	if err != nil {
		t.Fatalf("ParseToken() error: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", claims.Subject)
	}
	if claims.Role != string(users.RoleAdmin) {
		t.Errorf("Role = %q, want %q", claims.Role, users.RoleAdmin)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		t.Fatal("expected IssuedAt and ExpiresAt to be set")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt.Time) {
		t.Error("ExpiresAt should be after IssuedAt")
	}
}

func TestParseToken_RejectsExpiredToken(t *testing.T) {
	token, err := GenerateToken(testSecret, "user-123", users.RoleCustomer, -time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	if _, err := ParseToken(testSecret, token); err == nil {
		t.Error("expected an error for an expired token, got nil")
	}
}

func TestParseToken_RejectsWrongSecret(t *testing.T) {
	token, err := GenerateToken(testSecret, "user-123", users.RoleCustomer, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	if _, err := ParseToken("a-different-secret", token); err == nil {
		t.Error("expected an error when verifying with the wrong secret, got nil")
	}
}

func TestParseToken_RejectsTamperedToken(t *testing.T) {
	token, err := GenerateToken(testSecret, "user-123", users.RoleCustomer, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	// Flip a character in the payload segment.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a 3-segment JWT, got %d segments", len(parts))
	}
	tampered := parts[0] + "." + parts[1] + "x" + "." + parts[2]

	if _, err := ParseToken(testSecret, tampered); err == nil {
		t.Error("expected an error for a tampered token, got nil")
	}
}

func TestParseToken_RejectsMalformedToken(t *testing.T) {
	if _, err := ParseToken(testSecret, "not-a-jwt-at-all"); err == nil {
		t.Error("expected an error for a malformed token string, got nil")
	}
}

func TestParseToken_RejectsUnsupportedAlgorithm(t *testing.T) {
	// Craft a token signed with the "none" algorithm — must never be accepted.
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-123"},
		Role:             string(users.RoleAdmin),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to craft none-alg token: %v", err)
	}

	if _, err := ParseToken(testSecret, signed); err == nil {
		t.Error("expected the none algorithm to be rejected, got nil error")
	}
}
