import { apiGet } from './api'
import type { CustomerLookupResult } from '../types/auth'

// GET /api/v1/users/lookup — ADMIN only. Resolves a customer's email to
// their user id, for CreateOrderPage's admin-on-behalf-of-customer flow.
export function lookupCustomerByEmail(token: string, email: string): Promise<CustomerLookupResult> {
  return apiGet<CustomerLookupResult>(`/api/v1/users/lookup?email=${encodeURIComponent(email)}`, token)
}
