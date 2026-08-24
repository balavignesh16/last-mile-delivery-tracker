// The only input POST /orders/:id/assign accepts — no other field, no
// server-derived value is ever client-supplied (matches the backend's
// assignRequest exactly). POST /orders/:id/auto-assign takes no body
// at all, so it has no corresponding input type here.
export interface ManualAssignInput {
  agent_id: string
}

// Describes how auto-assignment picked its winning agent — mirrors the
// backend's assignmentInfo exactly. "DISTANCE" means a real Haversine
// distance to the pickup point decided it (distance_km present);
// "ZONE" means the zone-based fallback did (no usable coordinate on one
// or both sides) — see docs/assignment-engine.md.
export interface AssignmentInfo {
  method: 'DISTANCE' | 'ZONE'
  distance_km?: number
}
