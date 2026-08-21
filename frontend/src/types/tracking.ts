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
