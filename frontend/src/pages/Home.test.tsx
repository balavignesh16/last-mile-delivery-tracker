import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../hooks/useAuth'
import { Home } from './Home'

vi.mock('../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

describe('Home', () => {
  beforeEach(() => {
    vi.mocked(useAuth).mockReturnValue({
      status: 'unauthenticated',
      token: null,
      user: null,
      login: vi.fn(),
      register: vi.fn(),
      updateProfile: vi.fn(),
      logout: vi.fn(),
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the product landing page with Sign In and Create Account actions', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, { status: 'ok', database: 'ok' })))

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>,
    )

    expect(screen.getByRole('heading', { name: 'Last-Mile Delivery Tracker' })).toBeTruthy()
    // "Sign In"/"Create Account" appear both in the hero and in Layout's
    // header nav for an unauthenticated visitor — assert presence.
    expect(screen.getAllByRole('link', { name: /sign in/i }).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: /create account/i }).length).toBeGreaterThan(0)

    await waitFor(() => expect(screen.getByText('System operational')).toBeTruthy())
  })

  it('shows a degraded system indicator when the health check fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(500, { error: 'unreachable' })))

    render(
      <MemoryRouter>
        <Home />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('System status unavailable')).toBeTruthy())
  })
})
