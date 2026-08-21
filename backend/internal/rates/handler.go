package rates

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"lastmiletracker/internal/server"
)

const timeLayout = time.RFC3339

type rateCardResponse struct {
	ID               string  `json:"id"`
	OrderType        string  `json:"order_type"`
	ZoneRelationship string  `json:"zone_relationship"`
	CODSurcharge     float64 `json:"cod_surcharge"`
	Active           bool    `json:"active"`
	CreatedAt        string  `json:"created_at"`
}

func toRateCardResponse(rc RateCard) rateCardResponse {
	return rateCardResponse{
		ID:               rc.ID,
		OrderType:        string(rc.OrderType),
		ZoneRelationship: string(rc.ZoneRelationship),
		CODSurcharge:     rc.CODSurcharge,
		Active:           rc.Active,
		CreatedAt:        rc.CreatedAt.Format(timeLayout),
	}
}

type slabResponse struct {
	ID         string   `json:"id"`
	RateCardID string   `json:"rate_card_id"`
	MinWeight  float64  `json:"min_weight"`
	MaxWeight  *float64 `json:"max_weight"`
	Price      float64  `json:"price"`
	CreatedAt  string   `json:"created_at"`
}

func toSlabResponse(s Slab) slabResponse {
	return slabResponse{
		ID:         s.ID,
		RateCardID: s.RateCardID,
		MinWeight:  s.MinWeight,
		MaxWeight:  s.MaxWeight,
		Price:      s.Price,
		CreatedAt:  s.CreatedAt.Format(timeLayout),
	}
}

// --- Rate cards ---

// createRateCardRequest has no Active field — every rate card is
// created inactive; see CreateRateCardInput's doc.
type createRateCardRequest struct {
	OrderType        string  `json:"order_type"`
	ZoneRelationship string  `json:"zone_relationship"`
	CODSurcharge     float64 `json:"cod_surcharge"`
}

// CreateRateCardHandler handles POST /api/v1/rates (admin-only —
// enforced by the RBAC middleware in routes.go).
func CreateRateCardHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRateCardRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		orderType, ok := ParseOrderType(req.OrderType)
		if !ok {
			server.WriteError(w, http.StatusUnprocessableEntity, "order_type must be one of B2B, B2C")
			return
		}
		zoneRelationship, ok := ParseZoneRelationship(req.ZoneRelationship)
		if !ok {
			server.WriteError(w, http.StatusUnprocessableEntity, "zone_relationship must be one of INTRA, INTER")
			return
		}
		if req.CODSurcharge < 0 {
			server.WriteError(w, http.StatusUnprocessableEntity, "cod_surcharge must not be negative")
			return
		}

		created, err := repo.CreateRateCard(r.Context(), CreateRateCardInput{
			OrderType:        orderType,
			ZoneRelationship: zoneRelationship,
			CODSurcharge:     req.CODSurcharge,
		})
		if err != nil {
			slog.Error("rate card creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create rate card")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toRateCardResponse(created))
	}
}

// ListRateCardsHandler handles GET /api/v1/rates (admin-only). Returns
// every rate card, active or not.
func ListRateCardsHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := repo.ListRateCards(r.Context())
		if err != nil {
			slog.Error("rate card listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list rate cards")
			return
		}
		responses := make([]rateCardResponse, len(list))
		for i, rc := range list {
			responses[i] = toRateCardResponse(rc)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

// GetRateCardHandler handles GET /api/v1/rates/{id} (admin-only).
func GetRateCardHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		rc, err := repo.FindRateCardByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrRateCardNotFound) {
				server.WriteError(w, http.StatusNotFound, "rate card not found")
				return
			}
			slog.Error("rate card lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not load rate card")
			return
		}

		server.WriteJSON(w, http.StatusOK, toRateCardResponse(rc))
	}
}

// updateRateCardRequest has no order_type/zone_relationship field — a
// rate card's identity is immutable after creation, the same reasoning
// M04 used for not letting an area move between zones. Active is a
// pointer so an omitted field means "leave unchanged" — see
// RateCardUpdate's doc.
type updateRateCardRequest struct {
	CODSurcharge float64 `json:"cod_surcharge"`
	Active       *bool   `json:"active"`
}

