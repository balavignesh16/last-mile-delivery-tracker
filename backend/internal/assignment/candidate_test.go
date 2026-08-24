package assignment

import (
	"errors"
	"testing"
	"time"

	"lastmiletracker/internal/agents"
)

func zonePtr(id string) *string      { return &id }
func timePtr(t time.Time) *time.Time { return &t }
func floatPtr(f float64) *float64    { return &f }

const (
	zoneA = "zone-a"
	zoneB = "zone-b"
)

// --- IsEligible ---

func TestIsEligible_InactiveExcluded(t *testing.T) {
	c := Candidate{AgentID: "a1", Active: false, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)}
	if IsEligible(c) {
		t.Error("IsEligible() = true, want false for an inactive agent")
	}
}

func TestIsEligible_BusyExcluded(t *testing.T) {
	c := Candidate{AgentID: "a1", Active: true, Availability: agents.AvailabilityBusy, CurrentZoneID: zonePtr(zoneA)}
	if IsEligible(c) {
		t.Error("IsEligible() = true, want false for a BUSY agent")
	}
}

func TestIsEligible_OfflineExcluded(t *testing.T) {
	c := Candidate{AgentID: "a1", Active: true, Availability: agents.AvailabilityOffline, CurrentZoneID: zonePtr(zoneA)}
	if IsEligible(c) {
		t.Error("IsEligible() = true, want false for an OFFLINE agent")
	}
}

func TestIsEligible_NoZoneExcluded(t *testing.T) {
	c := Candidate{AgentID: "a1", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: nil}
	if IsEligible(c) {
		t.Error("IsEligible() = true, want false for an agent with no usable current_zone_id")
	}
}

func TestIsEligible_ActiveAvailableWithZoneIncluded(t *testing.T) {
	c := Candidate{AgentID: "a1", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)}
	if !IsEligible(c) {
		t.Error("IsEligible() = false, want true for an active/AVAILABLE agent with a usable zone")
	}
}

// --- SelectCandidate ---

func TestSelectCandidate_NoCandidatesReturnsError(t *testing.T) {
	_, err := SelectCandidate(nil, zoneA, nil, nil)
	if !errors.Is(err, ErrNoEligibleCandidate) {
		t.Errorf("error = %v, want ErrNoEligibleCandidate", err)
	}
}

func TestSelectCandidate_AllIneligibleReturnsError(t *testing.T) {
	pool := []Candidate{
		{AgentID: "a1", Active: false, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "a2", Active: true, Availability: agents.AvailabilityBusy, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "a3", Active: true, Availability: agents.AvailabilityOffline, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "a4", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: nil},
	}
	_, err := SelectCandidate(pool, zoneA, nil, nil)
	if !errors.Is(err, ErrNoEligibleCandidate) {
		t.Errorf("error = %v, want ErrNoEligibleCandidate", err)
	}
}

func TestSelectCandidate_SameZonePreferredOverCrossZone(t *testing.T) {
	pool := []Candidate{
		{AgentID: "cross", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB)},
		{AgentID: "same", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "same" {
		t.Errorf("winner = %s, want same-zone agent", winner.AgentID)
	}
}

func TestSelectCandidate_CrossZoneAcceptedWhenNoSameZoneCandidate(t *testing.T) {
	pool := []Candidate{
		{AgentID: "cross", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB)},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "cross" {
		t.Errorf("winner = %s, want the only (cross-zone) candidate", winner.AgentID)
	}
}

func TestSelectCandidate_LastAssignedAtAscendingNullFirst(t *testing.T) {
	now := time.Now()
	pool := []Candidate{
		{AgentID: "recent", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now)},
		{AgentID: "never", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: nil},
		{AgentID: "older", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now.Add(-time.Hour))},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "never" {
		t.Errorf("winner = %s, want the never-assigned (NULL last_assigned_at) agent to rank first", winner.AgentID)
	}
}

func TestSelectCandidate_LastAssignedAtOlderBeatsNewer(t *testing.T) {
	now := time.Now()
	pool := []Candidate{
		{AgentID: "recent", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now)},
		{AgentID: "older", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now.Add(-time.Hour))},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "older" {
		t.Errorf("winner = %s, want the longer-idle agent", winner.AgentID)
	}
}

