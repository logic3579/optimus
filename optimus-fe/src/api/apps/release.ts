import type { AxiosInstance } from 'axios'
import type {
  ReleaseStatus, RevisionRow,
  InstallRequest, UpgradeRequest, RollbackRequest, UninstallRequest,
  InstallResult,
} from '@/types/apps'
import type { Envelope } from '@/types/api'

/**
 * Helm release lifecycle for an application.
 * Maps 1:1 to /api/v1/apps/applications/:id/release/*.
 * Each action is RBAC-gated by a distinct apps:release:* permission on the BE.
 */
export function makeAppsReleaseApi(client: AxiosInstance) {
  const base = (appId: number) => `/apps/applications/${appId}/release`
  return {
    status: async (appId: number) => {
      const r = await client.get<Envelope<ReleaseStatus>>(base(appId))
      return r.data.data
    },
    history: async (appId: number) => {
      const r = await client.get<Envelope<{ items: RevisionRow[] }>>(`${base(appId)}/history`)
      return r.data.data
    },
    install: async (appId: number, body: InstallRequest) => {
      const r = await client.post<Envelope<InstallResult>>(`${base(appId)}/install`, body)
      return r.data.data
    },
    upgrade: async (appId: number, body: UpgradeRequest) => {
      const r = await client.post<Envelope<InstallResult>>(`${base(appId)}/upgrade`, body)
      return r.data.data
    },
    rollback: async (appId: number, body: RollbackRequest) => {
      const r = await client.post<Envelope<InstallResult>>(`${base(appId)}/rollback`, body)
      return r.data.data
    },
    uninstall: async (appId: number, body: UninstallRequest = {}) => {
      await client.post<Envelope<null>>(`${base(appId)}/uninstall`, body)
    },
  }
}

export type AppsReleaseApi = ReturnType<typeof makeAppsReleaseApi>
