import { apiPost } from './api'
import type { ManualAssignInput } from '../types/assignment'
import type { Order } from '../types/order'

// Both endpoints return the updated order (never a separate assignment
// resource) — the tracking timeline this transition also writes is
// available, unchanged, via the existing getOrderTracking call.
export function assignOrder(token: string, orderId: string, input: ManualAssignInput): Promise<Order> {
  return apiPost<Order>(`/api/v1/orders/${orderId}/assign`, input, token)
}

export function autoAssignOrder(token: string, orderId: string): Promise<Order> {
  return apiPost<Order>(`/api/v1/orders/${orderId}/auto-assign`, {}, token)
}
