// Package agents owns delivery-agent operational data: availability,
// current location, and admin provisioning. It depends on internal/auth
// (RBAC middleware, password hashing) and internal/users (the linked
// user record every agent has) — nothing depends back on agents, so
// neither of those dependencies is a cycle.
//
// M03 provides the domain data and management APIs only. M09's actual
// candidate-selection algorithm (ranking, concurrency-safe assignment)
// is a later module — see CreateAgentInput and Repository for the exact
// fields M09 will read.
package agents

import "time"

// Availability is one of three operational states. Stored as a plain
// string constrained by a CHECK constraint (same pattern as users.Role),
// not a Postgres ENUM, so adding a state later never needs ALTER TYPE.
type Availability string

const (
	AvailabilityAvailable Availability = "AVAILABLE"
	AvailabilityBusy      Availability = "BUSY"
	AvailabilityOffline   Availability = "OFFLINE"
)

// ParseAvailability validates a raw string against the three permitted
// values. Used both by the API handler (reject "FREE"/"ONLINE"/etc. with
// a 422) and available for M09 to reuse if it ever needs to parse a
// stored value defensively.
func ParseAvailability(raw string) (Availability, bool) {
	switch Availability(raw) {
	case AvailabilityAvailable, AvailabilityBusy, AvailabilityOffline:
		return Availability(raw), true
	default:
		return "", false
	}
}

// AgentWithUser is the joined shape every Repository method returns:
// delivery_agents' operational columns plus the linked user's identity
// fields (name, email, phone) — never password_hash. users and
// delivery_agents deliberately do not duplicate full_name/email/phone;
// this join is where they meet for API responses.
type AgentWithUser struct {
	ID                string
	UserID            string
	FullName          string
	Email             string
	Phone             *string
	Availability      Availability
	CurrentLat        *float64
	CurrentLng        *float64
	CurrentZoneID     *string
	LocationUpdatedAt *time.Time
	// LastAssignedAt and Active are read but not written by M03 — M09
	// sets LastAssignedAt on assignment; Active is exposed for admin
	// visibility now, with the eligibility/assignment algorithm that
	// actually checks it landing in M09. active and availability are
	// deliberately independent: active=false with availability=AVAILABLE
	// must not make an agent eligible for assignment once M09 exists,
	// even though M03 itself does not enforce that yet.
	LastAssignedAt *time.Time
	Active         bool
	CreatedAt      time.Time
}

// CreateAgentInput is Create's only input — the password is already
// hashed by the caller (CreateAgentHandler, reusing auth.HashPassword;
// this package never hashes a password itself). There is no Role field:
// Create always provisions a DELIVERY_AGENT, the same server-controlled
// pattern as auth.RegisterHandler always provisioning a CUSTOMER.
type CreateAgentInput struct {
	Email        string
	PasswordHash string
	FullName     string
	Phone        *string
}
