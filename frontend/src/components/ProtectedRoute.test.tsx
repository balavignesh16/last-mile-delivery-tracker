import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import { ProtectedRoute } from './ProtectedRoute'

vi.mock('../hooks/useAuth')

function mockAuth(status: AuthContextValue['status']) {
  vi.mocked(useAuth).mockReturnValue({
    status,
    token: null,
    user: null,
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  })
}

function renderProtected() {
  return render(
    <MemoryRouter initialEntries={['/app']}>
      <Routes>
        <Route path="/login" element={<div>login page</div>} />
        <Route
          path="/app"
          element={
            <ProtectedRoute>
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

  it('renders the protected content when authenticated', () => {
    mockAuth('authenticated')
    renderProtected()
    expect(screen.getByText('secret content')).toBeTruthy()
  })
})
