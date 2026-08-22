import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

function mockAuth(role: 'CUSTOMER' | 'ADMIN' | 'DELIVERY_AGENT' = 'CUSTOMER') {
  const user: UserProfile = {
    id: 'customer-1',
    email: 'customer@example.com',
    full_name: 'Customer',
    phone: null,
    role,
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: 'test-token',
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
  assigned_agent_id: null as string | null,
  status: 'CREATED',
  created_at: '2026-08-21T10:00:00Z',
}

const agentsList = [
  { id: 'agent-1', user_id: 'u-agent-1', full_name: 'Alice Agent', email: 'alice@example.com', phone: null, availability: 'AVAILABLE', current_lat: null, current_lng: null, current_zone_id: 'zone-a', location_updated_at: null, last_assigned_at: null, active: true, created_at: '2026-01-01T00:00:00Z' },
]

const events = [
  {
    id: 'event-1',
    order_id: 'order-1',
    previous_status: null,
    new_status: 'CREATED',
    actor_id: 'customer-1',
    metadata: null,
    created_at: '2026-08-21T10:00:00Z',
  },
]

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/orders/:id" element={<OrderDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

function fetchRouter(orderStatus = 404, orderBody: unknown = { error: 'order not found' }, eventsBody: unknown = events, orderOverride: unknown = order, reschedulesBody: unknown = []) {
  return vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, orderOverride))
    if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, eventsBody))
    if (url === '/api/v1/orders/order-1/reschedules') return Promise.resolve(jsonResponse(200, reschedulesBody))
    if (url === '/api/v1/orders/not-mine') return Promise.resolve(jsonResponse(orderStatus, orderBody))
    if (url === '/api/v1/orders/not-mine/tracking') return Promise.resolve(jsonResponse(orderStatus, orderBody))
    if (url === '/api/v1/orders/not-mine/reschedules') return Promise.resolve(jsonResponse(orderStatus, orderBody))
    if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, agentsList))
    return Promise.reject(new Error(`unexpected fetch: ${url}`))
  })
}

