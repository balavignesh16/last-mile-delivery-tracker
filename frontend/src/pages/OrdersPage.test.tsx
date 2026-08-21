import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import type { UserProfile } from '../types/auth'
import { OrdersPage } from './OrdersPage'

vi.mock('../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function mockAuth(role: 'CUSTOMER' | 'ADMIN') {
  const user: UserProfile = {
    id: 'user-1',
    email: 'user@example.com',
    full_name: 'User',
    phone: null,
    role,
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

const order = {
  id: 'order-1',
  customer_id: 'user-1',
  order_type: 'B2C',
  payment_type: 'COD',
  zone_relationship: 'INTRA',
  final_amount: 70,
  status: 'CREATED',
}

describe('OrdersPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the empty state when there are no orders', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No orders yet.')).toBeTruthy())
    expect(screen.getByText('My orders')).toBeTruthy()
  })

  it('loads and displays the order list', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [order])))

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2C · INTRA · COD/)).toBeTruthy())
    expect(screen.getByText('₹70.00')).toBeTruthy()
  })

  it('shows "All orders" for an ADMIN caller', async () => {
    mockAuth('ADMIN')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('All orders')).toBeTruthy())
  })

  it('shows a server error banner when the order list fails to load', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(500, { error: 'could not list orders' })))

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('could not list orders')).toBeTruthy())
  })
})
