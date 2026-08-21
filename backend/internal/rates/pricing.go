package rates

import (
	"context"
	"errors"
	"fmt"
	"math"

	"lastmiletracker/internal/zones"
)

// PaymentType is how the customer will settle an order's charge.
// Stored/compared as a plain string, not a Postgres ENUM — same
// reasoning as OrderType.
type PaymentType string

const (
	PaymentTypePrepaid PaymentType = "PREPAID"
	PaymentTypeCOD     PaymentType = "COD"
)

// ParsePaymentType validates a raw string against the two permitted
// values.
func ParsePaymentType(raw string) (PaymentType, bool) {
	switch PaymentType(raw) {
	case PaymentTypePrepaid, PaymentTypeCOD:
		return PaymentType(raw), true
	default:
		return "", false
	}
}

// ErrNoMatchingSlab is returned when a rate card's active combination
// exists, but none of its slabs cover the calculated chargeable weight
// — a gap below the first slab, between two slabs, above every closed
// slab with no open-ended slab configured, or a rate card with no slabs
// at all.
var ErrNoMatchingSlab = errors.New("no rate slab configured for this chargeable weight")

// CalculateVolumetricWeight implements the blueprint's fixed formula.
// Dimensions are assumed to be centimeters and the result kilograms —
// the universal convention for this exact divisor, though the source
// documents never state units explicitly (see docs/rate-calculation.md).
func CalculateVolumetricWeight(lengthCM, breadthCM, heightCM float64) float64 {
	return lengthCM * breadthCM * heightCM / 5000
}

// CalculateChargeableWeight is the higher of the two candidate weights —
// a courier is charged for whichever is greater, never their average or
// the actual weight alone.
func CalculateChargeableWeight(actualWeightKG, volumetricWeightKG float64) float64 {
	return math.Max(actualWeightKG, volumetricWeightKG)
}

// SelectSlab finds the slab whose [MinWeight, MaxWeight) range covers
// chargeableWeight. slabs is expected pre-sorted by MinWeight (which is
// what ListSlabsByRateCard already returns, per its ORDER BY), but this
// function does not depend on that ordering for correctness — every
// candidate is checked independently, so a caller passing an unsorted
// slice still gets the right answer, just checked in a different order.
//
// A single uniform check handles every boundary case M05's [min, max)
// convention defines: exact min (inclusive match), exact max (excluded,
// falls through to the next slab), a gap between slabs (matches
// neither, falls through to ErrNoMatchingSlab), weight below the first
// configured slab, weight above every closed slab with no open-ended
// slab present, and the open-ended slab itself (MaxWeight == nil,
// matches any weight >= MinWeight). None of these need special-casing
// — they are all just different ways a single [min, max) window can
// fail to (or does) contain a value.
func SelectSlab(chargeableWeight float64, slabs []Slab) (Slab, error) {
	for _, s := range slabs {
		if chargeableWeight >= s.MinWeight && (s.MaxWeight == nil || chargeableWeight < *s.MaxWeight) {
			return s, nil
		}
	}
	return Slab{}, ErrNoMatchingSlab
}

// QuoteInput is CalculateQuote's only input — the same raw, client-
// suppliable fields for both POST /orders/quote (this milestone) and the
// order-confirmation step M07 will add later. There is deliberately no
// field here for anything server-derived (zone ids, zone relationship,
// rate card id, chargeable weight, price) — those only ever appear on
// QuoteResult, never as something a caller passes in.
type QuoteInput struct {
	PickupAreaID   string
	DropAreaID     string
	OrderType      OrderType
	PaymentType    PaymentType
	LengthCM       float64
	BreadthCM      float64
	HeightCM       float64
	ActualWeightKG float64
}

// QuoteResult is CalculateQuote's complete output: every input echoed
// back alongside every value the calculation derived, so a caller (an
// HTTP handler today, M07's order-confirmation handler later) never has
// to re-derive or re-fetch anything to display or persist a full quote.
type QuoteResult struct {
	PickupArea       zones.Area
	PickupZone       zones.Zone
	DropArea         zones.Area
	DropZone         zones.Zone
	ZoneRelationship ZoneRelationship

	OrderType   OrderType
	PaymentType PaymentType

	LengthCM       float64
	BreadthCM      float64
	HeightCM       float64
	ActualWeightKG float64

	VolumetricWeightKG float64
	ChargeableWeightKG float64

	RateCard RateCard
	Slab     Slab

	BaseRate     float64
	CODSurcharge float64
	FinalAmount  float64
}

// CalculateQuote is the one authoritative pricing pipeline: zone
// resolution (M04) -> rate card selection (M05) -> weight calculation ->
// slab selection -> COD application -> final amount. POST
// /orders/quote's handler calls this directly; M07's order-confirmation
// handler is expected to call the exact same function with the exact
// same input shape rather than re-deriving a price from a client-
// supplied quote — see docs/rate-calculation.md for why that is what
// makes "never trust a client-supplied price" actually true rather than
// just documented.
//
// Every failure mode fails explicitly (zones.ErrAreaNotFound,
// zones.ErrZoneInactive, ErrRateCardNotFound, ErrNoMatchingSlab) —
// nothing here guesses a default zone, silently substitutes a fallback
// rate card, or rounds an unmatched weight into the nearest slab.
func CalculateQuote(ctx context.Context, zonesRepo zones.Repository, ratesRepo Repository, input QuoteInput) (QuoteResult, error) {
	resolution, err := zones.ResolvePickupDrop(ctx, zonesRepo, input.PickupAreaID, input.DropAreaID)
	if err != nil {
		return QuoteResult{}, err
	}

	volumetric := CalculateVolumetricWeight(input.LengthCM, input.BreadthCM, input.HeightCM)
	chargeable := CalculateChargeableWeight(input.ActualWeightKG, volumetric)

	card, err := ratesRepo.FindActiveCard(ctx, input.OrderType, resolution.Relationship)
	if err != nil {
		return QuoteResult{}, err
	}

	slabs, err := ratesRepo.ListSlabsByRateCard(ctx, card.ID)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("list slabs for active rate card: %w", err)
	}

	slab, err := SelectSlab(chargeable, slabs)
	if err != nil {
		return QuoteResult{}, err
	}

	// Flat price per band, not per kg — M05's finalized pricing model.
	// COD surcharge only applies when the order is actually COD; blueprint
	// step 8 states no Prepaid-side behavior beyond "no surcharge".
	baseRate := slab.Price
	codSurcharge := 0.0
	if input.PaymentType == PaymentTypeCOD {
		codSurcharge = card.CODSurcharge
	}

	return QuoteResult{
		PickupArea:       resolution.PickupArea,
		PickupZone:       resolution.PickupZone,
		DropArea:         resolution.DropArea,
		DropZone:         resolution.DropZone,
		ZoneRelationship: resolution.Relationship,

		OrderType:   input.OrderType,
		PaymentType: input.PaymentType,

		LengthCM:       input.LengthCM,
		BreadthCM:      input.BreadthCM,
		HeightCM:       input.HeightCM,
		ActualWeightKG: input.ActualWeightKG,

		VolumetricWeightKG: volumetric,
		ChargeableWeightKG: chargeable,

		RateCard: card,
		Slab:     slab,

		BaseRate:     baseRate,
		CODSurcharge: codSurcharge,
		FinalAmount:  baseRate + codSurcharge,
	}, nil
}
