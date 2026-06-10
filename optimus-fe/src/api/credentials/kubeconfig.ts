import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  KubeconfigSummary,
  KubeconfigCreateRequest, KubeconfigUpdateRequest, KubeconfigListQuery,
} from '@/types/api'

export interface KubeconfigListParams extends KubeconfigListQuery {
  page: number
  page_size: number
}

export function makeKubeconfigApi(client: AxiosInstance) {
  const base = '/credentials/kubeconfigs'
  return {
    list: async (params: KubeconfigListParams) => {
      const r = await client.get<Envelope<PageResp<KubeconfigSummary>>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<KubeconfigSummary>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: KubeconfigCreateRequest) => {
      const r = await client.post<Envelope<KubeconfigSummary>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: KubeconfigUpdateRequest) => {
      const r = await client.put<Envelope<KubeconfigSummary>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
  }
}

export type KubeconfigApi = ReturnType<typeof makeKubeconfigApi>
