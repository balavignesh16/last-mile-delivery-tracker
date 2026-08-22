import type { OrderStatus } from '../types/order'

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
const STYLES: Record<OrderStatus, string> = {
  CREATED: 'bg-slate-100 text-slate-700 border-slate-300',
  ASSIGNED: 'bg-blue-100 text-blue-800 border-blue-300',
  PICKED_UP: 'bg-blue-100 text-blue-800 border-blue-300',
  IN_TRANSIT: 'bg-indigo-100 text-indigo-800 border-indigo-300',
  OUT_FOR_DELIVERY: 'bg-amber-100 text-amber-800 border-amber-300',
  DELIVERED: 'bg-emerald-100 text-emerald-800 border-emerald-300',
  FAILED: 'bg-red-100 text-red-800 border-red-300',
  RESCHEDULED: 'bg-amber-100 text-amber-800 border-amber-300',
}

const LABELS: Record<OrderStatus, string> = {
  CREATED: 'Created',
  ASSIGNED: 'Assigned',
  PICKED_UP: 'Picked up',
  IN_TRANSIT: 'In transit',
  OUT_FOR_DELIVERY: 'Out for delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Failed',
  RESCHEDULED: 'Rescheduled',
}

export function OrderStatusBadge({ status }: { status: OrderStatus }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium whitespace-nowrap ${STYLES[status]}`}
    >
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-current" aria-hidden="true" />
      {LABELS[status]}
    </span>
  )
}
