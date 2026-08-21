package rates

import "lastmiletracker/internal/zones"

// ZoneRelationship is a type alias for zones.ZoneRelationship, not a new
// type. M05 reuses M04's INTRA/INTER values directly — there is exactly
// one definition of what those two strings mean across the backend, and
// this package never risks drifting from it.
type ZoneRelationship = zones.ZoneRelationship

const (
	RelationshipIntra = zones.RelationshipIntra
	RelationshipInter = zones.RelationshipInter
)

// ParseZoneRelationship validates a raw string against the two
// permitted values.
func ParseZoneRelationship(raw string) (ZoneRelationship, bool) {
	switch ZoneRelationship(raw) {
	case RelationshipIntra, RelationshipInter:
		return ZoneRelationship(raw), true
	default:
		return "", false
	}
}
