// Hand-written DTOs mirroring optimus-be /api/v1 contracts.
// Source of truth: docs/api/swagger.json + internal/modules/*/dto.go.
// When BE contracts change, update this file in the same PR.

export interface Envelope<T> {
  code: number
  data: T
  message: string
  message_key?: string
}

// Auth
export interface LoginRequest {
  username: string
  password: string
}
export interface TokenPair {
  access_token: string
  refresh_token: string
  expires_at: string // ISO timestamp
}
export interface RefreshRequest {
  refresh_token: string
}
export interface LogoutRequest {
  refresh_token?: string
}

// Me
export interface MeUser {
  id: number
  username: string
  email: string
  display_name: string
  avatar_url: string
  status: 'enabled' | 'disabled'
  last_login_at?: string | null
}

export interface MeMenuNode {
  id: number
  code: string
  name: string                   // i18n key, e.g. "menu.system.users"
  path: string                   // e.g. "/system/users"
  component: string              // e.g. "system/users/List"; "" = group node
  icon: string
  permission_code?: string | null
  sort_order: number
  hidden: boolean
  children?: MeMenuNode[]
}

export interface UpdateMeRequest {
  email?: string
  display_name?: string
  avatar_url?: string
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}
