import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import type { Role, UserProfile } from '../types/auth'
import { Layout } from './Layout'

vi.mock('../hooks/useAuth')

function profile(role: Role): UserProfile {
  return {
    id: 'u1',
    email: 'person@example.com',
    full_name: 'Person',
    phone: null,
    role,
    created_at: '2026-01-01T00:00:00Z',
  }
}

function mockAuth(overrides: Partial<AuthContextValue>) {
  vi.mocked(useAuth).mockReturnValue({
    status: 'unauthenticated',
    token: null,
    user: null,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  })
}

function renderLayout() {
  return render(
    <MemoryRouter>
      <Layout>content</Layout>
    </MemoryRouter>,
  )
}

describe('Layout', () => {
  it('shows Sign In and Create Account links when unauthenticated, and no role-specific nav', () => {
    mockAuth({ status: 'unauthenticated' })
    renderLayout()

    // Both appear twice: once in the header nav, once in the Footer's
    // unauthenticated CTA — assert presence, not a single unique match.
    expect(screen.getAllByRole('link', { name: 'Sign In' }).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: 'Create Account' }).length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: /log out/i })).toBeNull()
  })

  it("shows a CUSTOMER's own navigation only", () => {
    mockAuth({ status: 'authenticated', user: profile('CUSTOMER') })
    renderLayout()

    expect(screen.getByRole('link', { name: 'My Orders' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Create Order' })).toBeTruthy()
    expect(screen.queryByRole('link', { name: 'Operations' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Agents' })).toBeNull()
  })

  it("shows a DELIVERY_AGENT's own navigation only", () => {
    mockAuth({ status: 'authenticated', user: profile('DELIVERY_AGENT') })
    renderLayout()

    expect(screen.getByRole('link', { name: 'Assigned Deliveries' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Operations' })).toBeTruthy()
    expect(screen.queryByRole('link', { name: 'Create Order' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Rate Cards' })).toBeNull()
  })

  it("shows an ADMIN's own navigation only", () => {
    mockAuth({ status: 'authenticated', user: profile('ADMIN') })
    renderLayout()

    expect(screen.getByRole('link', { name: 'Agents' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Zones' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Rate Cards' })).toBeTruthy()
    expect(screen.queryByRole('link', { name: 'Create Order' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Operations' })).toBeNull()
  })

  it('always shows an Account link and a log out button when authenticated', () => {
    mockAuth({ status: 'authenticated', user: profile('CUSTOMER') })
    renderLayout()

    expect(screen.getByRole('link', { name: 'Account' })).toBeTruthy()
    expect(screen.getByRole('button', { name: /log out/i })).toBeTruthy()
  })
})
