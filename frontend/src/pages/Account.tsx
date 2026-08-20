import { Layout } from '../components/Layout'
import { StatusBadge } from '../components/StatusBadge'
import { useAuth } from '../hooks/useAuth'

const ROLE_LABELS: Record<string, string> = {
  CUSTOMER: 'Customer',
  DELIVERY_AGENT: 'Delivery Agent',
  ADMIN: 'Admin',
}

// Minimal authenticated shell, per M02's scope: identity, role, logout,
// and a placeholder for what comes later. Role-specific dashboards
// (customer order history, agent delivery list, admin controls) are
// M12's job, not this one.
export function Account() {
  const { user } = useAuth()

  if (!user) return null

  return (
    <Layout>
      <div className="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">{user.full_name}</h1>
          <StatusBadge label={ROLE_LABELS[user.role] ?? user.role} state="ok" />
        </div>
        <dl className="mt-4 space-y-2 text-sm">
          <div className="flex gap-2">
            <dt className="w-24 text-slate-500">Email</dt>
            <dd>{user.email}</dd>
          </div>
          {user.phone && (
            <div className="flex gap-2">
              <dt className="w-24 text-slate-500">Phone</dt>
              <dd>{user.phone}</dd>
            </div>
          )}
          <div className="flex gap-2">
            <dt className="w-24 text-slate-500">Member since</dt>
            <dd>{new Date(user.created_at).toLocaleDateString()}</dd>
          </div>
        </dl>
      </div>

      <div className="mt-6 rounded-lg border border-dashed border-slate-300 p-6 text-sm text-slate-500">
        Role-specific dashboards (orders, deliveries, admin controls) arrive in later modules.
      </div>
    </Layout>
  )
}
