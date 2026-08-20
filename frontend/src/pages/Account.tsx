import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { StatusBadge } from '../components/StatusBadge'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'

const ROLE_LABELS: Record<string, string> = {
  CUSTOMER: 'Customer',
  DELIVERY_AGENT: 'Delivery Agent',
  ADMIN: 'Admin',
}

// Minimal authenticated shell: identity, an editable profile (M03), role
// shown but never editable here, logout, and role-specific entry points
// to the small amount of M03 UI that exists so far. Full role-specific
// dashboards (order history, delivery lists, admin analytics) are M12's
// job, not this one.
export function Account() {
  const { user, updateProfile } = useAuth()

  const [fullName, setFullName] = useState(user?.full_name ?? '')
  const [phone, setPhone] = useState(user?.phone ?? '')
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [saving, setSaving] = useState(false)

  if (!user) return null

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSuccess(false)

    if (!fullName.trim()) {
      setError('Full name is required.')
      return
    }

    setSaving(true)
    try {
      await updateProfile({ full_name: fullName.trim(), phone: phone.trim() || undefined })
      setSuccess(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save changes. Please try again.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Layout>
      <div className="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">My Account</h1>
          {/* Role is displayed, never an editable field — the backend
              is the only thing that ever assigns a role. */}
          <StatusBadge label={ROLE_LABELS[user.role] ?? user.role} state="ok" />
        </div>

        <form onSubmit={handleSubmit} className="mt-6 space-y-4">
          <ErrorBanner message={error} />
          {success && (
            <div role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-4 py-2 text-sm text-emerald-800">
              Profile updated.
            </div>
          )}

          <div>
            <span className="block text-sm font-medium text-slate-700">Email</span>
            <p className="mt-1 text-sm text-slate-500">{user.email}</p>
          </div>

          <div>
            <label htmlFor="full_name" className="block text-sm font-medium text-slate-700">
              Name
            </label>
            <input
              id="full_name"
              type="text"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="phone" className="block text-sm font-medium text-slate-700">
              Phone
            </label>
            <input
              id="phone"
              type="tel"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div>
            <span className="block text-sm font-medium text-slate-700">Member since</span>
            <p className="mt-1 text-sm text-slate-500">{new Date(user.created_at).toLocaleDateString()}</p>
          </div>

          <button
            type="submit"
            disabled={saving}
            className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save changes'}
          </button>
        </form>
      </div>

      {user.role === 'ADMIN' && (
        <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
          <h2 className="text-sm font-semibold text-slate-700">Admin</h2>
          <p className="mt-1 text-sm text-slate-500">
            <Link to="/admin/agents" className="font-medium text-slate-900 underline">
              Manage delivery agents
            </Link>
          </p>
        </div>
      )}

      {user.role === 'DELIVERY_AGENT' && (
        <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
          <h2 className="text-sm font-semibold text-slate-700">Delivery agent</h2>
          <p className="mt-1 text-sm text-slate-500">
            <Link to="/agent" className="font-medium text-slate-900 underline">
              Manage availability &amp; location
            </Link>
          </p>
        </div>
      )}

      <div className="mt-6 rounded-lg border border-dashed border-slate-300 p-6 text-sm text-slate-500">
        Order history and delivery dashboards arrive in later modules.
      </div>
    </Layout>
  )
}
