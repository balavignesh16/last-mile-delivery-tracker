import { createContext } from 'react'
import type { RegisterInput, UserProfile } from '../types/auth'

export type AuthStatus = 'loading' | 'authenticated' | 'unauthenticated'

export interface AuthContextValue {
  token: string | null
  user: UserProfile | null
  status: AuthStatus
  login: (email: string, password: string) => Promise<void>
  register: (input: RegisterInput) => Promise<void>
  logout: () => void
}

// Plain .ts file, not .tsx: holds only the context object and its types,
// no component — React Fast Refresh works best when a .tsx file exports
// only components, so AuthProvider (contexts/AuthContext.tsx) and useAuth
// (hooks/useAuth.ts) each import this instead of defining their own.
export const AuthContext = createContext<AuthContextValue | undefined>(undefined)
