import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../../contexts/auth-context'
import { useAuth } from '../../hooks/useAuth'
import type { UserProfile } from '../../types/auth'
import { AgentsPage } from './AgentsPage'

vi.mock('../../hooks/useAuth')

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as Response
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

const detailAgent = {
  id: 'a1',
  user_id: 'u1',
  full_name: 'Detail Agent',
  email: 'detail@example.com',
  phone: '+1 555-0100',
  availability: 'AVAILABLE',
  current_lat: null,
  current_lng: null,
  current_zone_id: 'zone-1',
  location_updated_at: null,
  last_assigned_at: null,
  active: true,
  created_at: '2026-01-01T00:00:00Z',
}

const zone1 = { id: 'zone-1', name: 'North Zone', active: true, created_at: '2026-01-01T00:00:00Z' }

const assignedOrder = {
  id: 'order-1',
  customer_id: 'cust-1',
  order_type: 'B2C',
  payment_type: 'COD',
  zone_relationship: 'INTRA',
  final_amount: 55,
  status: 'ASSIGNED',
  created_at: '2026-01-02T00:00:00Z',
}

// Round 6: the resource table renders both a desktop <table> and a
// CSS-only ("sm:hidden"/"hidden sm:block") mobile card list for the
// same rows — real browsers show only one via the media query, but
// jsdom has no viewport, so both stay in the DOM during a test. Row
// selection here is scoped to the <table> to get a single, real match.
function clickAgentRow(name: string) {
  fireEvent.click(within(screen.getByRole('table')).getByText(name))
}

function detailFetchRouter() {
  return vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, [detailAgent]))
    if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, [zone1]))
    if (url === '/api/v1/orders?agent=a1') return Promise.resolve(jsonResponse(200, [assignedOrder]))
    if (url === '/api/v1/agents/a1/active') return Promise.resolve(jsonResponse(200, { ...detailAgent, active: false }))
    return Promise.reject(new Error(`unexpected fetch: ${url}`))
  })
}

describe('AgentsPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows agent detail, resolved zone name, and orders on selection', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', detailFetchRouter())

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickAgentRow('Detail Agent')

    await waitFor(() => expect(screen.getByText('North Zone')).toBeTruthy())
    // "detail@example.com" appears both in the list row and the detail
    // pane — assert presence, not a single unique match.
    expect(screen.getAllByText('detail@example.com').length).toBeGreaterThan(0)
    expect(screen.getByText('+1 555-0100')).toBeTruthy()

    await waitFor(() => expect(screen.getByText(/B2C · order-1/)).toBeTruthy())
    expect(screen.getByText('₹55.00')).toBeTruthy()
  })

  it('deactivates and reflects the new state in both the list and detail pane', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', detailFetchRouter())

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickAgentRow('Detail Agent')

    await waitFor(() => expect(screen.getByRole('button', { name: 'Deactivate' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Deactivate' }))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Activate' })).toBeTruthy())
    expect(screen.getAllByText('Inactive').length).toBeGreaterThan(0)
  })

  it('shows the empty-orders state when the agent has no assigned orders', async () => {
    mockAdminAuth()
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, [detailAgent]))
        if (url === '/api/v1/zones') return Promise.resolve(jsonResponse(200, [zone1]))
        if (url === '/api/v1/orders?agent=a1') return Promise.resolve(jsonResponse(200, []))
        return Promise.reject(new Error(`unexpected fetch: ${url}`))
      }),
    )

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickAgentRow('Detail Agent')

    await waitFor(() => expect(screen.getByText('No orders currently or most-recently assigned to this agent.')).toBeTruthy())
  })

  it('loads and displays the agent list', async () => {
    mockAdminAuth()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(200, [
          {
            id: 'a1',
            user_id: 'u1',
            full_name: 'Existing Agent',
            email: 'existing@example.com',
            phone: null,
            availability: 'AVAILABLE',
            current_lat: null,
            current_lng: null,
            current_zone_id: null,
            location_updated_at: null,
            last_assigned_at: null,
            active: true,
            created_at: '2026-01-01T00:00:00Z',
          },
        ]),
      ),
    )

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    const table = screen.getByRole('table')
    expect(within(table).getByText('Existing Agent')).toBeTruthy()
    expect(within(table).getByText('existing@example.com')).toBeTruthy()
  })

  it('search filters the agent list by name or email, client-side', async () => {
    mockAdminAuth()
    const agentA = {
      id: 'a1', user_id: 'u1', full_name: 'Alice Agent', email: 'alice@example.com', phone: null,
      availability: 'AVAILABLE', current_lat: null, current_lng: null, current_zone_id: null,
      location_updated_at: null, last_assigned_at: null, active: true, created_at: '2026-01-01T00:00:00Z',
    }
    const agentB = {
      id: 'a2', user_id: 'u2', full_name: 'Bob Agent', email: 'bob@example.com', phone: null,
      availability: 'AVAILABLE', current_lat: null, current_lng: null, current_zone_id: null,
      location_updated_at: null, last_assigned_at: null, active: true, created_at: '2026-01-01T00:00:00Z',
    }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [agentA, agentB])))

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    expect(within(screen.getByRole('table')).getByText('Alice Agent')).toBeTruthy()
    expect(within(screen.getByRole('table')).getByText('Bob Agent')).toBeTruthy()

    fireEvent.change(screen.getByLabelText('Search agents'), { target: { value: 'alice' } })

    await waitFor(() => expect(screen.queryByText('Bob Agent')).toBeNull())
    expect(within(screen.getByRole('table')).getByText('Alice Agent')).toBeTruthy()
  })

  it('creates an agent and adds it to the list', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return jsonResponse(201, {
          id: 'a2',
          user_id: 'u2',
          full_name: 'New Agent',
          email: 'new@example.com',
          phone: null,
          availability: 'OFFLINE',
          current_lat: null,
          current_lng: null,
          current_zone_id: null,
          location_updated_at: null,
          last_assigned_at: null,
          active: true,
          created_at: '2026-01-01T00:00:00Z',
        })
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No delivery agents yet')).toBeTruthy())

    // Round 6: creation moved into a modal — the page's own trigger
    // button is unambiguous before the modal opens (the submit button
    // with the same "Create agent" text doesn't exist yet), so it's
    // opened first, then the form fields/submit are scoped to the
    // dialog to disambiguate from the still-present trigger button.
    fireEvent.click(screen.getByRole('button', { name: 'Create agent' }))
    const dialog = screen.getByRole('dialog')

    fireEvent.change(within(dialog).getByLabelText('Full name'), { target: { value: 'New Agent' } })
    fireEvent.change(within(dialog).getByLabelText('Email'), { target: { value: 'new@example.com' } })
    fireEvent.change(within(dialog).getByLabelText('Temporary password'), { target: { value: 'password123' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(within(screen.getByRole('table')).getByText('New Agent')).toBeTruthy())
  })

  it('shows the backend error when creation fails', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return jsonResponse(409, { error: 'an account with this email already exists' })
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No delivery agents yet')).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Create agent' }))
    const dialog = screen.getByRole('dialog')

    fireEvent.change(within(dialog).getByLabelText('Full name'), { target: { value: 'Dup Agent' } })
    fireEvent.change(within(dialog).getByLabelText('Email'), { target: { value: 'dup@example.com' } })
    fireEvent.change(within(dialog).getByLabelText('Temporary password'), { target: { value: 'password123' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(screen.getByText('an account with this email already exists')).toBeTruthy())
  })
})
