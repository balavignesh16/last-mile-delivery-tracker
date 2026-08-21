import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { StatusBadge } from '../components/StatusBadge'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import { listOrders } from '../services/orders'
import type { Order } from '../types/order'
import { formatCurrency } from '../utils/currency'

// ADMIN sees every order; CUSTOMER sees only their own — the backend
// decides which by the caller's role (GET /orders), so this page never
// sends a filter of its own. No status/zone/agent filtering here by
// design — M07 doesn't add it (see docs/order-management.md); those
// fields aren't meaningful until M08/M09 exist.
export function OrdersPage() {
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'ADMIN'

  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    listOrders(token)
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
  }, [token])

  return (
    <Layout>
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{isAdmin ? 'All orders' : 'My orders'}</h1>
        <Link to="/orders/new" className="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700">
          New order
        </Link>
      </div>

      <div className="mt-6 rounded-lg border border-slate-200 bg-white shadow-sm">
        <ErrorBanner message={error} />
        {loading ? (
          <p className="px-6 py-4 text-sm text-slate-500">Loading…</p>
        ) : orders.length === 0 ? (
          <p className="px-6 py-8 text-center text-sm text-slate-500">No orders yet.</p>
        ) : (
          <ul>
            {orders.map((o) => (
              <li key={o.id} className="border-t border-slate-100 first:border-t-0">
                <Link to={`/orders/${o.id}`} className="flex items-center justify-between px-6 py-3 text-sm hover:bg-slate-50">
                  <span>
                    {o.order_type} · {o.zone_relationship} · {o.payment_type}
                    <span className="ml-2 text-slate-400">{formatCurrency(o.final_amount)}</span>
                  </span>
                  <StatusBadge label={o.status} state={o.status === 'DELIVERED' ? 'ok' : o.status === 'FAILED' ? 'error' : 'loading'} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Layout>
  )
}
