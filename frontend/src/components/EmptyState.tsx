import type { ComponentType, ReactNode } from 'react'

interface EmptyStateProps {
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
  action?: ReactNode
}

// A compact, purposeful empty state for a resource table (Round 6) —
// replaces a bare "No X yet." line with an icon, a short explanation,
// and (when there's something the viewer can do about it) a primary
// action, matching OrdersPage's existing empty-state shape/sizing.
export function EmptyState({ icon: Icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="px-6 py-14 text-center">
      <Icon className="mx-auto h-10 w-10 text-slate-300" aria-hidden="true" />
      <p className="mt-3 text-sm font-medium text-slate-700">{title}</p>
      <p className="mt-1 text-sm text-slate-500">{description}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