// UpdateRateCardHandler handles PUT /api/v1/rates/{id} (admin-only).
// This is also how a rate card is activated/deactivated — there is no
// separate endpoint for that, the same design as zones.
func UpdateRateCardHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req updateRateCardRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}
		if req.CODSurcharge < 0 {
			server.WriteError(w, http.StatusUnprocessableEntity, "cod_surcharge must not be negative")
			return
		}

		updated, err := repo.UpdateRateCard(r.Context(), id, RateCardUpdate{CODSurcharge: req.CODSurcharge, Active: req.Active})
		if err != nil {
			if errors.Is(err, ErrRateCardNotFound) {
				server.WriteError(w, http.StatusNotFound, "rate card not found")
				return
			}
			if errors.Is(err, ErrActiveCombinationExists) {
				server.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			slog.Error("rate card update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update rate card")
			return
		}

		server.WriteJSON(w, http.StatusOK, toRateCardResponse(updated))
	}
}

// --- Slabs ---

// createSlabRequest has no rate_card_id field — the same structural
// pattern as zones.createAreaRequest having no zone_id field. MinWeight
// and Price are pointers so an omitted field is rejected outright
// rather than silently treated as 0 — 0 is a legitimate value for both
// (the first slab starts at 0; a free slab has price 0), so "not
// provided" must stay distinguishable from "explicitly zero", the same
// reasoning agents.updateLocationRequest uses *float64 for coordinates.
type createSlabRequest struct {
	MinWeight *float64 `json:"min_weight"`
	MaxWeight *float64 `json:"max_weight"`
	Price     *float64 `json:"price"`
}

// CreateSlabHandler handles POST /api/v1/rates/{rateCardID}/slabs
// (admin-only).
func CreateSlabHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rateCardID := chi.URLParam(r, "rateCardID")

		var req createSlabRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		if problem := validateSlabRequest(req.MinWeight, req.MaxWeight, req.Price); problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		created, err := repo.CreateSlab(r.Context(), rateCardID, CreateSlabInput{
			MinWeight: *req.MinWeight,
			MaxWeight: req.MaxWeight,
			Price:     *req.Price,
		})
		if err != nil {
			if status, message, ok := mapSlabDomainError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("slab creation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not create slab")
			return
		}

		server.WriteJSON(w, http.StatusCreated, toSlabResponse(created))
	}
}

// ListSlabsHandler handles GET /api/v1/rates/{rateCardID}/slabs
// (admin-only). Checks the rate card exists first so "rate card doesn't
// exist" (404) and "rate card exists but has no slabs yet" (200, empty
// list) are distinguishable — the frontend needs that distinction for
// its empty state, the same reasoning as zones.ListAreasHandler.
func ListSlabsHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rateCardID := chi.URLParam(r, "rateCardID")

		if _, err := repo.FindRateCardByID(r.Context(), rateCardID); err != nil {
			if errors.Is(err, ErrRateCardNotFound) {
				server.WriteError(w, http.StatusNotFound, "rate card not found")
				return
			}
			slog.Error("rate card lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list slabs")
			return
		}

		list, err := repo.ListSlabsByRateCard(r.Context(), rateCardID)
		if err != nil {
			slog.Error("slab listing failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not list slabs")
			return
		}
		responses := make([]slabResponse, len(list))
		for i, s := range list {
			responses[i] = toSlabResponse(s)
		}
		server.WriteJSON(w, http.StatusOK, responses)
	}
}

type updateSlabRequest struct {
	MinWeight *float64 `json:"min_weight"`
	MaxWeight *float64 `json:"max_weight"`
	Price     *float64 `json:"price"`
}

