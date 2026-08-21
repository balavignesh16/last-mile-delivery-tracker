package rates

import (
	"errors"
	"testing"
)

func f(v float64) *float64 { return &v }

func TestSlabRangesOverlap(t *testing.T) {
	cases := []struct {
		name string
		aMin float64
		aMax *float64
		bMin float64
		bMax *float64
		want bool
	}{
		{"adjacent, touching boundary, no overlap", 0, f(5), 5, f(10), false},
		{"exact duplicate range", 0, f(5), 0, f(5), true},
		{"partial overlap", 0, f(5), 3, f(10), true},
		{"fully nested", 0, f(10), 3, f(5), true},
		{"disjoint, gap between", 0, f(5), 10, f(15), false},
		{"open-ended vs finite that reaches into it", 10, nil, 5, f(15), true},
		{"open-ended vs finite fully below it", 10, nil, 0, f(5), false},
		{"open-ended vs finite ending exactly at its start", 10, nil, 0, f(10), false},
		{"two open-ended ranges always intersect", 10, nil, 20, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := slabRangesOverlap(tc.aMin, tc.aMax, tc.bMin, tc.bMax); got != tc.want {
				t.Errorf("slabRangesOverlap(%v,%v,%v,%v) = %v, want %v", tc.aMin, tc.aMax, tc.bMin, tc.bMax, got, tc.want)
			}
		})
	}
}

// TestValidateSlabAgainstExisting_DemoConfiguration verifies the exact
// boundary examples specified for this module: 4.999 kg belongs to
// 0-5, 5.000 kg belongs to 5-10 (not 0-5), 9.999 kg belongs to 5-10,
// 10.000 kg belongs to 10-15. Expressed here as "the demo 0/5/10/15/20
// slab set is accepted as non-overlapping" — the boundary-to-slab
// lookup itself is M06's job, not this package's, but the underlying
// range semantics this module stores must support exactly that lookup
// giving those answers.
func TestValidateSlabAgainstExisting_DemoConfiguration(t *testing.T) {
	existing := []Slab{
		{ID: "s1", MinWeight: 0, MaxWeight: f(5)},
		{ID: "s2", MinWeight: 5, MaxWeight: f(10)},
		{ID: "s3", MinWeight: 10, MaxWeight: f(15)},
	}
	// Adding the fourth demo slab, 15-20, must succeed — touches s3's
	// boundary but does not overlap it.
	if err := validateSlabAgainstExisting(15, f(20), "", existing); err != nil {
		t.Errorf("adding 15-20 to the demo config: unexpected error: %v", err)
	}
}

func TestValidateSlabAgainstExisting_RejectsOverlap(t *testing.T) {
	existing := []Slab{{ID: "s1", MinWeight: 0, MaxWeight: f(5)}}
	err := validateSlabAgainstExisting(3, f(10), "", existing)
	if !errors.Is(err, ErrSlabOverlap) {
		t.Errorf("error = %v, want ErrSlabOverlap", err)
	}
}

func TestValidateSlabAgainstExisting_AllowsAdjacentBoundary(t *testing.T) {
	existing := []Slab{{ID: "s1", MinWeight: 0, MaxWeight: f(5)}}
	if err := validateSlabAgainstExisting(5, f(10), "", existing); err != nil {
		t.Errorf("adjacent slab starting exactly at the prior slab's max: unexpected error: %v", err)
	}
}

func TestValidateSlabAgainstExisting_RejectsSecondOpenEndedSlab(t *testing.T) {
	existing := []Slab{{ID: "s1", MinWeight: 20, MaxWeight: nil}}
	err := validateSlabAgainstExisting(25, nil, "", existing)
	if !errors.Is(err, ErrMultipleOpenEndedSlabs) {
		t.Errorf("error = %v, want ErrMultipleOpenEndedSlabs", err)
	}
}

func TestValidateSlabAgainstExisting_OpenEndedOverlappingFiniteSlabRejected(t *testing.T) {
	existing := []Slab{{ID: "s1", MinWeight: 20, MaxWeight: nil}}
	err := validateSlabAgainstExisting(15, f(25), "", existing)
	if !errors.Is(err, ErrSlabOverlap) {
		t.Errorf("error = %v, want ErrSlabOverlap", err)
	}
}

func TestValidateSlabAgainstExisting_ExcludesOwnPriorVersionOnUpdate(t *testing.T) {
	existing := []Slab{
		{ID: "s1", MinWeight: 0, MaxWeight: f(5)},
		{ID: "s2", MinWeight: 5, MaxWeight: f(10)},
	}
	// Editing s2 to the exact same range it already has must not
	// spuriously conflict with itself.
	if err := validateSlabAgainstExisting(5, f(10), "s2", existing); err != nil {
		t.Errorf("editing a slab to its own unchanged range: unexpected error: %v", err)
	}
	// Editing s2 to overlap s1 must still be rejected.
	if err := validateSlabAgainstExisting(3, f(10), "s2", existing); !errors.Is(err, ErrSlabOverlap) {
		t.Errorf("editing s2 to overlap s1: error = %v, want ErrSlabOverlap", err)
	}
}
