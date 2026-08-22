import { useEffect, useState, type ReactNode } from 'react'
import { fetchCurrentUser, loginUser, registerUser, updateProfile as updateProfileRequest } from '../services/auth'
import type { ProfileUpdateInput, RegisterInput, UserProfile } from '../types/auth'
import { AuthContext, type AuthStatus } from './auth-context'

// Token storage tradeoff, documented plainly rather than overstated:
// sessionStorage is simple, needs no backend cookie/CSRF handling, and
// survives a page refresh within the same tab — but, like any
// JavaScript-readable storage, it is vulnerable to token theft via XSS.
// A production system would use an httpOnly cookie instead. That adds
// real complexity (Set-Cookie on the backend, SameSite/CSRF handling)
// that the frozen architecture's "keep authentication simple, no
// over-engineering" instruction explicitly rules out for this project's
// scope. This is a deliberate, bounded tradeoff — not an oversight.
const TOKEN_STORAGE_KEY = 'lmt_auth_token'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => sessionStorage.getItem(TOKEN_STORAGE_KEY))
  const [user, setUser] = useState<UserProfile | null>(null)
  // Derived at init from whether a token exists, rather than always
  // starting at 'loading' and having the effect below correct it — so the
  // effect never needs a synchronous setState call for the "no token"
  // case, only for the async fetch's own resolution.
  const [status, setStatus] = useState<AuthStatus>(() => (token ? 'loading' : 'unauthenticated'))

  useEffect(() => {
    if (!token) return

    let cancelled = false
    fetchCurrentUser(token)
      .then((profile) => {
        if (cancelled) return
        setUser(profile)
        setStatus('authenticated')
      })
      .catch(() => {
        if (cancelled) return
        // Token is missing, expired, or otherwise rejected by the
        // backend — the backend is authoritative here, so clear local
        // state to match rather than trusting a stale token further.
        sessionStorage.removeItem(TOKEN_STORAGE_KEY)
        setToken(null)
        setUser(null)
        setStatus('unauthenticated')
      })

    return () => {
      cancelled = true
    }
  }, [token])

  // Fetches the profile inline (rather than relying on the token-change
  // effect above, which still also fires and re-confirms the same data)
  // so the caller can redirect by role immediately, without a second
  // source of truth for "what role is this user" — it's the same
  // fetchCurrentUser call the effect would otherwise make, just awaited
  // here so LoginPage/RegisterPage can act on it right away.
  async function login(email: string, password: string): Promise<UserProfile> {
    const { token: newToken } = await loginUser({ email, password })
    const profile = await fetchCurrentUser(newToken)
    sessionStorage.setItem(TOKEN_STORAGE_KEY, newToken)
    setToken(newToken)
    setUser(profile)
    setStatus('authenticated')
    return profile
  }

  async function register(input: RegisterInput): Promise<UserProfile> {
    await registerUser(input)
    return login(input.email, input.password)
  }

  async function updateProfile(input: ProfileUpdateInput) {
    if (!token) {
      throw new Error('updateProfile called while unauthenticated')
    }
    const updated = await updateProfileRequest(token, input)
    setUser(updated)
  }

  function logout() {
    sessionStorage.removeItem(TOKEN_STORAGE_KEY)
    setToken(null)
    setUser(null)
    setStatus('unauthenticated')
  }

  return (
    <AuthContext.Provider value={{ token, user, status, login, register, updateProfile, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
