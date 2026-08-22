import { CheckCircle2, MapPin, Package, ShieldCheck, Zap } from 'lucide-react'
import { Link } from 'react-router-dom'
import { DeliveryFlowVisual } from '../components/DeliveryFlowVisual'
import { Layout } from '../components/Layout'
import { useHealthCheck } from '../hooks/useHealthCheck'

const FEATURES = [
  {
    icon: Package,
    title: 'Auto-calculated pricing',
    description: 'Zone-aware, volumetric-weight-based charges, quoted before every order is confirmed.',
  },
  {
    icon: Zap,
    title: 'Intelligent assignment',
    description: 'Orders reach the nearest available agent automatically, or admins assign by hand.',
  },
  {
    icon: MapPin,
    title: 'Live tracking',
    description: 'A full, immutable status timeline customers and admins can follow from pickup to delivery.',
  },
  {
    icon: ShieldCheck,
    title: 'Role-based access',
    description: 'Customers, delivery agents, and admins each see exactly the tools their role needs — nothing more.',
  },
]

const HOW_IT_WORKS = [
  { step: '1', title: 'Place an order', description: 'Enter pickup/drop details and package info — see the exact charge before you confirm.' },
  { step: '2', title: 'Get assigned', description: 'The nearest available agent is found automatically, or an admin assigns one directly.' },
  { step: '3', title: 'Track in real time', description: 'Follow every status change — picked up, in transit, out for delivery — as it happens.' },
  { step: '4', title: 'Delivered', description: 'Get notified by email and SMS the moment your delivery is complete.' },
]

const CAPABILITIES = ['3 roles supported', '8 tracked delivery stages', 'Email + SMS notifications', 'Admin-configurable pricing']

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
      <section className="-mx-6 -mt-10 rounded-b-2xl bg-navy-900 px-6 py-16 text-center sm:py-24">
        <p className="text-xs font-semibold tracking-widest text-amber-500 uppercase">Last-Mile Logistics Platform</p>
        <h1 className="mx-auto mt-3 max-w-2xl text-4xl font-extrabold tracking-tight text-white sm:text-5xl">
          Last-Mile Delivery Tracker
        </h1>
        <p className="mx-auto mt-4 max-w-xl text-lg text-navy-100">
          Manage every delivery from order creation to doorstep — auto-calculated pricing, intelligent agent
          assignment, and live tracking in one place.
        </p>

        <div className="mt-8 flex items-center justify-center gap-3">
          <Link
            to="/login"
            className="rounded-md bg-amber-500 px-5 py-2.5 text-sm font-semibold text-navy-900 transition-colors hover:bg-amber-600"
          >
            Sign In
          </Link>
          <Link
            to="/register"
            className="rounded-md border border-white/30 px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-white/10"
          >
            Create Account
          </Link>
        </div>

        <div className="mt-12 flex justify-center">
          <DeliveryFlowVisual />
        </div>

        <div className="mt-8 inline-flex items-center gap-2 text-xs text-navy-200">
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              health.status === 'loading' ? 'bg-navy-300' : systemOk ? 'bg-emerald-400' : 'bg-red-400'
            }`}
            aria-hidden="true"
          />
          {health.status === 'loading' && 'Checking system status…'}
          {health.status === 'success' && (systemOk ? 'System operational' : 'System degraded')}
          {health.status === 'error' && 'System status unavailable'}
        </div>
      </section>

      <section className="py-14">
        <h2 className="text-center text-sm font-semibold tracking-wide text-navy-600 uppercase">How it works</h2>
        <div className="mt-8 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {HOW_IT_WORKS.map((item) => (
            <div key={item.step} className="relative rounded-lg border border-navy-100 bg-white p-5">
              <span className="flex h-8 w-8 items-center justify-center rounded-full bg-navy-600 text-sm font-bold text-white">
                {item.step}
              </span>
              <h3 className="mt-3 text-sm font-semibold text-slate-900">{item.title}</h3>
              <p className="mt-1.5 text-sm text-slate-500">{item.description}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="border-t border-navy-100 py-14">
        <h2 className="text-center text-sm font-semibold tracking-wide text-navy-600 uppercase">Built for the whole journey</h2>
        <div className="mt-8 grid gap-4 sm:grid-cols-2">
          {FEATURES.map((f) => {
            const Icon = f.icon
            return (
              <div key={f.title} className="flex gap-4 rounded-lg border border-navy-100 bg-white p-5">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-navy-50 text-navy-600">
                  <Icon className="h-5 w-5" aria-hidden="true" />
                </span>
                <div>
                  <h3 className="text-sm font-semibold text-slate-900">{f.title}</h3>
                  <p className="mt-1 text-sm text-slate-500">{f.description}</p>
                </div>
              </div>
            )
          })}
        </div>
      </section>

      <section className="border-t border-navy-100 py-10">
        <div className="flex flex-wrap items-center justify-center gap-3">
          {CAPABILITIES.map((c) => (
            <span
              key={c}
              className="inline-flex items-center gap-1.5 rounded-full border border-navy-100 bg-white px-3.5 py-1.5 text-xs font-medium text-navy-700"
            >
              <CheckCircle2 className="h-3.5 w-3.5 text-navy-500" aria-hidden="true" />
              {c}
            </span>
          ))}
        </div>
      </section>
    </Layout>
  )
}
