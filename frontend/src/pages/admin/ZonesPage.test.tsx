import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../../contexts/auth-context'
import { useAuth } from '../../hooks/useAuth'
import type { UserProfile } from '../../types/auth'
import { ZonesPage } from './ZonesPage'

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

const existingZone = { id: 'z1', name: 'North Zone', active: true, created_at: '2026-01-01T00:00:00Z' }

// Round 6: the resource table renders both a desktop <table> and a
// CSS-only ("sm:hidden"/"hidden sm:block") mobile card list for the
// same rows — real browsers show only one via the media query, but
// jsdom has no viewport, so both stay in the DOM during a test. Row
// selection here is scoped to the <table> to get a single, real match.
function clickZoneRow(name: string) {
  fireEvent.click(within(screen.getByRole('table')).getByText(name))
}

describe('ZonesPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the empty state when there are no zones', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [])))

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No zones yet')).toBeTruthy())
  })

  it('loads and displays the zone list', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(200, [existingZone])))

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    expect(within(screen.getByRole('table')).getByText('North Zone')).toBeTruthy()
    expect(screen.getAllByText('Active').length).toBeGreaterThan(0)
  })

  it('shows a server error banner when the zone list fails to load', async () => {
    mockAdminAuth()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(500, { error: 'could not list zones' })))

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('could not list zones')).toBeTruthy())
  })

  it('creates a zone and adds it to the list', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (init?.method === 'POST' && url === '/api/v1/zones') {
        return jsonResponse(201, { id: 'z2', name: 'New Zone', active: true, created_at: '2026-01-01T00:00:00Z' })
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No zones yet')).toBeTruthy())

    // Round 6: creation moved into a modal — the trigger button is
    // unambiguous before the modal opens, then the form/submit are
    // scoped to the dialog to disambiguate from the still-present
    // trigger button (both read "Create zone").
    fireEvent.click(screen.getByRole('button', { name: 'Create zone' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Zone name'), { target: { value: 'New Zone' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /create zone/i }))

    await waitFor(() => expect(within(screen.getByRole('table')).getByText('New Zone')).toBeTruthy())
  })

  it('shows the backend error when zone creation fails', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (init?.method === 'POST' && url === '/api/v1/zones') {
        return jsonResponse(409, { error: 'a zone with this name already exists' })
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('No zones yet')).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Create zone' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Zone name'), { target: { value: 'Dup' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /create zone/i }))

    await waitFor(() => expect(screen.getByText('a zone with this name already exists')).toBeTruthy())
  })

  it('selecting a zone loads and displays its areas', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/zones') return jsonResponse(200, [existingZone])
      if (url === '/api/v1/zones/z1/areas') {
        return jsonResponse(200, [{ id: 'a1', name: 'Downtown', zone_id: 'z1', active: true, created_at: '2026-01-01T00:00:00Z' }])
      }
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickZoneRow('North Zone')

    await waitFor(() => expect(screen.getByText('Downtown')).toBeTruthy())
  })

  it('shows the empty state for a zone with no areas yet', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/zones') return jsonResponse(200, [existingZone])
      if (url === '/api/v1/zones/z1/areas') return jsonResponse(200, [])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickZoneRow('North Zone')

    await waitFor(() => expect(screen.getByText('No areas in this zone yet.')).toBeTruthy())
  })

  it('creates an area under the selected zone', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/zones') return jsonResponse(200, [existingZone])
      if (url === '/api/v1/zones/z1/areas' && init?.method === 'POST') {
        return jsonResponse(201, { id: 'a2', name: 'Suburb', zone_id: 'z1', active: true, created_at: '2026-01-01T00:00:00Z' })
      }
      if (url === '/api/v1/zones/z1/areas') return jsonResponse(200, [])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickZoneRow('North Zone')
    await waitFor(() => expect(screen.getByText('No areas in this zone yet.')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('New area name'), { target: { value: 'Suburb' } })
    fireEvent.click(screen.getByRole('button', { name: /add area/i }))

    await waitFor(() => expect(screen.getByText('Suburb')).toBeTruthy())
  })

  it('toggles a zone active state', async () => {
    mockAdminAuth()
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/zones' && (!init || init.method === undefined)) return jsonResponse(200, [existingZone])
      if (url === '/api/v1/zones/z1' && init?.method === 'PUT') {
        return jsonResponse(200, { ...existingZone, active: false })
      }
      if (url === '/api/v1/zones/z1/areas') return jsonResponse(200, [])
      return jsonResponse(200, [existingZone])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickZoneRow('North Zone')
    await waitFor(() => expect(screen.getByRole('button', { name: /deactivate/i })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: /deactivate/i }))

    await waitFor(() => expect(screen.getByRole('button', { name: /activate/i })).toBeTruthy())
  })

  it('toggles an area active state independently of its zone', async () => {
    mockAdminAuth()
    const area = { id: 'a1', name: 'Downtown', zone_id: 'z1', active: true, created_at: '2026-01-01T00:00:00Z' }
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/zones') return jsonResponse(200, [existingZone])
      if (url === '/api/v1/zones/z1/areas/a1' && init?.method === 'PUT') {
        return jsonResponse(200, { ...area, active: false })
      }
      if (url === '/api/v1/zones/z1/areas') return jsonResponse(200, [area])
      return jsonResponse(200, [])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(
      <MemoryRouter>
        <ZonesPage />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByRole('table')).toBeTruthy())
    clickZoneRow('North Zone')
    await waitFor(() => expect(screen.getByText('Downtown')).toBeTruthy())

    // Two "Deactivate" buttons exist once an area is shown: the zone's
    // own and this area's — the area's is the second one.
    const deactivateButtons = screen.getAllByRole('button', { name: /deactivate/i })
    fireEvent.click(deactivateButtons[deactivateButtons.length - 1])

    await waitFor(() => expect(screen.getAllByText('Inactive').length).toBeGreaterThan(0))

    const putCall = fetchMock.mock.calls.find(([url, init]) => String(url) === '/api/v1/zones/z1/areas/a1' && (init as RequestInit | undefined)?.method === 'PUT')
    expect(putCall).toBeTruthy()
    const [, init] = putCall as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ name: 'Downtown', active: false })
  })
})
