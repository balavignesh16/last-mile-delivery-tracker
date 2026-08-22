import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import type { Role, UserProfile } from '../types/auth'
import { LoginPage } from './LoginPage'

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

function mockAuth(overrides: Partial<AuthContextValue> = {}) {
  const value: AuthContextValue = {
    status: 'unauthenticated',
    token: null,
    user: null,
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  }
  vi.mocked(useAuth).mockReturnValue(value)
  return value
}

function renderLogin() {
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/customer/dashboard" element={<div>customer dashboard page</div>} />
        <Route path="/agent/dashboard" element={<div>agent dashboard page</div>} />
        <Route path="/admin/dashboard" element={<div>admin dashboard page</div>} />
        <Route path="/register" element={<div>register page</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

async function submit(email: string, password: string) {
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: email } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: password } })
  fireEvent.click(screen.getByRole('button', { name: /sign in/i }))
}

describe('LoginPage', () => {
  it('has no role selector of any kind', () => {
    mockAuth()
    renderLogin()

    expect(screen.queryByRole('combobox')).toBeNull()
    expect(screen.queryAllByRole('radio').length).toBe(0)
    expect(screen.queryByLabelText(/role/i)).toBeNull()
  })

  it('redirects a CUSTOMER to the customer dashboard on successful login', async () => {
    const auth = mockAuth({ login: vi.fn().mockResolvedValue(profile('CUSTOMER')) })
    renderLogin()

    await submit('person@example.com', 'password123')

    await waitFor(() => expect(screen.getByText('customer dashboard page')).toBeTruthy())
    expect(auth.login).toHaveBeenCalledWith('person@example.com', 'password123')
  })

  it('redirects a DELIVERY_AGENT to the agent dashboard on successful login', async () => {
    mockAuth({ login: vi.fn().mockResolvedValue(profile('DELIVERY_AGENT')) })
    renderLogin()

    await submit('agent@example.com', 'password123')

    await waitFor(() => expect(screen.getByText('agent dashboard page')).toBeTruthy())
  })

  it('redirects an ADMIN to the admin dashboard on successful login', async () => {
    mockAuth({ login: vi.fn().mockResolvedValue(profile('ADMIN')) })
    renderLogin()

    await submit('admin@example.com', 'password123')

    await waitFor(() => expect(screen.getByText('admin dashboard page')).toBeTruthy())
  })

  it('shows a validation error when a field is missing, without calling login', async () => {
    const auth = mockAuth()
    renderLogin()

    fireEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => expect(screen.getByText('Email and password are required.')).toBeTruthy())
    expect(auth.login).not.toHaveBeenCalled()
  })

  it('shows the backend error message when login is rejected', async () => {
    mockAuth({ login: vi.fn().mockRejectedValue(new ApiError('invalid email or password', 401)) })
    renderLogin()

    await submit('person@example.com', 'wrong-password')

    await waitFor(() => expect(screen.getByText('invalid email or password')).toBeTruthy())
  })

  it('the register link navigates to the register page', () => {
    mockAuth()
    renderLogin()

    fireEvent.click(screen.getByRole('link', { name: /create one/i }))
    expect(screen.getByText('register page')).toBeTruthy()
  })
})