func TestSelectCandidate_UUIDTiebreakIsFinal(t *testing.T) {
	now := time.Now()
	pool := []Candidate{
		{AgentID: "b-agent", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now)},
		{AgentID: "a-agent", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now)},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "a-agent" {
		t.Errorf("winner = %s, want the lexicographically smaller agent id as the final tiebreak", winner.AgentID)
	}
}

func TestSelectCandidate_IneligibleCandidatesNeverSelected(t *testing.T) {
	pool := []Candidate{
		{AgentID: "ineligible", Active: false, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "eligible", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB)},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "eligible" {
		t.Errorf("winner = %s, want the only eligible candidate even though it is cross-zone", winner.AgentID)
	}
}

// TestSelectCandidate_Deterministic runs the same pool through
// SelectCandidate under many different input orderings and asserts the
// winner never changes — proving rankLess is a genuine strict total
// order, not just "usually stable" sort output.
func TestSelectCandidate_Deterministic(t *testing.T) {
	now := time.Now()
	base := []Candidate{
		{AgentID: "a1", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB), LastAssignedAt: timePtr(now)},
		{AgentID: "a2", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: nil},
		{AgentID: "a3", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), LastAssignedAt: timePtr(now.Add(-time.Minute))},
		{AgentID: "a4", Active: false, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "a5", Active: true, Availability: agents.AvailabilityBusy, CurrentZoneID: zonePtr(zoneA)},
	}
	orderings := [][]int{
		{0, 1, 2, 3, 4},
		{4, 3, 2, 1, 0},
		{2, 0, 4, 1, 3},
		{1, 2, 0, 3, 4},
	}
	var want string
	for i, order := range orderings {
		pool := make([]Candidate, len(order))
		for j, idx := range order {
			pool[j] = base[idx]
		}
		winner, err := SelectCandidate(pool, zoneA, nil, nil)
		if err != nil {
			t.Fatalf("SelectCandidate() error: %v", err)
		}
		if i == 0 {
			want = winner.AgentID
			continue
		}
		if winner.AgentID != want {
			t.Errorf("ordering %d: winner = %s, want %s (same regardless of input order)", i, winner.AgentID, want)
		}
	}
	if want != "a2" {
		t.Errorf("winner = %s, want a2 (same-zone, never-assigned)", want)
	}
}

// --- SelectCandidate: distance-based ranking ---
//
// pickup is a fixed reference point (0,0); candidate longitudes below
// are chosen so their approximate distance from it is easy to reason
// about (~111km per degree of longitude at the equator — see
// internal/geo's own TestHaversineKM_OneDegreeAtEquator), without
// needing exact real-world coordinates.

func TestSelectCandidate_NearestAgentWinsAmongThree(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	pool := []Candidate{
		// ~5.6km away
		{AgentID: "agent-a", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.05)},
		// ~2.2km away — nearest
		{AgentID: "agent-b", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.02)},
		// ~8.9km away
		{AgentID: "agent-c", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.08)},
	}
	winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "agent-b" {
		t.Errorf("winner = %s, want agent-b (nearest of the three)", winner.AgentID)
	}
}

func TestSelectCandidate_RealDistanceBeatsZoneMatch(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	pool := []Candidate{
		// Same zone as pickup, but no known location — ranked by the
		// zone-based fallback only.
		{AgentID: "same-zone-no-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		// A different zone, but a real, known, close distance — this
		// candidate has strictly better information and must win.
		{AgentID: "cross-zone-with-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB), Latitude: floatPtr(0), Longitude: floatPtr(0.02)},
	}
	winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "cross-zone-with-location" {
		t.Errorf("winner = %s, want the candidate with a real computable distance, even though it's cross-zone", winner.AgentID)
	}
}

// Mirrors the feature spec's "Test 3": the geographically nearest agent
// is excluded because it isn't AVAILABLE — eligibility is checked
// before ranking, so proximity alone never overrides it.
func TestSelectCandidate_UnavailableNearestAgentExcluded(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	pool := []Candidate{
		// Nearest, but BUSY — must never be selected.
		{AgentID: "nearest-busy", Active: true, Availability: agents.AvailabilityBusy, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.01)},
		// Farther, but AVAILABLE.
		{AgentID: "farther-available", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.03)},
	}
	winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "farther-available" {
		t.Errorf("winner = %s, want the farther but AVAILABLE agent, never the nearer BUSY one", winner.AgentID)
	}
}

