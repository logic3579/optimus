import type { AxiosInstance } from 'axios'
import type {
  ApplicationDetail,
  ApplicationCreateRequest, ApplicationUpdateRequest,
  ApplicationListQuery, ApplicationListResponse,
} from '@/types/apps'
import type { Envelope } from '@/types/api'

/**
 * Application metadata CRUD.
 * Maps 1:1 to /api/v1/apps/applications/*.
 * NOTE: application is just the BE-stored metadata row; the actual Helm
 * release lifecycle lives under /apps/applications/:id/release/* (see release.ts).
 */
export function makeAppsApplicationApi(client: AxiosInstance) {
  const base = '/apps/applications'
  return {
    list: async (params: ApplicationListQuery = {}) => {
      const r = await client.get<Envelope<ApplicationListResponse>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<ApplicationDetail>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: ApplicationCreateRequest) => {
      const r = await client.post<Envelope<ApplicationDetail>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: ApplicationUpdateRequest) => {
      const r = await client.put<Envelope<ApplicationDetail>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
  }
}

export type AppsApplicationApi = ReturnType<typeof makeAppsApplicationApi>
