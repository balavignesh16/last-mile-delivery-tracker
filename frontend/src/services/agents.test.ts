import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { createAgent, listAgents, updateAgentAvailability, updateAgentLocation } from './agents'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('agents service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listAgents fetches /api/v1/agents with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await listAgents('admin-token')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/agents')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer admin-token')
  })

  it('listAgents surfaces a 403 for a non-admin caller', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'insufficient role for this operation' })))

    await expect(listAgents('customer-token')).rejects.toMatchObject({ status: 403 })
  })

  it('createAgent posts to /api/v1/agents', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'a1', availability: 'OFFLINE' }))
    vi.stubGlobal('fetch', fetchMock)

    await createAgent('admin-token', { email: 'a@b.com', password: 'password123', full_name: 'A' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/agents')
    expect(init.method).toBe('POST')
  })

  it('createAgent surfaces a 409 on duplicate email', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'an account with this email already exists' })))

    await expect(
      createAgent('admin-token', { email: 'dup@b.com', password: 'password123', full_name: 'A' }),
    ).rejects.toBeInstanceOf(ApiError)
  })

  it('updateAgentAvailability PUTs to the availability endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'a1', availability: 'AVAILABLE' }))
    vi.stubGlobal('fetch', fetchMock)

    await updateAgentAvailability('agent-token', 'a1', 'AVAILABLE')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/agents/a1/availability')
    expect(init.method).toBe('PUT')
    expect(init.body).toBe(JSON.stringify({ availability: 'AVAILABLE' }))
  })

  it('updateAgentLocation PUTs latitude/longitude to the location endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'a1', current_lat: 1, current_lng: 2 }))
    vi.stubGlobal('fetch', fetchMock)

    await updateAgentLocation('agent-token', 'a1', 1, 2)

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/agents/a1/location')
    expect(init.body).toBe(JSON.stringify({ latitude: 1, longitude: 2 }))
  })

  it('updateAgentLocation surfaces a 403 when updating another agent', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(403, { error: "cannot update another agent's location" })),
    )

    await expect(updateAgentLocation('agent-token', 'someone-elses-id', 1, 2)).rejects.toMatchObject({ status: 403 })
  })
})
