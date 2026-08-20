import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

export function Layout({ children }: { children: ReactNode }) {
  const { status, user, logout } = useAuth()

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-4xl items-center justify-between px-6 py-4">
          <Link to="/" className="text-lg font-semibold">
            Last-Mile Delivery Tracker
          </Link>
          <nav className="flex items-center gap-4 text-sm">
            {status === 'authenticated' && user ? (
              <>
                <Link to="/app" className="text-slate-600 hover:text-slate-900">
                  My Account
                </Link>
                {user.role === 'ADMIN' && (
                  <Link to="/admin/agents" className="text-slate-600 hover:text-slate-900">
                    Agents
                  </Link>
                )}
                {user.role === 'DELIVERY_AGENT' && (
                  <Link to="/agent" className="text-slate-600 hover:text-slate-900">
                    Operations
                  </Link>
                )}
                <span className="text-slate-400">{user.email}</span>
                <button
                  type="button"
                  onClick={logout}
                  className="rounded-md border border-slate-300 px-3 py-1 text-slate-700 hover:bg-slate-100"
                >
                  Log out
                </button>
              </>
            ) : (
              <>
                <Link to="/login" className="text-slate-600 hover:text-slate-900">
                  Log in
                </Link>
                <Link
                  to="/register"
                  className="rounded-md bg-slate-900 px-3 py-1 text-white hover:bg-slate-700"
                >
                  Register
                </Link>
              </>
            )}
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-4xl px-6 py-10">{children}</main>
    </div>
  )
}
