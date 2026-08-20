export type Availability = 'AVAILABLE' | 'BUSY' | 'OFFLINE'

export interface Agent {
  id: string
  user_id: string
  full_name: string
  email: string
  phone: string | null
  availability: Availability
  current_lat: number | null
  current_lng: number | null
  current_zone_id: string | null
  location_updated_at: string | null
  last_assigned_at: string | null
  active: boolean
  created_at: string
}

export interface CreateAgentInput {
  email: string
  password: string
  full_name: string
  phone?: string
}
