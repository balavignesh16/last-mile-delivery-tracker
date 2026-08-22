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
      className="block rounded-lg border border-slate-200 bg-white p-4 shadow-sm transition hover:border-slate-300 hover:shadow-md"
    >
      <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
      <p className="mt-1 text-sm text-slate-500">{description}</p>
    </Link>
  )
}
