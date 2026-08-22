import { CheckCircle2, Package, PackagePlus, PackageSearch } from 'lucide-react'
import { useEffect, useState } from 'react'
import { DashboardLink } from '../../components/DashboardLink'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { listOrders } from '../../services/orders'
import type { Order } from '../../types/order'

// The CUSTOMER dashboard (M12) — a thin navigation layer into pages
// that already exist and are already fully implemented/tested:
// CreateOrderPage and OrdersPage. Order Details, the Tracking Timeline,
// and Reschedule Failed Order (the blueprint's other three Customer
// Dashboard sub-items) are all reached through My Orders -> an order's
// own detail page, exactly as they already were before this page
// existed — this page does not duplicate any of OrderDetailPage's
// content, and every order shown is still resolved by the backend's
// own customer-scoped GET /orders query, never widened here.
//
// The stat row below reuses that same GET /orders call
// (services/orders.listOrders, already used by OrdersPage/admin
// DashboardPage) purely to compute two client-side counts — no new
// endpoint, no widened query.
export function DashboardPage() {
  const { user, token } = useAuth()
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
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Could not load your order summary.')
      })
    return () => {
      cancelled = true
    }
  }, [token])

  const deliveredCount = orders?.filter((o) => o.status === 'DELIVERED').length ?? 0
  const activeCount = orders ? orders.length - deliveredCount : 0

  return (
    <Layout>
      <h1 className="text-2xl font-semibold tracking-tight text-slate-900">{user ? `Welcome, ${user.full_name}` : 'Dashboard'}</h1>
      <p className="mt-1 text-sm text-slate-500">Create a delivery order, track its journey, and reschedule a failed delivery.</p>

      <ErrorBanner message={error} />
      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <div className="flex items-center gap-4 rounded-lg border border-navy-100 bg-white p-5 shadow-sm">
          <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md bg-navy-50 text-navy-600">
            <PackageSearch className="h-5 w-5" aria-hidden="true" />
          </span>
          <div>
            <p className="text-xs font-medium text-slate-500">Active orders</p>
            <p className="text-2xl font-semibold tabular-nums text-slate-900">{orders === null ? '—' : activeCount}</p>
          </div>
        </div>
        <div className="flex items-center gap-4 rounded-lg border border-navy-100 bg-white p-5 shadow-sm">
          <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md bg-emerald-50 text-emerald-600">
            <CheckCircle2 className="h-5 w-5" aria-hidden="true" />
          </span>
          <div>
            <p className="text-xs font-medium text-slate-500">Delivered</p>
            <p className="text-2xl font-semibold tabular-nums text-slate-900">{orders === null ? '—' : deliveredCount}</p>
          </div>
        </div>
      </div>

      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <DashboardLink
          to="/orders/new"
          title="Create order"
          description="Place a new delivery order and see the exact charge before you confirm it."
          icon={PackagePlus}
        />
        <DashboardLink
          to="/orders"
          title="My orders"
          description="View every order you've placed, open its live status and full tracking timeline, and reschedule a failed delivery."
          icon={Package}
        />
      </div>
    </Layout>
  )
}
