import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiGet, ApiError } from './api'

describe('apiGet', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns parsed JSON on a successful response', async () => {
    const mockResponse = {
      ok: true,
      json: async () => ({ status: 'ok', database: 'ok' }),
    } as Response
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse))

    const result = await apiGet<{ status: string; database: string }>('/health')

    expect(result).toEqual({ status: 'ok', database: 'ok' })
  })

  it('throws an ApiError carrying the status code on a non-2xx response', async () => {
    const mockResponse = { ok: false, status: 503 } as Response
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse))

    await expect(apiGet('/health')).rejects.toBeInstanceOf(ApiError)
    await expect(apiGet('/health')).rejects.toMatchObject({ status: 503 })
  })

  it('throws a friendly ApiError when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(apiGet('/health')).rejects.toThrow(/could not reach the backend/i)
  })
})
