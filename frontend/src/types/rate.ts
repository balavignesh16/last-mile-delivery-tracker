export type OrderType = 'B2B' | 'B2C'
export type ZoneRelationship = 'INTRA' | 'INTER'

export interface RateCard {
  id: string
  order_type: OrderType
  zone_relationship: ZoneRelationship
  cod_surcharge: number
  active: boolean
  created_at: string
}

export interface CreateRateCardInput {
  order_type: OrderType
  zone_relationship: ZoneRelationship
  cod_surcharge?: number
}

// active is optional: omitting it leaves the rate card's active state
// unchanged on the backend — see rates.RateCardUpdate's doc on the Go
// side.
export interface UpdateRateCardInput {
  cod_surcharge: number
  active?: boolean
}

export interface Slab {
  id: string
  rate_card_id: string
  min_weight: number
  max_weight: number | null
  price: number
  created_at: string
}

export interface CreateSlabInput {
  min_weight: number
  max_weight?: number
  price: number
}

export interface UpdateSlabInput {
  min_weight: number
  max_weight?: number
  price: number
}
