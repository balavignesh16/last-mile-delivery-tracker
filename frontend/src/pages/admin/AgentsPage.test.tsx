import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

describe('AgentsPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
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

    await waitFor(() => expect(screen.getByText('Existing Agent')).toBeTruthy())
    expect(screen.getByText('existing@example.com')).toBeTruthy()
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

    await waitFor(() => expect(screen.getByText('No agents yet.')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Full name'), { target: { value: 'New Agent' } })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'new@example.com' } })
    fireEvent.change(screen.getByLabelText('Temporary password'), { target: { value: 'password123' } })
    fireEvent.click(screen.getByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(screen.getByText('New Agent')).toBeTruthy())
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

    await waitFor(() => expect(screen.getByText('No agents yet.')).toBeTruthy())

    fireEvent.change(screen.getByLabelText('Full name'), { target: { value: 'Dup Agent' } })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'dup@example.com' } })
    fireEvent.change(screen.getByLabelText('Temporary password'), { target: { value: 'password123' } })
    fireEvent.click(screen.getByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(screen.getByText('an account with this email already exists')).toBeTruthy())
  })
})
