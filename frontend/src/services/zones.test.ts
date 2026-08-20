import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { createArea, createZone, listAreas, listZones, updateArea, updateZone } from './zones'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('zones service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listZones fetches /api/v1/zones with a Bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await listZones('admin-token')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/zones')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer admin-token')
  })

  it('listZones surfaces a 403 for a non-admin caller', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(403, { error: 'insufficient role for this operation' })))

    await expect(listZones('customer-token')).rejects.toMatchObject({ status: 403 })
  })

  it('createZone posts to /api/v1/zones', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'z1', name: 'North', active: true }))
    vi.stubGlobal('fetch', fetchMock)

    await createZone('admin-token', { name: 'North' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/zones')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ name: 'North' }))
  })

  it('createZone surfaces a 409 on duplicate name', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'a zone with this name already exists' })))

    await expect(createZone('admin-token', { name: 'Dup' })).rejects.toBeInstanceOf(ApiError)
  })

  it('updateZone PUTs name and active to the zone endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'z1', name: 'North', active: false }))
    vi.stubGlobal('fetch', fetchMock)

    await updateZone('admin-token', 'z1', { name: 'North', active: false })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/zones/z1')
    expect(init.method).toBe('PUT')
    expect(init.body).toBe(JSON.stringify({ name: 'North', active: false }))
  })

  it('listAreas fetches areas scoped to a zone', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, []))
    vi.stubGlobal('fetch', fetchMock)

    await listAreas('admin-token', 'z1')

    const [path] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/zones/z1/areas')
  })

  it('createArea posts to the zone-scoped areas endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: 'a1', name: 'Downtown', zone_id: 'z1' }))
    vi.stubGlobal('fetch', fetchMock)

    await createArea('admin-token', 'z1', { name: 'Downtown' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/zones/z1/areas')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ name: 'Downtown' }))
  })

  it('createArea surfaces a 404 for an unknown zone', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'zone not found' })))

    await expect(createArea('admin-token', 'missing', { name: 'X' })).rejects.toMatchObject({ status: 404 })
  })

  it('updateArea PUTs to the zone- and area-scoped endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { id: 'a1', name: 'Renamed', zone_id: 'z1' }))
    vi.stubGlobal('fetch', fetchMock)

    await updateArea('admin-token', 'z1', 'a1', { name: 'Renamed' })

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/zones/z1/areas/a1')
    expect(init.method).toBe('PUT')
  })
})
