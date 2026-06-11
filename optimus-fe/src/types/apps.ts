// Hand-written DTOs mirroring optimus-be apps/{repo,application,release}/dto.go.
// Naming convention matches user.Summary / user.Detail used by P0.
// Source of truth: docs/api/swagger.json + internal/modules/apps/*/dto.go.
// When BE contracts change, update this file in the same PR.

// ─── Chart repository ───────────────────────────────────────────────────────
export interface ChartRepoSummary {
  id: number
  name: string
  type: 'oci' | 'http'
  url: string
  username: string
  has_password: boolean
  description: string
  created_at: string
  updated_at: string
}
export type ChartRepoDetail = ChartRepoSummary

export interface ChartRepoCreateRequest {
  name: string
  type: 'oci' | 'http'
  url: string
  username?: string
  password?: string
  description?: string
}
export interface ChartRepoUpdateRequest {
  name?: string
  url?: string
  username?: string
  // `null` clears the stored password; `undefined`/omitted keeps it as-is.
  password?: string | null
  description?: string
}

export interface ChartRepoListQuery {
  page?: number
  page_size?: number
  name?: string
  type?: 'oci' | 'http'
}
export interface ChartRepoListResponse {
  items: ChartRepoSummary[]
  total: number
  page: number
  page_size: number
}

// ─── Chart / version enumeration ────────────────────────────────────────────
export interface ChartSummary {
  name: string
  version_count: number
  description: string
}
export interface VersionSummary {
  version: string
  app_version: string
  created: string
}

// ─── Application ────────────────────────────────────────────────────────────
export interface ApplicationSummary {
  id: number
  name: string
  cluster_id: number
  cluster_name: string
  namespace: string
  release_name: string
  chart_repo_id: number
  chart_name: string
  description: string
  tags: string[]
  owner_user_id?: number
  owner_name?: string
  created_at: string
  updated_at: string
}
export interface ApplicationDetail extends ApplicationSummary {
  status?: 'deployed' | 'failed' | 'pending' | 'unknown' | ''
  revision?: number
  chart_version?: string
  app_version?: string
  last_deployed_at?: string
}
export interface ApplicationCreateRequest {
  name: string
  cluster_id: number
  namespace: string
  release_name: string
  chart_repo_id: number
  chart_name: string
  description?: string
  tags?: string[]
  owner_user_id?: number
}
export interface ApplicationUpdateRequest {
  description?: string
  tags?: string[]
  owner_user_id?: number
}

export interface ApplicationListQuery {
  page?: number
  page_size?: number
  name?: string
  cluster_id?: number
  namespace?: string
  owner_user_id?: number
  tag?: string
}
export interface ApplicationListResponse {
  items: ApplicationSummary[]
  total: number
  page: number
  page_size: number
}

// ─── Release (helm-backed actions) ──────────────────────────────────────────
export interface ReleaseStatus {
  status: string
  revision: number
  chart_version: string
  app_version: string
  last_deployed_at: string
  notes?: string
}
export interface RevisionRow {
  revision: number
  status: string
  chart_version: string
  app_version: string
  updated_at: string
  description: string
}
export interface InstallRequest {
  chart_version: string
  values_yaml: string
}
export interface UpgradeRequest {
  chart_repo_id?: number
  chart_version: string
  values_yaml: string
}
export interface RollbackRequest {
  revision: number
}
export interface UninstallRequest {
  keep_history?: boolean
}
export interface InstallResult {
  revision: number
  status: string
  chart_version: string
  last_deployed_at: string
}
