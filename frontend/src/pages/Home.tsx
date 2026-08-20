import { Layout } from '../components/Layout'
import { StatusBadge } from '../components/StatusBadge'
import { useHealthCheck } from '../hooks/useHealthCheck'

export function Home() {
  const health = useHealthCheck()

  return (
    <Layout>
      <div className="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <h1 className="text-xl font-semibold">Application shell</h1>
        <p className="mt-1 text-sm text-slate-600">
          This page proves the frontend can reach the backend&apos;s health endpoint. Business
          screens (login, orders, tracking, dashboards) are not part of M01.
        </p>

        <div className="mt-6 space-y-3">
          <div className="flex items-center gap-3">
            <span className="w-28 text-sm text-slate-500">Backend</span>
            {health.status === 'loading' && <StatusBadge label="Checking…" state="loading" />}
            {health.status === 'success' && (
              <StatusBadge
                label={health.data.status === 'ok' ? 'Reachable' : 'Degraded'}
                state={health.data.status === 'ok' ? 'ok' : 'degraded'}
              />
            )}
            {health.status === 'error' && <StatusBadge label={health.message} state="error" />}
          </div>

          <div className="flex items-center gap-3">
            <span className="w-28 text-sm text-slate-500">Database</span>
            {health.status === 'loading' && <StatusBadge label="Checking…" state="loading" />}
            {health.status === 'success' && (
              <StatusBadge
                label={health.data.database === 'ok' ? 'Connected' : 'Unavailable'}
                state={health.data.database === 'ok' ? 'ok' : 'degraded'}
              />
            )}
            {health.status === 'error' && <StatusBadge label="Unknown" state="error" />}
          </div>
        </div>
      </div>
    </Layout>
  )
}
