import type { OrderType, ZoneRelationship } from './rate'

export type PaymentType = 'PREPAID' | 'COD'

// QuoteRequest carries only client-suppliable fields — never anything
// the backend derives itself (zone ids, zone relationship, rate card
// id, chargeable weight, price). See backend's quoteRequest DTO
// (internal/rates/quote_handler.go) for the matching server-side
// contract.
export interface QuoteRequest {
  pickup_area_id: string
  drop_area_id: string
  order_type: OrderType
  payment_type: PaymentType
  length_cm: number
  breadth_cm: number
  height_cm: number
  actual_weight_kg: number
}

export interface QuoteResult {
  pickup_area_id: string
  pickup_zone_id: string
  drop_area_id: string
  drop_zone_id: string
  zone_relationship: ZoneRelationship

  order_type: OrderType
  payment_type: PaymentType

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
}
