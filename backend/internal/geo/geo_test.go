package geo

import (
	"math"
	"testing"
)

func withinTolerance(got, want, toleranceKM float64) bool {
	return math.Abs(got-want) <= toleranceKM
}

// --- ValidateCoordinates ---

func TestValidateCoordinates_ValidPairReturnsNoProblem(t *testing.T) {
	if problem := ValidateCoordinates(12.9716, 77.5946); problem != "" {
		t.Errorf("ValidateCoordinates() = %q, want no problem for a valid pair", problem)
	}
}

func TestValidateCoordinates_LatitudeOutOfRange(t *testing.T) {
	if problem := ValidateCoordinates(90.1, 0); problem == "" {
		t.Error("ValidateCoordinates() = no problem, want a latitude-range error for 90.1")
	}
	if problem := ValidateCoordinates(-90.1, 0); problem == "" {
		t.Error("ValidateCoordinates() = no problem, want a latitude-range error for -90.1")
	}
}

func TestValidateCoordinates_LongitudeOutOfRange(t *testing.T) {
	if problem := ValidateCoordinates(0, 180.1); problem == "" {
		t.Error("ValidateCoordinates() = no problem, want a longitude-range error for 180.1")
	}
	if problem := ValidateCoordinates(0, -180.1); problem == "" {
		t.Error("ValidateCoordinates() = no problem, want a longitude-range error for -180.1")
	}
}

func TestValidateCoordinates_BoundaryValuesAreValid(t *testing.T) {
	for _, tc := range []struct{ lat, lng float64 }{
		{90, 180}, {-90, -180}, {0, 0},
	} {
		if problem := ValidateCoordinates(tc.lat, tc.lng); problem != "" {
			t.Errorf("ValidateCoordinates(%v, %v) = %q, want no problem at the exact boundary", tc.lat, tc.lng, problem)
		}
	}
}

func TestValidateCoordinates_NaNAndInfRejected(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)
	if problem := ValidateCoordinates(nan, 0); problem == "" {
		t.Error("ValidateCoordinates() = no problem, want a finite-number error for NaN latitude")
	}
	if problem := ValidateCoordinates(0, inf); problem == "" {
		t.Error("ValidateCoordinates() = no problem, want a finite-number error for +Inf longitude")
	}
}

// --- HaversineKM ---

func TestHaversineKM_SamePointIsZero(t *testing.T) {
	d := HaversineKM(12.9716, 77.5946, 12.9716, 77.5946)
	if d != 0 {
		t.Errorf("HaversineKM(same point) = %v, want 0", d)
	}
}

// One degree of longitude at the equator is a well-known, easily
// independently verified distance: circumference/360 using the mean
// Earth radius this package uses, ≈ 111.19 km.
func TestHaversineKM_OneDegreeAtEquator(t *testing.T) {
	d := HaversineKM(0, 0, 0, 1)
	if !withinTolerance(d, 111.19, 1.0) {
		t.Errorf("HaversineKM(0,0 -> 0,1) = %v km, want ~111.19 km (±1km)", d)
	}
}

// London to Paris: a commonly cited reference great-circle distance of
// ~343.5 km — verifies the formula against a known real-world pair, not
// just a synthetic one.
func TestHaversineKM_LondonToParis(t *testing.T) {
	d := HaversineKM(51.5074, -0.1278, 48.8566, 2.3522)
	if !withinTolerance(d, 343.5, 5.0) {
		t.Errorf("HaversineKM(London -> Paris) = %v km, want ~343.5 km (±5km)", d)
	}
}

func TestHaversineKM_SymmetricBothDirections(t *testing.T) {
	a := HaversineKM(12.9716, 77.5946, 13.0827, 80.2707)
	b := HaversineKM(13.0827, 80.2707, 12.9716, 77.5946)
	if a != b {
		t.Errorf("HaversineKM is not symmetric: A->B = %v, B->A = %v", a, b)
	}
}

func TestHaversineKM_NearerPointHasSmallerDistance(t *testing.T) {
	origin := [2]float64{12.9716, 77.5946} // Bengaluru
	near := [2]float64{12.2958, 76.6394}   // Mysuru, ~140km away
	far := [2]float64{19.0760, 72.8777}    // Mumbai, ~840km away

	dNear := HaversineKM(origin[0], origin[1], near[0], near[1])
	dFar := HaversineKM(origin[0], origin[1], far[0], far[1])
	if dNear >= dFar {
		t.Errorf("dNear = %v km, dFar = %v km — want the nearer point to have the smaller distance", dNear, dFar)
	}
}
