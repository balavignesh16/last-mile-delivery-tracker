// The only input POST /orders/:id/assign accepts — no other field, no
// server-derived value is ever client-supplied (matches the backend's
// assignRequest exactly). POST /orders/:id/auto-assign takes no body
// at all, so it has no corresponding input type here.
export interface ManualAssignInput {
  agent_id: string
}
