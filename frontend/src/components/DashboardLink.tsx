import { Link } from 'react-router-dom'

// A single navigation card into an already-existing page — the only
// building block the three M12 dashboards need. Dashboards are a thin
// composition/navigation layer over functionality M01-M11 already
// built, not a reimplementation of it, so this component intentionally
// carries no data-fetching or business logic of its own.
export function DashboardLink({ to, title, description }: { to: string; title: string; description: string }) {
  return (
    <Link
      to={to}
      className="group block rounded-lg border border-slate-200 bg-white p-4 shadow-sm transition-all hover:-translate-y-0.5 hover:border-brand-200 hover:shadow-md"
    >
      <h3 className="flex items-center justify-between text-sm font-semibold text-slate-900">
        {title}
        <span className="text-slate-300 transition-transform group-hover:translate-x-0.5 group-hover:text-brand-500" aria-hidden="true">
          →
        </span>
      </h3>
      <p className="mt-1 text-sm text-slate-500">{description}</p>
    </Link>
  )
}
