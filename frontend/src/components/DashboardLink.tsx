import type { LucideIcon } from 'lucide-react'
import { Link } from 'react-router-dom'

// A single navigation card into an already-existing page — the only
// building block the three M12 dashboards need. Dashboards are a thin
// composition/navigation layer over functionality M01-M11 already
// built, not a reimplementation of it, so this component intentionally
// carries no data-fetching or business logic of its own.
export function DashboardLink({ to, title, description, icon: Icon }: { to: string; title: string; description: string; icon?: LucideIcon }) {
  return (
    <Link
      to={to}
      className="group block rounded-lg border border-navy-100 bg-white p-4 shadow-sm transition-all hover:-translate-y-0.5 hover:border-navy-200 hover:shadow-md"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          {Icon && (
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-navy-50 text-navy-600 transition-colors group-hover:bg-navy-100">
              <Icon className="h-4 w-4" aria-hidden="true" />
            </span>
          )}
          <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
        </div>
        <span className="mt-1 shrink-0 text-slate-300 transition-transform group-hover:translate-x-0.5 group-hover:text-amber-500" aria-hidden="true">
          →
        </span>
      </div>
      <p className="mt-2 text-sm text-slate-500">{description}</p>
    </Link>
  )
}
