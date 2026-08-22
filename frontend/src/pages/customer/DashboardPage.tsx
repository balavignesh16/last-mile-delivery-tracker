import { DashboardLink } from '../../components/DashboardLink'
import { Layout } from '../../components/Layout'
import { useAuth } from '../../hooks/useAuth'

// The CUSTOMER dashboard (M12) — a thin navigation layer into pages
// that already exist and are already fully implemented/tested:
// CreateOrderPage and OrdersPage. Order Details, the Tracking Timeline,
// and Reschedule Failed Order (the blueprint's other three Customer
// Dashboard sub-items) are all reached through My Orders -> an order's
// own detail page, exactly as they already were before this page
// existed — this page does not duplicate any of OrderDetailPage's
// content, and every order shown is still resolved by the backend's
// own customer-scoped GET /orders query, never widened here.
export function DashboardPage() {
  const { user } = useAuth()

  return (
    <Layout>
      <h1 className="text-2xl font-semibold tracking-tight text-slate-900">{user ? `Welcome, ${user.full_name}` : 'Dashboard'}</h1>
      <p className="mt-1 text-sm text-slate-500">Create a delivery order, track its journey, and reschedule a failed delivery.</p>

      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <DashboardLink to="/orders/new" title="Create order" description="Place a new delivery order and see the exact charge before you confirm it." />
        <DashboardLink
          to="/orders"
          title="My orders"
          description="View every order you've placed, open its live status and full tracking timeline, and reschedule a failed delivery."
        />
      </div>
    </Layout>
  )
}
