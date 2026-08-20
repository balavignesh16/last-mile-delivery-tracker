import { apiGet, apiPost, apiPut } from './api'
import type { LoginInput, LoginResponse, ProfileUpdateInput, RegisterInput, UserProfile } from '../types/auth'

export function registerUser(input: RegisterInput): Promise<UserProfile> {
  return apiPost<UserProfile>('/api/v1/auth/register', input)
}

export function loginUser(input: LoginInput): Promise<LoginResponse> {
  return apiPost<LoginResponse>('/api/v1/auth/login', input)
}

export function fetchCurrentUser(token: string): Promise<UserProfile> {
  return apiGet<UserProfile>('/api/v1/users/me', token)
}

export function updateProfile(token: string, input: ProfileUpdateInput): Promise<UserProfile> {
  return apiPut<UserProfile>('/api/v1/users/me', input, token)
}
