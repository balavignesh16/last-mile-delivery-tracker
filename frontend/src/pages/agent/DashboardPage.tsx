import { DashboardLink } from '../../components/DashboardLink'
import { Layout } from '../../components/Layout'
import { useAuth } from '../../hooks/useAuth'

// The DELIVERY_AGENT dashboard (M12) — a thin navigation layer into
// pages that already exist: OrdersPage (agent-scoped to their own
// assigned orders, unchanged) and OperationsPage (availability +
// location, M03). Delivery Details and Update Status (the blueprint's
// other two Agent Dashboard sub-items) are reached through Assigned
// Deliveries -> an order's own detail page, which now (M12) also
// exposes the status-update control this role was always backend-
// authorized for but never had a UI path to use — see OrderDetailPage.
export function DashboardPage() {
  const { user } = useAuth()

  return (
    <Layout>
      <h1 className="text-xl font-semibold">{user ? `Welcome, ${user.full_name}` : 'Dashboard'}</h1>
      <p className="mt-1 text-sm text-slate-600">Manage your assigned deliveries and your own operational status.</p>

      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <DashboardLink
          to="/orders"
          title="Assigned deliveries"
          description="View orders assigned to you, open delivery details, and update status as a delivery progresses."
        />
        <DashboardLink to="/agent" title="Availability & location" description="Set whether you're available for new deliveries and report your current location." />
      </div>
    </Layout>
  )
}