describe('OrderDetailPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads and displays the full order breakdown', async () => {
    mockAuth()
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.getByText('INTER')).toBeTruthy()
    expect(screen.getByText('₹60.00')).toBeTruthy()
    expect(screen.getByText('₹15.00')).toBeTruthy()
    expect(screen.getByText('CREATED', { selector: 'span' })).toBeTruthy()
  })

  it('shows a not-found message for an order that does not exist or is not owned by the caller', async () => {
    mockAuth()
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/not-mine')

    await waitFor(() => expect(screen.getAllByText('order not found').length).toBeGreaterThan(0))
  })

  it('renders the tracking timeline', async () => {
    mockAuth()
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('CREATED', { selector: 'p' })).toBeTruthy())
    expect(screen.getByText(/Actor: customer-1/)).toBeTruthy()
  })

  it('shows the empty-timeline state when there are no events', async () => {
    mockAuth()
    vi.stubGlobal('fetch', fetchRouter(200, order, []))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('No tracking events yet.')).toBeTruthy())
  })

  it('shows a tracking error banner on failure', async () => {
    mockAuth()
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, order))
        if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(500, { error: 'could not load tracking history' }))
        return Promise.reject(new Error(`unexpected fetch: ${url}`))
      }),
    )

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('could not load tracking history')).toBeTruthy())
  })

  it('does not show status-update controls to a CUSTOMER', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.queryByText('Update status')).toBeNull()
  })

  it('shows only the legal next-status buttons to an ADMIN', async () => {
    mockAuth('ADMIN')
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('Update status')).toBeTruthy())
    // order.status is CREATED -> only ASSIGNED is a legal next status.
    expect(screen.getByRole('button', { name: 'Mark as ASSIGNED' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Mark as DELIVERED/ })).toBeNull()
  })

  it('admin transition posts to the status endpoint and refreshes order + timeline', async () => {
    mockAuth('ADMIN')
    const assignedOrder = { ...order, status: 'ASSIGNED' }
    const assignedEvents = [...events, { ...events[0], id: 'event-2', previous_status: 'CREATED', new_status: 'ASSIGNED', actor_id: 'admin-1' }]

    let transitioned = false
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/status' && init?.method === 'POST') {
        transitioned = true
        return Promise.resolve(jsonResponse(201, assignedEvents[1]))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, transitioned ? assignedOrder : order))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, transitioned ? assignedEvents : events))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByRole('button', { name: 'Mark as ASSIGNED' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Mark as ASSIGNED' }))

    await waitFor(() => expect(screen.getByText('ASSIGNED', { selector: 'span' })).toBeTruthy())

    const statusCall = fetchMock.mock.calls.find(([url]) => String(url) === '/api/v1/orders/order-1/status')
    expect(statusCall).toBeTruthy()
    const [, init] = statusCall as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ status: 'ASSIGNED' })
  })

  it('shows an error banner when a transition is rejected', async () => {
    mockAuth('ADMIN')
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/status' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse(409, { error: "this status transition is not permitted from the order's current status" }))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, order))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByRole('button', { name: 'Mark as ASSIGNED' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Mark as ASSIGNED' }))

    await waitFor(() => expect(screen.getByText("this status transition is not permitted from the order's current status")).toBeTruthy())
  })

  it('shows a terminal-state message for a DELIVERED order (ADMIN view)', async () => {
    mockAuth('ADMIN')
    const deliveredOrder = { ...order, status: 'DELIVERED' }
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, deliveredOrder))
        if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
        return Promise.reject(new Error(`unexpected fetch: ${url}`))
      }),
    )

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('This order is in a terminal state — no further transitions are possible.')).toBeTruthy())
  })

  // --- M09: assignment ---

  it('shows "Unassigned" when no agent has been assigned', async () => {
    mockAuth('ADMIN')
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('Unassigned')).toBeTruthy())
  })

  it("shows the assigned agent's name once resolved via the agents list (ADMIN)", async () => {
    mockAuth('ADMIN')
    const assignedOrder = { ...order, assigned_agent_id: 'agent-1' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, assignedOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('Alice Agent')).toBeTruthy())
  })

  it('does not show assignment controls to a CUSTOMER', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.queryByText('Assign delivery agent')).toBeNull()
  })

  it('does not show assignment controls once the order is no longer in an assignable status', async () => {
    mockAuth('ADMIN')
    const deliveredOrder = { ...order, status: 'DELIVERED' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, deliveredOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('This order is in a terminal state — no further transitions are possible.')).toBeTruthy())
    expect(screen.queryByText('Assign delivery agent')).toBeNull()
  })

  it('admin manual assignment posts to the assign endpoint and refreshes the order', async () => {
    mockAuth('ADMIN')
    const assignedOrder = { ...order, status: 'ASSIGNED', assigned_agent_id: 'agent-1' }
    let assigned = false
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/assign' && init?.method === 'POST') {
        assigned = true
        return Promise.resolve(jsonResponse(200, assignedOrder))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, assigned ? assignedOrder : order))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, agentsList))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByRole('option', { name: 'Alice Agent (AVAILABLE)' })).toBeTruthy())
    fireEvent.change(screen.getByLabelText('Delivery agent'), { target: { value: 'agent-1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Assign' }))

    await waitFor(() => expect(screen.getByText('Order assigned.')).toBeTruthy())
    const assignCall = fetchMock.mock.calls.find(([url]) => String(url) === '/api/v1/orders/order-1/assign')
    expect(assignCall).toBeTruthy()
    const [, init] = assignCall as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ agent_id: 'agent-1' })
  })

  it('admin auto-assignment posts to the auto-assign endpoint and refreshes the order', async () => {
    mockAuth('ADMIN')
    const assignedOrder = { ...order, status: 'ASSIGNED', assigned_agent_id: 'agent-1' }
    let assigned = false
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/auto-assign' && init?.method === 'POST') {
        assigned = true
        return Promise.resolve(jsonResponse(200, assignedOrder))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, assigned ? assignedOrder : order))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, agentsList))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByRole('button', { name: 'Auto-assign' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Auto-assign' }))

    await waitFor(() => expect(screen.getByText('Order auto-assigned to agent agent-1.')).toBeTruthy())
  })

  it('shows an error banner when assignment fails (e.g. no eligible candidate)', async () => {
    mockAuth('ADMIN')
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/auto-assign' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse(409, { error: 'no eligible delivery agent is available for assignment' }))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, order))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      if (url === '/api/v1/agents') return Promise.resolve(jsonResponse(200, agentsList))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByRole('button', { name: 'Auto-assign' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Auto-assign' }))

    await waitFor(() => expect(screen.getByText('no eligible delivery agent is available for assignment')).toBeTruthy())
  })

  it('a DELIVERY_AGENT can view an order assigned to them, without the tracking timeline (still ADMIN/CUSTOMER-only)', async () => {
    mockAuth('DELIVERY_AGENT')
    const assignedOrder = { ...order, status: 'ASSIGNED', assigned_agent_id: 'agent-1' }
    const fetchMock = fetchRouter(404, { error: 'order not found' }, events, assignedOrder)
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.queryByText('Assign delivery agent')).toBeNull()
    expect(screen.queryByText('Tracking timeline')).toBeNull()
    expect(fetchMock.mock.calls.some(([url]) => String(url) === '/api/v1/orders/order-1/tracking')).toBe(false)
  })

  // --- M12: DELIVERY_AGENT status-update UI ---

  it('shows only the agent-authorized next-status button to a DELIVERY_AGENT on their assigned order', async () => {
    mockAuth('DELIVERY_AGENT')
    const assignedOrder = { ...order, status: 'ASSIGNED', assigned_agent_id: 'agent-1' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, assignedOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('Update status')).toBeTruthy())
    // ASSIGNED -> PICKED_UP is the only edge a DELIVERY_AGENT may
    // perform from ASSIGNED (internal/tracking/statemachine.go).
    expect(screen.getByRole('button', { name: 'Mark as PICKED_UP' })).toBeTruthy()
  })

  it('a DELIVERY_AGENT transition posts to the status endpoint and refreshes the order', async () => {
    mockAuth('DELIVERY_AGENT')
    const assignedOrder = { ...order, status: 'ASSIGNED', assigned_agent_id: 'agent-1' }
    const pickedUpOrder = { ...assignedOrder, status: 'PICKED_UP' }
    const pickedUpEvents = [...events, { ...events[0], id: 'event-2', previous_status: 'ASSIGNED', new_status: 'PICKED_UP', actor_id: 'agent-1' }]

    let transitioned = false
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/status' && init?.method === 'POST') {
        transitioned = true
        return Promise.resolve(jsonResponse(201, pickedUpEvents[1]))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, transitioned ? pickedUpOrder : assignedOrder))
      // A DELIVERY_AGENT must never call GET /orders/:id/tracking (M08:
      // ADMIN/CUSTOMER only) — asserting via rejection, not a 404
      // response, so an accidental call fails this test loudly.
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByRole('button', { name: 'Mark as PICKED_UP' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Mark as PICKED_UP' }))

    await waitFor(() => expect(screen.getByText('PICKED_UP', { selector: 'span' })).toBeTruthy())

    const statusCall = fetchMock.mock.calls.find(([url]) => String(url) === '/api/v1/orders/order-1/status')
    expect(statusCall).toBeTruthy()
    const [, init] = statusCall as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ status: 'PICKED_UP' })
    expect(fetchMock.mock.calls.some(([url]) => String(url) === '/api/v1/orders/order-1/tracking')).toBe(false)
  })

  it('shows a role-specific message (not "terminal state") to a DELIVERY_AGENT when the only legal next edge is ADMIN-only', async () => {
    mockAuth('DELIVERY_AGENT')
    // FAILED -> RESCHEDULED is a legal edge, but ADMIN-only — an agent
    // must never see a "terminal state" message (which would be
    // factually wrong) nor a button they aren't authorized to use.
    const failedOrder = { ...order, status: 'FAILED', assigned_agent_id: 'agent-1' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, failedOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('No status update is available to you for this order right now.')).toBeTruthy())
    expect(screen.queryByRole('button', { name: /Mark as/ })).toBeNull()
  })

  it('shows the terminal-state message to a DELIVERY_AGENT for a DELIVERED order', async () => {
    mockAuth('DELIVERY_AGENT')
    const deliveredOrder = { ...order, status: 'DELIVERED', assigned_agent_id: 'agent-1' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, deliveredOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('This order is in a terminal state — no further transitions are possible.')).toBeTruthy())
  })

  // --- M10: rescheduling ---

  it('shows the reschedule form to a CUSTOMER when the order is FAILED', async () => {
    mockAuth('CUSTOMER')
    const failedOrder = { ...order, status: 'FAILED' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, failedOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('Reschedule delivery')).toBeTruthy())
    expect(screen.getByLabelText('New delivery date')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Reschedule' })).toBeTruthy()
  })

  it('shows the reschedule form to an ADMIN when the order is FAILED', async () => {
    mockAuth('ADMIN')
    const failedOrder = { ...order, status: 'FAILED' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, failedOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('Reschedule delivery')).toBeTruthy())
  })

  it('does not show the reschedule form when the order is not FAILED', async () => {
    mockAuth('CUSTOMER')
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.queryByText('Reschedule delivery')).toBeNull()
  })

  it('does not show the reschedule form to a DELIVERY_AGENT', async () => {
    mockAuth('DELIVERY_AGENT')
    const failedOrder = { ...order, status: 'FAILED', assigned_agent_id: 'agent-1' }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, failedOrder))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('₹75.00')).toBeTruthy())
    expect(screen.queryByText('Reschedule delivery')).toBeNull()
  })

  it('customer reschedule submission posts the right payload and refreshes order + history', async () => {
    mockAuth('CUSTOMER')
    const failedOrder = { ...order, status: 'FAILED' }
    const rescheduledOrder = { ...order, status: 'RESCHEDULED' }
    const rescheduleRecord = {
      id: 'reschedule-1', order_id: 'order-1', requested_by: 'customer-1',
      requested_date: '2099-01-01', reason: 'Not home', created_at: '2026-08-21T12:00:00Z',
    }
    let rescheduled = false
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/reschedule' && init?.method === 'POST') {
        rescheduled = true
        return Promise.resolve(jsonResponse(200, rescheduledOrder))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, rescheduled ? rescheduledOrder : failedOrder))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      if (url === '/api/v1/orders/order-1/reschedules') return Promise.resolve(jsonResponse(200, rescheduled ? [rescheduleRecord] : []))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByLabelText('New delivery date')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('New delivery date'), { target: { value: '2099-01-01' } })
    fireEvent.change(screen.getByLabelText('Reason (optional)'), { target: { value: 'Not home' } })
    fireEvent.click(screen.getByRole('button', { name: 'Reschedule' }))

    await waitFor(() => expect(screen.getByText('Reschedule request submitted.')).toBeTruthy())

    const rescheduleCall = fetchMock.mock.calls.find(([url]) => String(url) === '/api/v1/orders/order-1/reschedule')
    expect(rescheduleCall).toBeTruthy()
    const [, init] = rescheduleCall as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ requested_date: '2099-01-01', reason: 'Not home' })

    await waitFor(() => expect(screen.getByText('New date: 2099-01-01')).toBeTruthy())
  })

  it('shows an error banner when a reschedule is rejected', async () => {
    mockAuth('CUSTOMER')
    const failedOrder = { ...order, status: 'FAILED' }
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1/reschedule' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse(409, { error: 'order is not currently FAILED' }))
      }
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, failedOrder))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      if (url === '/api/v1/orders/order-1/reschedules') return Promise.resolve(jsonResponse(200, []))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByLabelText('New delivery date')).toBeTruthy())
    fireEvent.change(screen.getByLabelText('New delivery date'), { target: { value: '2099-01-01' } })
    fireEvent.click(screen.getByRole('button', { name: 'Reschedule' }))

    await waitFor(() => expect(screen.getByText('order is not currently FAILED')).toBeTruthy())
  })

  it('shows the empty reschedule-history state when there are no reschedules', async () => {
    mockAuth('ADMIN')
    vi.stubGlobal('fetch', fetchRouter())

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('No reschedule requests yet.')).toBeTruthy())
  })

  it('renders reschedule history entries', async () => {
    mockAuth('ADMIN')
    const rescheduleRecord = {
      id: 'reschedule-1', order_id: 'order-1', requested_by: 'customer-1',
      requested_date: '2099-01-01', reason: 'Not home', created_at: '2026-08-21T12:00:00Z',
    }
    vi.stubGlobal('fetch', fetchRouter(404, { error: 'order not found' }, events, order, [rescheduleRecord]))

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('New date: 2099-01-01')).toBeTruthy())
    expect(screen.getByText('Reason: Not home')).toBeTruthy()
    expect(screen.getByText(/Requested by: customer-1/)).toBeTruthy()
  })

  it('shows a reschedule-history error banner on failure', async () => {
    mockAuth('ADMIN')
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/orders/order-1') return Promise.resolve(jsonResponse(200, order))
      if (url === '/api/v1/orders/order-1/tracking') return Promise.resolve(jsonResponse(200, events))
      if (url === '/api/v1/orders/order-1/reschedules') return Promise.resolve(jsonResponse(500, { error: 'could not load reschedule history' }))
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderAt('/orders/order-1')

    await waitFor(() => expect(screen.getByText('could not load reschedule history')).toBeTruthy())
  })
})
