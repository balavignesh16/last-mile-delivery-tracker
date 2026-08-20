import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { fetchCurrentUser, loginUser, registerUser } from './auth'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('auth service', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('registerUser posts to /api/v1/auth/register and returns the profile', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(201, { id: '1', email: 'a@b.com', full_name: 'A', phone: null, role: 'CUSTOMER', created_at: '2026-01-01T00:00:00Z' }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await registerUser({ email: 'a@b.com', password: 'password123', full_name: 'A' })

    expect(result.role).toBe('CUSTOMER')
    const [path] = fetchMock.mock.calls[0] as [string]
    expect(path).toBe('/api/v1/auth/register')
  })

  it('registerUser surfaces a conflict error on duplicate email', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(409, { error: 'an account with this email already exists' })))

    await expect(registerUser({ email: 'a@b.com', password: 'password123', full_name: 'A' })).rejects.toBeInstanceOf(ApiError)
  })

  it('loginUser posts to /api/v1/auth/login and returns a token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { token: 'jwt-token' }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await loginUser({ email: 'a@b.com', password: 'password123' })

    expect(result.token).toBe('jwt-token')
    const [path] = fetchMock.mock.calls[0] as [string]
    expect(path).toBe('/api/v1/auth/login')
  })

  it('loginUser surfaces invalid-credentials error on 401', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(401, { error: 'invalid email or password' })))

    await expect(loginUser({ email: 'a@b.com', password: 'wrong' })).rejects.toMatchObject({
      status: 401,
      message: 'invalid email or password',
    })
  })

  it('fetchCurrentUser sends the token as a Bearer header', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(200, { id: '1', email: 'a@b.com', full_name: 'A', phone: null, role: 'CUSTOMER', created_at: '2026-01-01T00:00:00Z' }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await fetchCurrentUser('my-token')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/v1/users/me')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer my-token')
  })

  it('fetchCurrentUser rejects with a 401 ApiError when the token is invalid', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(401, { error: 'invalid or expired token' })))

    await expect(fetchCurrentUser('bad-token')).rejects.toMatchObject({ status: 401 })
  })
})
