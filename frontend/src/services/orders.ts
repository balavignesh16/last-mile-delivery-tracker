import { apiGet, apiPost } from './api'
import type { AdminOrderPackageInput, Order, OrderPackageInput } from '../types/order'

export function createOrder(token: string, input: OrderPackageInput | AdminOrderPackageInput): Promise<Order> {
  return apiPost<Order>('/api/v1/orders', input, token)
}

export function listOrders(token: string): Promise<Order[]> {
  return apiGet<Order[]>('/api/v1/orders', token)
}

export function getOrder(token: string, orderId: string): Promise<Order> {
  return apiGet<Order>(`/api/v1/orders/${orderId}`, token)
}
