// Package orders implements M07 — Order Management: persisting a priced
// order and letting a customer or admin retrieve it. All pricing —
// zone resolution, rate-card selection, weight calculation, slab
// selection, COD application — is M06's job (internal/rates). This
// package never recomputes any of it; it calls rates.CalculateQuote and
// persists the QuoteResult it returns, unmodified, as an immutable
// snapshot. See docs/order-management.md.
package orders

import (
	"time"

	"lastmiletracker/internal/rates"
)

// StatusCreated is the only status this module ever writes. The full
// M08 state-machine value set already exists in the orders table's
// CHECK constraint (see migration 0008) so a later module doesn't need
// a schema change just to widen it, but no code here writes anything
// else — status transitions are explicitly M08's responsibility, not
// this module's.
const StatusCreated = "CREATED"

// Order is a persisted, priced order — one row in the orders table.
// Every pricing-derived field (ZoneRelationship through FinalAmount) is
// a snapshot captured once at creation time from rates.QuoteResult,
// never recomputed on read and never affected by a later edit to the
// rate card it was priced against.
type Order struct {
	ID         string
	CustomerID string
	CreatedBy  string

	OrderType   rates.OrderType
	PaymentType rates.PaymentType

	PickupAreaID     string
	DropAreaID       string
	PickupZoneID     string
	DropZoneID       string
	ZoneRelationship rates.ZoneRelationship

	LengthCM       float64
	BreadthCM      float64
	HeightCM       float64
	ActualWeightKG float64

	VolumetricWeightKG float64
	ChargeableWeightKG float64

	RateCardID   string
	BaseRate     float64
	CODSurcharge float64
	FinalAmount  float64

	Status    string
	CreatedAt time.Time
}

// CreateOrderInput is CreateOrder's only input. Quote is the exact,
// already-computed result of a prior rates.CalculateQuote call — this
// package never accepts raw pricing fields (base rate, cod surcharge,
// final amount, ...) as separate arguments, so there is no code path
// through which a value could reach the database without having come
// from CalculateQuote.
type CreateOrderInput struct {
	CustomerID string
	CreatedBy  string
	Quote      rates.QuoteResult
}
