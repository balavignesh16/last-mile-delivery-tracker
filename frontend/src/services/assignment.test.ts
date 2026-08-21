import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { assignOrder, autoAssignOrder } from './assignment'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('assignment service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('assignOrder posts to /api/v1/orders/{id}/assign with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'order-1', status: 'ASSIGNED', assigned_agent_id: 'agent-1' }))
    vi.stubGlobal('fetch', fetchMock)

    await assignOrder('admin-token', 'order-1', { agent_id: 'agent-1' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/order-1/assign')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ agent_id: 'agent-1' }))
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer admin-token')
  })

  it('assignOrder surfaces a 409 for an ineligible agent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'delivery agent is not eligible for assignment' })))

    await expect(assignOrder('admin-token', 'order-1', { agent_id: 'agent-1' })).rejects.toBeInstanceOf(ApiError)
    await expect(assignOrder('admin-token', 'order-1', { agent_id: 'agent-1' })).rejects.toMatchObject({ status: 409 })
  })

  it('assignOrder surfaces a 404 for an unknown agent or order', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'delivery agent not found' })))

    await expect(assignOrder('admin-token', 'order-1', { agent_id: 'missing' })).rejects.toMatchObject({ status: 404 })
  })

  it('assignOrder surfaces a 403 for a non-admin caller', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'insufficient role for this operation' })))

    await expect(assignOrder('customer-token', 'order-1', { agent_id: 'agent-1' })).rejects.toMatchObject({ status: 403 })
  })

  it('autoAssignOrder posts to /api/v1/orders/{id}/auto-assign with an empty body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'order-1', status: 'ASSIGNED', assigned_agent_id: 'agent-2' }))
    vi.stubGlobal('fetch', fetchMock)

    await autoAssignOrder('admin-token', 'order-1')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/orders/order-1/auto-assign')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({}))
  })

  it('autoAssignOrder surfaces a 409 when no eligible agent exists', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'no eligible delivery agent is available for assignment' })))

    await expect(autoAssignOrder('admin-token', 'order-1')).rejects.toMatchObject({ status: 409 })
  })
})
