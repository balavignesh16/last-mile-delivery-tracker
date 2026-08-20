import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

// Frontend route protection is a UX convenience, not a security boundary
// — the backend's RequireAuth/RequireRole middleware is authoritative.
// Nothing prevents a user from editing frontend state to render a
// protected component locally; what matters is that every API call it
// makes still gets rejected by the backend if the token is missing,
// expired, or the wrong role. This component only decides what to show,
// never what to allow.
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { status } = useAuth()

  if (status === 'loading') {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-slate-500">
        Loading…
      </div>
    )
  }

  if (status === 'unauthenticated') {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}
