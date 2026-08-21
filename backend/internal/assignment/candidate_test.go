package assignment

import (
	"errors"
	"testing"
	"time"

	"lastmiletracker/internal/agents"
)

func zonePtr(id string) *string      { return &id }
func timePtr(t time.Time) *time.Time { return &t }

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
	_, err := SelectCandidate(nil, zoneA)
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
	_, err := SelectCandidate(pool, zoneA)
	if !errors.Is(err, ErrNoEligibleCandidate) {
		t.Errorf("error = %v, want ErrNoEligibleCandidate", err)
	}
}

func TestSelectCandidate_SameZonePreferredOverCrossZone(t *testing.T) {
	pool := []Candidate{
		{AgentID: "cross", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneB)},
		{AgentID: "same", Active: true, Availability: agents.AvailabilityAvailable, CurrentZoneID: zonePtr(zoneA)},
	}
	winner, err := SelectCandidate(pool, zoneA)
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
	winner, err := SelectCandidate(pool, zoneA)
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
	winner, err := SelectCandidate(pool, zoneA)
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
	winner, err := SelectCandidate(pool, zoneA)
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
	winner, err := SelectCandidate(pool, zoneA)
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
	winner, err := SelectCandidate(pool, zoneA)
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
		winner, err := SelectCandidate(pool, zoneA)
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
