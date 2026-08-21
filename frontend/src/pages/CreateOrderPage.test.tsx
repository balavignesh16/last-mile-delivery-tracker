import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import type { UserProfile } from '../types/auth'
import { CreateOrderPage } from './CreateOrderPage'

vi.mock('../hooks/useAuth')

const navigateMock = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => navigateMock }
})

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function mockAuth(role: 'CUSTOMER' | 'ADMIN') {
  const user: UserProfile = {
    id: `${role.toLowerCase()}-1`,
    email: `${role.toLowerCase()}@example.com`,
    full_name: role,
    phone: null,
    role,
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: `${role.toLowerCase()}-token`,
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  } as AuthContextValue)
}

const zoneA = { id: 'zone-a', name: 'Zone A', active: true, created_at: '2026-01-01T00:00:00Z' }
const areaA1 = { id: 'area-a1', name: 'Area A1', zone_id: 'zone-a', created_at: '2026-01-01T00:00:00Z' }

const quoteResult = {
  pickup_area_id: 'area-a1',
  pickup_zone_id: 'zone-a',
  drop_area_id: 'area-a1',
  drop_zone_id: 'zone-a',
  zone_relationship: 'INTRA',
  order_type: 'B2C',
  payment_type: 'PREPAID',
  length_cm: 10,
  breadth_cm: 10,
  height_cm: 10,
  actual_weight_kg: 5,
  volumetric_weight_kg: 0.2,
  chargeable_weight_kg: 5,
  rate_card_id: 'card-1',
  base_rate: 60,
  cod_surcharge: 0,
  final_amount: 60,
}

const createdOrder = { id: 'order-1', status: 'CREATED' }

function fetchRouter() {
  return vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
    const url = String(input)
    if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, [zoneA]))
    if (url === '/api/v1/zones/zone-a/areas') return Promise.resolve(jsonResponse(200, [areaA1]))
    if (url === '/api/v1/orders/quote') return Promise.resolve(jsonResponse(200, quoteResult))
    if (url === '/api/v1/orders') return Promise.resolve(jsonResponse(201, createdOrder))
    return Promise.reject(new Error(`unexpected fetch: ${url}`))
  })
}

async function fillAreas() {
  await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))
  fireEvent.change(screen.getByLabelText('Zone', { selector: '#order_pickup_zone' }), { target: { value: 'zone-a' } })
  await waitFor(() => expect(screen.getByText('Area A1')).toBeTruthy())
  fireEvent.change(screen.getByLabelText('Area', { selector: '#order_pickup_area' }), { target: { value: 'area-a1' } })
  fireEvent.change(screen.getByLabelText('Zone', { selector: '#order_drop_zone' }), { target: { value: 'zone-a' } })
  await waitFor(() => expect((screen.getByLabelText('Area', { selector: '#order_drop_area' }) as HTMLSelectElement).options.length).toBeGreaterThan(1))
  fireEvent.change(screen.getByLabelText('Area', { selector: '#order_drop_area' }), { target: { value: 'area-a1' } })
  fireEvent.change(screen.getByLabelText('Length (cm)'), { target: { value: '10' } })
  fireEvent.change(screen.getByLabelText('Breadth (cm)'), { target: { value: '10' } })
  fireEvent.change(screen.getByLabelText('Height (cm)'), { target: { value: '10' } })
  fireEvent.change(screen.getByLabelText('Actual weight (kg)'), { target: { value: '5' } })
}

describe('CreateOrderPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    navigateMock.mockClear()
  })

  it('does not show a customer_id field for a CUSTOMER caller', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', fetchRouter())

    render(
      <MemoryRouter>
        <CreateOrderPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))
    expect(screen.queryByLabelText('Customer ID')).toBeNull()
  })

  it('shows a customer_id field for an ADMIN caller', async () => {
    mockAuth('ADMIN')
    vi.stubGlobal('fetch', fetchRouter())

    render(
      <MemoryRouter>
        <CreateOrderPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))
    expect(screen.getByLabelText('Customer ID')).toBeTruthy()
  })

  it('previews a quote and then confirms, navigating to the created order', async () => {
    mockAuth('CUSTOMER')
    const fetchMock = fetchRouter()
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <CreateOrderPage />
      </MemoryRouter>,
    )
    await fillAreas()

    fireEvent.click(screen.getByRole('button', { name: 'Preview quote' }))
    await waitFor(() => expect(screen.getByText('₹60.00')).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Confirm & place order' }))
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith('/orders/order-1'))

    const orderCall = fetchMock.mock.calls.find(([url]) => String(url) === '/api/v1/orders')
    expect(orderCall).toBeTruthy()
    const [, init] = orderCall as [string, RequestInit]
    const body = JSON.parse(init.body as string) as Record<string, unknown>
    expect(body.customer_id).toBeUndefined()
    expect(body.pickup_area_id).toBe('area-a1')
  })

  it('admin confirm includes the entered customer_id', async () => {
    mockAuth('ADMIN')
    const fetchMock = fetchRouter()
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <CreateOrderPage />
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))
    fireEvent.change(screen.getByLabelText('Customer ID'), { target: { value: 'customer-99' } })
    await fillAreas()

    fireEvent.click(screen.getByRole('button', { name: 'Preview quote' }))
    await waitFor(() => expect(screen.getByText('₹60.00')).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Confirm & place order' }))
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith('/orders/order-1'))

    const orderCall = fetchMock.mock.calls.find(([url]) => String(url) === '/api/v1/orders')
    const [, init] = orderCall as [string, RequestInit]
    const body = JSON.parse(init.body as string) as Record<string, unknown>
    expect(body.customer_id).toBe('customer-99')
  })

  it('shows a validation error instead of calling the quote endpoint when areas are not selected', async () => {
    mockAuth('CUSTOMER')
    const fetchMock = fetchRouter()
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <CreateOrderPage />
      </MemoryRouter>,
    )
    await waitFor(() => expect(screen.getAllByText('Zone A').length).toBeGreaterThan(0))

    fireEvent.click(screen.getByRole('button', { name: 'Preview quote' }))

    await waitFor(() => expect(screen.getByText('Select a pickup area and a drop area.')).toBeTruthy())
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/orders/quote', expect.anything())
  })

  it('shows a server error banner when order creation fails', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, [zoneA]))
        if (url === '/api/v1/zones/zone-a/areas') return Promise.resolve(jsonResponse(200, [areaA1]))
        if (url === '/api/v1/orders/quote') return Promise.resolve(jsonResponse(200, quoteResult))
        if (url === '/api/v1/orders') return Promise.resolve(jsonResponse(422, { error: 'no active rate card is configured for this order type and zone relationship' }))
        return Promise.reject(new Error(`unexpected fetch: ${url}`))
      }) as unknown as typeof fetch,
    )

    render(
      <MemoryRouter>
        <CreateOrderPage />
      </MemoryRouter>,
    )
    await fillAreas()
    fireEvent.click(screen.getByRole('button', { name: 'Preview quote' }))
    await waitFor(() => expect(screen.getByText('₹60.00')).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Confirm & place order' }))

    await waitFor(() =>
      expect(screen.getByText('no active rate card is configured for this order type and zone relationship')).toBeTruthy(),
    )
    expect(navigateMock).not.toHaveBeenCalled()
  })
})
