import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import type { UserProfile } from '../types/auth'
import { ProtectedRoute } from './ProtectedRoute'

vi.mock('../hooks/useAuth')

function mockAuth(status: AuthContextValue['status'], user: UserProfile | null = null) {
  vi.mocked(useAuth).mockReturnValue({
    status,
    token: null,
    user,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
  })
}

function customer(): UserProfile {
  return {
    id: 'u1',
    email: 'customer@example.com',
    full_name: 'A Customer',
    phone: null,
    role: 'CUSTOMER',
    created_at: '2026-01-01T00:00:00Z',
  }
}

function admin(): UserProfile {
  return { ...customer(), id: 'u2', email: 'admin@example.com', role: 'ADMIN' }
}

function renderProtected(roles?: UserProfile['role'][]) {
  return render(
    <MemoryRouter initialEntries={['/admin/agents']}>
      <Routes>
        <Route path="/login" element={<div>login page</div>} />
        <Route path="/customer/dashboard" element={<div>customer dashboard page</div>} />
        <Route
          path="/admin/agents"
          element={
            <ProtectedRoute roles={roles}>
              <div>secret content</div>
            </ProtectedRoute>
          }
        />
      </Routes>
    </MemoryRouter>,
  )
}

describe('ProtectedRoute', () => {
  it('shows a loading state while auth status is unresolved', () => {
    mockAuth('loading')
    renderProtected()
    expect(screen.getByText(/loading/i)).toBeTruthy()
    expect(screen.queryByText('secret content')).toBeNull()
  })

  it('redirects to /login when unauthenticated', () => {
    mockAuth('unauthenticated')
    renderProtected()
    expect(screen.getByText('login page')).toBeTruthy()
    expect(screen.queryByText('secret content')).toBeNull()
  })

  it('renders the protected content when authenticated and no roles are required', () => {
    mockAuth('authenticated', customer())
    renderProtected()
    expect(screen.getByText('secret content')).toBeTruthy()
  })

  it('renders the protected content when the user has a permitted role', () => {
    mockAuth('authenticated', admin())
    renderProtected(['ADMIN'])
    expect(screen.getByText('secret content')).toBeTruthy()
  })

  it("redirects to the user's own dashboard when they lack the required role", () => {
    mockAuth('authenticated', customer())
    renderProtected(['ADMIN'])
    expect(screen.getByText('customer dashboard page')).toBeTruthy()
    expect(screen.queryByText('secret content')).toBeNull()
  })
})
