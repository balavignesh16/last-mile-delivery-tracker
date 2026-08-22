import { CheckCircle2, Package, Truck, UserCheck } from 'lucide-react'

const STAGES = [
  { label: 'Order', icon: Package },
  { label: 'Assign', icon: UserCheck },
  { label: 'Transit', icon: Truck },
  { label: 'Delivered', icon: CheckCircle2 },
]

// A small, hand-built visual of the product's own core flow — used on
// the landing hero (full size) and, scaled down, on the auth pages'
// branded panel. Pure CSS/inline-SVG, no chart/illustration library.
// The "Transit" node pulses gently to suggest live motion; every other
// node is static — deliberately not a decorative animation on every
// element, per the project's "restrained motion" convention already
// established in index.css.
export function DeliveryFlowVisual({ compact = false }: { compact?: boolean }) {
  const nodeSize = compact ? 'h-10 w-10' : 'h-14 w-14'
  const iconSize = compact ? 'h-5 w-5' : 'h-6 w-6'

  return (
    <div className={`flex items-center ${compact ? 'gap-2' : 'gap-3'}`} role="img" aria-label="Delivery flow: order, assign, transit, delivered">
      {STAGES.map((stage, i) => {
        const Icon = stage.icon
        const isTransit = stage.label === 'Transit'
        return (
          <div key={stage.label} className="flex items-center">
            <div className="flex flex-col items-center gap-1.5">
              <div
                className={`flex ${nodeSize} items-center justify-center rounded-full ${
                  isTransit ? 'flow-pulse bg-amber-500' : 'bg-navy-600'
                } text-white shadow-sm`}
              >
                <Icon className={iconSize} aria-hidden="true" />
              </div>
              {!compact && <span className="text-xs font-medium text-navy-100">{stage.label}</span>}
            </div>
            {i < STAGES.length - 1 && <div className={`${compact ? 'w-4' : 'w-8'} h-0.5 bg-navy-500`} aria-hidden="true" />}
          </div>
        )
      })}
    </div>
  )
}
