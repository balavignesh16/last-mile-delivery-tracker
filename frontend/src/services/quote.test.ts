import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { requestQuote } from './quote'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

const sampleInput = {
  pickup_area_id: 'area-1',
  drop_area_id: 'area-2',
  order_type: 'B2C' as const,
  payment_type: 'COD' as const,
  length_cm: 10,
  breadth_cm: 10,
  height_cm: 10,
  actual_weight_kg: 5,
}

describe('quote service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('requestQuote posts to /api/v1/orders/quote with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { final_amount: 65 }))
    vi.stubGlobal('fetch', fetchMock)

    await requestQuote('customer-token', sampleInput)

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/quote')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify(sampleInput))
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer customer-token')
  })

  it('requestQuote surfaces a 422 for an unserviceable request', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(422, { error: 'no active rate card is configured for this order type and zone relationship' })),
    )

    await expect(requestQuote('customer-token', sampleInput)).rejects.toBeInstanceOf(ApiError)
    await expect(requestQuote('customer-token', sampleInput)).rejects.toMatchObject({ status: 422 })
  })

  it('requestQuote surfaces a 403 for a delivery agent caller', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'insufficient role for this operation' })))

    await expect(requestQuote('agent-token', sampleInput)).rejects.toMatchObject({ status: 403 })
  })
})
