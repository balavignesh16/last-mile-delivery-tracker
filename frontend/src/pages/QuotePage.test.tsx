import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import type { UserProfile } from '../types/auth'
import { QuotePage } from './QuotePage'

vi.mock('../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function mockCustomerAuth() {
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

const zoneA = { id: 'zone-a', name: 'Zone A', active: true, created_at: '2026-01-01T00:00:00Z' }
const zoneB = { id: 'zone-b', name: 'Zone B', active: true, created_at: '2026-01-01T00:00:00Z' }
const areaA1 = { id: 'area-a1', name: 'Area A1', zone_id: 'zone-a', created_at: '2026-01-01T00:00:00Z' }
const areaB1 = { id: 'area-b1', name: 'Area B1', zone_id: 'zone-b', created_at: '2026-01-01T00:00:00Z' }

const quoteResult = {
  pickup_area_id: 'area-a1',
  pickup_zone_id: 'zone-a',
  drop_area_id: 'area-b1',
  drop_zone_id: 'zone-b',
  zone_relationship: 'INTER',
  order_type: 'B2C',
  payment_type: 'COD',
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
}

function fetchRouter(): typeof fetch {
  return vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, [zoneA, zoneB]))
    if (url === '/api/v1/zones/zone-a/areas') return Promise.resolve(jsonResponse(200, [areaA1]))
    if (url === '/api/v1/zones/zone-b/areas') return Promise.resolve(jsonResponse(200, [areaB1]))
    if (url === '/api/v1/orders/quote') return Promise.resolve(jsonResponse(200, quoteResult))
    return Promise.reject(new Error(`unexpected fetch: ${url}`))
  }) as unknown as typeof fetch
}

describe('QuotePage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads zones on mount', async () => {
    mockCustomerAuth()
    vi.stubGlobal('fetch', fetchRouter())

    render(
      <MemoryRouter>
        <QuotePage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))
    expect(screen.getAllByText('Zone B').length).toBeGreaterThan(0)
  })

  it('loads areas for the selected pickup zone', async () => {
    mockCustomerAuth()
    vi.stubGlobal('fetch', fetchRouter())

    render(
      <MemoryRouter>
        <QuotePage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))
    fireEvent.change(screen.getByLabelText('Zone', { selector: '#pickup_zone' }), { target: { value: 'zone-a' } })

    await waitFor(() => expect(screen.getByText('Area A1')).toBeTruthy())
  })

  it('shows a client-side validation error for non-positive dimensions without calling the quote endpoint', async () => {
    mockCustomerAuth()
    const fetchMock = fetchRouter()
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <QuotePage />
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))

    fireEvent.click(screen.getByRole('button', { name: 'Get quote' }))

    await waitFor(() => expect(screen.getByText('Select a pickup area and a drop area.')).toBeTruthy())
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/orders/quote', expect.anything())
  })

  it('requests and displays a full quote breakdown', async () => {
    mockCustomerAuth()
    vi.stubGlobal('fetch', fetchRouter())

    render(
      <MemoryRouter>
        <QuotePage />
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))

    fireEvent.change(screen.getByLabelText('Zone', { selector: '#pickup_zone' }), { target: { value: 'zone-a' } })
    await waitFor(() => expect(screen.getByText('Area A1')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Area', { selector: '#pickup_area' }), { target: { value: 'area-a1' } })

    fireEvent.change(screen.getByLabelText('Zone', { selector: '#drop_zone' }), { target: { value: 'zone-b' } })
    await waitFor(() => expect(screen.getByText('Area B1')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Area', { selector: '#drop_area' }), { target: { value: 'area-b1' } })

    fireEvent.change(screen.getByLabelText('Length (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Breadth (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Height (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Actual weight (kg)'), { target: { value: '5' } })
    fireEvent.change(screen.getByLabelText('Payment type'), { target: { value: 'COD' } })

    fireEvent.click(screen.getByRole('button', { name: 'Get quote' }))

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.getByText('INTER')).toBeTruthy()
    expect(screen.getByText('₹60.00')).toBeTruthy()
    expect(screen.getByText('₹15.00')).toBeTruthy()
  })

  it('shows a server error banner when the quote request fails', async () => {
    mockCustomerAuth()
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, [zoneA, zoneB]))
        if (url === '/api/v1/zones/zone-a/areas') return Promise.resolve(jsonResponse(200, [areaA1]))
        if (url === '/api/v1/zones/zone-b/areas') return Promise.resolve(jsonResponse(200, [areaB1]))
        if (url === '/api/v1/orders/quote') {
          return Promise.resolve(jsonResponse(422, { error: 'no active rate card is configured for this order type and zone relationship' }))
        }
        return Promise.reject(new Error(`unexpected fetch: ${url}`))
      }) as unknown as typeof fetch,
    )

    render(
      <MemoryRouter>
        <QuotePage />
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))

    fireEvent.change(screen.getByLabelText('Zone', { selector: '#pickup_zone' }), { target: { value: 'zone-a' } })
    await waitFor(() => expect(screen.getByText('Area A1')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Area', { selector: '#pickup_area' }), { target: { value: 'area-a1' } })

    fireEvent.change(screen.getByLabelText('Zone', { selector: '#drop_zone' }), { target: { value: 'zone-b' } })
    await waitFor(() => expect(screen.getByText('Area B1')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Area', { selector: '#drop_area' }), { target: { value: 'area-b1' } })

    fireEvent.change(screen.getByLabelText('Length (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Breadth (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Height (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Actual weight (kg)'), { target: { value: '5' } })

    fireEvent.click(screen.getByRole('button', { name: 'Get quote' }))

    await waitFor(() =>
      expect(screen.getByText('no active rate card is configured for this order type and zone relationship')).toBeTruthy(),
    )
  })
})
