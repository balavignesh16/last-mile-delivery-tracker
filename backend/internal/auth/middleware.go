package auth

import (
	"context"
	"net/http"
	"strings"

	"lastmiletracker/internal/server"
	"lastmiletracker/internal/users"
)

type contextKey string

const identityContextKey contextKey = "identity"

// Identity is what RequireAuth places into the request context on a
// successful token check, and what RequireRole and handlers (e.g.
// GetMe) read back out.
type Identity struct {
	UserID string
	Role   users.Role
}

// IdentityFromContext retrieves the authenticated identity a prior
// RequireAuth call placed into the request context.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey).(Identity)
	return identity, ok
}

// RequireAuth reads the Authorization header, validates the bearer JWT,
// and places the resulting Identity into the request context. Every
// failure mode (missing header, malformed header, invalid signature,
// expired token, unsupported algorithm) is rejected identically with a
// generic 401 — this middleware never tells the caller which check
// failed, which is deliberate: that detail is only useful to an attacker
// probing the token format.
func RequireAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				server.WriteError(w, http.StatusUnauthorized, "missing or malformed authorization header")
				return
			}

			claims, err := ParseToken(jwtSecret, token)
			if err != nil {
				server.WriteError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			identity := Identity{UserID: claims.Subject, Role: users.Role(claims.Role)}
			ctx := context.WithValue(r.Context(), identityContextKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole must be chained after RequireAuth. It centralizes role
// authorization in one place so handlers never need "if user.Role ==
// ADMIN" checks scattered through business logic.
func RequireRole(roles ...users.Role) func(http.Handler) http.Handler {
	allowed := make(map[users.Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := IdentityFromContext(r.Context())
			if !ok {
				// Defensive: only reachable if a route uses RequireRole
				// without RequireAuth ahead of it. 401, not 403 — there is
				// no authenticated identity to be forbidden.
				server.WriteError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if _, permitted := allowed[identity.Role]; !permitted {
				server.WriteError(w, http.StatusForbidden, "insufficient role for this operation")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the token from a "Bearer <token>" Authorization
// header value. Case-insensitive on the scheme name per RFC 7235.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
