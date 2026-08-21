// Mirrors backend's rescheduling.Reschedule exactly
// (internal/rescheduling/reschedule.go) via its own response shape
// (internal/rescheduling/handler.go's rescheduleResponse). No status
// field — a reschedule request is immediately effective once created,
// there is no pending/approved/rejected workflow.
export interface Reschedule {
  id: string
  order_id: string
  requested_by: string
  requested_date: string
  reason: string | null
  created_at: string
}

// POST /orders/:id/reschedule's only input — no field for anything
// server-derived (actor_id, requested_by, order_id, status,
// created_at), matching the backend's rescheduleRequest DTO exactly.
export interface RescheduleInput {
  requested_date: string
  reason?: string
}
