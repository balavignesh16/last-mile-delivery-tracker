package server

import (
	"context"
	"net/http"
	"time"
)

const healthCheckTimeout = 3 * time.Second

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// healthHandler reports whether the application and its database
// connection are both reachable. It deliberately never includes
// connection strings, credentials, or any other internal detail —
// only a status word for each dependency.
func healthHandler(pinger HealthPinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		resp := healthResponse{Status: "ok", Database: "ok"}
		statusCode := http.StatusOK

		if err := pinger.Ping(ctx); err != nil {
			resp.Status = "degraded"
			resp.Database = "unavailable"
			statusCode = http.StatusServiceUnavailable
		}

		WriteJSON(w, statusCode, resp)
	}
}
