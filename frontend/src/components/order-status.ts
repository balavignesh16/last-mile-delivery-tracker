import { CheckCircle2, Clock, PackageCheck, RotateCcw, Send, Truck, UserCheck, XCircle, type LucideIcon } from 'lucide-react'
import type { OrderStatus } from '../types/order'

// Plain .ts file, not .tsx: holds only the shared status→style/icon/label
// maps, no component — same reasoning contexts/auth-context.ts already
// documents (React Fast Refresh works best when a .tsx file exports only
// components). OrderStatusBadge and OrderDetailPage's tracking-timeline
// stepper both import from here rather than either one owning it.
export const STATUS_STYLE: Record<OrderStatus, string> = {
  CREATED: 'bg-slate-100 text-slate-700 border-slate-300',
  ASSIGNED: 'bg-blue-100 text-blue-800 border-blue-300',
  PICKED_UP: 'bg-blue-100 text-blue-800 border-blue-300',
  IN_TRANSIT: 'bg-indigo-100 text-indigo-800 border-indigo-300',
  OUT_FOR_DELIVERY: 'bg-amber-100 text-amber-800 border-amber-300',
  DELIVERED: 'bg-emerald-100 text-emerald-800 border-emerald-300',
  FAILED: 'bg-red-100 text-red-800 border-red-300',
  RESCHEDULED: 'bg-amber-100 text-amber-800 border-amber-300',
}

export const STATUS_LABEL: Record<OrderStatus, string> = {
  CREATED: 'Created',
  ASSIGNED: 'Assigned',
  PICKED_UP: 'Picked up',
  IN_TRANSIT: 'In transit',
  OUT_FOR_DELIVERY: 'Out for delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Failed',
  RESCHEDULED: 'Rescheduled',
}

export const STATUS_ICON: Record<OrderStatus, LucideIcon> = {
  CREATED: Clock,
  ASSIGNED: UserCheck,
  PICKED_UP: PackageCheck,
  IN_TRANSIT: Truck,
  OUT_FOR_DELIVERY: Send,
  DELIVERED: CheckCircle2,
  FAILED: XCircle,
  RESCHEDULED: RotateCcw,
}
