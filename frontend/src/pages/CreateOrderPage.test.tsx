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

// Opens a Select trigger, waits for the given option to render (the
// trigger itself may start out disabled — e.g. the area picker until a
// zone is chosen — and its options load asynchronously once enabled),
// then clicks it.
async function chooseOption(triggerLabel: string, triggerSelector: string, optionName: string) {
  const getTrigger = () => screen.getByLabelText(triggerLabel, { selector: triggerSelector }) as HTMLButtonElement
  await waitFor(() => expect(getTrigger().disabled).toBe(false))
  fireEvent.click(getTrigger())
  await waitFor(() => expect(screen.getByRole('option', { name: optionName })).toBeTruthy())
  fireEvent.click(screen.getByRole('option', { name: optionName }))
}

// Opens and closes a Select without choosing anything — used purely to
// wait for its async options (zones) to finish loading.
async function waitForZonesLoaded() {
  const trigger = screen.getByLabelText('Zone', { selector: '#order_pickup_zone' })
  fireEvent.click(trigger)
  await waitFor(() => expect(screen.getByRole('option', { name: 'Zone A' })).toBeTruthy())
  fireEvent.keyDown(trigger, { key: 'Escape' })
}

async function fillAreas() {
  await chooseOption('Zone', '#order_pickup_zone', 'Zone A')
  await chooseOption('Area', '#order_pickup_area', 'Area A1')
  await chooseOption('Zone', '#order_drop_zone', 'Zone A')
  await chooseOption('Area', '#order_drop_area', 'Area A1')
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

    await waitForZonesLoaded()
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

    await waitForZonesLoaded()
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
    await waitForZonesLoaded()
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
    await waitForZonesLoaded()

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
