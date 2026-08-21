import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { createOrder, getOrder, listOrders } from './orders'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

const customerInput = {
  pickup_area_id: 'area-1',
  drop_area_id: 'area-2',
  order_type: 'B2C' as const,
  payment_type: 'COD' as const,
  length_cm: 10,
  breadth_cm: 10,
  height_cm: 10,
  actual_weight_kg: 5,
}

describe('orders service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('createOrder posts to /api/v1/orders with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'o1', status: 'CREATED' }))
    vi.stubGlobal('fetch', fetchMock)

    await createOrder('customer-token', customerInput)

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify(customerInput))
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer customer-token')
  })

  it('createOrder with an admin payload includes customer_id', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'o1' }))
    vi.stubGlobal('fetch', fetchMock)

    const adminInput = { ...customerInput, customer_id: 'customer-42' }
    await createOrder('admin-token', adminInput)

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(init.body).toBe(JSON.stringify(adminInput))
  })

  it('createOrder surfaces a 422 for privilege-escalation attempts', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(422, { error: 'invalid request body: json: unknown field "customer_id"' })))

    await expect(createOrder('customer-token', customerInput)).rejects.toBeInstanceOf(ApiError)
    await expect(createOrder('customer-token', customerInput)).rejects.toMatchObject({ status: 422 })
  })

  it('listOrders fetches /api/v1/orders', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await listOrders('customer-token')

    const [path] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders')
  })

  it('getOrder fetches /api/v1/orders/{id}', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'o1' }))
    vi.stubGlobal('fetch', fetchMock)

    await getOrder('customer-token', 'o1')

    const [path] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/o1')
  })

  it('getOrder surfaces a 404 for another customer\'s order', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'order not found' })))

    await expect(getOrder('customer-token', 'not-mine')).rejects.toMatchObject({ status: 404 })
  })
})
