import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { getOrderTracking, transitionOrderStatus } from './tracking'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('tracking service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('transitionOrderStatus posts to /api/v1/orders/{id}/status with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'e1', new_status: 'ASSIGNED' }))
    vi.stubGlobal('fetch', fetchMock)

    await transitionOrderStatus('admin-token', 'order-1', { status: 'ASSIGNED' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/order-1/status')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ status: 'ASSIGNED' }))
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer admin-token')
  })

  it('transitionOrderStatus surfaces a 409 for an invalid transition', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'this status transition is not permitted from the order\'s current status' })))

    await expect(transitionOrderStatus('admin-token', 'order-1', { status: 'DELIVERED' })).rejects.toBeInstanceOf(ApiError)
    await expect(transitionOrderStatus('admin-token', 'order-1', { status: 'DELIVERED' })).rejects.toMatchObject({ status: 409 })
  })

  it('transitionOrderStatus surfaces a 403 for an unauthorized role/edge', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'this role is not authorized to perform this specific status transition' })))

    await expect(transitionOrderStatus('agent-token', 'order-1', { status: 'ASSIGNED' })).rejects.toMatchObject({ status: 403 })
  })

  it('getOrderTracking fetches /api/v1/orders/{id}/tracking', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await getOrderTracking('customer-token', 'order-1')

    const [path] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/order-1/tracking')
  })

  it('getOrderTracking surfaces a 404 for another customer\'s order', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'order not found' })))

    await expect(getOrderTracking('customer-token', 'not-mine')).rejects.toMatchObject({ status: 404 })
  })
})
