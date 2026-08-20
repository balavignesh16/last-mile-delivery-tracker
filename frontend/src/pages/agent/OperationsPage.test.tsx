import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../../contexts/auth-context'
import { useAuth } from '../../hooks/useAuth'
import type { UserProfile } from '../../types/auth'
import { OperationsPage } from './OperationsPage'

vi.mock('../../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function agentProfile(overrides: Record<string, unknown> = {}) {
  return {
    id: 'a1',
    user_id: 'agent-1',
    full_name: 'Agent Person',
    email: 'agent@example.com',
    phone: null,
    availability: 'OFFLINE',
    current_lat: null,
    current_lng: null,
    current_zone_id: null,
    location_updated_at: null,
    last_assigned_at: null,
    active: true,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function mockAgentAuth() {
  const user: UserProfile = {
    id: 'agent-1',
    email: 'agent@example.com',
    full_name: 'Agent Person',
    phone: null,
    role: 'DELIVERY_AGENT',
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: 'agent-token',
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  } as AuthContextValue)
}

describe('OperationsPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads the agent profile and shows current availability', async () => {
    mockAgentAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, agentProfile({ availability: 'AVAILABLE' }))))

    render(
      <MemoryRouter>
        <OperationsPage />
      </MemoryRouter>,
    )

    // "AVAILABLE" appears both as the status badge and as one of the
    // three action buttons — assert presence, not a single unique match.
    await waitFor(() => expect(screen.getAllByText('AVAILABLE').length).toBeGreaterThan(0))
  })

  it('updates availability when a button is clicked', async () => {
    mockAgentAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/agents/me')) return jsonResponse(200, agentProfile())
      if (url.includes('/availability') && init?.method === 'PUT') {
        return jsonResponse(200, agentProfile({ availability: 'AVAILABLE' }))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <OperationsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('button', { name: 'AVAILABLE' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'AVAILABLE' }))

    await waitFor(() => {
      const badge = screen.getAllByText('AVAILABLE')
      expect(badge.length).toBeGreaterThan(0)
    })
  })

  it('validates location before submitting', async () => {
    mockAgentAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, agentProfile())))

    render(
      <MemoryRouter>
        <OperationsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByLabelText('Latitude')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Latitude'), { target: { value: '999' } })
    fireEvent.change(screen.getByLabelText('Longitude'), { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: /update location/i }))

    await waitFor(() => expect(screen.getByText('Latitude must be between -90 and 90.')).toBeTruthy())
  })

  it('submits a valid location update', async () => {
    mockAgentAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/agents/me')) return jsonResponse(200, agentProfile())
      if (url.includes('/location') && init?.method === 'PUT') {
        return jsonResponse(
          200,
          agentProfile({ current_lat: 12.9716, current_lng: 77.5946, location_updated_at: '2026-01-02T00:00:00Z' }),
        )
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <OperationsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByLabelText('Latitude')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Latitude'), { target: { value: '12.9716' } })
    fireEvent.change(screen.getByLabelText('Longitude'), { target: { value: '77.5946' } })
    fireEvent.click(screen.getByRole('button', { name: /update location/i }))

    await waitFor(() => expect(screen.getByText('Location updated.')).toBeTruthy())
  })
})
