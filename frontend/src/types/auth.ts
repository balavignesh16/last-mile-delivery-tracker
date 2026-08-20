export type Role = 'CUSTOMER' | 'DELIVERY_AGENT' | 'ADMIN'

export interface UserProfile {
  id: string
  email: string
  full_name: string
  phone: string | null
  role: Role
  created_at: string
}

export interface LoginResponse {
  token: string
}

export interface RegisterInput {
  email: string
  password: string
  full_name: string
  phone?: string
}

export interface LoginInput {
  email: string
  password: string
}

export interface ProfileUpdateInput {
  full_name: string
  phone?: string
}
