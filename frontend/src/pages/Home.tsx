import { CheckCircle2, MapPin, Package, ShieldCheck, Zap } from 'lucide-react'
import { Link } from 'react-router-dom'
import { DeliveryFlowVisual } from '../components/DeliveryFlowVisual'
import { Layout } from '../components/Layout'
import { Reveal } from '../components/Reveal'
import { useHealthCheck } from '../hooks/useHealthCheck'

const CAPABILITIES = [
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

const DELIVERY_FLOW = [
  { step: '1', title: 'Place an order', description: 'Enter pickup/drop details and package info — see the exact charge before you confirm.' },
  { step: '2', title: 'Get assigned', description: 'The nearest available agent is found automatically, or an admin assigns one directly.' },
  { step: '3', title: 'Track in real time', description: 'Follow every status change — picked up, in transit, out for delivery — as it happens.' },
  { step: '4', title: 'Delivered', description: 'Get notified by email and SMS the moment your delivery is complete.' },
]

const TRUST_STRIP = ['3 roles supported', '8 tracked delivery stages', 'Email + SMS notifications', 'Admin-configurable pricing']

const ROLE_VALUE = [
  { role: 'Customers', description: 'Know the price before committing, and follow every delivery in real time — from placement to doorstep.' },
  { role: 'Delivery agents', description: 'See exactly what’s assigned, update status as work progresses, and control your own availability and zone.' },
  { role: 'Admins', description: 'Configure pricing, manage agents and zones, and step in to assign, reassign, or reschedule whenever it’s needed.' },
]

// The public entry point. Every claim on this page maps to a real,
// implemented capability — no invented statistics, no ETA/GPS claims,
// no fabricated company details. useHealthCheck is unchanged and still
// backs the hero's status indicator; the underlying GET /health call and
// its hook are untouched.
export function Home() {
  const health = useHealthCheck()

  const systemOk = health.status === 'success' && health.data.status === 'ok' && health.data.database === 'ok'

  return (
    <Layout>
      {/* Hero — always visible, never scroll-revealed: this is the first
          thing a visitor sees, so it must render immediately. Full-bleed
          (relative/left-1/2/-translate-x-1/2/w-screen) so the band spans
          the true viewport edge to edge on wide screens instead of
          stopping at Layout's max-w-6xl content column, which otherwise
          reads as a boxed-in card with dead space on either side. */}
      <section className="relative left-1/2 -mt-10 w-screen -translate-x-1/2 bg-navy-900 px-6 py-16 sm:py-24">
        <div className="mx-auto max-w-3xl text-center">
          <p className="text-xs font-semibold tracking-widest text-amber-500 uppercase">Last-Mile Logistics Platform</p>
          <h1 className="mx-auto mt-4 max-w-2xl text-5xl font-extrabold tracking-tight text-balance text-white sm:text-6xl">
            Last-Mile Delivery Tracker
          </h1>
          <p className="mx-auto mt-5 max-w-xl text-lg text-navy-100">
            Pricing, assignment, and tracking for every delivery, end to end — one connected system instead of
            separate tools for quoting, dispatch, and status updates.
          </p>

          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              to="/register"
              className="rounded-md bg-amber-500 px-6 py-3 text-sm font-semibold text-navy-900 transition-colors hover:bg-amber-600"
            >
              Create Account
            </Link>
            <Link
              to="/login"
              className="rounded-md border border-white/30 px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-white/10"
            >
              Sign In
            </Link>
          </div>
        </div>

        <div className="mt-14 flex justify-center">
          <div className="sm:hidden">
            <DeliveryFlowVisual compact />
          </div>
          <div className="hidden sm:block">
            <DeliveryFlowVisual />
          </div>
        </div>

        <div className="mt-8 flex items-center justify-center gap-2 text-xs text-navy-200">
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

        <div className="mx-auto mt-8 flex max-w-2xl flex-wrap items-center justify-center gap-x-6 gap-y-2 border-t border-white/10 pt-6 text-xs text-navy-200">
          {TRUST_STRIP.map((c) => (
            <span key={c} className="inline-flex items-center gap-1.5">
              <CheckCircle2 className="h-3.5 w-3.5 text-amber-500" aria-hidden="true" />
              {c}
            </span>
          ))}
        </div>
      </section>

      {/* What the platform does */}
      <Reveal>
        <section className="mx-auto max-w-2xl py-16 text-center sm:py-20">
          <h2 className="text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">One system for the full delivery lifecycle</h2>
          <p className="mt-4 text-lg text-slate-600">
            From the moment an order is placed to the moment it's delivered, pricing, assignment, and tracking all
            happen in one place — with a live, auditable status record kept for every order.
          </p>
        </section>
      </Reveal>

      {/* How delivery flows through the system */}
      <Reveal>
        <section className="border-t border-navy-100 py-16 sm:py-20">
          <h2 className="text-center text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">How a delivery flows through the system</h2>
          <div className="relative mx-auto mt-12 max-w-5xl">
            <span className="absolute top-[18px] right-[12.5%] left-[12.5%] hidden h-0.5 bg-navy-100 sm:block" aria-hidden="true" />
            <div className="grid gap-y-10 sm:grid-cols-4 sm:gap-x-4">
              {DELIVERY_FLOW.map((item) => (
                <div key={item.step} className="relative text-center">
                  <span className="relative z-10 mx-auto flex h-9 w-9 items-center justify-center rounded-full bg-navy-600 text-sm font-bold text-white">
                    {item.step}
                  </span>
                  <h3 className="mt-3 text-sm font-semibold text-slate-900">{item.title}</h3>
                  <p className="mt-1.5 text-sm text-slate-500">{item.description}</p>
                </div>
              ))}
            </div>
          </div>
        </section>
      </Reveal>

      {/* Core capabilities */}
      <Reveal>
        <section className="border-t border-navy-100 py-16 sm:py-20">
          <h2 className="text-center text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">Core capabilities</h2>
          <div className="mx-auto mt-12 max-w-3xl divide-y divide-navy-100">
            {CAPABILITIES.map((f) => {
              const Icon = f.icon
              return (
                <div key={f.title} className="flex gap-4 py-5 first:pt-0 last:pb-0">
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
      </Reveal>

      {/* Why the platform is useful, by role */}
      <Reveal>
        <section className="border-t border-navy-100 py-16 sm:py-20">
          <h2 className="text-center text-3xl font-bold tracking-tight text-slate-900 sm:text-4xl">Built for every role in the delivery process</h2>
          <div className="mx-auto mt-12 grid max-w-4xl gap-10 sm:grid-cols-3">
            {ROLE_VALUE.map((r) => (
              <div key={r.role}>
                <h3 className="text-sm font-semibold tracking-wide text-navy-600 uppercase">{r.role}</h3>
                <p className="mt-2 text-sm text-slate-600">{r.description}</p>
              </div>
            ))}
          </div>
        </section>
      </Reveal>

      {/* Closing CTA */}
      <Reveal>
        <section className="relative left-1/2 mt-4 w-screen -translate-x-1/2 bg-navy-900 px-6 py-14 text-center sm:py-16">
          <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">Ready to get started?</h2>
          <p className="mx-auto mt-3 max-w-md text-navy-100">
            Create an account to place your first order, or sign in if you already have one.
          </p>
          <div className="mt-7 flex flex-wrap items-center justify-center gap-3">
            <Link
              to="/register"
              className="rounded-md bg-amber-500 px-6 py-3 text-sm font-semibold text-navy-900 transition-colors hover:bg-amber-600"
            >
              Create Account
            </Link>
            <Link
              to="/login"
              className="rounded-md border border-white/30 px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-white/10"
            >
              Sign In
            </Link>
          </div>
        </section>
      </Reveal>
    </Layout>
  )
}
