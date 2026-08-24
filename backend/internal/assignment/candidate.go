// Package assignment implements M09 — Assignment Engine: manual and
// automatic delivery-agent assignment. It owns exactly two endpoints
// (POST /orders/:id/assign, POST /orders/:id/auto-assign) and one new
// column (orders.assigned_agent_id) — no new table, no assignment
// history table (M08's order_tracking_events already provides that via
// each ASSIGNED event's own metadata).
//
// This package owns assignment orchestration only. Pricing stays in
// internal/rates (M06), order persistence/ownership stays in
// internal/orders (M07), and the order status state machine stays in
// internal/tracking (M08) — assignment never recomputes a price, never
// writes orders.status directly, and never inserts a tracking event
// directly. Every status change this package causes goes through
// tracking.Repository.TransitionTx, M08's own authoritative,
// unmodified state-machine implementation. See docs/assignment-engine.md.
package assignment

import (
	"errors"
	"sort"
	"time"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/geo"
)

// ErrNoEligibleCandidate is returned when no delivery agent satisfies
// the eligibility rule below — auto-assignment never guesses or
// substitutes a fallback candidate.
var ErrNoEligibleCandidate = errors.New("no eligible delivery agent is available for assignment")

// Candidate is the minimal agent shape the ranking algorithm needs —
// deliberately decoupled from agents.AgentWithUser so this file has no
// database dependency at all and is independently, purely unit-testable
// (mirrors rates.SelectSlab's and tracking.IsValidTransition's
// precedent: the decision function is pure, the database plumbing is a
// separate, integration-tested concern).
// Latitude/Longitude are the agent's own current_lat/current_lng
// (internal/agents) — optional (nullable in the database), used only
// for ranking, never eligibility (see IsEligible: a candidate with no
// coordinates is still eligible, it just can't be ranked by distance
// and falls back to zone-based ranking — see rankLess).
type Candidate struct {
	AgentID        string
	Active         bool
	Availability   agents.Availability
	CurrentZoneID  *string
	Latitude       *float64
	Longitude      *float64
	LastAssignedAt *time.Time
}

// IsEligible is the one, single eligibility rule — used both to filter
// auto-assignment's candidate pool and to re-check a manually-targeted
// agent under lock (internal/assignment's manual and automatic paths
// call this exact same function, so eligibility can never drift between
// "who auto-assign would pick" and "who a manual assignment is allowed
// to target"). Finalized M09 rule: active, AVAILABLE, and a resolvable
// current zone — nothing else.
func IsEligible(c Candidate) bool {
	return c.Active && c.Availability == agents.AvailabilityAvailable && c.CurrentZoneID != nil
}

// SelectCandidate implements the finalized M09 deterministic ranking,
// extended with real-distance ranking when it's actually computable:
//  1. active = true
//  2. availability = AVAILABLE
//  3. a usable current_zone_id exists
//  4. if this candidate AND the pickup point both have known
//     coordinates, rank by real Haversine distance ascending (nearer
//     wins) — a candidate with a computable distance always outranks
//     one without, since a real distance is strictly more informative
//     than the zone-match proxy below
//  5. otherwise (or on an exact distance tie), same-zone agents rank
//     ahead of cross-zone agents (cross-zone is allowed, never excluded
//     — this is the fallback for when precise coordinates aren't
//     available, exactly as the source blueprint describes)
//  6. last_assigned_at ascending, NULL first (fair rotation — agents
//     never assigned, or idle longest, are preferred)
//  7. agent id ascending — the final, unconditional tiebreaker that
//     guarantees this function can never be non-deterministic
//
// pickupLat/pickupLng are the pickup area's own coordinates (nil, nil
// if that area has none set) — see internal/zones.Area.
//
// Returns ErrNoEligibleCandidate if the pool has no eligible member —
// never a guess, never a fallback substitution.
func SelectCandidate(candidates []Candidate, pickupZoneID string, pickupLat, pickupLng *float64) (Candidate, error) {
	eligible := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if IsEligible(c) {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return Candidate{}, ErrNoEligibleCandidate
	}

	sort.Slice(eligible, func(i, j int) bool {
		return rankLess(eligible[i], eligible[j], pickupZoneID, pickupLat, pickupLng)
	})
	return eligible[0], nil
}

// distanceToPickupKM returns the Haversine distance in kilometres from
// c's current location to the pickup point, or nil if either side lacks
// a coordinate — nil is the "not computable" signal rankLess and
// AutoAssign both check for, never a fabricated/zero distance standing
// in for "unknown".
func distanceToPickupKM(c Candidate, pickupLat, pickupLng *float64) *float64 {
	if c.Latitude == nil || c.Longitude == nil || pickupLat == nil || pickupLng == nil {
		return nil
	}
	d := geo.HaversineKM(*c.Latitude, *c.Longitude, *pickupLat, *pickupLng)
	return &d
}

// rankLess implements steps 4-7 of SelectCandidate's ranking as a
// strict total order (never returns true for both (a,b) and (b,a)
// unless a and b are the same agent), which is what actually makes
// sort.Slice's result deterministic regardless of the input's original
// order.
func rankLess(a, b Candidate, pickupZoneID string, pickupLat, pickupLng *float64) bool {
	aDist := distanceToPickupKM(a, pickupLat, pickupLng)
	bDist := distanceToPickupKM(b, pickupLat, pickupLng)
	switch {
	case aDist != nil && bDist != nil && *aDist != *bDist:
		return *aDist < *bDist
	case (aDist != nil) != (bDist != nil):
		// Exactly one candidate has a real, computable distance — it
		// always outranks the other, regardless of zone match.
		return aDist != nil
	}
	// Neither has a computable distance, or both do and are exactly
	// tied on it: fall through to the zone-based signal.

	aSame := a.CurrentZoneID != nil && *a.CurrentZoneID == pickupZoneID
	bSame := b.CurrentZoneID != nil && *b.CurrentZoneID == pickupZoneID
	if aSame != bSame {
		return aSame
	}

	aNil, bNil := a.LastAssignedAt == nil, b.LastAssignedAt == nil
	if aNil != bNil {
		return aNil
	}
	if !aNil && !bNil && !a.LastAssignedAt.Equal(*b.LastAssignedAt) {
		return a.LastAssignedAt.Before(*b.LastAssignedAt)
	}

	return a.AgentID < b.AgentID
}
