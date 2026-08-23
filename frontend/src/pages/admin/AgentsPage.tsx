import { ChevronRight, Plus, Search, Users } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { Modal } from '../../components/Modal'
import { OrderStatusBadge } from '../../components/OrderStatusBadge'
import { PageHeader } from '../../components/PageHeader'
import { Pagination } from '../../components/Pagination'
import { Select } from '../../components/Select'
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

const AVAILABILITIES: Availability[] = ['AVAILABLE', 'BUSY', 'OFFLINE']

// Rows shown per page — beyond this, agents paginate out of view
// instead of piling into one long, ever-growing list.
const PAGE_SIZE = 20

const AVAILABILITY_STATE: Record<Availability, 'ok' | 'degraded' | 'error'> = {
  AVAILABLE: 'ok',
  BUSY: 'degraded',
  OFFLINE: 'error',
}

// Compact operational location text — coordinates only, never a map.
// No geofencing/live-tracking implied: this is exactly the same
// current_lat/current_lng snapshot the agent last self-reported via
// OperationsPage.
function formatLocation(agent: Agent): string {
  if (agent.current_lat === null || agent.current_lng === null) return 'Not set'
  return `${agent.current_lat.toFixed(4)}, ${agent.current_lng.toFixed(4)}`
}

// Admin agent management (Round 6): a full-width operational workforce
// table — search/filter toolbar, dense table, and a full-width detail
// workspace that replaces the table (not a permanently-reserved empty
// side panel) once an agent is selected. Creation moved from a
// page-dominating form into a modal, per the same reasoning.
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

  const [createOpen, setCreateOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fullName, setFullName] = useState('')
  const [phone, setPhone] = useState('')
  const [createError, setCreateError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const [agentSearch, setAgentSearch] = useState('')
  const [availabilityFilter, setAvailabilityFilter] = useState('')
  const [zoneFilter, setZoneFilter] = useState('')

  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null)
  const [activeError, setActiveError] = useState<string | null>(null)
  const [togglingActive, setTogglingActive] = useState(false)

  const [orders, setOrders] = useState<Order[]>([])
  const [ordersLoading, setOrdersLoading] = useState(false)
  const [ordersError, setOrdersError] = useState<string | null>(null)

  const [page, setPage] = useState(1)

  const selectedAgent = agents.find((a) => a.id === selectedAgentId) ?? null

  // Client-side only, over the already-loaded agent list — no new
  // endpoint, same reasoning as OrdersPage's own search/filters.
  const displayedAgents = useMemo(() => {
    const q = agentSearch.trim().toLowerCase()
    return agents.filter((a) => {
      if (q && !a.full_name.toLowerCase().includes(q) && !a.email.toLowerCase().includes(q)) return false
      if (availabilityFilter && a.availability !== availabilityFilter) return false
      if (zoneFilter && a.current_zone_id !== zoneFilter) return false
      return true
    })
  }, [agents, agentSearch, availabilityFilter, zoneFilter])

  const hasActiveFilter = agentSearch.trim() !== '' || availabilityFilter !== '' || zoneFilter !== ''

  // A new search/filter can change how many pages exist — always land
  // back on page 1 rather than risk stranding the viewer on a now-empty
  // later page. Adjusted during render (React's documented pattern for
  // resetting state when an input changes) rather than in an effect.
  const filterKey = `${agentSearch}|${availabilityFilter}|${zoneFilter}`
  const [prevFilterKey, setPrevFilterKey] = useState(filterKey)
  if (filterKey !== prevFilterKey) {
    setPrevFilterKey(filterKey)
    setPage(1)
  }

  const pagedAgents = useMemo(() => displayedAgents.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE), [displayedAgents, page])

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
        // Non-fatal: the detail view just falls back to showing a raw
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
      setCreateOpen(false)
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : 'Could not create agent.')
    } finally {
      setCreating(false)
    }
  }

  const createButton = (
    <button
      type="button"
      onClick={() => setCreateOpen(true)}
      className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-navy-700"
    >
      <Plus className="h-4 w-4" aria-hidden="true" />
      Create agent
    </button>
  )

  return (
    <Layout>
      <PageHeader icon={Users} title="Delivery Agents" description="Manage delivery agents, availability and current operating zones." action={createButton} />

      {selectedAgent ? (
        <div className="mt-6">
          <button type="button" onClick={() => setSelectedAgentId(null)} className="text-sm text-slate-500 hover:text-slate-800">
            ← Back to agents
          </button>

          <div className="mt-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold text-slate-900">{selectedAgent.full_name}</h2>
              <p className="text-sm text-slate-500">{selectedAgent.email}</p>
            </div>
            <div className="flex items-center gap-2">
              <StatusBadge label={selectedAgent.active ? 'Active' : 'Inactive'} state={selectedAgent.active ? 'ok' : 'error'} />
              <button
                type="button"
                onClick={() => void handleToggleActive()}
                disabled={togglingActive}
                className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
              >
                {togglingActive ? 'Saving…' : selectedAgent.active ? 'Deactivate' : 'Activate'}
              </button>
            </div>
          </div>
          <ErrorBanner message={activeError} />

          <div className="mt-6 grid gap-6 lg:grid-cols-3">
            <div className="rounded-lg border border-navy-100 bg-white p-6 shadow-sm lg:col-span-1">
              <h3 className="text-sm font-semibold text-slate-700">Agent details</h3>
              <dl className="mt-4 space-y-3 text-sm">
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
                  <dd className="mt-0.5">
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
                <div>
                  <dt className="text-xs text-slate-500">Location</dt>
                  <dd className="tabular-nums text-slate-900">{formatLocation(selectedAgent)}</dd>
                  {selectedAgent.location_updated_at && (
                    <dd className="mt-0.5 text-xs text-slate-400">Last reported {new Date(selectedAgent.location_updated_at).toLocaleString()}</dd>
                  )}
                </div>
              </dl>
            </div>

            <div className="rounded-lg border border-navy-100 bg-white shadow-sm lg:col-span-2">
              <h3 className="border-b border-navy-100 px-6 py-4 text-sm font-semibold text-slate-700">Orders</h3>
              <div className="px-6 py-4">
                <ErrorBanner message={ordersError} />
                {ordersLoading ? (
                  <p className="text-sm text-slate-500">Loading…</p>
                ) : orders.length === 0 ? (
                  <p className="text-sm text-slate-500">No orders currently or most-recently assigned to this agent.</p>
                ) : (
                  <ul className="space-y-2">
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
            </div>
          </div>
        </div>
      ) : (
        <>
          <div className="mt-6 flex flex-wrap items-end gap-3 rounded-lg border border-navy-100 bg-white p-4 shadow-sm">
            <div className="min-w-56 flex-1">
              <label htmlFor="agent-search" className="block text-xs font-medium text-slate-500">
                Search
              </label>
              <div className="relative mt-1">
                <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
                <input
                  id="agent-search"
                  type="text"
                  aria-label="Search agents"
                  value={agentSearch}
                  onChange={(e) => setAgentSearch(e.target.value)}
                  placeholder="Search by name or email…"
                  className="w-full rounded-md border border-slate-300 py-2 pr-3 pl-9 text-sm transition-colors focus:border-navy-500"
                />
              </div>
            </div>
            <div>
              <label htmlFor="availability-filter" className="block text-xs font-medium text-slate-500">
                Availability
              </label>
              <Select
                id="availability-filter"
                value={availabilityFilter}
                onChange={setAvailabilityFilter}
                placeholder="All availability"
                className="mt-1 w-40"
                options={AVAILABILITIES.map((a) => ({ value: a, label: a }))}
              />
            </div>
            <div>
              <label htmlFor="agent-zone-filter" className="block text-xs font-medium text-slate-500">
                Zone
              </label>
              <Select
                id="agent-zone-filter"
                value={zoneFilter}
                onChange={setZoneFilter}
                placeholder="All zones"
                className="mt-1 w-40"
                options={zones.map((z) => ({ value: z.id, label: z.name }))}
              />
            </div>
            {hasActiveFilter && (
              <button
                type="button"
                onClick={() => {
                  setAgentSearch('')
                  setAvailabilityFilter('')
                  setZoneFilter('')
                }}
                className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                Clear filters
              </button>
            )}
          </div>

          <div className="mt-6 overflow-hidden rounded-lg border border-navy-100 bg-white shadow-sm">
            <ErrorBanner message={loadError} />
            {loading ? (
              <div className="space-y-3 p-6">
                {[0, 1, 2].map((i) => (
                  <div key={i} className="h-14 animate-pulse rounded-md bg-slate-100" />
                ))}
              </div>
            ) : agents.length === 0 ? (
              // No duplicate "Create agent" action here — the page
              // header's own button (always visible, above) already
              // does this; repeating it would put two identically
              // labeled controls on screen for the same action.
              <EmptyState icon={Users} title="No delivery agents yet" description="Provision your first delivery agent to begin assigning orders." />
            ) : displayedAgents.length === 0 ? (
              <EmptyState icon={Search} title="No agents match your search." description="Try a different name, email, or filter." />
            ) : (
              <>
                <div className="hidden sm:block">
                  <table className="w-full text-left text-sm">
                    <thead className="border-b border-navy-100 bg-navy-50/95 text-xs font-medium tracking-wide text-navy-700 uppercase">
                      <tr>
                        <th className="px-6 py-3">Agent</th>
                        <th className="px-6 py-3">Email</th>
                        <th className="hidden px-6 py-3 lg:table-cell">Phone</th>
                        <th className="px-6 py-3">Availability</th>
                        <th className="px-6 py-3">Current zone</th>
                        <th className="hidden px-6 py-3 lg:table-cell">Location</th>
                        <th className="px-6 py-3" aria-hidden="true" />
                      </tr>
                    </thead>
                    <tbody>
                      {pagedAgents.map((agent) => (
                        <tr
                          key={agent.id}
                          onClick={() => handleSelectAgent(agent)}
                          className="group cursor-pointer border-t border-slate-100 transition-colors hover:bg-navy-50/50"
                        >
                          <td className="max-w-[12rem] px-6 py-3">
                            <span className="flex min-w-0 items-center gap-2">
                              <button
                                type="button"
                                onClick={() => handleSelectAgent(agent)}
                                title={agent.full_name}
                                className="min-w-0 truncate font-medium text-navy-700 hover:underline"
                              >
                                {agent.full_name}
                              </button>
                              {!agent.active && <StatusBadge label="Inactive" state="error" />}
                            </span>
                          </td>
                          <td className="max-w-[13rem] truncate px-6 py-3 text-slate-600" title={agent.email}>
                            {agent.email}
                          </td>
                          <td className="hidden px-6 py-3 whitespace-nowrap text-slate-600 lg:table-cell">{agent.phone ?? '—'}</td>
                          <td className="px-6 py-3 whitespace-nowrap">
                            <StatusBadge label={agent.availability} state={AVAILABILITY_STATE[agent.availability]} />
                          </td>
                          <td className="max-w-[10rem] truncate px-6 py-3 text-slate-600">
                            {agent.current_zone_id ? (zones.find((z) => z.id === agent.current_zone_id)?.name ?? agent.current_zone_id) : 'Not set'}
                          </td>
                          <td className="hidden px-6 py-3 text-xs whitespace-nowrap tabular-nums text-slate-500 lg:table-cell">{formatLocation(agent)}</td>
                          <td className="px-6 py-3 text-right">
                            <ChevronRight className="h-4 w-4 text-slate-300 transition-colors group-hover:text-navy-500" aria-hidden="true" />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <div className="divide-y divide-slate-100 sm:hidden">
                  {pagedAgents.map((agent) => (
                    <button
                      key={agent.id}
                      type="button"
                      onClick={() => handleSelectAgent(agent)}
                      className="flex w-full flex-col gap-1 px-4 py-3 text-left transition-colors hover:bg-navy-50/50"
                    >
                      <span className="flex items-center justify-between gap-2">
                        <span className="font-medium text-slate-900">{agent.full_name}</span>
                        <StatusBadge label={agent.availability} state={AVAILABILITY_STATE[agent.availability]} />
                      </span>
                      <span className="text-sm text-slate-500">{agent.email}</span>
                      <span className="flex items-center gap-2 text-xs text-slate-500">
                        {agent.current_zone_id ? (zones.find((z) => z.id === agent.current_zone_id)?.name ?? agent.current_zone_id) : 'No zone set'}
                        {!agent.active && <StatusBadge label="Inactive" state="error" />}
                      </span>
                    </button>
                  ))}
                </div>

                <Pagination page={page} totalItems={displayedAgents.length} pageSize={PAGE_SIZE} onPageChange={setPage} />
              </>
            )}
          </div>
        </>
      )}

      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="Create agent">
        <form onSubmit={handleCreate} className="space-y-4">
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

          <button
            type="submit"
            disabled={creating}
            className="w-full rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
          >
            {creating ? 'Creating…' : 'Create agent'}
          </button>
        </form>
      </Modal>
    </Layout>
  )
}
