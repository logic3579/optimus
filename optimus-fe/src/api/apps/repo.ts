import type { AxiosInstance } from 'axios'
import type {
  ChartRepoSummary, ChartRepoDetail,
  ChartRepoCreateRequest, ChartRepoUpdateRequest,
  ChartRepoListQuery, ChartRepoListResponse,
  ChartSummary, VersionSummary,
} from '@/types/apps'
import type { Envelope } from '@/types/api'

/**
 * Chart-repository CRUD + chart/version enumeration.
 * Maps 1:1 to /api/v1/apps/repos/*.
 */
export function makeAppsRepoApi(client: AxiosInstance) {
  const base = '/apps/repos'
  return {
    list: async (params: ChartRepoListQuery = {}) => {
      const r = await client.get<Envelope<ChartRepoListResponse>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<ChartRepoDetail>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: ChartRepoCreateRequest) => {
      const r = await client.post<Envelope<ChartRepoDetail>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: ChartRepoUpdateRequest) => {
      const r = await client.put<Envelope<ChartRepoDetail>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
    listCharts: async (id: number) => {
      const r = await client.get<Envelope<{ items: ChartSummary[] }>>(`${base}/${id}/charts`)
      return r.data.data
    },
    listVersions: async (id: number, chart: string) => {
      const r = await client.get<Envelope<{ items: VersionSummary[] }>>(
        `${base}/${id}/charts/${encodeURIComponent(chart)}/versions`,
      )
      return r.data.data
    },
    getDefaultValues: async (id: number, chart: string, version: string) => {
      const r = await client.get<Envelope<{ values_yaml: string }>>(
        `${base}/${id}/charts/${encodeURIComponent(chart)}/versions/${encodeURIComponent(version)}/values`,
      )
      return r.data.data
    },
  }
}

export type AppsRepoApi = ReturnType<typeof makeAppsRepoApi>
// Convenience alias kept for the rare external import that uses ChartRepoSummary directly.
export type { ChartRepoSummary }
