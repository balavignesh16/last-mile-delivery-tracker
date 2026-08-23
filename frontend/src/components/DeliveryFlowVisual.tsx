import { CheckCircle2, Package, Truck, UserCheck } from 'lucide-react'
import type { OrderStatus } from '../types/order'

export type FlowStage = 'order' | 'assign' | 'transit' | 'delivered'

const STAGES: { key: FlowStage; label: string; icon: typeof Package }[] = [
  { key: 'order', label: 'Order', icon: Package },
  { key: 'assign', label: 'Assign', icon: UserCheck },
  { key: 'transit', label: 'Transit', icon: Truck },
  { key: 'delivered', label: 'Delivered', icon: CheckCircle2 },
]

// Maps a real order status to its position on this 4-stage happy path.
// FAILED and RESCHEDULED are deliberately unmapped (undefined) — they
// are recovery branches, not a further position on the linear path, so
// no stage is honestly "current" for them; the visual shows a plain,
// unhighlighted strip rather than falsely implying progress.
const STATUS_TO_STAGE: Partial<Record<OrderStatus, FlowStage>> = {
  CREATED: 'order',
  ASSIGNED: 'assign',
  PICKED_UP: 'transit',
  IN_TRANSIT: 'transit',
  OUT_FOR_DELIVERY: 'transit',
  DELIVERED: 'delivered',
}

interface DeliveryFlowVisualProps {
  compact?: boolean
  // Dark: white-on-navy, for the marketing hero/auth split panel. Light:
  // navy-on-white, for use inside a white content card (order detail).
  variant?: 'dark' | 'light'
  // A real order's current status, mapped via STATUS_TO_STAGE — when
  // given, replaces the default always-pulsing "Transit" demo stage with
  // whichever stage that order has actually reached. Omit entirely for
  // the marketing/decorative usage (landing hero, auth panels), which
  // keeps its original always-"Transit" behavior unchanged.
  status?: OrderStatus
}

// A small, hand-built visual of the product's own core flow — used on
// the landing hero (full size) and, scaled down, on the auth pages'
// branded panel and an order's own detail page. Pure CSS/inline-SVG, no
// chart/illustration library. Exactly one node pulses gently to suggest
// motion; every other node is static — deliberately not a decorative
// animation on every element, per the project's "restrained motion"
// convention already established in index.css.
export function DeliveryFlowVisual({ compact = false, variant = 'dark', status }: DeliveryFlowVisualProps) {
  const nodeSize = compact ? 'h-10 w-10' : 'h-14 w-14'
  const iconSize = compact ? 'h-5 w-5' : 'h-6 w-6'
  const isLight = variant === 'light'

  // status === undefined -> decorative/marketing usage, always "transit".
  // status given but unmapped (FAILED/RESCHEDULED) -> no stage active.
  const activeStage = status === undefined ? 'transit' : STATUS_TO_STAGE[status]

  return (
    <div
      className={`flex items-center ${compact ? 'gap-2' : 'gap-3'}`}
      role="img"
      aria-label={status ? `Delivery stage: ${STAGES.find((s) => s.key === activeStage)?.label ?? 'in recovery'}` : 'Delivery flow: order, assign, transit, delivered'}
    >
      {STAGES.map((stage, i) => {
        const Icon = stage.icon
        const isActive = stage.key === activeStage
        const isPast = activeStage ? STAGES.findIndex((s) => s.key === activeStage) > i : false
        return (
          <div key={stage.key} className="flex items-center">
            <div className="flex flex-col items-center gap-1.5">
              <div
                className={`flex ${nodeSize} items-center justify-center rounded-full text-white shadow-sm ${
                  isActive ? 'flow-pulse bg-amber-500' : isPast ? 'bg-navy-600' : isLight ? 'bg-navy-200' : 'bg-navy-600/60'
                }`}
              >
                <Icon className={iconSize} aria-hidden="true" />
              </div>
              {!compact && <span className={`text-xs font-medium ${isLight ? 'text-slate-500' : 'text-navy-100'}`}>{stage.label}</span>}
            </div>
            {i < STAGES.length - 1 && (
              <div className={`${compact ? 'w-4' : 'w-8'} h-0.5 ${isLight ? 'bg-navy-100' : 'bg-navy-500'}`} aria-hidden="true" />
            )}
          </div>
        )
      })}
    </div>
  )
}
