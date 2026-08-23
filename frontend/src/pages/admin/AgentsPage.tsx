import { Users } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { OrderStatusBadge } from '../../components/OrderStatusBadge'
import { StatusBadge } from '../../components/StatusBadge'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { createAgent, listAgents, updateAgentActive } from '../../services/agents'
import { listOrders } from '../../services/orders'
import { listZones } from '../../services/zones'
import type { Agent, Availability } from '../../types/agent'
import type { Order } from '../../types/order'
import type { Zone } from '../../types/zone'
import { formatCurrency } from '../../utils/currency'

const AVAILABILITY_STATE: Record<Availability, 'ok' | 'degraded' | 'error'> = {
  AVAILABLE: 'ok',
  BUSY: 'degraded',
  OFFLINE: 'error',
}

// Admin agent management: view, create, inspect availability/location/
// active state, and now (per an admin's own request) open a single
// agent to see their current zone, activate/deactivate their account,
// and see the orders currently or most-recently assigned to them.
//
// "Orders" reuses the same GET /orders?agent= filter OrdersPage's own
// admin filter bar already calls — it reflects each order's current
// assigned_agent_id, not a full reassignment history (that column is
// overwritten on every (re)assignment; the full historical trail only
// lives in tracking-event metadata with no query-by-agent endpoint).
// This is the same semantics "assigned agent" already has everywhere
// else in the app, so the section is labeled "Orders", not "Order
// history", to avoid implying more than it shows.
export function AgentsPage() {
  const { token } = useAuth()

  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [zones, setZones] = useState<Zone[]>([])

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fullName, setFullName] = useState('')
  const [phone, setPhone] = useState('')
  const [createError, setCreateError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null)
  const [activeError, setActiveError] = useState<string | null>(null)
  const [togglingActive, setTogglingActive] = useState(false)

  const [orders, setOrders] = useState<Order[]>([])
  const [ordersLoading, setOrdersLoading] = useState(false)
  const [ordersError, setOrdersError] = useState<string | null>(null)

  const selectedAgent = agents.find((a) => a.id === selectedAgentId) ?? null

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
    listZones(token)
      .then((list) => {
        if (!cancelled) setZones(list)
      })
      .catch(() => {
        // Non-fatal: the detail pane just falls back to showing a raw
        // zone id if the zone list failed to load.
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    if (!token || !selectedAgentId) return
    let cancelled = false

    async function loadOrders(currentToken: string, agentId: string) {
      setOrdersLoading(true)
      setOrdersError(null)
      try {
        const list = await listOrders(currentToken, { agent: agentId })
        if (!cancelled) setOrders(list)
      } catch (err) {
        if (!cancelled) setOrdersError(err instanceof ApiError ? err.message : 'Could not load orders for this agent.')
      } finally {
        if (!cancelled) setOrdersLoading(false)
      }
    }

    void loadOrders(token, selectedAgentId)
    return () => {
      cancelled = true
    }
  }, [token, selectedAgentId])

  function handleSelectAgent(agent: Agent) {
    setSelectedAgentId(agent.id)
    setActiveError(null)
    setOrders([])
  }

  async function handleToggleActive() {
    if (!token || !selectedAgent) return
    setActiveError(null)
    setTogglingActive(true)
    try {
      const updated = await updateAgentActive(token, selectedAgent.id, !selectedAgent.active)
      setAgents((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
    } catch (err) {
      setActiveError(err instanceof ApiError ? err.message : 'Could not update this agent.')
    } finally {
      setTogglingActive(false)
    }
  }

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
      <div className="flex items-center gap-2.5">
        <span className="flex h-9 w-9 items-center justify-center rounded-md bg-navy-50 text-navy-600">
          <Users className="h-5 w-5" aria-hidden="true" />
        </span>
        <h1 className="text-xl font-semibold">Delivery Agents</h1>
      </div>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Provision a new agent</h2>
        <form onSubmit={handleCreate} className="mt-4 grid gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <ErrorBanner message={createError} />
          </div>

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
              className="rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
            >
              {creating ? 'Creating…' : 'Create agent'}
            </button>
          </div>
        </form>
      </div>

      <div className="mt-6 grid gap-6 md:grid-cols-2">
        <div className="rounded-lg border border-navy-100 bg-white shadow-sm">
          <h2 className="border-b border-navy-100 px-6 py-4 text-sm font-semibold text-slate-700">All agents</h2>
          <ErrorBanner message={loadError} />
          {loading ? (
            <p className="px-6 py-4 text-sm text-slate-500">Loading…</p>
          ) : agents.length === 0 ? (
            <p className="px-6 py-4 text-sm text-slate-500">No agents yet.</p>
          ) : (
            <ul>
              {agents.map((agent) => (
                <li key={agent.id} className="border-t border-slate-100 first:border-t-0">
                  <button
                    type="button"
                    onClick={() => handleSelectAgent(agent)}
                    className={`flex w-full items-center justify-between gap-3 px-6 py-3 text-left text-sm transition-colors hover:bg-navy-50 ${
                      agent.id === selectedAgentId ? 'bg-navy-50' : ''
                    }`}
                  >
                    <span className="min-w-0">
                      <span className="block truncate font-medium text-slate-900">{agent.full_name}</span>
                      <span className="block truncate text-xs text-slate-500">{agent.email}</span>
                    </span>
                    <span className="flex shrink-0 items-center gap-2">
                      {!agent.active && <StatusBadge label="Inactive" state="error" />}
                      <StatusBadge label={agent.availability} state={AVAILABILITY_STATE[agent.availability]} />
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="rounded-lg border border-navy-100 bg-white shadow-sm">
          {!selectedAgent ? (
            <p className="px-6 py-8 text-center text-sm text-slate-500">Select an agent to view their details.</p>
          ) : (
            <>
              <div className="border-b border-navy-100 px-6 py-4">
                <div className="flex items-center justify-between gap-3">
                  <h2 className="text-sm font-semibold text-slate-700">{selectedAgent.full_name}</h2>
                  <StatusBadge label={selectedAgent.active ? 'Active' : 'Inactive'} state={selectedAgent.active ? 'ok' : 'error'} />
                </div>
                <ErrorBanner message={activeError} />
                <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                  <div>
                    <dt className="text-xs text-slate-500">Email</dt>
                    <dd className="text-slate-900">{selectedAgent.email}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-slate-500">Phone</dt>
                    <dd className="text-slate-900">{selectedAgent.phone ?? 'Not provided'}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-slate-500">Availability</dt>
                    <dd>
                      <StatusBadge label={selectedAgent.availability} state={AVAILABILITY_STATE[selectedAgent.availability]} />
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-slate-500">Current zone</dt>
                    <dd className="text-slate-900">
                      {selectedAgent.current_zone_id
                        ? (zones.find((z) => z.id === selectedAgent.current_zone_id)?.name ?? selectedAgent.current_zone_id)
                        : 'Not set'}
                    </dd>
                  </div>
                </dl>
                <button
                  type="button"
                  onClick={() => void handleToggleActive()}
                  disabled={togglingActive}
                  className="mt-4 rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                >
                  {togglingActive ? 'Saving…' : selectedAgent.active ? 'Deactivate' : 'Activate'}
                </button>
              </div>

              <div className="px-6 py-4">
                <h3 className="text-sm font-semibold text-slate-700">Orders</h3>
                <ErrorBanner message={ordersError} />
                {ordersLoading ? (
                  <p className="mt-3 text-sm text-slate-500">Loading…</p>
                ) : orders.length === 0 ? (
                  <p className="mt-3 text-sm text-slate-500">No orders currently or most-recently assigned to this agent.</p>
                ) : (
                  <ul className="mt-3 space-y-2">
                    {orders.map((order) => (
                      <li key={order.id}>
                        <Link
                          to={`/orders/${order.id}`}
                          className="flex items-center justify-between rounded-md border border-slate-100 px-3 py-2 text-sm transition-colors hover:bg-navy-50"
                        >
                          <span className="text-slate-700">
                            {order.order_type} · {order.id.slice(0, 8)}
                          </span>
                          <span className="flex items-center gap-3">
                            <span className="tabular-nums text-slate-500">{formatCurrency(order.final_amount)}</span>
                            <OrderStatusBadge status={order.status} />
                          </span>
                        </Link>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </Layout>
  )
}
