// Package zones owns the geographic configuration used by later modules:
// zones, the areas within them, and the deterministic address -> area ->
// zone resolution chain M06's rate engine will consume to classify an
// order as INTRA-zone or INTER-zone. It has no HTTP dependency on any
// other business package and nothing depends back on it in M04 — M06/M07
// are the first modules expected to import it.
package zones

import "time"

// Zone is the top-level geographic configuration unit. Deactivating a
// zone (Active=false) does not delete it or the areas within it — see
// the 0003 migration's comment for why physical deletion isn't offered.
type Zone struct {
	ID        string
	Name      string
	Active    bool
	CreatedAt time.Time
}

// Area belongs to exactly one Zone. There is no "unassigned" area state
// — ZoneID is always a real zones.id, enforced by the database FK, not
// just application code.
//
// Active mirrors Zone.Active — deactivating an area (Active=false) does
// not delete it, only blocks it from being resolved for new orders/
// quotes (see ResolveArea's ErrAreaInactive). Areas originally had no
// independent active flag of their own (their parent zone's Active was
// considered sufficient — see the 0004 migration's original comment),
// but an admin legitimately needs to retire a single area without
// deactivating every other area in the same zone.
// Latitude/Longitude are optional (both nil, or both set — see
// CreateAreaInput/AreaUpdate) representative coordinates for the area,
// used by internal/assignment to rank eligible delivery agents by real
// distance (internal/geo.HaversineKM) to an order's pickup point rather
// than zone-match alone. Nil for every area until an admin sets real
// ones — never geocoded, never a fabricated default.
type Area struct {
	ID        string
	Name      string
	ZoneID    string
	Active    bool
	Latitude  *float64
	Longitude *float64
	CreatedAt time.Time
}

// CreateZoneInput is CreateZone's only input.
type CreateZoneInput struct {
	Name string
}

// ZoneUpdate is UpdateZone's only input. Active is a pointer so omitting
// it from a request means "leave active state unchanged" rather than
// silently deactivating a zone the caller only meant to rename — the
// same reasoning agents.updateLocationRequest uses *float64 for lat/lng
// so 0 isn't confused with "not provided".
type ZoneUpdate struct {
	Name   string
	Active *bool
}

// CreateAreaInput is CreateArea's only input. There is deliberately no
// ZoneID field here — the caller (CreateAreaHandler) always passes the
// zone id from the URL path as a separate argument, never from anything
// the request body could influence. See handler.go's createAreaRequest.
//
// Latitude/Longitude are optional — nil, nil is a completely normal
// area with no coordinates yet (the auto-assignment fallback to
// zone-based ranking applies). The handler enforces "both or neither";
// this type itself doesn't, since that's a request-validation concern.
type CreateAreaInput struct {
	Name      string
	Latitude  *float64
	Longitude *float64
}

// AreaUpdate is UpdateArea's only input. Renaming, active-toggling, and
// (new) setting coordinates — moving an area to a different zone is not
// a requirement the assignment or frozen architecture states, so it
// isn't offered. Active/Latitude/Longitude are pointers with the same
// "nil means unchanged" contract as ZoneUpdate.Active; there is
// currently no way to clear a once-set coordinate back to unset via
// this endpoint, the same limitation Active already has for "unset".
type AreaUpdate struct {
	Name      string
	Active    *bool
	Latitude  *float64
	Longitude *float64
}

// ZoneRelationship classifies a pickup/drop area pair for M06's rate
// engine.
type ZoneRelationship string

const (
	RelationshipIntra ZoneRelationship = "INTRA"
	RelationshipInter ZoneRelationship = "INTER"
)

// DetermineZoneRelationship compares stable zone IDs, never zone or area
// names — the frozen architecture's explicit requirement, and the only
// safe choice: a zone's name is just a mutable display string an admin
// can rename at any time via UpdateZone, so it cannot be what later
// pricing logic keys off.
func DetermineZoneRelationship(pickupZoneID, dropZoneID string) ZoneRelationship {
	if pickupZoneID == dropZoneID {
		return RelationshipIntra
	}
	return RelationshipInter
}
