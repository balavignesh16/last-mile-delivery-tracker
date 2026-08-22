import { MapPin, Package, Truck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { DashboardLink } from '../../components/DashboardLink'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { StatusBadge } from '../../components/StatusBadge'
import { useAuth } from '../../hooks/useAuth'
import { fetchMyAgentProfile } from '../../services/agents'
import { ApiError } from '../../services/api'
import type { Agent, Availability } from '../../types/agent'

const AVAILABILITY_STATE: Record<Availability, 'ok' | 'degraded' | 'error'> = {
  AVAILABLE: 'ok',
  BUSY: 'degraded',
  OFFLINE: 'error',
}

// The DELIVERY_AGENT dashboard (M12) — a thin navigation layer into
// pages that already exist: OrdersPage (agent-scoped to their own
// assigned orders, unchanged) and OperationsPage (availability +
// location, M03). Delivery Details and Update Status (the blueprint's
// other two Agent Dashboard sub-items) are reached through Assigned
// Deliveries -> an order's own detail page, which now (M12) also
// exposes the status-update control this role was always backend-
// authorized for but never had a UI path to use — see OrderDetailPage.
//
// The stat row below reuses services/agents.fetchMyAgentProfile
// (already called by OperationsPage) purely to surface the agent's own
// current operational state at a glance — no new endpoint.
export function DashboardPage() {
  const { user, token } = useAuth()
  const [agent, setAgent] = useState<Agent | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    fetchMyAgentProfile(token)
      .then((profile) => {
        if (!cancelled) setAgent(profile)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Could not load your operational status.')
      })
    return () => {
      cancelled = true
    }
  }, [token])

  return (
    <Layout>
      <h1 className="text-2xl font-semibold tracking-tight text-slate-900">{user ? `Welcome, ${user.full_name}` : 'Dashboard'}</h1>
      <p className="mt-1 text-sm text-slate-500">Manage your assigned deliveries and your own operational status.</p>

      <ErrorBanner message={error} />
      {agent && (
        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          <div className="flex items-center justify-between rounded-lg border border-navy-100 bg-white p-5 shadow-sm">
            <p className="text-xs font-medium text-slate-500">Availability</p>
            <StatusBadge label={agent.availability} state={AVAILABILITY_STATE[agent.availability]} />
          </div>
          <div className="flex items-center gap-4 rounded-lg border border-navy-100 bg-white p-5 shadow-sm">
            <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md bg-navy-50 text-navy-600">
              <MapPin className="h-5 w-5" aria-hidden="true" />
            </span>
            <div>
              <p className="text-xs font-medium text-slate-500">Current zone</p>
              <p className="text-sm font-semibold text-slate-900">{agent.current_zone_id ? 'Set' : 'Not set'}</p>
            </div>
          </div>
        </div>
      )}

      <div className="mt-6 grid gap-4 sm:grid-cols-2">
        <DashboardLink
          to="/orders"
          title="Assigned deliveries"
          description="View orders assigned to you, open delivery details, and update status as a delivery progresses."
          icon={Package}
        />
        <DashboardLink
          to="/agent"
          title="Availability & location"
          description="Set whether you're available for new deliveries and report your current location."
          icon={Truck}
        />
      </div>
    </Layout>
  )
}
