import type { ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import type { Role } from '../types/auth'
import { dashboardPathForRole } from '../utils/role'

interface NavItem {
  to: string
  label: string
}

// Each role's primary navigation, in the exact set the product spec
// calls for — no role sees a link to a page it isn't authorized to use
// (ProtectedRoute/backend RBAC remain the actual authority; this list is
// UX only, keeping irrelevant destinations out of view rather than
// relying on the user to notice a 403).
const NAV_ITEMS: Record<Role, NavItem[]> = {
  CUSTOMER: [
    { to: '/customer/dashboard', label: 'Dashboard' },
    { to: '/orders', label: 'My Orders' },
    { to: '/orders/new', label: 'Create Order' },
  ],
  DELIVERY_AGENT: [
    { to: '/agent/dashboard', label: 'Dashboard' },
    { to: '/orders', label: 'Assigned Deliveries' },
    { to: '/agent', label: 'Operations' },
  ],
  ADMIN: [
    { to: '/admin/dashboard', label: 'Dashboard' },
    { to: '/orders', label: 'Orders' },
    { to: '/admin/agents', label: 'Agents' },
    { to: '/admin/zones', label: 'Zones' },
    { to: '/admin/rates', label: 'Rate Cards' },
  ],
}

export function Layout({ children }: { children: ReactNode }) {
  const { status, user, logout } = useAuth()
  const location = useLocation()

  const navItems = user ? NAV_ITEMS[user.role] : []

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="sticky top-0 z-10 border-b border-slate-200 bg-white/90 backdrop-blur-sm">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-3.5">
          <Link
            to={status === 'authenticated' && user ? dashboardPathForRole(user.role) : '/'}
            className="flex items-center gap-2 text-[15px] font-semibold tracking-tight text-slate-900"
          >
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-brand-600 text-xs font-bold text-white">
              LM
            </span>
            <span className="hidden sm:inline">Last-Mile Delivery Tracker</span>
          </Link>

          {status === 'authenticated' && user ? (
            <nav className="flex items-center gap-1 overflow-x-auto text-sm">
              {navItems.map((item) => {
                const active = location.pathname === item.to
                return (
                  <Link
                    key={item.to}
                    to={item.to}
                    className={`shrink-0 rounded-md px-3 py-1.5 font-medium whitespace-nowrap transition-colors ${
                      active ? 'bg-brand-50 text-brand-700' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
                    }`}
                  >
                    {item.label}
                  </Link>
                )
              })}
              <span className="mx-1 h-5 w-px shrink-0 bg-slate-200" aria-hidden="true" />
              <Link
                to="/app"
                className={`shrink-0 rounded-md px-3 py-1.5 font-medium whitespace-nowrap transition-colors ${
                  location.pathname === '/app' ? 'bg-brand-50 text-brand-700' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
                }`}
              >
                Account
              </Link>
              <button
                type="button"
                onClick={logout}
                className="ml-2 shrink-0 rounded-md border border-slate-300 px-3 py-1.5 font-medium whitespace-nowrap text-slate-700 transition-colors hover:bg-slate-100"
              >
                Log out
              </button>
            </nav>
          ) : (
            <nav className="flex items-center gap-3 text-sm">
              <Link to="/login" className="font-medium text-slate-600 hover:text-slate-900">
                Sign In
              </Link>
              <Link
                to="/register"
                className="rounded-md bg-slate-900 px-3.5 py-1.5 font-medium text-white transition-colors hover:bg-slate-700"
              >
                Create Account
              </Link>
            </nav>
          )}
        </div>
      </header>
      <main className="page-fade-in mx-auto max-w-6xl px-6 py-10">{children}</main>
    </div>
  )
}
