import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import type { UserProfile } from '../types/auth'
import { OrderDetailPage } from './OrderDetailPage'

vi.mock('../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function mockAuth() {
  const user: UserProfile = {
    id: 'customer-1',
    email: 'customer@example.com',
    full_name: 'Customer',
    phone: null,
    role: 'CUSTOMER',
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: 'customer-token',
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  } as AuthContextValue)
}

const order = {
  id: 'order-1',
  customer_id: 'customer-1',
  created_by: 'customer-1',
  order_type: 'B2C',
  payment_type: 'COD',
  pickup_area_id: 'area-a',
  drop_area_id: 'area-b',
  pickup_zone_id: 'zone-a',
  drop_zone_id: 'zone-b',
  zone_relationship: 'INTER',
  length_cm: 10,
  breadth_cm: 10,
  height_cm: 10,
  actual_weight_kg: 5,
  volumetric_weight_kg: 0.2,
  chargeable_weight_kg: 5,
  rate_card_id: 'card-1',
  base_rate: 60,
  cod_surcharge: 15,
  final_amount: 75,
  status: 'CREATED',
  created_at: '2026-08-21T10:00:00Z',
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/orders/:id" element={<OrderDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('OrderDetailPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads and displays the full order breakdown', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, order)))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.getByText('INTER')).toBeTruthy()
    expect(screen.getByText('₹60.00')).toBeTruthy()
    expect(screen.getByText('₹15.00')).toBeTruthy()
    expect(screen.getByText('CREATED')).toBeTruthy()
  })

  it('shows a not-found message for an order that does not exist or is not owned by the caller', async () => {
    mockAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(404, { error: 'order not found' })))

    renderAt('/orders/not-mine')

    await waitFor(() => expect(screen.getByText('order not found')).toBeTruthy())
  })
})
