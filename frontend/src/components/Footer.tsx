import { Truck } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { dashboardPathForRole } from '../utils/role'

export function Footer() {
  const { status, user } = useAuth()

  return (
    <footer className="border-t border-navy-100 bg-white">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-5 px-6 py-10 text-sm text-slate-500 sm:flex-row">
        <div className="flex items-center gap-2.5 font-semibold text-navy-900">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-navy-600 text-white">
            <Truck className="h-4 w-4" aria-hidden="true" />
          </span>
          Last-Mile Delivery Tracker
        </div>

        <p className="text-center sm:text-left">Pricing, assignment, and tracking for every delivery, end to end.</p>

        {status === 'authenticated' && user ? (
          <Link to={dashboardPathForRole(user.role)} className="font-medium text-navy-600 hover:text-navy-700">
            Go to dashboard
          </Link>
        ) : (
          <div className="flex items-center gap-5">
            <Link to="/login" className="font-medium text-navy-600 hover:text-navy-700">
              Sign In
            </Link>
            <Link to="/register" className="font-medium text-navy-600 hover:text-navy-700">
              Create Account
            </Link>
          </div>
        )}
      </div>
    </footer>
  )
}