// Mirrors the feature spec's "Test 4": an AVAILABLE agent with no valid
// location must not be preferred just because they are "available" —
// the agent with a real, known location wins.
func TestSelectCandidate_AgentWithoutLocationLosesToAgentWithLocation(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	pool := []Candidate{
		{AgentID: "no-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "with-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.05)},
	}
	winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "with-location" {
		t.Errorf("winner = %s, want the agent with a real computable distance", winner.AgentID)
	}
}

// When the pickup point itself has no coordinates (the common case
// today — an area an admin hasn't set coordinates for), ranking must
// fall back to exactly the pre-existing zone-based behavior, even
// though the agents themselves do have coordinates.
func TestSelectCandidate_PickupWithoutCoordinatesFallsBackToZone(t *testing.T) {
	pool := []Candidate{
		{AgentID: "cross-zone-with-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB), Latitude: floatPtr(0), Longitude: floatPtr(0.01)},
		{AgentID: "same-zone-with-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.05)},
	}
	winner, err := SelectCandidate(pool, zoneA, nil, nil)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "same-zone-with-location" {
		t.Errorf("winner = %s, want the same-zone agent (zone-based fallback, pickup has no coordinates)", winner.AgentID)
	}
}

// When an agent's own coordinates are unknown but the pickup point's
// are, that agent simply has no computable distance — same fallback.
func TestSelectCandidate_AgentWithoutCoordinatesFallsBackToZoneForThatAgent(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	pool := []Candidate{
		{AgentID: "same-zone-no-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "cross-zone-no-location", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB)},
	}
	winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "same-zone-no-location" {
		t.Errorf("winner = %s, want the same-zone agent (zone-based fallback, no agent has a usable location)", winner.AgentID)
	}
}

// An exact distance tie must not be arbitrary — it falls through to the
// same last_assigned_at / agent id tiebreakers an ordinary zone-based
// tie already uses.
func TestSelectCandidate_ExactDistanceTieFallsThroughToExistingTiebreakers(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	now := time.Now()
	pool := []Candidate{
		{AgentID: "b-agent", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.05), LastAssignedAt: timePtr(now)},
		{AgentID: "a-agent", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.05), LastAssignedAt: timePtr(now)},
	}
	winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
	if err != nil {
		t.Fatalf("SelectCandidate() error: %v", err)
	}
	if winner.AgentID != "a-agent" {
		t.Errorf("winner = %s, want the lexicographically smaller agent id on an exact distance tie", winner.AgentID)
	}
}

// Same proof-of-determinism discipline as TestSelectCandidate_Deterministic,
// with real distances now in play.
func TestSelectCandidate_DeterministicWithDistance(t *testing.T) {
	pickupLat, pickupLng := floatPtr(0), floatPtr(0)
	base := []Candidate{
		{AgentID: "a1", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB), Latitude: floatPtr(0), Longitude: floatPtr(0.08)},
		{AgentID: "a2", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.02)},
		{AgentID: "a3", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
		{AgentID: "a4", Active: false, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.01)},
		{AgentID: "a5", Active: true, Availability: agents.AvailabilityBusy, CurrentZoneID: zonePtr(zoneA), Latitude: floatPtr(0), Longitude: floatPtr(0.01)},
	}
	orderings := [][]int{
		{0, 1, 2, 3, 4},
		{4, 3, 2, 1, 0},
		{2, 0, 4, 1, 3},
		{1, 2, 0, 3, 4},
	}
	var want string
	for i, order := range orderings {
		pool := make([]Candidate, len(order))
		for j, idx := range order {
			pool[j] = base[idx]
		}
		winner, err := SelectCandidate(pool, zoneA, pickupLat, pickupLng)
		if err != nil {
			t.Fatalf("SelectCandidate() error: %v", err)
		}
		if i == 0 {
			want = winner.AgentID
			continue
		}
		if winner.AgentID != want {
			t.Errorf("ordering %d: winner = %s, want %s (same regardless of input order)", i, winner.AgentID, want)
		}
	}
	if want != "a2" {
		t.Errorf("winner = %s, want a2 (only eligible candidate with a real, computable distance)", want)
	}
}
