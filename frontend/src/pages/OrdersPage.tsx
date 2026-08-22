import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { OrderStatusBadge } from '../components/OrderStatusBadge'
import { useAuth } from '../hooks/useAuth'
import { listAgents } from '../services/agents'
import { ApiError } from '../services/api'
import { listOrders } from '../services/orders'
import { listZones } from '../services/zones'
import type { Agent } from '../types/agent'
import { ORDER_STATUSES, type Order, type OrderFilter } from '../types/order'
import type { Zone } from '../types/zone'
import { formatCurrency } from '../utils/currency'

// ADMIN sees every order (optionally narrowed by the status/zone/agent
// filters below, M12), CUSTOMER sees only their own, and (since M09)
// DELIVERY_AGENT sees only orders currently assigned to them — the
// backend decides which by the caller's role (GET /orders); a CUSTOMER/
// DELIVERY_AGENT can never widen their own view by supplying a filter
// (see internal/orders/handler.go's own doc comment), so this page only
// ever renders the filter controls for an ADMIN viewer.
export function OrdersPage() {
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'ADMIN'
  const isDeliveryAgent = user?.role === 'DELIVERY_AGENT'

  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [statusFilter, setStatusFilter] = useState('')
  const [zoneFilter, setZoneFilter] = useState('')
  const [agentFilter, setAgentFilter] = useState('')

  const [zones, setZones] = useState<Zone[]>([])
  const [agents, setAgents] = useState<Agent[]>([])

  useEffect(() => {
    if (!token || !isAdmin) return
    let cancelled = false
    Promise.all([listZones(token), listAgents(token)])
      .then(([zoneList, agentList]) => {
        if (!cancelled) {
          setZones(zoneList)
          setAgents(agentList)
        }
      })
      .catch(() => {
        // Non-fatal: the order list itself still loads and works; only
        // the filter dropdowns' option lists would stay empty.
      })
    return () => {
      cancelled = true
    }
  }, [token, isAdmin])

  useEffect(() => {
    if (!token) return
    let cancelled = false
    const filter: OrderFilter | undefined = isAdmin
      ? {
          status: statusFilter ? (statusFilter as OrderFilter['status']) : undefined,
          zone: zoneFilter || undefined,
          agent: agentFilter || undefined,
        }
      : undefined
    listOrders(token, filter)
      .then((list) => {
        if (!cancelled) setOrders(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Could not load orders.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, isAdmin, statusFilter, zoneFilter, agentFilter])

  const hasActiveFilter = statusFilter !== '' || zoneFilter !== '' || agentFilter !== ''

  return (
    <Layout>
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{isAdmin ? 'All orders' : isDeliveryAgent ? 'My assigned orders' : 'My orders'}</h1>
        {!isDeliveryAgent && (
          <Link to="/orders/new" className="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700">
            New order
          </Link>
        )}
      </div>

      {isAdmin && (
        <div className="mt-4 flex flex-wrap items-end gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
          <div>
            <label htmlFor="status-filter" className="block text-xs font-medium text-slate-500">
              Status
            </label>
            <select
              id="status-filter"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              <option value="">All statuses</option>
              {ORDER_STATUSES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="zone-filter" className="block text-xs font-medium text-slate-500">
              Zone
            </label>
            <select
              id="zone-filter"
              value={zoneFilter}
              onChange={(e) => setZoneFilter(e.target.value)}
              className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              <option value="">All zones</option>
              {zones.map((z) => (
                <option key={z.id} value={z.id}>
                  {z.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="agent-filter" className="block text-xs font-medium text-slate-500">
              Agent
            </label>
            <select
              id="agent-filter"
              value={agentFilter}
              onChange={(e) => setAgentFilter(e.target.value)}
              className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              <option value="">All agents</option>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.full_name}
                </option>
              ))}
            </select>
          </div>
          {hasActiveFilter && (
            <button
              type="button"
              onClick={() => {
                setStatusFilter('')
                setZoneFilter('')
                setAgentFilter('')
              }}
              className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
            >
              Clear filters
            </button>
          )}
        </div>
      )}

      <div className="mt-6 overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <ErrorBanner message={error} />
        {loading ? (
          <div className="space-y-3 p-6">
            {[0, 1, 2].map((i) => (
              <div key={i} className="h-14 animate-pulse rounded-md bg-slate-100" />
            ))}
          </div>
        ) : orders.length === 0 ? (
          <div className="px-6 py-12 text-center">
            <p className="text-sm font-medium text-slate-700">{hasActiveFilter ? 'No orders match these filters.' : 'No orders yet.'}</p>
            <p className="mt-1 text-sm text-slate-500">
              {hasActiveFilter ? 'Try clearing a filter to see more results.' : 'Your orders will appear here once you create a delivery.'}
            </p>
          </div>
        ) : (
          <ul>
            {orders.map((o) => (
              <li key={o.id} className="border-t border-slate-100 first:border-t-0">
                <Link
                  to={`/orders/${o.id}`}
                  className="flex items-center justify-between gap-4 px-6 py-4 text-sm transition-colors hover:bg-slate-50"
                >
                  <span className="text-slate-700">
                    {o.order_type} · {o.zone_relationship} · {o.payment_type}
                    <span className="ml-2 font-medium text-slate-900">{formatCurrency(o.final_amount)}</span>
                  </span>
                  <OrderStatusBadge status={o.status} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Layout>
  )
}
