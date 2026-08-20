type BadgeState = 'ok' | 'degraded' | 'error' | 'loading'

interface StatusBadgeProps {
  label: string
  state: BadgeState
}

const STYLES: Record<BadgeState, string> = {
  ok: 'bg-emerald-100 text-emerald-800 border-emerald-300',
  degraded: 'bg-amber-100 text-amber-800 border-amber-300',
  error: 'bg-red-100 text-red-800 border-red-300',
  loading: 'bg-slate-100 text-slate-600 border-slate-300',
}

export function StatusBadge({ label, state }: StatusBadgeProps) {
  return (
    <span
      className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-sm font-medium ${STYLES[state]}`}
    >
      <span className="h-2 w-2 rounded-full bg-current" />
      {label}
    </span>
  )
}
