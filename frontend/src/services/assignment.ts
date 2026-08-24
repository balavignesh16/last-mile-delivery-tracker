import { apiPost } from './api'
import type { AssignmentInfo, ManualAssignInput } from '../types/assignment'
import type { Order } from '../types/order'

// Both endpoints return the updated order — manual assign returns
// nothing else (the admin already chose the agent, there is nothing to
// rank or report); auto-assign additionally reports how its winning
// candidate was picked. The tracking timeline either transition writes
// is available, unchanged, via the existing getOrderTracking call.
export function assignOrder(token: string, orderId: string, input: ManualAssignInput): Promise<Order> {
  return apiPost<Order>(`/api/v1/orders/${orderId}/assign`, input, token)
}

export type AutoAssignResult = Order & { assignment: AssignmentInfo }

export function autoAssignOrder(token: string, orderId: string): Promise<AutoAssignResult> {
  return apiPost<AutoAssignResult>(`/api/v1/orders/${orderId}/auto-assign`, {}, token)
}
