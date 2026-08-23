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
    id: 'admin-1',
    email: 'admin@example.com',
    full_name: 'Ada Admin',
    phone: null,
    role: 'ADMIN',
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: 'admin-token',
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  } as AuthContextValue)
}

function order(status: string, id: string) {
  return {
    id,
    customer_id: 'customer-1',
    created_by: 'customer-1',
    order_type: 'B2C',
    payment_type: 'COD',
    pickup_area_id: 'area-a',
    drop_area_id: 'area-b',
    pickup_zone_id: 'zone-a',
    drop_zone_id: 'zone-b',
    zone_relationship: 'INTER',
    length_cm: 1,
    breadth_cm: 1,
    height_cm: 1,
    actual_weight_kg: 1,
    volumetric_weight_kg: 1,
    chargeable_weight_kg: 1,
    rate_card_id: 'card-1',
    base_rate: 50,
    cod_surcharge: 0,
    final_amount: 50,
    assigned_agent_id: null,
    status,
    created_at: '2026-08-21T10:00:00Z',
  }
}

describe('admin DashboardPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders links to Orders, Agents, Zones, and Rate cards', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    // Layout's own top nav also renders "Orders"/"Agents"/"Zones" links
    // for an ADMIN, so these are matched by href, not accessible name,
    // to unambiguously identify the dashboard's own card links.
    await waitFor(() => expect(screen.getByText('Order statistics')).toBeTruthy())
    const hrefs = screen.getAllByRole('link').map((link) => link.getAttribute('href'))
    expect(hrefs).toContain('/orders')
    expect(hrefs).toContain('/admin/agents')
    expect(hrefs).toContain('/admin/zones')
    expect(hrefs).toContain('/admin/rates')
  })

  it('computes order statistics from real, unfiltered order data — never hardcoded', async () => {
    mockAuth()
    const orders = [order('FAILED', 'o-1'), order('FAILED', 'o-2'), order('DELIVERED', 'o-3'), order('CREATED', 'o-4')]
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, orders)))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    // Humanized labels (Round 3) — matches the status-label convention
    // already used everywhere else in the app (OrderStatusBadge,
    // OrdersPage), rather than the raw SCREAMING_SNAKE_CASE enum value.
    await waitFor(() => expect(screen.getByText('Failed')).toBeTruthy())
    const failedCount = screen.getByText('Failed').nextElementSibling
    expect(failedCount?.textContent).toBe('2')
    const deliveredCount = screen.getByText('Delivered').nextElementSibling
    expect(deliveredCount?.textContent).toBe('1')
    const assignedCount = screen.getByText('Assigned').nextElementSibling
    expect(assignedCount?.textContent).toBe('0')
  })

  it('shows the total order count and both charts when there are real orders', async () => {
    mockAuth()
    const orders = [order('FAILED', 'o-1'), order('DELIVERED', 'o-2')]
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, orders)))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('2')).toBeTruthy())
    expect(screen.getByText('total')).toBeTruthy()
    expect(screen.getByText('Order volume, last 14 days')).toBeTruthy()
    expect(screen.getByText('Status distribution')).toBeTruthy()
  })

  it('does not show the charts when there are no orders yet', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('total')).toBeTruthy())
    expect(screen.queryByText('Order volume, last 14 days')).toBeNull()
    expect(screen.queryByText('Status distribution')).toBeNull()
  })

  it('shows an error banner when order statistics fail to load', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(500, { error: 'could not list orders' })))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('could not list orders')).toBeTruthy())
  })

  it('shows no customer- or agent-only dashboard content', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('Order statistics')).toBeTruthy())
    expect(screen.queryByText('Create order')).toBeNull()
    expect(screen.queryByText('Availability & location')).toBeNull()
  })
})
