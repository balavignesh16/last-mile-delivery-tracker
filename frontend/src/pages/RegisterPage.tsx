import { Eye, EyeOff, Lock, Mail } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { DeliveryFlowVisual } from '../components/DeliveryFlowVisual'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import { dashboardPathForRole } from '../utils/role'

const MIN_PASSWORD_LENGTH = 8

// Public registration always provisions a CUSTOMER — there is no role
// field on this form, and none is sent to the backend (see
// services/auth.ts's RegisterInput). Agent/admin accounts are
// provisioned separately by an admin; this page cannot create either.
export function RegisterPage() {
  const { status, user, register } = useAuth()
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [fullName, setFullName] = useState('')
  const [phone, setPhone] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  if (status === 'authenticated' && user) {
    return <Navigate to={dashboardPathForRole(user.role)} replace />
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)

    if (!email || !fullName) {
      setError('Email and full name are required.')
      return
    }
    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`)
      return
    }

    setSubmitting(true)
    try {
      const profile = await register({ email, password, full_name: fullName, phone: phone || undefined })
      navigate(dashboardPathForRole(profile.role))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not register. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Layout>
      <div className="-mx-6 -my-10 flex min-h-[calc(100vh-9rem)] items-center px-6 py-10">
        <div className="grid w-full overflow-hidden rounded-2xl border border-navy-100 bg-white shadow-sm md:grid-cols-2">
          <div className="hidden flex-col justify-between bg-navy-900 p-10 md:flex">
            <div>
              <p className="text-xs font-semibold tracking-widest text-amber-500 uppercase">Get started</p>
              <h2 className="mt-3 text-2xl font-bold text-white">Book your first delivery in minutes.</h2>
              <p className="mt-3 text-sm text-navy-100">
                See your exact charge before you confirm, then track it from pickup to doorstep.
              </p>
            </div>
            <DeliveryFlowVisual compact />
          </div>

          <div className="p-8 sm:p-10">
            <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Create your account</h1>
            <p className="mt-1 text-sm text-slate-500">
              Public registration creates a customer account. Agent and admin accounts are provisioned separately.
            </p>
            <form onSubmit={handleSubmit} className="mt-6 space-y-4">
              <ErrorBanner message={error} />

              <div>
                <label htmlFor="full_name" className="block text-sm font-medium text-slate-700">
                  Full name
                </label>
                <input
                  id="full_name"
                  type="text"
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm transition-colors focus:border-navy-500"
                  autoComplete="name"
                />
              </div>

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
                <label htmlFor="phone" className="block text-sm font-medium text-slate-700">
                  Phone <span className="text-slate-400">(optional)</span>
                </label>
                <input
                  id="phone"
                  type="tel"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm transition-colors focus:border-navy-500"
                  autoComplete="tel"
                />
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
                    autoComplete="new-password"
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
                <p className="mt-1 text-xs text-slate-500">At least {MIN_PASSWORD_LENGTH} characters.</p>
              </div>

              <button
                type="submit"
                disabled={submitting}
                className="w-full rounded-md bg-navy-600 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-navy-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {submitting ? 'Creating account…' : 'Create account'}
              </button>
            </form>

            <p className="mt-5 text-center text-sm text-slate-600">
              Already have an account?{' '}
              <Link to="/login" className="font-medium text-navy-600 hover:text-navy-700">
                Sign in
              </Link>
            </p>
          </div>
        </div>
      </div>
    </Layout>
  )
}
