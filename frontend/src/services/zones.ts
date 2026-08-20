import { apiGet, apiPost, apiPut } from './api'
import type { Area, CreateAreaInput, CreateZoneInput, UpdateAreaInput, UpdateZoneInput, Zone } from '../types/zone'

export function listZones(token: string): Promise<Zone[]> {
  return apiGet<Zone[]>('/api/v1/zones', token)
}

export function createZone(token: string, input: CreateZoneInput): Promise<Zone> {
  return apiPost<Zone>('/api/v1/zones', input, token)
}

export function updateZone(token: string, zoneId: string, input: UpdateZoneInput): Promise<Zone> {
  return apiPut<Zone>(`/api/v1/zones/${zoneId}`, input, token)
}

export function listAreas(token: string, zoneId: string): Promise<Area[]> {
  return apiGet<Area[]>(`/api/v1/zones/${zoneId}/areas`, token)
}

export function createArea(token: string, zoneId: string, input: CreateAreaInput): Promise<Area> {
  return apiPost<Area>(`/api/v1/zones/${zoneId}/areas`, input, token)
}

export function updateArea(token: string, zoneId: string, areaId: string, input: UpdateAreaInput): Promise<Area> {
  return apiPut<Area>(`/api/v1/zones/${zoneId}/areas/${areaId}`, input, token)
}
