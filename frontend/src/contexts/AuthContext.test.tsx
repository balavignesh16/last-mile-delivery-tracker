import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../hooks/useAuth'
import { AuthProvider } from './AuthContext'

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

// A minimal consumer that renders the pieces of auth state these tests
// need to observe, and exposes the action functions as buttons — avoids
// needing a real login/register form just to exercise the context.
function Consumer() {
  const { status, user, login, logout } = useAuth()
  return (
    <div>
      <div data-testid="status">{status}</div>
      <div data-testid="email">{user?.email ?? ''}</div>
      <button onClick={() => void login('person@example.com', 'password123')}>login</button>
      <button onClick={logout}>logout</button>
    </div>
  )
}

describe('AuthProvider', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    sessionStorage.clear()
  })

  it('starts unauthenticated when no token is stored', async () => {
    vi.stubGlobal('fetch', vi.fn())
    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )

    await waitFor(() => expect(screen.getByTestId('status').textContent).toBe('unauthenticated'))
  })

  it('login stores a token and loads the current user', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes('/auth/login')) {
        return jsonResponse(200, { token: 'fake-jwt-token' })
      }
      if (url.includes('/users/me')) {
        return jsonResponse(200, {
          id: 'u1',
          email: 'person@example.com',
          full_name: 'Person',
          phone: null,
          role: 'CUSTOMER',
          created_at: '2026-01-01T00:00:00Z',
        })
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('status').textContent).toBe('unauthenticated'))

    await act(async () => {
      screen.getByText('login').click()
    })

    await waitFor(() => expect(screen.getByTestId('status').textContent).toBe('authenticated'))
    expect(screen.getByTestId('email').textContent).toBe('person@example.com')
    expect(sessionStorage.getItem('lmt_auth_token')).toBe('fake-jwt-token')
  })

  it('logout clears authentication state and storage', async () => {
    sessionStorage.setItem('lmt_auth_token', 'existing-token')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(200, {
          id: 'u1',
          email: 'already@example.com',
          full_name: 'Already',
          phone: null,
          role: 'CUSTOMER',
          created_at: '2026-01-01T00:00:00Z',
        }),
      ),
    )

    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('status').textContent).toBe('authenticated'))

    act(() => {
      screen.getByText('logout').click()
    })

    expect(screen.getByTestId('status').textContent).toBe('unauthenticated')
    expect(screen.getByTestId('email').textContent).toBe('')
    expect(sessionStorage.getItem('lmt_auth_token')).toBeNull()
  })

  it('clears state when a stored token is rejected by the backend', async () => {
    sessionStorage.setItem('lmt_auth_token', 'stale-token')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(401, { error: 'invalid or expired token' })))

    render(
      <AuthProvider>
        <Consumer />
      </AuthProvider>,
    )

    await waitFor(() => expect(screen.getByTestId('status').textContent).toBe('unauthenticated'))
    expect(sessionStorage.getItem('lmt_auth_token')).toBeNull()
  })
})
