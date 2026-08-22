package server

import "net/http"

// CORS returns middleware that adds the standard CORS response headers
// for any request whose Origin header exactly matches one of
// allowedOrigins, and short-circuits a preflight OPTIONS request with a
// 204 rather than forwarding it into the router. A nil/empty
// allowedOrigins adds no headers at all and changes no behavior — this
// is why CORS is a separate wrapper around the router rather than a new
// parameter on NewRouter itself: every existing test in this project
// builds its router via server.NewRouter directly and never needed to
// know CORS exists; only cmd/server/main.go wraps the real router with
// this middleware, reading allowed origins from CORS_ALLOWED_ORIGINS.
//
// No cookie is ever used for authentication in this API (JWTs travel in
// the Authorization header only), so Access-Control-Allow-Credentials is
// deliberately never set — that header exists for cookie-based auth,
// which this project doesn't have.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				// Always vary on Origin once an origin-conditional header
				// might be set — otherwise a shared cache could serve one
				// origin's CORS headers to a different origin's request.
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
