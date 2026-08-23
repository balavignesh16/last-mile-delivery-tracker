import type { ComponentType, ReactNode } from 'react'

interface PageHeaderProps {
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
  action?: ReactNode
}

// The shared header for an operations resource workspace (Round 6:
// Agents/Zones/Rate Cards) — title, one explanatory sentence, and one
// primary action. Deliberately not a hero: this is an authenticated
// operations product, not a marketing surface.
export function PageHeader({ icon: Icon, title, description, action }: PageHeaderProps) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-navy-50 text-navy-600">
          <Icon className="h-5 w-5" aria-hidden="true" />
        </span>
        <div>
          <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
          <p className="mt-0.5 text-sm text-slate-500">{description}</p>
        </div>
      </div>
      {action}
    </div>
  )
}
