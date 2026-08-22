import type { PaymentType } from './quote'
import type { OrderType, ZoneRelationship } from './rate'

// Mirrors the backend orders table's full CHECK-constraint value set
// (migration 0008) — M07 only ever writes 'CREATED', the rest exist so
// the type is ready for M08's transitions without a later breaking
// change here.
export type OrderStatus = 'CREATED' | 'ASSIGNED' | 'PICKED_UP' | 'IN_TRANSIT' | 'OUT_FOR_DELIVERY' | 'DELIVERED' | 'FAILED' | 'RESCHEDULED'

export interface Order {
  id: string
  customer_id: string
  created_by: string

  order_type: OrderType
  payment_type: PaymentType

  pickup_area_id: string
  drop_area_id: string
  pickup_zone_id: string
  drop_zone_id: string
  zone_relationship: ZoneRelationship

  length_cm: number
  breadth_cm: number
  height_cm: number
  actual_weight_kg: number

  volumetric_weight_kg: number
  chargeable_weight_kg: number

  rate_card_id: string
  base_rate: number
  cod_surcharge: number
  final_amount: number

  // assigned_agent_id is null until M09 assigns a delivery agent to
  // this order; it holds the current assignment only — history lives
  // in the tracking timeline's own ASSIGNED-event metadata.
  assigned_agent_id: string | null

  status: OrderStatus
  created_at: string
}

// The fields every order-creation request carries, regardless of
// caller role — matches the backend's customerCreateOrderRequest shape
// exactly (no customer_id field here).
export interface OrderPackageInput {
  pickup_area_id: string
  drop_area_id: string
  order_type: OrderType
  payment_type: PaymentType
  length_cm: number
  breadth_cm: number
  height_cm: number
  actual_weight_kg: number
}

// The one additional field only an ADMIN caller's request may carry —
// matches the backend's adminCreateOrderRequest shape.
export interface AdminOrderPackageInput extends OrderPackageInput {
  customer_id: string
}

// The optional GET /orders query parameters M12 adds — honored by the
// backend for ADMIN only (internal/orders/handler.go's parseOrderFilter).
// All three are independently optional and combinable.
export interface OrderFilter {
  status?: OrderStatus
  zone?: string
  agent?: string
}

// The full, ordered status vocabulary — mirrors tracking.Status
// (internal/tracking/event.go) exactly. Used by the admin dashboard's
// order-statistics widget and status-filter dropdown so neither ever
// hardcodes a copy of this list.
export const ORDER_STATUSES: OrderStatus[] = ['CREATED', 'ASSIGNED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED', 'RESCHEDULED']
