package zones

import (
	"context"
	"errors"
	"testing"
)

func TestResolvePickupDrop_SameZoneIsIntra(t *testing.T) {
	repo := newFakeRepo()
	zoneA, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZoneA"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	areaA1, err := repo.CreateArea(context.Background(), zoneA.ID, CreateAreaInput{Name: "A1"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	result, err := ResolvePickupDrop(context.Background(), repo, areaA1.ID, areaA1.ID)
	if err != nil {
		t.Fatalf("ResolvePickupDrop() error: %v", err)
	}
	if result.Relationship != RelationshipIntra {
		t.Errorf("relationship = %v, want INTRA", result.Relationship)
	}
}

func TestResolvePickupDrop_DifferentZonesIsInter(t *testing.T) {
	repo := newFakeRepo()
	zoneA, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZoneA2"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	zoneB, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZoneB2"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	areaA1, err := repo.CreateArea(context.Background(), zoneA.ID, CreateAreaInput{Name: "A1"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	areaB1, err := repo.CreateArea(context.Background(), zoneB.ID, CreateAreaInput{Name: "B1"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	result, err := ResolvePickupDrop(context.Background(), repo, areaA1.ID, areaB1.ID)
	if err != nil {
		t.Fatalf("ResolvePickupDrop() error: %v", err)
	}
	if result.Relationship != RelationshipInter {
		t.Errorf("relationship = %v, want INTER", result.Relationship)
	}
	if result.PickupZone.ID != zoneA.ID || result.DropZone.ID != zoneB.ID {
		t.Errorf("resolved zones = %v/%v, want %v/%v", result.PickupZone.ID, result.DropZone.ID, zoneA.ID, zoneB.ID)
	}
}

func TestResolvePickupDrop_NonexistentAreaFailsExplicitly(t *testing.T) {
	repo := newFakeRepo()
	zoneA, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ZoneA3"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	areaA1, err := repo.CreateArea(context.Background(), zoneA.ID, CreateAreaInput{Name: "A1"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	_, err = ResolvePickupDrop(context.Background(), repo, areaA1.ID, "does-not-exist")
	if !errors.Is(err, ErrAreaNotFound) {
		t.Errorf("error = %v, want wrapped ErrAreaNotFound", err)
	}
}

func TestResolveArea_InactiveZoneIsRejected(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "InactiveZone"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "AreaInInactiveZone"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	inactive := false
	if _, err := repo.UpdateZone(context.Background(), zone.ID, ZoneUpdate{Name: zone.Name, Active: &inactive}); err != nil {
		t.Fatalf("deactivate zone failed: %v", err)
	}

	_, _, err = ResolveArea(context.Background(), repo, area.ID)
	if !errors.Is(err, ErrZoneInactive) {
		t.Errorf("error = %v, want ErrZoneInactive", err)
	}
}

// TestResolveArea_InactiveAreaIsRejected verifies deactivating a single
// area blocks resolution even when its parent zone is still active —
// independent of TestResolveArea_InactiveZoneIsRejected, which covers
// the zone-level check.
func TestResolveArea_InactiveAreaIsRejected(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "StillActiveZone"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "InactiveArea"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	inactive := false
	if _, err := repo.UpdateArea(context.Background(), area.ID, AreaUpdate{Name: area.Name, Active: &inactive}); err != nil {
		t.Fatalf("deactivate area failed: %v", err)
	}

	_, _, err = ResolveArea(context.Background(), repo, area.ID)
	if !errors.Is(err, ErrAreaInactive) {
		t.Errorf("error = %v, want ErrAreaInactive", err)
	}
}

func TestResolveArea_ActiveZoneSucceeds(t *testing.T) {
	repo := newFakeRepo()
	zone, err := repo.CreateZone(context.Background(), CreateZoneInput{Name: "ActiveZone"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}
	area, err := repo.CreateArea(context.Background(), zone.ID, CreateAreaInput{Name: "AreaInActiveZone"})
	if err != nil {
		t.Fatalf("seed create failed: %v", err)
	}

	resolvedArea, resolvedZone, err := ResolveArea(context.Background(), repo, area.ID)
	if err != nil {
		t.Fatalf("ResolveArea() error: %v", err)
	}
	if resolvedArea.ID != area.ID || resolvedZone.ID != zone.ID {
		t.Errorf("resolved (area, zone) = (%v, %v), want (%v, %v)", resolvedArea.ID, resolvedZone.ID, area.ID, zone.ID)
	}
}
