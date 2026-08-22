import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

function mockAuth(role: 'CUSTOMER' | 'ADMIN' | 'DELIVERY_AGENT') {
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

  it('shows "My assigned orders" and no "New order" link for a DELIVERY_AGENT caller', async () => {
    mockAuth('DELIVERY_AGENT')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [order])))

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('My assigned orders')).toBeTruthy())
    expect(screen.queryByText('New order')).toBeNull()
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

  // --- M12: admin filters ---

  it('does not show filter controls to a CUSTOMER or DELIVERY_AGENT', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No orders yet.')).toBeTruthy())
    expect(screen.queryByLabelText('Status')).toBeNull()
    expect(screen.queryByLabelText('Zone')).toBeNull()
    expect(screen.queryByLabelText('Agent')).toBeNull()
  })

  it('shows filter controls to an ADMIN and requests the selected status filter', async () => {
    mockAuth('ADMIN')
    const zones = [{ id: 'zone-1', name: 'Zone One', active: true, created_at: '2026-01-01T00:00:00Z' }]
    const agentsList = [
      { id: 'agent-1', user_id: 'u-1', full_name: 'Alice Agent', email: 'a@example.com', phone: null, availability: 'AVAILABLE', current_lat: null, current_lng: null, current_zone_id: null, location_updated_at: null, last_assigned_at: null, active: true, created_at: '2026-01-01T00:00:00Z' },
    ]
    const failedOrder = { ...order, id: 'order-2', status: 'FAILED' }

    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, zones))
      if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, agentsList))
      if (url === '/api/v1/orders?status=FAILED') return Promise.resolve(jsonResponse(200, [failedOrder]))
      if (url === '/api/v1/orders') return Promise.resolve(jsonResponse(200, [order, failedOrder]))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByLabelText('Status')).toBeTruthy())
    await waitFor(() => expect(screen.getByRole('option', { name: 'Zone One' })).toBeTruthy())
    await waitFor(() => expect(screen.getByRole('option', { name: 'Alice Agent' })).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'FAILED' } })

    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url) === '/api/v1/orders?status=FAILED')).toBe(true))
  })

  it('combines status, zone, and agent filters into one query string', async () => {
    mockAuth('ADMIN')
    const zones = [{ id: 'zone-1', name: 'Zone One', active: true, created_at: '2026-01-01T00:00:00Z' }]
    const agentsList = [
      { id: 'agent-1', user_id: 'u-1', full_name: 'Alice Agent', email: 'a@example.com', phone: null, availability: 'AVAILABLE', current_lat: null, current_lng: null, current_zone_id: null, location_updated_at: null, last_assigned_at: null, active: true, created_at: '2026-01-01T00:00:00Z' },
    ]
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, zones))
      if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, agentsList))
      return Promise.resolve(jsonResponse(200, []))
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('option', { name: 'Zone One' })).toBeTruthy())
    await waitFor(() => expect(screen.getByRole('option', { name: 'Alice Agent' })).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'FAILED' } })
    fireEvent.change(screen.getByLabelText('Zone'), { target: { value: 'zone-1' } })
    fireEvent.change(screen.getByLabelText('Agent'), { target: { value: 'agent-1' } })

    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([url]) => String(url) === '/api/v1/orders?status=FAILED&zone=zone-1&agent=agent-1')).toBe(true),
    )
  })

  it('clearing filters restores the full, unfiltered admin list', async () => {
    mockAuth('ADMIN')
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, []))
      if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, []))
      return Promise.resolve(jsonResponse(200, [order]))
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <OrdersPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByLabelText('Status')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'FAILED' } })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Clear filters' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))

    await waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url) === '/api/v1/orders').length).toBeGreaterThan(0))
    expect(screen.queryByRole('button', { name: 'Clear filters' })).toBeNull()
  })
})
