package rates

import (
	"errors"
	"math"
)

// ErrSlabOverlap is returned when a new or edited slab's [min, max)
// range intersects an existing slab in the same rate card.
var ErrSlabOverlap = errors.New("slab weight range overlaps an existing slab in this rate card")

// ErrMultipleOpenEndedSlabs is returned when a new or edited slab would
// be the second open-ended (MaxWeight == nil) slab in the same rate
// card. Also enforced at the database level (see the 0007 migration's
// partial unique index) — this check gives a clean domain error instead
// of a raw constraint-violation message when the application-level path
// catches it first.
var ErrMultipleOpenEndedSlabs = errors.New("a rate card can have at most one open-ended slab")

// validateSlabAgainstExisting checks a candidate slab's [minWeight,
// maxWeight) range against every other slab already in the same rate
// card. excludeSlabID lets UpdateSlab compare a slab against its
// siblings without spuriously "overlapping" its own prior version —
// pass "" from CreateSlab, where there is no prior version to exclude.
func validateSlabAgainstExisting(minWeight float64, maxWeight *float64, excludeSlabID string, existing []Slab) error {
	openEnded := maxWeight == nil

	for _, s := range existing {
		if s.ID == excludeSlabID {
			continue
		}
		if openEnded && s.MaxWeight == nil {
			return ErrMultipleOpenEndedSlabs
		}
		if slabRangesOverlap(minWeight, maxWeight, s.MinWeight, s.MaxWeight) {
			return ErrSlabOverlap
		}
	}
	return nil
}

// slabRangesOverlap implements the standard half-open-interval overlap
// test: [aMin, aMax) and [bMin, bMax) overlap iff aMin < bMax && bMin <
// aMax. A nil upper bound is treated as +Inf. Two intervals that only
// touch at a boundary (e.g. [0,5) and [5,10)) do NOT overlap under this
// test — exactly the [min, max) convention this module uses throughout.
func slabRangesOverlap(aMin float64, aMax *float64, bMin float64, bMax *float64) bool {
	aHi := math.Inf(1)
	if aMax != nil {
		aHi = *aMax
	}
	bHi := math.Inf(1)
	if bMax != nil {
		bHi = *bMax
	}
	return aMin < bHi && bMin < aHi
}
