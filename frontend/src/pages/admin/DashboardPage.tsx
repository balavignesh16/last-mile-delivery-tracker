import { MapPin, Package, Users, Wallet } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Bar, BarChart, CartesianGrid, Cell, Legend, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { DashboardLink } from '../../components/DashboardLink'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { STATUS_ICON, STATUS_LABEL } from '../../components/order-status'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { listOrders } from '../../services/orders'
import { ORDER_STATUSES, type Order, type OrderStatus } from '../../types/order'

// A light background tint per status, applied to each stat's wrapping
// container only — deliberately not touching the <dt>/<dd> pair's own
// markup or sibling relationship, since
// TestDashboardPage_ComputesOrderStatisticsFromRealData reads the count
// via `getByText(label).nextElementSibling`.
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

// Real hex equivalents of the same status colors used everywhere else
// in the app (STAT_ACCENT above, OrderStatusBadge's STATUS_STYLE) —
// recharts renders to raw SVG, so its `fill` prop needs an actual color
// value, not a Tailwind class. Chosen to exactly match Tailwind's own
// default palette for each shade already in use, not a new color
// language invented for charts.
const STATUS_HEX: Record<OrderStatus, string> = {
  CREATED: '#94a3b8',
  ASSIGNED: '#60a5fa',
  PICKED_UP: '#3b82f6',
  IN_TRANSIT: '#6366f1',
  OUT_FOR_DELIVERY: '#fbbf24',
  DELIVERED: '#10b981',
  FAILED: '#f87171',
  RESCHEDULED: '#f59e0b',
}

const TREND_DAYS = 14

// The ADMIN dashboard (M12) — order statistics computed from real,
// unfiltered GET /orders data (never hardcoded), plus a thin navigation
// layer into pages that already exist: OrdersPage (which itself now
// carries the M12 status/zone/agent filters and, via each order's own
// detail page, assignment and status override), AgentsPage, ZonesPage
// (Areas are managed inside it already), and RatesPage. This page adds
// no order list of its own — that would duplicate OrdersPage, not
// compose it.
//
// Round 3 adds two client-side charts (recharts) — an order-volume
// trend and a status-distribution donut — computed entirely from the
// same already-fetched order list, same as the stat tiles. No new
// endpoint, no server-side aggregation, no invented numbers: a sparse
// real dataset just renders a sparse real chart.
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

  // Last TREND_DAYS calendar days, oldest first, each bucket's count
  // computed from real order created_at timestamps — an order placed
  // more than TREND_DAYS ago simply falls outside the window, the same
  // way any real "recent activity" chart works.
  const trend = useMemo(() => {
    const days: { date: string; label: string; count: number }[] = []
    const today = new Date()
    for (let i = TREND_DAYS - 1; i >= 0; i--) {
      const d = new Date(today)
      d.setDate(d.getDate() - i)
      days.push({ date: d.toISOString().slice(0, 10), label: d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }), count: 0 })
    }
    const byDate = new Map(days.map((d) => [d.date, d]))
    for (const o of orders ?? []) {
      const bucket = byDate.get(new Date(o.created_at).toISOString().slice(0, 10))
      if (bucket) bucket.count += 1
    }
    return days
  }, [orders])

  const distribution = ORDER_STATUSES.filter((s) => counts[s] > 0).map((status) => ({
    status,
    name: STATUS_LABEL[status],
    value: counts[status],
    color: STATUS_HEX[status],
  }))

  return (
    <Layout>
      <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Admin dashboard</h1>
      <p className="mt-1 text-sm text-slate-500">Operational overview and navigation to every admin capability.</p>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <div className="flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-slate-700">Order statistics</h2>
          {orders !== null && (
            <p className="text-sm text-slate-500">
              <span className="text-base font-semibold tabular-nums text-slate-900">{total}</span> total
            </p>
          )}
        </div>
        <ErrorBanner message={error} />
        {orders === null && !error ? (
          <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
            {ORDER_STATUSES.map((s) => (
              <div key={s} className="h-16 animate-pulse rounded-md bg-slate-100" />
            ))}
          </div>
        ) : (
          <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            {ORDER_STATUSES.map((status) => {
              const Icon = STATUS_ICON[status]
              return (
                <div key={status} className={`rounded-md px-3 py-2.5 ${STAT_ACCENT[status]}`}>
                  <Icon className="h-4 w-4 text-slate-400" aria-hidden="true" />
                  <dt className="mt-1.5 text-xs font-medium text-slate-500">{STATUS_LABEL[status]}</dt>
                  <dd className="mt-0.5 text-xl font-semibold tabular-nums text-slate-900">{counts[status]}</dd>
                </div>
              )
            })}
          </dl>
        )}
      </div>

      {total > 0 && (
        <div className="mt-6 grid gap-6 lg:grid-cols-2">
          <div className="rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
            <h2 className="text-sm font-semibold text-slate-700">Order volume, last {TREND_DAYS} days</h2>
            <div className="mt-4 h-64" role="img" aria-label={`Order volume per day for the last ${TREND_DAYS} days`}>
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={trend} margin={{ left: -20, right: 8, top: 4, bottom: 0 }}>
                  <CartesianGrid vertical={false} stroke="#eef3fa" />
                  <XAxis
                    dataKey="label"
                    tick={{ fontSize: 11, fill: '#64748b' }}
                    axisLine={{ stroke: '#d9e4f3' }}
                    tickLine={false}
                    interval="preserveStartEnd"
                  />
                  <YAxis allowDecimals={false} tick={{ fontSize: 11, fill: '#64748b' }} axisLine={false} tickLine={false} width={28} />
                  <Tooltip
                    cursor={{ fill: '#eef3fa' }}
                    contentStyle={{ fontSize: 12, borderRadius: 6, borderColor: '#d9e4f3' }}
                    labelStyle={{ color: '#0b1f3a', fontWeight: 600 }}
                    formatter={(value) => [String(value), 'Orders']}
                  />
                  <Bar dataKey="count" fill="#1e3a5f" radius={[3, 3, 0, 0]} maxBarSize={28} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>

          <div className="rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
            <h2 className="text-sm font-semibold text-slate-700">Status distribution</h2>
            <div className="mt-4 h-64" role="img" aria-label="Order count by status">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
                  <Pie data={distribution} dataKey="value" nameKey="name" cx="50%" cy="46%" innerRadius={50} outerRadius={80} paddingAngle={2}>
                    {distribution.map((d) => (
                      <Cell key={d.status} fill={d.color} stroke="white" strokeWidth={2} />
                    ))}
                  </Pie>
                  <Tooltip contentStyle={{ fontSize: 12, borderRadius: 6, borderColor: '#d9e4f3' }} formatter={(value) => [String(value), 'Orders']} />
                  <Legend
                    layout="horizontal"
                    verticalAlign="bottom"
                    align="center"
                    iconType="circle"
                    iconSize={8}
                    wrapperStyle={{ fontSize: 12, color: '#334155' }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>
      )}

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
