import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { createRateCard, createSlab, deleteSlab, listRateCards, listSlabs, updateRateCard, updateSlab } from './rates'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('rates service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listRateCards fetches /api/v1/rates with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await listRateCards('admin-token')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer admin-token')
  })

  it('listRateCards surfaces a 403 for a non-admin caller', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'insufficient role for this operation' })))

    await expect(listRateCards('customer-token')).rejects.toMatchObject({ status: 403 })
  })

  it('createRateCard posts to /api/v1/rates', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'r1', order_type: 'B2B', zone_relationship: 'INTRA', active: false }))
    vi.stubGlobal('fetch', fetchMock)

    await createRateCard('admin-token', { order_type: 'B2B', zone_relationship: 'INTRA', cod_surcharge: 10 })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ order_type: 'B2B', zone_relationship: 'INTRA', cod_surcharge: 10 }))
  })

  it('updateRateCard PUTs cod_surcharge and active to the rate card endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'r1', active: true }))
    vi.stubGlobal('fetch', fetchMock)

    await updateRateCard('admin-token', 'r1', { cod_surcharge: 20, active: true })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates/r1')
    expect(init.method).toBe('PUT')
    expect(init.body).toBe(JSON.stringify({ cod_surcharge: 20, active: true }))
  })

  it('updateRateCard surfaces a 409 when activating a conflicting combination', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(409, { error: 'another rate card is already active for this order type and zone relationship' })),
    )

    await expect(updateRateCard('admin-token', 'r2', { cod_surcharge: 0, active: true })).rejects.toMatchObject({ status: 409 })
  })

  it('listSlabs fetches slabs scoped to a rate card', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await listSlabs('admin-token', 'r1')

    const [path] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates/r1/slabs')
  })

  it('createSlab posts to the rate-card-scoped slabs endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 's1', rate_card_id: 'r1', min_weight: 0, max_weight: 5, price: 50 }))
    vi.stubGlobal('fetch', fetchMock)

    await createSlab('admin-token', 'r1', { min_weight: 0, max_weight: 5, price: 50 })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates/r1/slabs')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ min_weight: 0, max_weight: 5, price: 50 }))
  })

  it('createSlab surfaces a 409 for an overlapping slab', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'slab weight range overlaps an existing slab in this rate card' })))

    await expect(createSlab('admin-token', 'r1', { min_weight: 3, max_weight: 10, price: 80 })).rejects.toMatchObject({ status: 409 })
  })

  it('updateSlab PUTs to the rate-card- and slab-scoped endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 's1', rate_card_id: 'r1', min_weight: 0, max_weight: 5, price: 60 }))
    vi.stubGlobal('fetch', fetchMock)

    await updateSlab('admin-token', 'r1', 's1', { min_weight: 0, max_weight: 5, price: 60 })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates/r1/slabs/s1')
    expect(init.method).toBe('PUT')
  })

  it('deleteSlab DELETEs the rate-card- and slab-scoped endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204, json: async () => undefined } as Response)
    vi.stubGlobal('fetch', fetchMock)

    await deleteSlab('admin-token', 'r1', 's1')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/rates/r1/slabs/s1')
    expect(init.method).toBe('DELETE')
  })

  it('deleteSlab surfaces a 404 for a slab under the wrong rate card', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'slab not found' })))

    await expect(deleteSlab('admin-token', 'wrong-card', 's1')).rejects.toBeInstanceOf(ApiError)
  })
})
