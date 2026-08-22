import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../../contexts/auth-context'
import { useAuth } from '../../hooks/useAuth'
import type { UserProfile } from '../../types/auth'
import { DashboardPage } from './DashboardPage'

vi.mock('../../hooks/useAuth')

function mockAuth() {
  const user: UserProfile = {
    id: 'agent-1',
    email: 'agent@example.com',
    full_name: 'Alex Agent',
    phone: null,
    role: 'DELIVERY_AGENT',
    created_at: '2026-01-01T00:00:00Z',
  }
  vi.mocked(useAuth).mockReturnValue({
    status: 'authenticated',
    token: 'token',
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  } as AuthContextValue)
}

describe('agent DashboardPage', () => {
  it('renders a personalized greeting and links to assigned deliveries and availability/location', () => {
    mockAuth()

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.getByText('Welcome, Alex Agent')).toBeTruthy()
    const deliveriesLink = screen.getByRole('link', { name: /Assigned deliveries/ })
    const operationsLink = screen.getByRole('link', { name: /Availability & location/ })
    expect(deliveriesLink.getAttribute('href')).toBe('/orders')
    expect(operationsLink.getAttribute('href')).toBe('/agent')
  })

  it('shows no customer- or admin-only dashboard content', () => {
    mockAuth()

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.queryByText('Create order')).toBeNull()
    expect(screen.queryByText('Agents')).toBeNull()
    expect(screen.queryByText('Rate cards')).toBeNull()
    expect(screen.queryByText('Order statistics')).toBeNull()
  })
})
