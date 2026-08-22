import { LayoutDashboard, LogOut, MapPin, Package, PackagePlus, Truck, User, Users, Wallet } from 'lucide-react'
import type { ComponentType, ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import type { Role } from '../types/auth'
import { dashboardPathForRole } from '../utils/role'
import { Footer } from './Footer'

interface NavItem {
  to: string
  label: string
  icon: ComponentType<{ className?: string }>
}

// Each role's primary navigation, in the exact set the product spec
// calls for — no role sees a link to a page it isn't authorized to use
// (ProtectedRoute/backend RBAC remain the actual authority; this list is
// UX only, keeping irrelevant destinations out of view rather than
// relying on the user to notice a 403).
const NAV_ITEMS: Record<Role, NavItem[]> = {
  CUSTOMER: [
    { to: '/customer/dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { to: '/orders', label: 'My Orders', icon: Package },
    { to: '/orders/new', label: 'Create Order', icon: PackagePlus },
  ],
  DELIVERY_AGENT: [
    { to: '/agent/dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { to: '/orders', label: 'Assigned Deliveries', icon: Package },
    { to: '/agent', label: 'Operations', icon: Truck },
  ],
  ADMIN: [
    { to: '/admin/dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { to: '/orders', label: 'Orders', icon: Package },
    { to: '/admin/agents', label: 'Agents', icon: Users },
    { to: '/admin/zones', label: 'Zones', icon: MapPin },
    { to: '/admin/rates', label: 'Rate Cards', icon: Wallet },
  ],
}

export function Layout({ children }: { children: ReactNode }) {
  const { status, user, logout } = useAuth()
  const location = useLocation()

  const navItems = user ? NAV_ITEMS[user.role] : []

  return (
    <div className="flex min-h-screen flex-col bg-navy-50 text-slate-900">
      <header className="sticky top-0 z-10 border-b border-navy-100 bg-white/90 backdrop-blur-sm">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-3.5">
          <Link
            to={status === 'authenticated' && user ? dashboardPathForRole(user.role) : '/'}
            className="flex items-center gap-2 text-[15px] font-semibold tracking-tight text-navy-900"
          >
            <span className="flex h-8 w-8 items-center justify-center rounded-md bg-navy-600 text-white">
              <Truck className="h-4 w-4" aria-hidden="true" />
            </span>
            <span className="hidden sm:inline">Last-Mile Delivery Tracker</span>
          </Link>

          {status === 'authenticated' && user ? (
            <nav className="flex items-center gap-1 overflow-x-auto text-sm">
              {navItems.map((item) => {
                const active = location.pathname === item.to
                const Icon = item.icon
                return (
                  <Link
                    key={item.to}
                    to={item.to}
                    className={`flex shrink-0 items-center gap-1.5 rounded-md border-b-2 px-3 py-1.5 font-medium whitespace-nowrap transition-colors ${
                      active ? 'border-amber-500 bg-navy-50 text-navy-700' : 'border-transparent text-slate-600 hover:bg-navy-50 hover:text-navy-900'
                    }`}
                  >
                    <Icon className="h-4 w-4" aria-hidden="true" />
                    {item.label}
                  </Link>
                )
              })}
              <span className="mx-1 h-5 w-px shrink-0 bg-navy-100" aria-hidden="true" />
              <Link
                to="/app"
                className={`flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 font-medium whitespace-nowrap transition-colors ${
                  location.pathname === '/app' ? 'bg-navy-50 text-navy-700' : 'text-slate-600 hover:bg-navy-50 hover:text-navy-900'
                }`}
              >
                <User className="h-4 w-4" aria-hidden="true" />
                Account
              </Link>
              <button
                type="button"
                onClick={logout}
                className="ml-2 flex shrink-0 items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 font-medium whitespace-nowrap text-slate-700 transition-colors hover:bg-slate-100"
              >
                <LogOut className="h-4 w-4" aria-hidden="true" />
                Log out
              </button>
            </nav>
          ) : (
            <nav className="flex items-center gap-3 text-sm">
              <Link to="/login" className="font-medium text-slate-600 hover:text-navy-900">
                Sign In
              </Link>
              <Link
                to="/register"
                className="rounded-md bg-navy-600 px-3.5 py-1.5 font-medium text-white transition-colors hover:bg-navy-700"
              >
                Create Account
              </Link>
            </nav>
          )}
        </div>
      </header>
      <main className="page-fade-in mx-auto w-full max-w-6xl flex-1 px-6 py-10">{children}</main>
      <Footer />
    </div>
  )
}
