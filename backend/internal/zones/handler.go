package zones

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/geo"
	"lastmiletracker/internal/server"
)

const maxNameLength = 100

type zoneResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func toZoneResponse(z Zone) zoneResponse {
	return zoneResponse{
		ID:        z.ID,
		Name:      z.Name,
		Active:    z.Active,
		CreatedAt: z.CreatedAt.Format(time.RFC3339),
	}
}

type areaResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ZoneID    string   `json:"zone_id"`
	Active    bool     `json:"active"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	CreatedAt string   `json:"created_at"`
}

func toAreaResponse(a Area) areaResponse {
	return areaResponse{
		ID:        a.ID,
		Name:      a.Name,
		ZoneID:    a.ZoneID,
		Active:    a.Active,
		Latitude:  a.Latitude,
		Longitude: a.Longitude,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
}

// validateOptionalCoordinates enforces "both provided or neither" for
// an area's optional latitude/longitude, then delegates the actual
// range/finite-number check to geo.ValidateCoordinates — the same
// shared core internal/agents' own (required-pair) coordinate
// validation delegates to, so the range rules are defined in exactly
// one place. Unlike an agent's location, an area's coordinates are
// genuinely optional configuration (most areas will have none until an
// admin sets real ones), so "neither provided" is valid here in a way
// it deliberately is not for internal/agents.validateCoordinates.
func validateOptionalCoordinates(lat, lng *float64) string {
	if lat == nil && lng == nil {
		return ""
	}
	if lat == nil || lng == nil {
		return "latitude and longitude must both be provided together"
	}
	return geo.ValidateCoordinates(*lat, *lng)
}

// validateName is shared by zone and area name validation — both have
// identical rules (required, trimmed, bounded length), and the task
// explicitly says not to invent divergent naming rules for the two.
func validateName(raw string) (string, string) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", "name is required"
	}
	if len(name) > maxNameLength {
		return "", fmt.Sprintf("name must be at most %d characters", maxNameLength)
	}
	return name, ""
}

// --- Zones ---

type createZoneRequest struct {
	Name string `json:"name"`
}

// CreateZoneHandler handles POST /api/v1/zones (admin-only — enforced by
// the RBAC middleware in routes.go).
func CreateZoneHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createZoneRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		name, problem := validateName(req.Name)
		if problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		created, err := repo.CreateZone(r.Context(), CreateZoneInput{Name: name})
		if err != nil {
			if errors.Is(err, ErrZoneNameTaken) {
				server.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			slog.Error("zone creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create zone")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toZoneResponse(created))
	}
}

// ListZonesHandler handles GET /api/v1/zones (admin-only). Returns every
// zone, active or not — M04 exposes the active flag; it does not filter
// on it, since nothing in the assignment asks for an "active zones only"
// view yet.
func ListZonesHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := repo.ListZones(r.Context())
		if err != nil {
			slog.Error("zone listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list zones")
			return
		}
		responses := make([]zoneResponse, len(list))
		for i, z := range list {
			responses[i] = toZoneResponse(z)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

// GetZoneHandler handles GET /api/v1/zones/{id} (admin-only).
func GetZoneHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		zone, err := repo.FindZoneByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrZoneNotFound) {
				server.WriteError(w, http.StatusNotFound, "zone not found")
				return
			}
			slog.Error("zone lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load zone")
			return
		}

		server.WriteJSON(w, http.StatusOK, toZoneResponse(zone))
	}
}

type updateZoneRequest struct {
	Name string `json:"name"`
	// Active is a pointer so an omitted field means "leave unchanged" —
	// see ZoneUpdate's doc.
	Active *bool `json:"active"`
}

// UpdateZoneHandler handles PUT /api/v1/zones/{id} (admin-only). This is
// also how a zone is activated/deactivated — there is no separate
// activate/deactivate endpoint, since the responsibility maps cleanly
// onto a single PUT.
func UpdateZoneHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req updateZoneRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		name, problem := validateName(req.Name)
		if problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		updated, err := repo.UpdateZone(r.Context(), id, ZoneUpdate{Name: name, Active: req.Active})
		if err != nil {
			if errors.Is(err, ErrZoneNotFound) {
				server.WriteError(w, http.StatusNotFound, "zone not found")
				return
			}
			if errors.Is(err, ErrZoneNameTaken) {
				server.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			slog.Error("zone update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update zone")
			return
		}

		server.WriteJSON(w, http.StatusOK, toZoneResponse(updated))
	}
}

// --- Areas ---

// createAreaRequest has no zone_id field — the same structural pattern
// as agents.createAgentRequest having no role field. The zone an area
// belongs to is always the {zoneID} segment of the URL this handler is
// mounted on; a client cannot override that via the request body, because
// there is nowhere in this type for such a value to go, and
// DisallowUnknownFields rejects the whole request (422) if one is sent
// anyway.
type createAreaRequest struct {
	Name string `json:"name"`
	// Latitude/Longitude are optional — see CreateAreaInput's doc.
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// CreateAreaHandler handles POST /api/v1/zones/{zoneID}/areas
// (admin-only). Checks the zone exists first so an unknown zone id
// produces a clear 404 rather than a generic 500/constraint error.
func CreateAreaHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zoneID := chi.URLParam(r, "zoneID")

		if _, err := repo.FindZoneByID(r.Context(), zoneID); err != nil {
			if errors.Is(err, ErrZoneNotFound) {
				server.WriteError(w, http.StatusNotFound, "zone not found")
				return
			}
			slog.Error("zone lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create area")
			return
		}

		var req createAreaRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		name, problem := validateName(req.Name)
		if problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}
		if problem := validateOptionalCoordinates(req.Latitude, req.Longitude); problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		created, err := repo.CreateArea(r.Context(), zoneID, CreateAreaInput{Name: name, Latitude: req.Latitude, Longitude: req.Longitude})
		if err != nil {
			if errors.Is(err, ErrAreaNameTaken) {
				server.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, ErrZoneNotFound) {
				server.WriteError(w, http.StatusNotFound, "zone not found")
				return
			}
			slog.Error("area creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create area")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toAreaResponse(created))
	}
}

// ListAreasHandler handles GET /api/v1/zones/{zoneID}/areas (admin-only).
// Checks the zone exists first so "zone doesn't exist" (404) and "zone
// exists but has no areas yet" (200, empty list) are distinguishable —
// the frontend needs that distinction to show the right empty state.
func ListAreasHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zoneID := chi.URLParam(r, "zoneID")

		if _, err := repo.FindZoneByID(r.Context(), zoneID); err != nil {
			if errors.Is(err, ErrZoneNotFound) {
				server.WriteError(w, http.StatusNotFound, "zone not found")
				return
			}
			slog.Error("zone lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list areas")
			return
		}

		list, err := repo.ListAreasByZone(r.Context(), zoneID)
		if err != nil {
			slog.Error("area listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list areas")
			return
		}
		responses := make([]areaResponse, len(list))
		for i, a := range list {
			responses[i] = toAreaResponse(a)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

type updateAreaRequest struct {
	Name string `json:"name"`
	// Active/Latitude/Longitude are pointers so an omitted field means
	// "leave unchanged" — see AreaUpdate's doc.
	Active    *bool    `json:"active"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// UpdateAreaHandler handles PUT /api/v1/zones/{zoneID}/areas/{areaID}
