package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"lastmiletracker/internal/users"
)

// TokenExpiry is the assignment-appropriate session length: long enough
// that an evaluator working through a demo scenario is never logged out
// mid-flow, short enough to be a real expiration. No refresh tokens — out
// of scope per the frozen architecture; a user who wants a longer session
// just logs in again.
const TokenExpiry = 24 * time.Hour

// Claims is deliberately minimal: subject (user ID), role, issued-at, and
// expiration. Nothing else — no permissions list, no session ID, nothing
// that would need its own invalidation story.
type Claims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
}

// GenerateToken signs a new JWT for the given user using HS256 — a
// symmetric algorithm, the right choice here since one backend both
// issues and verifies every token; there is no separate issuer that would
// justify RS256's asymmetric keypair.
func GenerateToken(secret, userID string, role users.Role, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		Role: string(role),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken validates signature and expiration and returns the claims.
// Every failure mode — malformed, wrong signature, expired, unsupported
// algorithm — returns a plain error; callers must not inspect it for
// client-facing detail (see RequireAuth, which always responds with the
// same generic 401 message regardless of which check failed).
func ParseToken(secret, tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid || claims.Subject == "" {
		return nil, errors.New("token is not valid")
	}
	return claims, nil
}
