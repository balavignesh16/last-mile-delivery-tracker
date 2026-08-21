import { apiDelete, apiGet, apiPost, apiPut } from './api'
import type { CreateRateCardInput, CreateSlabInput, RateCard, Slab, UpdateRateCardInput, UpdateSlabInput } from '../types/rate'

export function listRateCards(token: string): Promise<RateCard[]> {
  return apiGet<RateCard[]>('/api/v1/rates', token)
}

export function createRateCard(token: string, input: CreateRateCardInput): Promise<RateCard> {
  return apiPost<RateCard>('/api/v1/rates', input, token)
}

export function updateRateCard(token: string, rateCardId: string, input: UpdateRateCardInput): Promise<RateCard> {
  return apiPut<RateCard>(`/api/v1/rates/${rateCardId}`, input, token)
}

export function listSlabs(token: string, rateCardId: string): Promise<Slab[]> {
  return apiGet<Slab[]>(`/api/v1/rates/${rateCardId}/slabs`, token)
}

export function createSlab(token: string, rateCardId: string, input: CreateSlabInput): Promise<Slab> {
  return apiPost<Slab>(`/api/v1/rates/${rateCardId}/slabs`, input, token)
}

export function updateSlab(token: string, rateCardId: string, slabId: string, input: UpdateSlabInput): Promise<Slab> {
  return apiPut<Slab>(`/api/v1/rates/${rateCardId}/slabs/${slabId}`, input, token)
}

export function deleteSlab(token: string, rateCardId: string, slabId: string): Promise<void> {
  return apiDelete<void>(`/api/v1/rates/${rateCardId}/slabs/${slabId}`, token)
}
