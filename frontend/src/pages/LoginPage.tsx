import { Eye, EyeOff, Lock, Mail } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { DeliveryFlowVisual } from '../components/DeliveryFlowVisual'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import { dashboardPathForRole } from '../utils/role'

// Email + password only — the backend's own authentication response is
// the sole source of the caller's role (see AuthContext.login/
// fetchCurrentUser). There is deliberately no role selector anywhere on
// this page; a role picked in the UI would be exactly the kind of
// client-trusted value the backend's RBAC is built to never accept.
export function LoginPage() {
  const { status, user, login } = useAuth()
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  if (status === 'authenticated' && user) {
    return <Navigate to={dashboardPathForRole(user.role)} replace />
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)

    if (!email || !password) {
      setError('Email and password are required.')
      return
    }

    setSubmitting(true)
    try {
      const profile = await login(email, password)
      navigate(dashboardPathForRole(profile.role))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not log in. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Layout>
      <div className="-mx-6 -my-10 grid overflow-hidden rounded-2xl border border-navy-100 bg-white shadow-sm md:grid-cols-2">
        <div className="hidden flex-col justify-between bg-navy-900 p-10 md:flex">
          <div>
            <p className="text-xs font-semibold tracking-widest text-amber-500 uppercase">Welcome back</p>
            <h2 className="mt-3 text-2xl font-bold text-white">Pick up where you left off.</h2>
            <p className="mt-3 text-sm text-navy-100">
              Track deliveries, manage assignments, and keep every order moving — all from one dashboard.
            </p>
          </div>
          <DeliveryFlowVisual compact />
        </div>

        <div className="p-8 sm:p-10">
          <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Sign in</h1>
          <p className="mt-1 text-sm text-slate-500">Enter your details to continue.</p>

          <form onSubmit={handleSubmit} className="mt-6 space-y-4">
            <ErrorBanner message={error} />

            <div>
              <label htmlFor="email" className="block text-sm font-medium text-slate-700">
                Email
              </label>
              <div className="relative mt-1">
                <Mail className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full rounded-md border border-slate-300 py-2 pr-3 pl-9 text-sm transition-colors focus:border-navy-500"
                  autoComplete="email"
                  placeholder="you@example.com"
                />
              </div>
            </div>

            <div>
              <label htmlFor="password" className="block text-sm font-medium text-slate-700">
                Password
              </label>
              <div className="relative mt-1">
                <Lock className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
                <input
                  id="password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full rounded-md border border-slate-300 py-2 pr-16 pl-9 text-sm transition-colors focus:border-navy-500"
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((v) => !v)}
                  className="absolute inset-y-0 right-0 flex items-center gap-1 px-3 text-xs font-medium text-slate-500 hover:text-slate-800"
                >
                  {showPassword ? <EyeOff className="h-3.5 w-3.5" aria-hidden="true" /> : <Eye className="h-3.5 w-3.5" aria-hidden="true" />}
                  {showPassword ? 'Hide' : 'Show'}
                </button>
              </div>
            </div>

            <button
              type="submit"
              disabled={submitting}
              className="w-full rounded-md bg-navy-600 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-navy-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {submitting ? 'Signing in…' : 'Sign In'}
            </button>
          </form>

          <p className="mt-5 text-center text-sm text-slate-600">
            Don&apos;t have an account?{' '}
            <Link to="/register" className="font-medium text-navy-600 hover:text-navy-700">
              Create one
            </Link>
          </p>
        </div>
      </div>
    </Layout>
  )
}