// (admin-only). Renaming and active-toggling — the same "one PUT does
// both" shape UpdateZoneHandler already uses; see AreaUpdate's doc for
// why moving an area between zones isn't offered.
func UpdateAreaHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zoneID := chi.URLParam(r, "zoneID")
		areaID := chi.URLParam(r, "areaID")

		area, err := repo.FindAreaByID(r.Context(), areaID)
		if err != nil {
			if errors.Is(err, ErrAreaNotFound) {
				server.WriteError(w, http.StatusNotFound, "area not found")
				return
			}
			slog.Error("area lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update area")
			return
		}
		if area.ZoneID != zoneID {
			// The area exists, but under a different zone — this URL
			// simply does not identify a resource. Treated identically to
			// "not found" rather than a more specific error, so this
			// endpoint never confirms an area's existence under a zone
			// the caller guessed wrong.
			server.WriteError(w, http.StatusNotFound, "area not found")
			return
		}

		var req updateAreaRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		name, problem := validateName(req.Name)
		if problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}
		if problem := validateOptionalCoordinates(req.Latitude, req.Longitude); problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		updated, err := repo.UpdateArea(r.Context(), areaID, AreaUpdate{Name: name, Active: req.Active, Latitude: req.Latitude, Longitude: req.Longitude})
		if err != nil {
			if errors.Is(err, ErrAreaNameTaken) {
				server.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, ErrAreaNotFound) {
				server.WriteError(w, http.StatusNotFound, "area not found")
				return
			}
			slog.Error("area update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update area")
			return
		}

		server.WriteJSON(w, http.StatusOK, toAreaResponse(updated))
	}
}
