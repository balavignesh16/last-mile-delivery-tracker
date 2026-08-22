import { apiGet, apiPost } from './api'
import type { AdminOrderPackageInput, Order, OrderFilter, OrderPackageInput } from '../types/order'

export function createOrder(token: string, input: OrderPackageInput | AdminOrderPackageInput): Promise<Order> {
  return apiPost<Order>('/api/v1/orders', input, token)
}

// filter's fields are honored by the backend for ADMIN only (M12) — a
// CUSTOMER/DELIVERY_AGENT caller may still pass one (e.g. if this
// function is reused from a non-admin page in the future), the backend
// just ignores it and returns their usual role-scoped result, so this
// function never needs to know the caller's role itself.
export function listOrders(token: string, filter?: OrderFilter): Promise<Order[]> {
  const params = new URLSearchParams()
  if (filter?.status) params.set('status', filter.status)
  if (filter?.zone) params.set('zone', filter.zone)
  if (filter?.agent) params.set('agent', filter.agent)
  const query = params.toString()
  return apiGet<Order[]>(`/api/v1/orders${query ? `?${query}` : ''}`, token)
}

export function getOrder(token: string, orderId: string): Promise<Order> {
  return apiGet<Order>(`/api/v1/orders/${orderId}`, token)
}
