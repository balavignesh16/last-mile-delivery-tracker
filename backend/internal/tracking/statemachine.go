package tracking

import "lastmiletracker/internal/users"

// legalTransitions is the complete, closed set of legal edges — the
// blueprint's own state diagram, transcribed exactly. Any (from, to)
// pair not listed here is illegal, including same-status pairs (no
// self-loops are listed) and every backward/skipped jump the blueprint
// explicitly calls out as forbidden (e.g. CREATED -> DELIVERED).
// DELIVERED has no outgoing edges at all — fully terminal.
var legalTransitions = map[Status][]Status{
	StatusCreated:        {StatusAssigned},
	StatusAssigned:       {StatusPickedUp},
	StatusPickedUp:       {StatusInTransit},
	StatusInTransit:      {StatusOutForDelivery},
	StatusOutForDelivery: {StatusDelivered, StatusFailed},
	StatusFailed:         {StatusRescheduled},
	StatusRescheduled:    {StatusAssigned},
	StatusDelivered:      {},
}

// IsValidTransition reports whether (from, to) is one of the edges
// above. A same-status pair is never valid — no edge is a self-loop.
func IsValidTransition(from, to Status) bool {
	for _, candidate := range legalTransitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

// edge identifies one specific transition, used as a map key for
// edgeAuthorizedRoles below.
type edge struct {
	From Status
	To   Status
}

// edgeAuthorizedRoles is the finalized M08 authorization matrix. Every
// edge maps to the exact roles allowed to perform it; CUSTOMER
// appears in none of them — customers have no status-transition
// authority in M08 (see docs/order-tracking.md). This is a per-edge
// table, not a per-role default, because the same actor (ADMIN) is
// authorized everywhere while DELIVERY_AGENT is authorized only for
// the five agent-tier edges — a single "role X can use this endpoint"
// check (route-level RBAC) is too coarse to express that difference.
var edgeAuthorizedRoles = map[edge][]users.Role{
	{StatusCreated, StatusAssigned}:         {users.RoleAdmin},
	{StatusAssigned, StatusPickedUp}:        {users.RoleAdmin, users.RoleDeliveryAgent},
	{StatusPickedUp, StatusInTransit}:       {users.RoleAdmin, users.RoleDeliveryAgent},
	{StatusInTransit, StatusOutForDelivery}: {users.RoleAdmin, users.RoleDeliveryAgent},
	{StatusOutForDelivery, StatusDelivered}: {users.RoleAdmin, users.RoleDeliveryAgent},
	{StatusOutForDelivery, StatusFailed}:    {users.RoleAdmin, users.RoleDeliveryAgent},
	{StatusFailed, StatusRescheduled}:       {users.RoleAdmin},
	{StatusRescheduled, StatusAssigned}:     {users.RoleAdmin},
}

// IsRoleAuthorized reports whether role may perform the (from, to)
// edge. Callers must check IsValidTransition first — this function
// makes no claim about whether the edge is legal at all, only about
// who may use it if it is.
func IsRoleAuthorized(from, to Status, role users.Role) bool {
	for _, r := range edgeAuthorizedRoles[edge{from, to}] {
		if r == role {
			return true
		}
	}
	return false
}
