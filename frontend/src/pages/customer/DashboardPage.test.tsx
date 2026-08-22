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
    id: 'customer-1',
    email: 'customer@example.com',
    full_name: 'Cara Customer',
    phone: null,
    role: 'CUSTOMER',
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

describe('customer DashboardPage', () => {
  it('renders a personalized greeting and links to Create order and My orders', () => {
    mockAuth()

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.getByText('Welcome, Cara Customer')).toBeTruthy()
    const createLink = screen.getByRole('link', { name: /Create order/ })
    const ordersLink = screen.getByRole('link', { name: /My orders/ })
    expect(createLink.getAttribute('href')).toBe('/orders/new')
    expect(ordersLink.getAttribute('href')).toBe('/orders')
  })

  it('shows no admin- or agent-only content', () => {
    mockAuth()

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    expect(screen.queryByText('Agents')).toBeNull()
    expect(screen.queryByText('Rate cards')).toBeNull()
    expect(screen.queryByText('Availability & location')).toBeNull()
    expect(screen.queryByText('Order statistics')).toBeNull()
  })
})
