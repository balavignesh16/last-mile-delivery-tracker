import { apiGet, apiPost } from './api'
import type { TrackingEvent, TransitionInput } from '../types/tracking'

export function transitionOrderStatus(token: string, orderId: string, input: TransitionInput): Promise<TrackingEvent> {
  return apiPost<TrackingEvent>(`/api/v1/orders/${orderId}/status`, input, token)
}

export function getOrderTracking(token: string, orderId: string): Promise<TrackingEvent[]> {
  return apiGet<TrackingEvent[]>(`/api/v1/orders/${orderId}/tracking`, token)
}
