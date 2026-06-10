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

// ─── Credentials (P1) ───────────────────────────────────────────────────────
export interface CredentialActor {
  id: number
  username?: string
  display_name?: string
}

// SSH keys
export interface SshKeySummary {
  id: number
  name: string
  description: string
  username: string
  created_by?: CredentialActor
  created_at: string
  updated_at: string
}
export interface SshKeyCreateRequest {
  name: string
  description?: string
  username: string
  private_key: string
  passphrase?: string
}
export interface SshKeyUpdateRequest {
  name?: string
  description?: string
  username?: string
  private_key?: string
  passphrase?: string
}
export interface SshKeyListQuery {
  q?: string
  username?: string
}

// Kubeconfigs
export interface KubeconfigSummary {
  id: number
  name: string
  description: string
  default_namespace: string
  created_by?: CredentialActor
  created_at: string
  updated_at: string
}
export interface KubeconfigCreateRequest {
  name: string
  description?: string
  default_namespace?: string
  kubeconfig: string
}
export interface KubeconfigUpdateRequest {
  name?: string
  description?: string
  default_namespace?: string
  kubeconfig?: string
}
export interface KubeconfigListQuery {
  q?: string
  default_namespace?: string
}

// Cloud keys
export type CloudProvider = 'aws' | 'gcp' | 'azure'
export interface CloudKeySummary {
  id: number
  name: string
  description: string
  provider: CloudProvider
  region: string
  created_by?: CredentialActor
  created_at: string
  updated_at: string
}
export interface CloudKeyCreateRequest {
  name: string
  description?: string
  provider: CloudProvider
  region?: string
  access_key_id: string
  secret_access_key: string
}
export interface CloudKeyUpdateRequest {
  name?: string
  description?: string
  provider?: CloudProvider
  region?: string
  access_key_id?: string
  secret_access_key?: string
}
export interface CloudKeyListQuery {
  q?: string
  provider?: CloudProvider
}

// ─── K8s (P2) ───────────────────────────────────────────────────────────────
// Cluster CRUD (mirrors optimus-be/internal/modules/k8s/cluster/dto.go).
export interface Cluster {
  id: number
  name: string
  kubeconfig_id: number
  kubeconfig_name?: string
  context: string
  description: string
  tags: string[]
  last_health_at?: string | null
  last_health_ok?: boolean | null
  last_health_msg?: string
  created_at: string
  updated_at: string
}
export interface ClusterCreateRequest {
  name: string
  kubeconfig_id: number
  context: string
  description?: string
  tags?: string[]
}
export interface ClusterUpdateRequest {
  name?: string
  kubeconfig_id?: number
  context?: string
  description?: string
  tags?: string[]
}
export interface ClusterListQuery {
  page?: number
  page_size?: number
  search?: string
  tag?: string
  kubeconfig_id?: number
}
export interface ClusterListResponse {
  items: Cluster[]
  total: number
  page: number
  page_size: number
}
export interface PingResult {
  ok: boolean
  server_version?: string
  message?: string
}

/**
 * Generic live-resource list envelope. Source: BE
 * `clusterscoped.ListResponse[T]` — used by every read-only k8s vertical
 * (namespace, node, event, workloads, network, configmap, secret).
 * `continue` is the apiserver opaque pagination token (empty when complete);
 * `truncated` mirrors `continue !== ""`.
 */
export interface K8sListResponse<T> {
  items: T[]
  continue?: string
  truncated: boolean
}

/** Shared query-string for every live-resource list endpoint. */
export interface K8sListQuery {
  namespace?: string
  limit?: number
  continue?: string
}

// Cluster-scoped: namespace / node / event
export interface NamespaceSummary {
  name: string
  phase: string
  labels?: Record<string, string>
  age: string
}
export interface NodeSummary {
  name: string
  ready: boolean
  schedulable: boolean
  roles: string[]
  kubelet_version: string
  cpu_capacity: string
  mem_capacity: string
  labels?: Record<string, string>
  age: string
}
export interface EventSummary {
  namespace?: string
  type: string
  reason: string
  message: string
  involved_kind: string
  involved_name: string
  count: number
  first_timestamp: string
  last_timestamp: string
}

