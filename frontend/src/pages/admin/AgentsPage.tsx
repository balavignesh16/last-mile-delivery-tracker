import { useEffect, useState, type FormEvent } from 'react'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { StatusBadge } from '../../components/StatusBadge'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { createAgent, listAgents } from '../../services/agents'
import type { Agent, Availability } from '../../types/agent'

const AVAILABILITY_STATE: Record<Availability, 'ok' | 'degraded' | 'error'> = {
  AVAILABLE: 'ok',
  BUSY: 'degraded',
  OFFLINE: 'error',
}

// Minimal admin agent management, per M03's explicit scope: view, create,
// inspect availability/location/active state. No search, no filters, no
// analytics — those are later-module territory.
export function AgentsPage() {
  const { token } = useAuth()

  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fullName, setFullName] = useState('')
  const [phone, setPhone] = useState('')
  const [createError, setCreateError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    listAgents(token)
      .then((list) => {
        if (!cancelled) setAgents(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setLoadError(err instanceof ApiError ? err.message : 'Could not load agents.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setCreateError(null)

    if (!token) return
    if (!email || !fullName) {
      setCreateError('Email and full name are required.')
      return
    }
    if (password.length < 8) {
      setCreateError('Password must be at least 8 characters.')
      return
    }

    setCreating(true)
    try {
      const created = await createAgent(token, { email, password, full_name: fullName, phone: phone || undefined })
      setAgents((prev) => [...prev, created])
      setEmail('')
      setPassword('')
      setFullName('')
      setPhone('')
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : 'Could not create agent.')
    } finally {
      setCreating(false)
    }
  }

  return (
    <Layout>
      <h1 className="text-xl font-semibold">Delivery Agents</h1>

      <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Provision a new agent</h2>
        <form onSubmit={handleCreate} className="mt-4 grid gap-4 sm:grid-cols-2">
          <ErrorBanner message={createError} />

          <div>
            <label htmlFor="agent_full_name" className="block text-sm font-medium text-slate-700">
              Full name
            </label>
            <input
              id="agent_full_name"
              type="text"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="agent_email" className="block text-sm font-medium text-slate-700">
              Email
            </label>
            <input
              id="agent_email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="agent_phone" className="block text-sm font-medium text-slate-700">
              Phone <span className="text-slate-400">(optional)</span>
            </label>
            <input
              id="agent_phone"
              type="tel"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="agent_password" className="block text-sm font-medium text-slate-700">
              Temporary password
            </label>
            <input
              id="agent_password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div className="sm:col-span-2">
            <button
              type="submit"
              disabled={creating}
              className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
            >
              {creating ? 'Creating…' : 'Create agent'}
            </button>
          </div>
        </form>
      </div>

      <div className="mt-6 rounded-lg border border-slate-200 bg-white shadow-sm">
        <h2 className="border-b border-slate-200 px-6 py-4 text-sm font-semibold text-slate-700">
          All agents
        </h2>
        <ErrorBanner message={loadError} />
        {loading ? (
          <p className="px-6 py-4 text-sm text-slate-500">Loading…</p>
        ) : agents.length === 0 ? (
          <p className="px-6 py-4 text-sm text-slate-500">No agents yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-6 py-2">Name</th>
                  <th className="px-6 py-2">Email</th>
                  <th className="px-6 py-2">Availability</th>
                  <th className="px-6 py-2">Location</th>
                  <th className="px-6 py-2">Active</th>
                </tr>
              </thead>
              <tbody>
                {agents.map((agent) => (
                  <tr key={agent.id} className="border-t border-slate-100">
                    <td className="px-6 py-3">{agent.full_name}</td>
                    <td className="px-6 py-3 text-slate-500">{agent.email}</td>
                    <td className="px-6 py-3">
                      <StatusBadge label={agent.availability} state={AVAILABILITY_STATE[agent.availability]} />
                    </td>
                    <td className="px-6 py-3 text-slate-500">
                      {agent.current_lat != null && agent.current_lng != null
                        ? `${agent.current_lat.toFixed(4)}, ${agent.current_lng.toFixed(4)}`
                        : 'Not reported'}
                    </td>
                    <td className="px-6 py-3 text-slate-500">{agent.active ? 'Yes' : 'No'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Layout>
  )
}
