// Package geo holds the small set of pure, dependency-free geographic
// primitives more than one module needs: coordinate-range validation
// (previously duplicated ad hoc in internal/agents, and now also needed
// by internal/zones for area coordinates) and great-circle distance
// (needed by internal/assignment to rank agents by real proximity to a
// pickup point). No internal imports at all, so every other package can
// depend on it without risk of a cycle.
package geo

import "math"

// ValidateCoordinates reports a validation problem for a latitude/
// longitude pair that is already known to be present (non-nil) —
// callers decide separately whether an absent coordinate is acceptable
// at all (e.g. a delivery agent's location update requires both;
// an area's coordinates are optional configuration). Returns "" when
// the pair is valid.
func ValidateCoordinates(lat, lng float64) string {
	// encoding/json rejects literal NaN/Infinity tokens outright (they
	// are not valid JSON numbers), but an extreme value like 1e400
	// decodes to +Inf without a decode error — checked explicitly rather
	// than relying solely on the range comparison below to catch it.
	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lng) || math.IsInf(lng, 0) {
		return "latitude and longitude must be finite numbers"
	}
	if lat < -90 || lat > 90 {
		return "latitude must be between -90 and 90"
	}
	if lng < -180 || lng > 180 {
		return "longitude must be between -180 and 180"
	}
	return ""
}

// earthRadiusKM is the IUGG mean Earth radius, the standard constant
// for a spherical-Earth Haversine approximation.
const earthRadiusKM = 6371.0088

// HaversineKM returns the great-circle distance in kilometres between
// two latitude/longitude points via the Haversine formula — the
// standard, numerically stable formula for this kind of ranking
// distance (accurate to within ~0.5% versus an ellipsoidal/geodesic
// model, which this project has no requirement for). Inputs are assumed
// already validated (ValidateCoordinates); this function does not
// re-validate them.
func HaversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	const degToRad = math.Pi / 180

	lat1Rad := lat1 * degToRad
	lat2Rad := lat2 * degToRad
	dLat := (lat2 - lat1) * degToRad
	dLng := (lng2 - lng1) * degToRad

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKM * c
}
