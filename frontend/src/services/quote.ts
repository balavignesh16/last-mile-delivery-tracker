import { apiPost } from './api'
import type { QuoteRequest, QuoteResult } from '../types/quote'

export function requestQuote(token: string, input: QuoteRequest): Promise<QuoteResult> {
  return apiPost<QuoteResult>('/api/v1/orders/quote', input, token)
}
