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

// ─── Pagination envelope (used by all paginated list endpoints) ─────────────
export interface PageResp<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

// ─── Users ──────────────────────────────────────────────────────────────────
export interface UserSummary {
  id: number
  username: string
  email: string
  display_name: string
  status: 'enabled' | 'disabled'
  last_login_at?: string | null
  created_at: string
}
export interface UserRoleRef {
  id: number
  code: string
  name: string
}
export interface UserDetail extends UserSummary {
  avatar_url: string
  roles: UserRoleRef[]
}
export interface UserCreateRequest {
  username: string
  email: string
  password: string
  display_name?: string
  role_ids?: number[]
}
export interface UserUpdateRequest {
  email?: string
  display_name?: string
  avatar_url?: string
}
export interface UserSetRolesRequest {
  role_ids: number[]
}
export interface UserSetStatusRequest {
  status: 'enabled' | 'disabled'
}
export interface UserSetPasswordRequest {
  password: string
}
export interface UserListQuery {
  search?: string
  status?: 'enabled' | 'disabled' | ''
}

// ─── Roles ──────────────────────────────────────────────────────────────────
export interface RoleSummary {
  id: number
  code: string
  name: string
  description: string
  is_builtin: boolean
  created_at: string
}
export interface RoleDetail extends RoleSummary {
  permission_codes: string[]
}
export interface RoleCreateRequest {
  code: string
  name: string
  description?: string
}
export interface RoleUpdateRequest {
  name?: string
  description?: string
}
export interface RoleSetPermissionsRequest {
  permission_codes: string[]
}

// ─── Menus ──────────────────────────────────────────────────────────────────
export interface MenuNode {
  id: number
  parent_id?: number | null
  code: string
  name: string
  path: string
  component: string
  icon: string
  permission_code?: string | null
  sort_order: number
  hidden: boolean
  children?: MenuNode[]
}
export interface MenuCreateRequest {
  parent_id?: number | null
  code: string
  name: string
  path?: string
  component?: string
  icon?: string
  permission_code?: string | null
  sort_order?: number
  hidden?: boolean
}
export interface MenuUpdateRequest {
  parent_id?: number | null
  name?: string
  path?: string
  component?: string
  icon?: string
  permission_code?: string | null
  sort_order?: number
  hidden?: boolean
}

// ─── Permissions ────────────────────────────────────────────────────────────
export interface Permission {
  id: number
  code: string
  name: string
  category: string
  description: string
}

// ─── Audit logs ─────────────────────────────────────────────────────────────
export interface AuditLogEntry {
  id: number
  user_id?: number | null
  action: string
  target_type?: string
  target_id?: string
  payload: unknown
  ip?: string
  user_agent?: string
  created_at: string
}
export interface AuditListQuery {
  action?: string
  user_id?: number
  start?: string  // RFC3339
  end?: string    // RFC3339
}
