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

function mockAuth() {
  const user: UserProfile = {
    id: 'customer-1',
    email: 'customer@example.com',
    full_name: 'Cara Customer',
    phone: null,
    role: 'CUSTOMER',
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

describe('customer DashboardPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders a personalized greeting and links to Create order and My orders', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.getByText('Welcome, Cara Customer')).toBeTruthy()
    const createLink = screen.getByRole('link', { name: /Create order/ })
    const ordersLink = screen.getByRole('link', { name: /My orders/ })
    expect(createLink.getAttribute('href')).toBe('/orders/new')
    expect(ordersLink.getAttribute('href')).toBe('/orders')
    await waitFor(() => expect(screen.getByText('Active orders')).toBeTruthy())
  })

  it('shows no admin- or agent-only content', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.queryByText('Agents')).toBeNull()
    expect(screen.queryByText('Rate cards')).toBeNull()
    expect(screen.queryByText('Availability & location')).toBeNull()
    expect(screen.queryByText('Order statistics')).toBeNull()
    await waitFor(() => expect(screen.getByText('Active orders')).toBeTruthy())
  })

  it('computes active/delivered counts from real order data', async () => {
    mockAuth()
    const orders = [
      { id: 'o-1', status: 'DELIVERED' },
      { id: 'o-2', status: 'DELIVERED' },
      { id: 'o-3', status: 'IN_TRANSIT' },
    ]
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, orders)))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('Active orders')).toBeTruthy())
    const activeCount = screen.getByText('Active orders').nextElementSibling
    expect(activeCount?.textContent).toBe('1')
    const deliveredCount = screen.getByText('Delivered').nextElementSibling
    expect(deliveredCount?.textContent).toBe('2')
  })

  it('shows an error banner when the order summary fails to load', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(500, { error: 'could not list orders' })))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('could not list orders')).toBeTruthy())
  })
})
