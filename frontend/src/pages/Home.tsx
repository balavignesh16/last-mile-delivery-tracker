import { Link } from 'react-router-dom'
import { Layout } from '../components/Layout'
import { useHealthCheck } from '../hooks/useHealthCheck'

const FEATURES = [
  {
    title: 'Auto-calculated pricing',
    description: 'Zone-aware, volumetric-weight-based charges, quoted before every order is confirmed.',
  },
  {
    title: 'Intelligent assignment',
    description: 'Orders reach the nearest available agent automatically, or admins assign by hand.',
  },
  {
    title: 'Live tracking',
    description: 'A full, immutable status timeline customers and admins can follow from pickup to delivery.',
  },
]

// The public entry point — replaces the M01-era health-check screen as
// the default landing experience. useHealthCheck is unchanged and still
// backs a real status indicator (below), just no longer the page's
// dominant content; the underlying GET /health call and its hook are
// untouched.
export function Home() {
  const health = useHealthCheck()

  const systemOk = health.status === 'success' && health.data.status === 'ok' && health.data.database === 'ok'

  return (
    <Layout>
      <section className="mx-auto max-w-2xl py-12 text-center sm:py-20">
        <h1 className="text-4xl font-bold tracking-tight text-slate-900 sm:text-5xl">Last-Mile Delivery Tracker</h1>
        <p className="mx-auto mt-4 max-w-xl text-lg text-slate-600">
          Manage every delivery from order creation to doorstep — auto-calculated pricing, intelligent agent
          assignment, and live tracking in one place.
        </p>

        <div className="mt-8 flex items-center justify-center gap-3">
          <Link
            to="/login"
            className="rounded-md bg-slate-900 px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-slate-700"
          >
            Sign In
          </Link>
          <Link
            to="/register"
            className="rounded-md border border-slate-300 bg-white px-5 py-2.5 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-100"
          >
            Create Account
          </Link>
        </div>

        <div className="mt-10 inline-flex items-center gap-2 text-xs text-slate-400">
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              health.status === 'loading' ? 'bg-slate-300' : systemOk ? 'bg-emerald-500' : 'bg-red-500'
            }`}
            aria-hidden="true"
          />
          {health.status === 'loading' && 'Checking system status…'}
          {health.status === 'success' && (systemOk ? 'System operational' : 'System degraded')}
          {health.status === 'error' && 'System status unavailable'}
        </div>
      </section>

      <section className="grid gap-4 border-t border-slate-200 pt-10 sm:grid-cols-3">
        {FEATURES.map((f) => (
          <div key={f.title}>
            <h2 className="text-sm font-semibold text-slate-900">{f.title}</h2>
            <p className="mt-1.5 text-sm text-slate-500">{f.description}</p>
          </div>
        ))}
      </section>
    </Layout>
  )
}
