import { apiGet, apiPost, apiPut } from './api'
import type { Agent, Availability, CreateAgentInput } from '../types/agent'

export function listAgents(token: string): Promise<Agent[]> {
  return apiGet<Agent[]>('/api/v1/agents', token)
}

export function createAgent(token: string, input: CreateAgentInput): Promise<Agent> {
  return apiPost<Agent>('/api/v1/agents', input, token)
}

export function fetchMyAgentProfile(token: string): Promise<Agent> {
  return apiGet<Agent>('/api/v1/agents/me', token)
}

export function fetchAgent(token: string, agentId: string): Promise<Agent> {
  return apiGet<Agent>(`/api/v1/agents/${agentId}`, token)
}

export function updateAgentActive(token: string, agentId: string, active: boolean): Promise<Agent> {
  return apiPut<Agent>(`/api/v1/agents/${agentId}/active`, { active }, token)
}

export function updateAgentAvailability(token: string, agentId: string, availability: Availability): Promise<Agent> {
  return apiPut<Agent>(`/api/v1/agents/${agentId}/availability`, { availability }, token)
}

export function updateAgentLocation(
  token: string,
  agentId: string,
  latitude: number,
  longitude: number,
): Promise<Agent> {
  return apiPut<Agent>(`/api/v1/agents/${agentId}/location`, { latitude, longitude }, token)
}

export function updateAgentZone(token: string, agentId: string, zoneId: string): Promise<Agent> {
  return apiPut<Agent>(`/api/v1/agents/${agentId}/zone`, { zone_id: zoneId }, token)
}
