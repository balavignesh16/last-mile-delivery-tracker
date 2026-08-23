import type { Role } from './auth'
import type { OrderStatus } from './order'

// Mirrors backend's tracking.Event exactly (internal/tracking/event.go).
// previous_status is null only for an order's very first event (its
// own creation) — every later event always has one.
export interface TrackingEvent {
  id: string
  order_id: string
  previous_status: OrderStatus | null
  new_status: OrderStatus
  actor_id: string
  // null only for a row written before this column existed (migration
  // 0015) — never backfilled, since there's no way to know a
  // historical actor's role after the fact.
  actor_role: Role | null
  metadata: Record<string, unknown> | null
  created_at: string
}

// POST /orders/:id/status's only input — no field for anything
// server-derived (actor_id, previous_status, id, created_at), matching
// the backend's transitionRequest DTO exactly.
export interface TransitionInput {
  status: OrderStatus
  metadata?: Record<string, unknown>
}

// Mirrors backend's legalTransitions table exactly
// (internal/tracking/statemachine.go) — used only to decide which
// transition buttons to offer an admin. This is a UX convenience, not
// a security boundary: the backend re-validates and re-authorizes
// every transition regardless of what this table suggests, the same
// disclaimer ProtectedRoute already carries for role-based routing.
export const LEGAL_TRANSITIONS: Record<OrderStatus, OrderStatus[]> = {
  CREATED: ['ASSIGNED'],
  ASSIGNED: ['PICKED_UP'],
  PICKED_UP: ['IN_TRANSIT'],
  IN_TRANSIT: ['OUT_FOR_DELIVERY'],
  OUT_FOR_DELIVERY: ['DELIVERED', 'FAILED'],
  DELIVERED: [],
  FAILED: ['RESCHEDULED'],
  RESCHEDULED: ['ASSIGNED'],
}

// Mirrors backend's edgeAuthorizedRoles table exactly
// (internal/tracking/statemachine.go) for the DELIVERY_AGENT role only
// — the five edges an agent may perform (CREATED->ASSIGNED and the two
// ADMIN-only reschedule-cycle edges are deliberately absent). Same UX-
// convenience-only disclaimer as LEGAL_TRANSITIONS: the backend
// re-validates and re-authorizes every transition regardless (M12).
const DELIVERY_AGENT_TRANSITIONS: Partial<Record<OrderStatus, OrderStatus[]>> = {
  ASSIGNED: ['PICKED_UP'],
  PICKED_UP: ['IN_TRANSIT'],
  IN_TRANSIT: ['OUT_FOR_DELIVERY'],
  OUT_FOR_DELIVERY: ['DELIVERED', 'FAILED'],
}

// transitionsForRole narrows LEGAL_TRANSITIONS to only the edges a given
// role may actually perform from the current status — ADMIN gets every
// legal edge (unchanged), DELIVERY_AGENT gets only the subset above,
// and every other role gets none (CUSTOMER has no status-transition
// authority in M08, unchanged by M12).
export function transitionsForRole(status: OrderStatus, role: Role): OrderStatus[] {
  if (role === 'ADMIN') return LEGAL_TRANSITIONS[status]
  if (role === 'DELIVERY_AGENT') return DELIVERY_AGENT_TRANSITIONS[status] ?? []
  return []
}