// UpdateSlabHandler handles PUT
// /api/v1/rates/{rateCardID}/slabs/{slabID} (admin-only). Verifies the
// slab actually belongs to the rate card named in the URL before
// allowing any change — the exact ownership/path-integrity check
// zones.UpdateAreaHandler uses for areas and zones.
func UpdateSlabHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rateCardID := chi.URLParam(r, "rateCardID")
		slabID := chi.URLParam(r, "slabID")

		slab, err := repo.FindSlabByID(r.Context(), slabID)
		if err != nil {
			if errors.Is(err, ErrSlabNotFound) {
				server.WriteError(w, http.StatusNotFound, "slab not found")
				return
			}
			slog.Error("slab lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update slab")
			return
		}
		if slab.RateCardID != rateCardID {
			// The slab exists, but under a different rate card — this
			// URL simply does not identify a resource. Treated
			// identically to "not found", never a more specific error,
			// so this endpoint never confirms a slab's existence under
			// the wrong parent.
			server.WriteError(w, http.StatusNotFound, "slab not found")
			return
		}

		var req updateSlabRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		if problem := validateSlabRequest(req.MinWeight, req.MaxWeight, req.Price); problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		updated, err := repo.UpdateSlab(r.Context(), slabID, SlabUpdate{
			MinWeight: *req.MinWeight,
			MaxWeight: req.MaxWeight,
			Price:     *req.Price,
		})
		if err != nil {
			if status, message, ok := mapSlabDomainError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("slab update failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not update slab")
			return
		}

		server.WriteJSON(w, http.StatusOK, toSlabResponse(updated))
	}
}

// DeleteSlabHandler handles DELETE
// /api/v1/rates/{rateCardID}/slabs/{slabID} (admin-only). Same
// ownership/path-integrity check as UpdateSlabHandler: a slab can only
// ever be deleted through the URL of the rate card it actually belongs
// to.
func DeleteSlabHandler(repo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rateCardID := chi.URLParam(r, "rateCardID")
		slabID := chi.URLParam(r, "slabID")

		slab, err := repo.FindSlabByID(r.Context(), slabID)
		if err != nil {
			if errors.Is(err, ErrSlabNotFound) {
				server.WriteError(w, http.StatusNotFound, "slab not found")
				return
			}
			slog.Error("slab lookup failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not delete slab")
			return
		}
		if slab.RateCardID != rateCardID {
			server.WriteError(w, http.StatusNotFound, "slab not found")
			return
		}

		if err := repo.DeleteSlab(r.Context(), slabID); err != nil {
			if errors.Is(err, ErrSlabNotFound) {
				server.WriteError(w, http.StatusNotFound, "slab not found")
				return
			}
			slog.Error("slab deletion failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not delete slab")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// validateSlabRequest is shared by create and update: both require
// MinWeight/Price, both treat MaxWeight's absence as "open-ended", and
// both apply the same range rule.
func validateSlabRequest(minWeight, maxWeight, price *float64) string {
	if minWeight == nil || price == nil {
		return "min_weight and price are both required"
	}
	if math.IsNaN(*minWeight) || math.IsInf(*minWeight, 0) || math.IsNaN(*price) || math.IsInf(*price, 0) {
		return "min_weight and price must be finite numbers"
	}
	if *minWeight < 0 {
		return "min_weight must not be negative"
	}
	if *price < 0 {
		return "price must not be negative"
	}
	if maxWeight != nil {
		if math.IsNaN(*maxWeight) || math.IsInf(*maxWeight, 0) {
			return "max_weight must be a finite number, or omitted for an open-ended slab"
		}
		if *maxWeight <= *minWeight {
			return "max_weight must be greater than min_weight"
		}
	}
	return ""
}

// mapSlabDomainError translates the slab package's domain errors into
// (status, message) pairs shared by CreateSlabHandler and
// UpdateSlabHandler.
func mapSlabDomainError(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, ErrRateCardNotFound):
		return http.StatusNotFound, "rate card not found", true
	case errors.Is(err, ErrSlabOverlap), errors.Is(err, ErrMultipleOpenEndedSlabs):
		return http.StatusConflict, err.Error(), true
	default:
		return 0, "", false
	}
}
