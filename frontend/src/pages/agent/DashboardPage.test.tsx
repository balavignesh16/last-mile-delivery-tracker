import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../../contexts/auth-context'
import { useAuth } from '../../hooks/useAuth'
import type { UserProfile } from '../../types/auth'
import { DashboardPage } from './DashboardPage'

vi.mock('../../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function agentProfile(overrides: Record<string, unknown> = {}) {
  return {
    id: 'a1',
    user_id: 'agent-1',
    full_name: 'Alex Agent',
    email: 'agent@example.com',
    phone: null,
    availability: 'AVAILABLE',
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

function mockAuth() {
  const user: UserProfile = {
    id: 'agent-1',
    email: 'agent@example.com',
    full_name: 'Alex Agent',
    phone: null,
    role: 'DELIVERY_AGENT',
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: 'token',
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  } as AuthContextValue)
}

describe('agent DashboardPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders a personalized greeting and links to assigned deliveries and availability/location', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, agentProfile())))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.getByText('Welcome, Alex Agent')).toBeTruthy()
    const deliveriesLink = screen.getByRole('link', { name: /Assigned deliveries/ })
    const operationsLink = screen.getByRole('link', { name: /Availability & location/ })
    expect(deliveriesLink.getAttribute('href')).toBe('/orders')
    expect(operationsLink.getAttribute('href')).toBe('/agent')
    await waitFor(() => expect(screen.getByText('AVAILABLE')).toBeTruthy())
  })

  it('shows no customer- or admin-only dashboard content', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, agentProfile())))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.queryByText('Create order')).toBeNull()
    expect(screen.queryByText('Agents')).toBeNull()
    expect(screen.queryByText('Rate cards')).toBeNull()
    expect(screen.queryByText('Order statistics')).toBeNull()
    await waitFor(() => expect(screen.getByText('AVAILABLE')).toBeTruthy())
  })

  it("shows the agent's current zone status", async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, agentProfile({ current_zone_id: 'zone-1' }))))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('Set')).toBeTruthy())
  })
})
