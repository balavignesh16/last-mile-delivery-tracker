import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import type { UserProfile } from '../types/auth'
import { RegisterPage } from './RegisterPage'

vi.mock('../hooks/useAuth')

function customerProfile(): UserProfile {
  return {
    id: 'u1',
    email: 'new@example.com',
    full_name: 'New Customer',
    phone: null,
    role: 'CUSTOMER',
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

function renderRegister() {
  return render(
    <MemoryRouter initialEntries={['/register']}>
      <Routes>
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/customer/dashboard" element={<div>customer dashboard page</div>} />
        <Route path="/login" element={<div>login page</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('RegisterPage', () => {
  it('has no role selector — public registration always creates a customer', () => {
    mockAuth()
    renderRegister()

    expect(screen.queryByRole('combobox')).toBeNull()
    expect(screen.queryByLabelText(/role/i)).toBeNull()
  })

  it('redirects to the customer dashboard after successful registration', async () => {
    const auth = mockAuth({ register: vi.fn().mockResolvedValue(customerProfile()) })
    renderRegister()

    fireEvent.change(screen.getByLabelText('Full name'), { target: { value: 'New Customer' } })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'new@example.com' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'password123' } })
    fireEvent.click(screen.getByRole('button', { name: /create account/i }))

    await waitFor(() => expect(screen.getByText('customer dashboard page')).toBeTruthy())
    expect(auth.register).toHaveBeenCalledWith({
      email: 'new@example.com',
      password: 'password123',
      full_name: 'New Customer',
      phone: undefined,
    })
  })

  it('rejects a short password before calling register', async () => {
    const auth = mockAuth()
    renderRegister()

    fireEvent.change(screen.getByLabelText('Full name'), { target: { value: 'New Customer' } })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'new@example.com' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'short' } })
    fireEvent.click(screen.getByRole('button', { name: /create account/i }))

    await waitFor(() => expect(screen.getByText('Password must be at least 8 characters.')).toBeTruthy())
    expect(auth.register).not.toHaveBeenCalled()
  })

  it('shows the backend error message when registration fails', async () => {
    mockAuth({ register: vi.fn().mockRejectedValue(new ApiError('an account with this email already exists', 409)) })
    renderRegister()

    fireEvent.change(screen.getByLabelText('Full name'), { target: { value: 'New Customer' } })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'new@example.com' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'password123' } })
    fireEvent.click(screen.getByRole('button', { name: /create account/i }))

    await waitFor(() => expect(screen.getByText('an account with this email already exists')).toBeTruthy())
  })

  it('the sign in link navigates to the login page', () => {
    mockAuth()
    renderRegister()

    // Exact, case-sensitive: distinguishes the page body's "Sign in" link
    // from Layout's own unauthenticated header nav, which also renders a
    // "Sign In" link.
    fireEvent.click(screen.getByRole('link', { name: 'Sign in' }))
    expect(screen.getByText('login page')).toBeTruthy()
  })
})
