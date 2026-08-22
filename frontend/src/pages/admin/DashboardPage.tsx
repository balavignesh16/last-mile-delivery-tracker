import { useEffect, useState } from 'react'
import { DashboardLink } from '../../components/DashboardLink'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { listOrders } from '../../services/orders'
import { ORDER_STATUSES, type Order } from '../../types/order'

// The ADMIN dashboard (M12) — order statistics computed from real,
// unfiltered GET /orders data (never hardcoded), plus a thin navigation
// layer into pages that already exist: OrdersPage (which itself now
// carries the M12 status/zone/agent filters and, via each order's own
// detail page, assignment and status override), AgentsPage, ZonesPage
// (Areas are managed inside it already), and RatesPage. This page adds
// no order list of its own — that would duplicate OrdersPage, not
// compose it.
export function DashboardPage() {
  const { token } = useAuth()
  const [orders, setOrders] = useState<Order[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    listOrders(token)
      .then((list) => {
        if (!cancelled) setOrders(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Could not load order statistics.')
      })
    return () => {
      cancelled = true
    }
  }, [token])

  const counts = ORDER_STATUSES.reduce<Record<string, number>>((acc, status) => {
    acc[status] = orders?.filter((o) => o.status === status).length ?? 0
    return acc
  }, {})

  return (
    <Layout>
      <h1 className="text-xl font-semibold">Admin dashboard</h1>
      <p className="mt-1 text-sm text-slate-600">Operational overview and navigation to every admin capability.</p>

      <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Order statistics</h2>
        <ErrorBanner message={error} />
        {orders === null && !error ? (
          <p className="mt-3 text-sm text-slate-500">Loading…</p>
        ) : (
          <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-4">
            {ORDER_STATUSES.map((status) => (
              <div key={status}>
                <dt className="text-slate-500">{status}</dt>
                <dd className="text-base font-semibold text-slate-900">{counts[status]}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>

      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <DashboardLink to="/orders" title="Orders" description="View every order, filter by status/zone/agent, override status, and manage assignment." />
        <DashboardLink to="/admin/agents" title="Agents" description="Provision delivery agents and manage their availability." />
        <DashboardLink to="/admin/zones" title="Zones & areas" description="Manage zones and the pickup/drop areas within them." />
        <DashboardLink to="/admin/rates" title="Rate cards" description="Configure B2B/B2C, intra/inter-zone rate cards and weight slabs." />
      </div>
    </Layout>
  )
}
