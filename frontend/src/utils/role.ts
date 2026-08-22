import type { Role } from '../types/auth'

// The one place "which dashboard does this role land on" is decided —
// used by the post-login/post-register redirect and by every other
// spot that needs to send an authenticated user "home" (Layout's brand
// link, ProtectedRoute's role-mismatch fallback). A single source of
// truth instead of the same ternary repeated at each call site.
export function dashboardPathForRole(role: Role): string {
  switch (role) {
    case 'ADMIN':
      return '/admin/dashboard'
    case 'DELIVERY_AGENT':
      return '/agent/dashboard'
    case 'CUSTOMER':
      return '/customer/dashboard'
  }
}
