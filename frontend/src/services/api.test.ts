import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiGet, apiPost, ApiError } from './api'

function jsonResponse(status: number, body: unknown, ok = status >= 200 && status < 300): Response {
  return { ok, status, json: async () => body } as Response
}

describe('apiGet', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns parsed JSON on a successful response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, { status: 'ok', database: 'ok' })))

    const result = await apiGet<{ status: string; database: string }>('/health')

    expect(result).toEqual({ status: 'ok', database: 'ok' })
  })

  it('throws an ApiError carrying the status code on a non-2xx response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(503, {})))

    await expect(apiGet('/health')).rejects.toBeInstanceOf(ApiError)
    await expect(apiGet('/health')).rejects.toMatchObject({ status: 503 })
  })

  it('surfaces the backend error message for a 401 response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(401, { error: 'invalid or expired token' })),
    )

    await expect(apiGet('/api/v1/users/me')).rejects.toMatchObject({
      status: 401,
      message: 'invalid or expired token',
    })
  })

  it('falls back to a generic message when the error body is not JSON', async () => {
    const response = {
      ok: false,
      status: 500,
      json: async () => {
        throw new SyntaxError('not json')
      },
    } as unknown as Response
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))

    await expect(apiGet('/health')).rejects.toMatchObject({ message: 'Request failed with status 500' })
  })

  it('throws a friendly ApiError when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    await expect(apiGet('/health')).rejects.toThrow(/could not reach the backend/i)
  })

  it('attaches an Authorization header when a token is provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, {}))
    vi.stubGlobal('fetch', fetchMock)

    await apiGet('/api/v1/users/me', 'my-token')

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer my-token')
  })
})

describe('apiPost', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends a JSON body and returns the parsed response', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(201, { id: '1' }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await apiPost<{ id: string }>('/api/v1/auth/register', { email: 'a@b.com' })

    expect(result).toEqual({ id: '1' })
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/auth/register')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ email: 'a@b.com' }))
  })

  it('surfaces a 409 conflict message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(409, { error: 'an account with this email already exists' })),
    )

    await expect(apiPost('/api/v1/auth/register', {})).rejects.toMatchObject({
      status: 409,
      message: 'an account with this email already exists',
    })
  })
})
