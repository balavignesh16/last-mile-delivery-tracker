import { apiGet, apiPost } from './api'
import type { Reschedule, RescheduleInput } from '../types/reschedule'
import type { Order } from '../types/order'

// Returns the updated order (never a separate "reschedule" response
// type) — the same convention every other M08/M09 write endpoint uses.
// The reschedule record itself is available, unchanged, via
// getReschedules below.
export function rescheduleOrder(token: string, orderId: string, input: RescheduleInput): Promise<Order> {
  return apiPost<Order>(`/api/v1/orders/${orderId}/reschedule`, input, token)
}

export function getReschedules(token: string, orderId: string): Promise<Reschedule[]> {
  return apiGet<Reschedule[]>(`/api/v1/orders/${orderId}/reschedules`, token)
}
