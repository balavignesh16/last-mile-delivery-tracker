// Package rates implements M05 — Rate Configuration: admin-managed rate
// cards and their weight slabs. M06 — Rate Calculation Engine — will
// extend this same package with the actual charge-calculation pipeline
// (chargeable weight, slab selection, COD application, quoting); this
// file and its siblings implement only M05's configuration surface.
//
// Depends on internal/zones for ZoneRelationship (reused directly, never
// redeclared — see zone_relationship.go) and internal/auth/internal/users
// for RBAC wiring, the same one-directional dependency shape
// internal/agents uses. Nothing depends back on internal/rates.
package rates

import "time"

// OrderType is one of the two order categories the assignment requires.
// Stored as a plain string constrained by a CHECK constraint, not a
// Postgres ENUM — same reasoning as users.Role/agents.Availability: a
// third order type later never needs an ALTER TYPE migration.
type OrderType string

const (
	OrderTypeB2B OrderType = "B2B"
	OrderTypeB2C OrderType = "B2C"
)

// ParseOrderType validates a raw string against the two permitted
// values.
func ParseOrderType(raw string) (OrderType, bool) {
	switch OrderType(raw) {
	case OrderTypeB2B, OrderTypeB2C:
		return OrderType(raw), true
	default:
		return "", false
	}
}

// RateCard is one admin-configured pricing profile for exactly one
// (OrderType, ZoneRelationship) combination. At most one RateCard per
// combination may have Active=true at any time — enforced by a partial
// unique index, not just application code (see the 0006 migration).
type RateCard struct {
	ID               string
	OrderType        OrderType
	ZoneRelationship ZoneRelationship
	// CODSurcharge is a flat amount added to an order's charge when its
	// payment type is COD. M05 only stores this value — applying it to
	// an order's total is M06's job, not this package's, at this stage.
	CODSurcharge float64
	Active       bool
	CreatedAt    time.Time
}

// Slab is one flat-price weight band within a RateCard. A chargeable
// weight in [MinWeight, MaxWeight) costs exactly Price — not Price per
// kg. MaxWeight = nil represents an open-ended top slab ("and above");
// at most one such Slab may exist per RateCard (enforced by a partial
// unique index, see the 0007 migration).
//
// M05 only stores slabs. Selecting which slab matches a given
// chargeable weight is M06's job — this package deliberately does not
// implement that lookup, so M06's own tests are what first exercise it.
type Slab struct {
	ID         string
	RateCardID string
	MinWeight  float64
	MaxWeight  *float64
	Price      float64
	CreatedAt  time.Time
}

// CreateRateCardInput is CreateRateCard's only input. There is
// deliberately no Active field: every rate card is created inactive,
// and becomes active only through a later, explicit UpdateRateCard call
// — see CreateRateCard's doc in repository.go for why.
type CreateRateCardInput struct {
	OrderType        OrderType
	ZoneRelationship ZoneRelationship
	CODSurcharge     float64
}

// RateCardUpdate is UpdateRateCard's only input. OrderType and
// ZoneRelationship are not editable — they are a rate card's identity,
// the same reasoning M04 used for not letting an area move between
// zones. Active is a pointer so omitting it leaves the current value
// unchanged (the same "nil means unchanged" contract as
// zones.ZoneUpdate.Active); CODSurcharge is always resent, the same way
// zones.ZoneUpdate.Name is always resent on a zone rename.
type RateCardUpdate struct {
	CODSurcharge float64
	Active       *bool
}

// CreateSlabInput is CreateSlab's only input. There is deliberately no
// RateCardID field — the caller (CreateSlabHandler) always passes the
// rate card id from the URL path as a separate argument, mirroring
// zones.CreateAreaInput.
type CreateSlabInput struct {
	MinWeight float64
	MaxWeight *float64
	Price     float64
}

// SlabUpdate is UpdateSlab's only input.
type SlabUpdate struct {
	MinWeight float64
	MaxWeight *float64
	Price     float64
}
