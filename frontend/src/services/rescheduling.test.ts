import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { getReschedules, rescheduleOrder } from './rescheduling'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('rescheduling service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('rescheduleOrder posts to /api/v1/orders/{id}/reschedule with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'order-1', status: 'RESCHEDULED' }))
    vi.stubGlobal('fetch', fetchMock)

    await rescheduleOrder('customer-token', 'order-1', { requested_date: '2099-01-01', reason: 'Not available' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/order-1/reschedule')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ requested_date: '2099-01-01', reason: 'Not available' }))
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer customer-token')
  })

  it('rescheduleOrder surfaces a 409 for a non-FAILED order', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'order is not currently FAILED' })))

    await expect(rescheduleOrder('customer-token', 'order-1', { requested_date: '2099-01-01' })).rejects.toBeInstanceOf(ApiError)
    await expect(rescheduleOrder('customer-token', 'order-1', { requested_date: '2099-01-01' })).rejects.toMatchObject({ status: 409 })
  })

  it('rescheduleOrder surfaces a 422 for an invalid date', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(422, { error: 'requested_date must not be before today' })))

    await expect(rescheduleOrder('customer-token', 'order-1', { requested_date: '2020-01-01' })).rejects.toMatchObject({ status: 422 })
  })

  it('rescheduleOrder surfaces a 404 for another customer\'s order', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'order not found' })))

    await expect(rescheduleOrder('customer-token', 'not-mine', { requested_date: '2099-01-01' })).rejects.toMatchObject({ status: 404 })
  })

  it('rescheduleOrder surfaces a 403 for a delivery agent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'insufficient role for this operation' })))

    await expect(rescheduleOrder('agent-token', 'order-1', { requested_date: '2099-01-01' })).rejects.toMatchObject({ status: 403 })
  })

  it('getReschedules fetches /api/v1/orders/{id}/reschedules', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await getReschedules('customer-token', 'order-1')

    const [path] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/order-1/reschedules')
  })

  it('getReschedules surfaces a 404 for another customer\'s order', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'order not found' })))

    await expect(getReschedules('customer-token', 'not-mine')).rejects.toMatchObject({ status: 404 })
  })
})
