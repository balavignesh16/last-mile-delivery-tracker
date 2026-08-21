package zones

import (
	"context"
	"errors"
	"fmt"
)

// ErrZoneInactive is returned by ResolveArea/ResolvePickupDrop when the
// resolved area's zone has been deactivated.
//
// This is a deliberate M04 decision, not deferred to a later module: an
// admin deactivates a zone specifically to take it out of operational
// use, so resolving an order through it silently would defeat the
// purpose of the active flag. A later module that genuinely needs a
// "resolve anyway, decide downstream" mode would have to introduce that
// as its own explicit change — M04 does not leave that door open by
// accident.
var ErrZoneInactive = errors.New("zone is inactive")

// ResolveArea resolves a single area to its zone — the Address -> Area ->
// Zone chain the frozen architecture specifies, minus "Address": this
// project has no geocoding, so the caller supplies an area id directly
// (e.g. one the customer picked from a list in M06's quote request, or
// M07's later order form), never a free-text address run through an
// external service. See the M04 report's STEP 9 discussion for why that
// boundary was chosen.
//
// Returns ErrAreaNotFound if the area does not exist, ErrZoneInactive if
// the area's zone has been deactivated, or a wrapped error if the area's
// zone_id fails to resolve at all — unreachable in practice given the
// areas.zone_id foreign key, but handled explicitly rather than assumed
// away, per the task's "area has invalid zone -> server/data-integrity
// error" requirement.
func ResolveArea(ctx context.Context, repo Repository, areaID string) (Area, Zone, error) {
	area, err := repo.FindAreaByID(ctx, areaID)
	if err != nil {
		return Area{}, Zone{}, err
	}

	zone, err := repo.FindZoneByID(ctx, area.ZoneID)
	if err != nil {
		if errors.Is(err, ErrZoneNotFound) {
			return Area{}, Zone{}, fmt.Errorf("area %s references a zone that no longer exists (data integrity error): %w", areaID, err)
		}
		return Area{}, Zone{}, fmt.Errorf("resolve area's zone: %w", err)
	}

	if !zone.Active {
		return Area{}, Zone{}, ErrZoneInactive
	}

	return area, zone, nil
}

// ResolutionResult is the deterministic output of resolving a pickup/drop
// area pair — the shape M06/M07 are expected to consume, so the
// intra/inter comparison is computed once here and never reimplemented
// downstream.
type ResolutionResult struct {
	PickupArea   Area
	PickupZone   Zone
	DropArea     Area
	DropZone     Zone
	Relationship ZoneRelationship
}

// ResolvePickupDrop resolves both legs of an order and classifies the
// result. Never guesses: if either area fails to resolve, the whole
// operation fails explicitly rather than falling back to some default
// zone or relationship.
func ResolvePickupDrop(ctx context.Context, repo Repository, pickupAreaID, dropAreaID string) (ResolutionResult, error) {
	pickupArea, pickupZone, err := ResolveArea(ctx, repo, pickupAreaID)
	if err != nil {
		return ResolutionResult{}, fmt.Errorf("resolve pickup area: %w", err)
	}

	dropArea, dropZone, err := ResolveArea(ctx, repo, dropAreaID)
	if err != nil {
		return ResolutionResult{}, fmt.Errorf("resolve drop area: %w", err)
	}

	return ResolutionResult{
		PickupArea:   pickupArea,
		PickupZone:   pickupZone,
		DropArea:     dropArea,
		DropZone:     dropZone,
		Relationship: DetermineZoneRelationship(pickupZone.ID, dropZone.ID),
	}, nil
}
