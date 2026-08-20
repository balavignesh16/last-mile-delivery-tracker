import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthContextValue } from '../contexts/auth-context'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import type { UserProfile } from '../types/auth'
import { Account } from './Account'

vi.mock('../hooks/useAuth')

function baseUser(): UserProfile {
  return {
    id: 'u1',
    email: 'person@example.com',
    full_name: 'Original Name',
    phone: null,
    role: 'CUSTOMER',
    created_at: '2026-01-01T00:00:00Z',
  }
}

function mockAuth(overrides: Partial<AuthContextValue> = {}) {
  const value: AuthContextValue = {
    status: 'authenticated',
    token: 'token',
    user: baseUser(),
    login: vi.fn(),
    register: vi.fn(),
    updateProfile: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn(),
    ...overrides,
  }
  vi.mocked(useAuth).mockReturnValue(value)
  return value
}

function renderAccount() {
  return render(
    <MemoryRouter>
      <Account />
    </MemoryRouter>,
  )
}

describe('Account', () => {
  it('loads the profile into the form fields', () => {
    mockAuth()
    renderAccount()
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Original Name')
    // Appears twice: once in Layout's header nav, once in the profile
    // section itself — both are correct, so assert presence, not count.
    expect(screen.getAllByText('person@example.com').length).toBeGreaterThan(0)
  })

  it('role is displayed but not an editable field', () => {
    mockAuth()
    renderAccount()
    expect(screen.getByText('Customer')).toBeTruthy()
    expect(screen.queryByLabelText(/role/i)).toBeNull()
  })

  it('saves permitted fields and shows a success message', async () => {
    const auth = mockAuth()
    renderAccount()

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'New Name' } })
    fireEvent.change(screen.getByLabelText('Phone'), { target: { value: '555-0100' } })
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(screen.getByText('Profile updated.')).toBeTruthy())
    expect(auth.updateProfile).toHaveBeenCalledWith({ full_name: 'New Name', phone: '555-0100' })
  })

  it('rejects an empty name before calling the API', async () => {
    const auth = mockAuth()
    renderAccount()

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '   ' } })
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(screen.getByText('Full name is required.')).toBeTruthy())
    expect(auth.updateProfile).not.toHaveBeenCalled()
  })

  it('shows the backend error message when the save fails', async () => {
    mockAuth({ updateProfile: vi.fn().mockRejectedValue(new ApiError('could not update profile', 500)) })
    renderAccount()

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'New Name' } })
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(screen.getByText('could not update profile')).toBeTruthy())
  })
})