// Workloads — 7 kinds. Each Summary mirrors a Go struct of the same name.
export interface DeploymentSummary {
  name: string
  namespace: string
  replicas_desired: number
  replicas_ready: number
  replicas_updated: number
  replicas_available: number
  strategy: string
  labels?: Record<string, string>
  age: string
}
export interface StatefulSetSummary {
  name: string
  namespace: string
  replicas: number
  ready_replicas: number
  service_name: string
  labels?: Record<string, string>
  age: string
}
export interface DaemonSetSummary {
  name: string
  namespace: string
  desired_number: number
  current_number: number
  ready_number: number
  available_number: number
  misscheduled: number
  labels?: Record<string, string>
  age: string
}
export interface JobSummary {
  name: string
  namespace: string
  completions: number
  succeeded: number
  failed: number
  start_time?: string | null
  end_time?: string | null
  labels?: Record<string, string>
  age: string
}
export interface CronJobSummary {
  name: string
  namespace: string
  schedule: string
  last_schedule_time?: string | null
  active_jobs: number
  suspended: boolean
  labels?: Record<string, string>
  age: string
}
export interface ReplicaSetSummary {
  name: string
  namespace: string
  replicas: number
  ready_replicas: number
  owner_kind?: string
  owner_name?: string
  labels?: Record<string, string>
  age: string
}
export interface PodSummary {
  name: string
  namespace: string
  phase: string
  ready_containers: number
  total_containers: number
  restart_count: number
  node_name: string
  pod_ip: string
  status_reason?: string
  labels?: Record<string, string>
  age: string
}

/** All workload kinds accepted by `/workloads/:kind`. */
export type WorkloadKind =
  | 'deployments'
  | 'statefulsets'
  | 'daemonsets'
  | 'jobs'
  | 'cronjobs'
  | 'replicasets'
  | 'pods'

/** Discriminated union of every workload Summary returned by `/workloads/:kind`. */
export type WorkloadSummary =
  | DeploymentSummary
  | StatefulSetSummary
  | DaemonSetSummary
  | JobSummary
  | CronJobSummary
  | ReplicaSetSummary
  | PodSummary

// Network — Service + Ingress
export interface ServicePort {
  name?: string
  port: number
  target_port: string
  protocol: string
  node_port?: number
}
export interface ServiceSummary {
  name: string
  namespace: string
  type: string
  cluster_ip: string
  external_ips?: string[]
  ports: ServicePort[]
  selector?: Record<string, string>
  labels?: Record<string, string>
  age: string
}
export interface IngressSummary {
  name: string
  namespace: string
  ingress_class?: string
  hosts: string[]
  load_balancer_ips?: string[]
  labels?: Record<string, string>
  age: string
}
export type NetworkKind = 'services' | 'ingresses'
export type NetworkSummary = ServiceSummary | IngressSummary

// ConfigMap (BE struct named MapSummary/MapDetail to dodge revive stutter).
export interface ConfigMapSummary {
  name: string
  namespace: string
  data_keys: string[]
  data_count: number
  labels?: Record<string, string>
  age: string
}
export interface ConfigMapDetail extends ConfigMapSummary {
  data: Record<string, string>
  binary_keys?: string[]
}

// Secret — list/get returns Summary only; values surface via `/data`.
export interface SecretSummary {
  name: string
  namespace: string
  type: string
  data_keys: string[]
  data_count: number
  labels?: Record<string, string>
  age: string
}
export type SecretDetail = SecretSummary
export interface SecretDataResponse {
  // UTF-8 values come back as plain strings; non-UTF-8 are wrapped as
  // { value: <base64>, base64: true }.
  data: Record<string, string | { value: string; base64: true }>
}
