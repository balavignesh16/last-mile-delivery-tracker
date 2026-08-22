import type { OrderStatus } from '../types/order'
import { STATUS_ICON, STATUS_LABEL, STATUS_STYLE } from './order-status'

// A dedicated badge for the 8 real order-lifecycle statuses — StatusBadge's
// generic ok/degraded/error/loading vocabulary collapses all 8 into 3
// visual buckets (see OrdersPage/OrderDetailPage's prior badgeState()),
// which loses exactly the distinction this component restores: IN_TRANSIT
// and ASSIGNED read identically to a viewer today. Colors follow the
// convention most delivery-tracking products already use, so the meaning
// is legible without reading the label: neutral (not yet moving), blue
// (actively progressing), amber (needs attention / in recovery), green
// (success), red (failure). The underlying status values and state
// machine are unchanged — this is presentation only.
export function OrderStatusBadge({ status }: { status: OrderStatus }) {
  const Icon = STATUS_ICON[status]
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium whitespace-nowrap ${STATUS_STYLE[status]}`}
    >
      <Icon className="h-3 w-3 shrink-0" aria-hidden="true" />
      {STATUS_LABEL[status]}
    </span>
  )
}
