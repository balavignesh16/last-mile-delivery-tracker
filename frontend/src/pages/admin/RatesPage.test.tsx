import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../../contexts/auth-context'
import { useAuth } from '../../hooks/useAuth'
import type { UserProfile } from '../../types/auth'
import { RatesPage } from './RatesPage'

vi.mock('../../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
}

function noContentResponse(): Response {
  return { ok: true, status: 204, json: async () => undefined } as Response
}

function mockAdminAuth() {
  const user: UserProfile = {
    id: 'admin-1',
    email: 'admin@example.com',
    full_name: 'Admin',
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

const existingCard = {
  id: 'r1',
  order_type: 'B2B' as const,
  zone_relationship: 'INTRA' as const,
  cod_surcharge: 15,
  active: true,
  created_at: '2026-01-01T00:00:00Z',
}

describe('RatesPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the empty state when there are no rate cards', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No rate cards yet.')).toBeTruthy())
  })

  it('loads and displays the rate card list', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [existingCard])))

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    expect(screen.getAllByText('Active').length).toBeGreaterThan(0)
  })

  it('shows a server error banner when the rate card list fails to load', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(500, { error: 'could not list rate cards' })))

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('could not list rate cards')).toBeTruthy())
  })

  it('creates a rate card and adds it to the list', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (init?.method === 'POST' && url === '/api/v1/rates') {
        return jsonResponse(201, {
          id: 'r2',
          order_type: 'B2C',
          zone_relationship: 'INTER',
          cod_surcharge: 0,
          active: false,
          created_at: '2026-01-01T00:00:00Z',
        })
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No rate cards yet.')).toBeTruthy())

    fireEvent.click(screen.getByLabelText('Order type'))
    fireEvent.click(screen.getByRole('option', { name: 'B2C' }))
    fireEvent.click(screen.getByLabelText('Zone relationship'))
    fireEvent.click(screen.getByRole('option', { name: 'INTER' }))
    fireEvent.click(screen.getByRole('button', { name: /create rate card/i }))

    await waitFor(() => expect(screen.getByText(/B2C · INTER/)).toBeTruthy())
  })

  it('shows the backend error when rate card creation fails', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (init?.method === 'POST' && url === '/api/v1/rates') {
        return jsonResponse(500, { error: 'could not create rate card' })
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No rate cards yet.')).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: /create rate card/i }))

    await waitFor(() => expect(screen.getByText('could not create rate card')).toBeTruthy())
  })

  it('selecting a rate card loads and displays its slabs', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/rates') return jsonResponse(200, [existingCard])
      if (url === '/api/v1/rates/r1/slabs') {
        return jsonResponse(200, [{ id: 's1', rate_card_id: 'r1', min_weight: 0, max_weight: 5, price: 50, created_at: '2026-01-01T00:00:00Z' }])
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))

    await waitFor(() => expect(screen.getByText(/0 kg – 5 kg/)).toBeTruthy())
  })

  it('shows the empty state for a rate card with no slabs yet', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/rates') return jsonResponse(200, [existingCard])
      if (url === '/api/v1/rates/r1/slabs') return jsonResponse(200, [])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))

    await waitFor(() => expect(screen.getByText('No slabs configured yet.')).toBeTruthy())
  })

  it('creates a slab under the selected rate card', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/rates') return jsonResponse(200, [existingCard])
      if (url === '/api/v1/rates/r1/slabs' && init?.method === 'POST') {
        return jsonResponse(201, { id: 's2', rate_card_id: 'r1', min_weight: 5, max_weight: 10, price: 80, created_at: '2026-01-01T00:00:00Z' })
      }
      if (url === '/api/v1/rates/r1/slabs') return jsonResponse(200, [])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))
    await waitFor(() => expect(screen.getByText('No slabs configured yet.')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Min weight (kg)'), { target: { value: '5' } })
    fireEvent.change(screen.getByLabelText('Max weight (kg)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Price'), { target: { value: '80' } })
    fireEvent.click(screen.getByRole('button', { name: /add slab/i }))

    await waitFor(() => expect(screen.getByText(/5 kg – 10 kg/)).toBeTruthy())
  })

  it('creates an open-ended slab when max weight is left blank', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/rates') return jsonResponse(200, [existingCard])
      if (url === '/api/v1/rates/r1/slabs' && init?.method === 'POST') {
        const body = JSON.parse(String(init.body)) as { max_weight?: number }
        expect(body.max_weight).toBeUndefined()
        return jsonResponse(201, { id: 's3', rate_card_id: 'r1', min_weight: 20, max_weight: null, price: 140, created_at: '2026-01-01T00:00:00Z' })
      }
      if (url === '/api/v1/rates/r1/slabs') return jsonResponse(200, [])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))
    await waitFor(() => expect(screen.getByText('No slabs configured yet.')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Min weight (kg)'), { target: { value: '20' } })
    fireEvent.change(screen.getByLabelText('Price'), { target: { value: '140' } })
    fireEvent.click(screen.getByRole('button', { name: /add slab/i }))

    await waitFor(() => expect(screen.getByText(/20 kg – ∞/)).toBeTruthy())
  })

  it('shows the backend error when a slab overlaps an existing one', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/rates') return jsonResponse(200, [existingCard])
      if (url === '/api/v1/rates/r1/slabs' && init?.method === 'POST') {
        return jsonResponse(409, { error: 'slab weight range overlaps an existing slab in this rate card' })
      }
      if (url === '/api/v1/rates/r1/slabs') return jsonResponse(200, [])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))
    await waitFor(() => expect(screen.getByText('No slabs configured yet.')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Min weight (kg)'), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText('Max weight (kg)'), { target: { value: '10' } })
    fireEvent.change(screen.getByLabelText('Price'), { target: { value: '80' } })
    fireEvent.click(screen.getByRole('button', { name: /add slab/i }))

    await waitFor(() => expect(screen.getByText('slab weight range overlaps an existing slab in this rate card')).toBeTruthy())
  })

  it('deletes a slab', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/rates') return jsonResponse(200, [existingCard])
      if (url === '/api/v1/rates/r1/slabs/s1' && init?.method === 'DELETE') return noContentResponse()
      if (url === '/api/v1/rates/r1/slabs') {
        return jsonResponse(200, [{ id: 's1', rate_card_id: 'r1', min_weight: 0, max_weight: 5, price: 50, created_at: '2026-01-01T00:00:00Z' }])
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))
    await waitFor(() => expect(screen.getByText(/0 kg – 5 kg/)).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))

    await waitFor(() => expect(screen.getByText('No slabs configured yet.')).toBeTruthy())
  })

  it('toggles a rate card active state', async () => {
    mockAdminAuth()
    const inactiveCard = { ...existingCard, active: false }
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/rates' && !init) return jsonResponse(200, [inactiveCard])
      if (url === '/api/v1/rates/r1' && init?.method === 'PUT') {
        return jsonResponse(200, { ...inactiveCard, active: true })
      }
      if (url === '/api/v1/rates/r1/slabs') return jsonResponse(200, [])
      return jsonResponse(200, [inactiveCard])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <RatesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText(/B2B · INTRA/)).toBeTruthy())
    fireEvent.click(screen.getByText(/B2B · INTRA/))
    await waitFor(() => expect(screen.getByRole('button', { name: /activate/i })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: /activate/i }))

    await waitFor(() => expect(screen.getByRole('button', { name: /deactivate/i })).toBeTruthy())
  })
})
