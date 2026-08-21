package tracking

import (
	"testing"

	"lastmiletracker/internal/users"
)

var allStatuses = []Status{
	StatusCreated, StatusAssigned, StatusPickedUp, StatusInTransit,
	StatusOutForDelivery, StatusDelivered, StatusFailed, StatusRescheduled,
}

// TestIsValidTransition_EveryLegalEdge confirms all 8 edges from the
// blueprint's own state diagram are accepted.
func TestIsValidTransition_EveryLegalEdge(t *testing.T) {
	legal := []struct{ from, to Status }{
		{StatusCreated, StatusAssigned},
		{StatusAssigned, StatusPickedUp},
		{StatusPickedUp, StatusInTransit},
		{StatusInTransit, StatusOutForDelivery},
		{StatusOutForDelivery, StatusDelivered},
		{StatusOutForDelivery, StatusFailed},
		{StatusFailed, StatusRescheduled},
		{StatusRescheduled, StatusAssigned},
	}
	for _, tc := range legal {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if !IsValidTransition(tc.from, tc.to) {
				t.Errorf("IsValidTransition(%s, %s) = false, want true", tc.from, tc.to)
			}
		})
	}
}

// TestIsValidTransition_EveryOtherPairIsIllegal exhaustively checks
// every (from, to) pair NOT in the legal set above is rejected —
// covers same-status no-ops, backward jumps, and skipped jumps
// (including the blueprint's own named counterexample,
// CREATED->DELIVERED) without hand-enumerating each one.
func TestIsValidTransition_EveryOtherPairIsIllegal(t *testing.T) {
	legalSet := map[[2]Status]bool{
		{StatusCreated, StatusAssigned}:         true,
		{StatusAssigned, StatusPickedUp}:        true,
		{StatusPickedUp, StatusInTransit}:       true,
		{StatusInTransit, StatusOutForDelivery}: true,
		{StatusOutForDelivery, StatusDelivered}: true,
		{StatusOutForDelivery, StatusFailed}:    true,
		{StatusFailed, StatusRescheduled}:       true,
		{StatusRescheduled, StatusAssigned}:     true,
	}
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := legalSet[[2]Status{from, to}]
			got := IsValidTransition(from, to)
			if got != want {
				t.Errorf("IsValidTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestIsValidTransition_NoSelfLoops(t *testing.T) {
	for _, s := range allStatuses {
		if IsValidTransition(s, s) {
			t.Errorf("IsValidTransition(%s, %s) = true, want false (same-status transitions must be rejected)", s, s)
		}
	}
}

func TestIsValidTransition_CreatedToDeliveredRejected(t *testing.T) {
	if IsValidTransition(StatusCreated, StatusDelivered) {
		t.Error("CREATED -> DELIVERED must be rejected (the blueprint's own named counterexample)")
	}
}

func TestIsValidTransition_DeliveredIsTerminal(t *testing.T) {
	for _, to := range allStatuses {
		if IsValidTransition(StatusDelivered, to) {
			t.Errorf("DELIVERED -> %s = true, want false (DELIVERED must have no outgoing edges)", to)
		}
	}
}

func TestIsValidTransition_DeliveredCannotReturnToTransit(t *testing.T) {
	if IsValidTransition(StatusDelivered, StatusInTransit) {
		t.Error("DELIVERED -> IN_TRANSIT must be rejected")
	}
}

// TestIsRoleAuthorized_FullMatrix verifies the exact finalized
// authorization table: ADMIN on every legal edge, DELIVERY_AGENT only
// on the five agent-tier edges, CUSTOMER on none.
func TestIsRoleAuthorized_FullMatrix(t *testing.T) {
	cases := []struct {
		from, to Status
		role     users.Role
		want     bool
	}{
		{StatusCreated, StatusAssigned, users.RoleAdmin, true},
		{StatusCreated, StatusAssigned, users.RoleDeliveryAgent, false},
		{StatusCreated, StatusAssigned, users.RoleCustomer, false},

		{StatusAssigned, StatusPickedUp, users.RoleAdmin, true},
		{StatusAssigned, StatusPickedUp, users.RoleDeliveryAgent, true},
		{StatusAssigned, StatusPickedUp, users.RoleCustomer, false},

		{StatusPickedUp, StatusInTransit, users.RoleAdmin, true},
		{StatusPickedUp, StatusInTransit, users.RoleDeliveryAgent, true},
		{StatusPickedUp, StatusInTransit, users.RoleCustomer, false},

		{StatusInTransit, StatusOutForDelivery, users.RoleAdmin, true},
		{StatusInTransit, StatusOutForDelivery, users.RoleDeliveryAgent, true},
		{StatusInTransit, StatusOutForDelivery, users.RoleCustomer, false},

		{StatusOutForDelivery, StatusDelivered, users.RoleAdmin, true},
		{StatusOutForDelivery, StatusDelivered, users.RoleDeliveryAgent, true},
		{StatusOutForDelivery, StatusDelivered, users.RoleCustomer, false},

		{StatusOutForDelivery, StatusFailed, users.RoleAdmin, true},
		{StatusOutForDelivery, StatusFailed, users.RoleDeliveryAgent, true},
		{StatusOutForDelivery, StatusFailed, users.RoleCustomer, false},

		{StatusFailed, StatusRescheduled, users.RoleAdmin, true},
		{StatusFailed, StatusRescheduled, users.RoleDeliveryAgent, false},
		{StatusFailed, StatusRescheduled, users.RoleCustomer, false},

		{StatusRescheduled, StatusAssigned, users.RoleAdmin, true},
		{StatusRescheduled, StatusAssigned, users.RoleDeliveryAgent, false},
		{StatusRescheduled, StatusAssigned, users.RoleCustomer, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to)+"/"+string(tc.role), func(t *testing.T) {
			got := IsRoleAuthorized(tc.from, tc.to, tc.role)
			if got != tc.want {
				t.Errorf("IsRoleAuthorized(%s, %s, %s) = %v, want %v", tc.from, tc.to, tc.role, got, tc.want)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	for _, s := range allStatuses {
		got, ok := ParseStatus(string(s))
		if !ok || got != s {
			t.Errorf("ParseStatus(%q) = (%v, %v), want (%v, true)", s, got, ok, s)
		}
	}
	if _, ok := ParseStatus("SHIPPED"); ok {
		t.Error("ParseStatus(SHIPPED) succeeded, want failure")
	}
	if _, ok := ParseStatus(""); ok {
		t.Error("ParseStatus(\"\") succeeded, want failure")
	}
}
