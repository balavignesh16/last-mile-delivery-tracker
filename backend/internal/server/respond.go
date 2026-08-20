package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeJSON is the centralized JSON response writer. Every handler in this
// module — and every handler later modules add — should respond through
// this function (or errorResponse below) so response shape and header
// handling stay consistent across the whole API.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

// writeError is the centralized error-response writer. It intentionally
// takes only a client-safe message — never a raw Go error — so internal
// details can never leak into an HTTP response by accident.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "the requested resource does not exist")
}

func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed on this resource")
}
