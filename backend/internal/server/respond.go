package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON is the centralized JSON response writer. Every handler — in
// this package and in every module that follows — should respond through
// this function (or WriteError below) so response shape and header
// handling stay consistent across the whole API. Exported so business
// packages (auth, users, and later modules) can reuse it instead of each
// reimplementing response writing.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// ErrorResponse is the shape of every error body this API returns.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteError is the centralized error-response writer. It intentionally
// takes only a client-safe message — never a raw Go error — so internal
// details (SQL errors, stack traces, secrets) can never leak into an HTTP
// response by accident.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotFound, "the requested resource does not exist")
}

func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusMethodNotAllowed, "method not allowed on this resource")
}
