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

// Opens a Select trigger, waits for the given option to render (the
// trigger may start out disabled — e.g. the area picker until a zone is
// chosen — and its options load asynchronously once enabled), then
// clicks it.
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
  const trigger = screen.getByLabelText('Zone', { selector: '#pickup_zone' })
  fireEvent.click(trigger)
  await waitFor(() => expect(screen.getByRole('option', { name: 'Zone A' })).toBeTruthy())
  expect(screen.getByRole('option', { name: 'Zone B' })).toBeTruthy()
  fireEvent.keyDown(trigger, { key: 'Escape' })
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

    await waitForZonesLoaded()
  })

  it('loads areas for the selected pickup zone', async () => {
    mockCustomerAuth()
    vi.stubGlobal('fetch', fetchRouter())

    render(
      <MemoryRouter>
        <QuotePage />
      </MemoryRouter>,
    )

    await chooseOption('Zone', '#pickup_zone', 'Zone A')

    const areaTrigger = screen.getByLabelText('Area', { selector: '#pickup_area' }) as HTMLButtonElement
    await waitFor(() => expect(areaTrigger.disabled).toBe(false))
    fireEvent.click(areaTrigger)
    await waitFor(() => expect(screen.getByRole('option', { name: 'Area A1' })).toBeTruthy())
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
    await waitForZonesLoaded()

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

    await chooseOption('Zone', '#pickup_zone', 'Zone A')
    await chooseOption('Area', '#pickup_area', 'Area A1')
    await chooseOption('Zone', '#drop_zone', 'Zone B')
    await chooseOption('Area', '#drop_area', 'Area B1')

    fireEvent.change(screen.getByLabelText('Length (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Breadth (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Height (cm)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Actual weight (kg)'), { target: { value: '5' } })
    await chooseOption('Payment type', '#quote_payment_type', 'COD')

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
    await chooseOption('Zone', '#pickup_zone', 'Zone A')
    await chooseOption('Area', '#pickup_area', 'Area A1')
    await chooseOption('Zone', '#drop_zone', 'Zone B')
    await chooseOption('Area', '#drop_area', 'Area B1')

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
