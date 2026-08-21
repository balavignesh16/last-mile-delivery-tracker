package rates

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"

	"lastmiletracker/internal/server"
	"lastmiletracker/internal/zones"
)

// quoteRequest is POST /orders/quote's only input. Every field here is
// something the caller must legitimately supply; everything the pricing
// pipeline derives (customer id, pickup/drop zone id, zone relationship,
// rate card id, volumetric/chargeable weight, base rate, cod surcharge,
// final amount, status) has deliberately no field to decode into.
// DisallowUnknownFields turns any attempt to send one of those anyway
// into a 422, the same structural mass-assignment protection every prior
// module in this project uses — never a silently-dropped field.
type quoteRequest struct {
	PickupAreaID   string   `json:"pickup_area_id"`
	DropAreaID     string   `json:"drop_area_id"`
	OrderType      string   `json:"order_type"`
	PaymentType    string   `json:"payment_type"`
	LengthCM       *float64 `json:"length_cm"`
	BreadthCM      *float64 `json:"breadth_cm"`
	HeightCM       *float64 `json:"height_cm"`
	ActualWeightKG *float64 `json:"actual_weight_kg"`
}

// validateQuoteRequest turns a decoded quoteRequest into a QuoteInput,
// or returns a client-safe validation message. Dimensions and actual
// weight are pointers so "omitted" (rejected) stays distinguishable from
// "explicitly zero" (also rejected, but with a different, more precise
// reason) — the same *float64 idiom createSlabRequest uses.
func validateQuoteRequest(req quoteRequest) (QuoteInput, string) {
	if req.PickupAreaID == "" || req.DropAreaID == "" {
		return QuoteInput{}, "pickup_area_id and drop_area_id are required"
	}

	orderType, ok := ParseOrderType(req.OrderType)
	if !ok {
		return QuoteInput{}, "order_type must be one of B2B, B2C"
	}

	paymentType, ok := ParsePaymentType(req.PaymentType)
	if !ok {
		return QuoteInput{}, "payment_type must be one of PREPAID, COD"
	}

	if req.LengthCM == nil || req.BreadthCM == nil || req.HeightCM == nil || req.ActualWeightKG == nil {
		return QuoteInput{}, "length_cm, breadth_cm, height_cm, and actual_weight_kg are all required"
	}

	dims := [4]float64{*req.LengthCM, *req.BreadthCM, *req.HeightCM, *req.ActualWeightKG}
	for _, v := range dims {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return QuoteInput{}, "length_cm, breadth_cm, height_cm, and actual_weight_kg must be finite numbers"
		}
		if v <= 0 {
			return QuoteInput{}, "length_cm, breadth_cm, height_cm, and actual_weight_kg must all be strictly greater than zero"
		}
	}

	return QuoteInput{
		PickupAreaID:   req.PickupAreaID,
		DropAreaID:     req.DropAreaID,
		OrderType:      orderType,
		PaymentType:    paymentType,
		LengthCM:       *req.LengthCM,
		BreadthCM:      *req.BreadthCM,
		HeightCM:       *req.HeightCM,
		ActualWeightKG: *req.ActualWeightKG,
	}, ""
}

type quoteResponse struct {
	PickupAreaID     string `json:"pickup_area_id"`
	PickupZoneID     string `json:"pickup_zone_id"`
	DropAreaID       string `json:"drop_area_id"`
	DropZoneID       string `json:"drop_zone_id"`
	ZoneRelationship string `json:"zone_relationship"`

	OrderType   string `json:"order_type"`
	PaymentType string `json:"payment_type"`

	LengthCM       float64 `json:"length_cm"`
	BreadthCM      float64 `json:"breadth_cm"`
	HeightCM       float64 `json:"height_cm"`
	ActualWeightKG float64 `json:"actual_weight_kg"`

	VolumetricWeightKG float64 `json:"volumetric_weight_kg"`
	ChargeableWeightKG float64 `json:"chargeable_weight_kg"`

	RateCardID string `json:"rate_card_id"`

	BaseRate     float64 `json:"base_rate"`
	CODSurcharge float64 `json:"cod_surcharge"`
	FinalAmount  float64 `json:"final_amount"`
}

func toQuoteResponse(q QuoteResult) quoteResponse {
	return quoteResponse{
		PickupAreaID:     q.PickupArea.ID,
		PickupZoneID:     q.PickupZone.ID,
		DropAreaID:       q.DropArea.ID,
		DropZoneID:       q.DropZone.ID,
		ZoneRelationship: string(q.ZoneRelationship),

		OrderType:   string(q.OrderType),
		PaymentType: string(q.PaymentType),

		LengthCM:       q.LengthCM,
		BreadthCM:      q.BreadthCM,
		HeightCM:       q.HeightCM,
		ActualWeightKG: q.ActualWeightKG,

		VolumetricWeightKG: q.VolumetricWeightKG,
		ChargeableWeightKG: q.ChargeableWeightKG,

		RateCardID: q.RateCard.ID,

		BaseRate:     q.BaseRate,
		CODSurcharge: q.CODSurcharge,
		FinalAmount:  q.FinalAmount,
	}
}

// QuoteHandler handles POST /api/v1/orders/quote (ADMIN + CUSTOMER —
// enforced by the RBAC middleware in routes.go). Stateless: no order is
// created, nothing is persisted. Given the same inputs, calling this
// again always recomputes from whatever rate configuration is active
// right now — there is no cached/stored "quote" a later confirm step
// could rely on going stale.
func QuoteHandler(zonesRepo zones.Repository, ratesRepo Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req quoteRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			server.WriteError(w, http.StatusUnprocessableEntity, "invalid request body: "+err.Error())
			return
		}

		input, problem := validateQuoteRequest(req)
		if problem != "" {
			server.WriteError(w, http.StatusUnprocessableEntity, problem)
			return
		}

		result, err := CalculateQuote(r.Context(), zonesRepo, ratesRepo, input)
		if err != nil {
			if status, message, ok := mapQuoteDomainError(err); ok {
				server.WriteError(w, status, message)
				return
			}
			slog.Error("quote calculation failed", "error", err)
			server.WriteError(w, http.StatusInternalServerError, "could not calculate quote")
			return
		}

		server.WriteJSON(w, http.StatusOK, toQuoteResponse(result))
	}
}

// mapQuoteDomainError translates CalculateQuote's domain errors into
// (status, message) pairs. Every one of these is treated as 422: the
// caller's input (an area id, an order type/zone relationship
// combination with no active rate card, a weight no configured slab
// covers) is what made the request unserviceable — none of them are "no
// such URL resource" (404) or "conflicting concurrent write" (409),
// the two other status codes this API otherwise reserves for path/
// mutation-specific failures.
func mapQuoteDomainError(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, zones.ErrAreaNotFound):
		return http.StatusUnprocessableEntity, "pickup_area_id or drop_area_id does not reference an existing area", true
	case errors.Is(err, zones.ErrZoneInactive):
		return http.StatusUnprocessableEntity, "the pickup or drop area's zone is not currently active", true
	case errors.Is(err, ErrRateCardNotFound):
		return http.StatusUnprocessableEntity, "no active rate card is configured for this order type and zone relationship", true
	case errors.Is(err, ErrNoMatchingSlab):
		return http.StatusUnprocessableEntity, "no rate slab is configured for the calculated chargeable weight", true
	default:
		return 0, "", false
	}
}
