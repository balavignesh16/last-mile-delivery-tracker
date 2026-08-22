import { PackageOpen, PackagePlus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
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
  const navigate = useNavigate()
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
          <Link
            to="/orders/new"
            className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3 py-2 text-sm font-medium text-white hover:bg-navy-700"
          >
            <PackagePlus className="h-4 w-4" aria-hidden="true" />
            New order
          </Link>
        )}
      </div>

      {isAdmin && (
        <div className="mt-4 flex flex-wrap items-end gap-3 rounded-lg border border-navy-100 bg-white p-4 shadow-sm">
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

      <div className="mt-6 overflow-hidden rounded-lg border border-navy-100 bg-white shadow-sm">
        <ErrorBanner message={error} />
        {loading ? (
          <div className="space-y-3 p-6">
            {[0, 1, 2].map((i) => (
              <div key={i} className="h-14 animate-pulse rounded-md bg-slate-100" />
            ))}
          </div>
        ) : orders.length === 0 ? (
          <div className="px-6 py-14 text-center">
            <PackageOpen className="mx-auto h-10 w-10 text-slate-300" aria-hidden="true" />
            <p className="mt-3 text-sm font-medium text-slate-700">{hasActiveFilter ? 'No orders match these filters.' : 'No orders yet.'}</p>
            <p className="mt-1 text-sm text-slate-500">
              {hasActiveFilter ? 'Try clearing a filter to see more results.' : 'Your orders will appear here once you create a delivery.'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-navy-100 bg-navy-50/60 text-xs font-medium tracking-wide text-navy-700 uppercase">
                <tr>
                  <th className="px-6 py-3">Order</th>
                  <th className="px-6 py-3">Route</th>
                  <th className="px-6 py-3">Payment</th>
                  <th className="px-6 py-3">Amount</th>
                  <th className="px-6 py-3">Status</th>
                  <th className="px-6 py-3">Placed</th>
                </tr>
              </thead>
              <tbody>
                {orders.map((o) => (
                  <tr
                    key={o.id}
                    onClick={() => navigate(`/orders/${o.id}`)}
                    className="cursor-pointer border-t border-slate-100 transition-colors hover:bg-navy-50/50"
                  >
                    <td className="px-6 py-3.5">
                      <Link to={`/orders/${o.id}`} className="font-medium text-navy-700 hover:underline">
                        {o.order_type} · {o.id.slice(0, 8)}
                      </Link>
                    </td>
                    <td className="px-6 py-3.5 text-slate-600">{o.zone_relationship}</td>
                    <td className="px-6 py-3.5 text-slate-600">{o.payment_type}</td>
                    <td className="px-6 py-3.5 font-medium tabular-nums text-slate-900">{formatCurrency(o.final_amount)}</td>
                    <td className="px-6 py-3.5">
                      <OrderStatusBadge status={o.status} />
                    </td>
                    <td className="px-6 py-3.5 whitespace-nowrap text-slate-500">{new Date(o.created_at).toLocaleDateString()}</td>
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
