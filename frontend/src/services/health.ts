import { apiGet } from './api'
import type { HealthResponse } from '../types/health'

export function getHealth(): Promise<HealthResponse> {
  return apiGet<HealthResponse>('/health')
}
