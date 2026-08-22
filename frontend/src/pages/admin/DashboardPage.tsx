import { MapPin, Package, Users, Wallet } from 'lucide-react'
import { useEffect, useState } from 'react'
import { DashboardLink } from '../../components/DashboardLink'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { STATUS_ICON } from '../../components/order-status'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { listOrders } from '../../services/orders'
import { ORDER_STATUSES, type Order, type OrderStatus } from '../../types/order'

// A light background tint per status, applied to each stat's wrapping
// container only — deliberately not touching the <dt>/<dd> pair's own
// markup or sibling relationship, since
// TestDashboardPage_ComputesOrderStatisticsFromRealData reads the count
// via `getByText(status).nextElementSibling`.
const STAT_ACCENT: Record<OrderStatus, string> = {
  CREATED: 'bg-slate-50',
  ASSIGNED: 'bg-blue-50',
  PICKED_UP: 'bg-blue-50',
  IN_TRANSIT: 'bg-indigo-50',
  OUT_FOR_DELIVERY: 'bg-amber-50',
  DELIVERED: 'bg-emerald-50',
  FAILED: 'bg-red-50',
  RESCHEDULED: 'bg-amber-50',
}

// Solid (non-tinted) fill colors for the distribution bar — the light
// STAT_ACCENT tints above would be invisible against a white track.
const STAT_BAR_COLOR: Record<OrderStatus, string> = {
  CREATED: 'bg-slate-400',
  ASSIGNED: 'bg-blue-400',
  PICKED_UP: 'bg-blue-500',
  IN_TRANSIT: 'bg-indigo-500',
  OUT_FOR_DELIVERY: 'bg-amber-400',
  DELIVERED: 'bg-emerald-500',
  FAILED: 'bg-red-400',
  RESCHEDULED: 'bg-amber-500',
}

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
  const total = orders?.length ?? 0

  return (
    <Layout>
      <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Admin dashboard</h1>
      <p className="mt-1 text-sm text-slate-500">Operational overview and navigation to every admin capability.</p>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Order statistics</h2>
        <ErrorBanner message={error} />
        {orders === null && !error ? (
          <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
            {ORDER_STATUSES.map((s) => (
              <div key={s} className="h-16 animate-pulse rounded-md bg-slate-100" />
            ))}
          </div>
        ) : (
          <>
            <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
              {ORDER_STATUSES.map((status) => {
                const Icon = STATUS_ICON[status]
                return (
                  <div key={status} className={`rounded-md px-3 py-2.5 ${STAT_ACCENT[status]}`}>
                    <Icon className="h-4 w-4 text-slate-400" aria-hidden="true" />
                    <dt className="mt-1.5 text-xs font-medium text-slate-500">{status}</dt>
                    <dd className="mt-0.5 text-xl font-semibold tabular-nums text-slate-900">{counts[status]}</dd>
                  </div>
                )
              })}
            </dl>

            {total > 0 && (
              <div className="mt-5">
                <p className="text-xs font-medium text-slate-500">Distribution</p>
                <div className="mt-2 flex h-2.5 w-full overflow-hidden rounded-full bg-slate-100">
                  {ORDER_STATUSES.filter((s) => counts[s] > 0).map((status) => (
                    <div
                      key={status}
                      className={STAT_BAR_COLOR[status]}
                      style={{ width: `${(counts[status] / total) * 100}%` }}
                      title={`${status}: ${counts[status]}`}
                    />
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>

      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <DashboardLink
          to="/orders"
          title="Orders"
          description="View every order, filter by status/zone/agent, override status, and manage assignment."
          icon={Package}
        />
        <DashboardLink to="/admin/agents" title="Agents" description="Provision delivery agents and manage their availability." icon={Users} />
        <DashboardLink to="/admin/zones" title="Zones & areas" description="Manage zones and the pickup/drop areas within them." icon={MapPin} />
        <DashboardLink
          to="/admin/rates"
          title="Rate cards"
          description="Configure B2B/B2C, intra/inter-zone rate cards and weight slabs."
          icon={Wallet}
        />
      </div>
    </Layout>
  )
}
