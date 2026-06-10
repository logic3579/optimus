import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  CloudKeySummary,
  CloudKeyCreateRequest, CloudKeyUpdateRequest, CloudKeyListQuery,
} from '@/types/api'

export interface CloudKeyListParams extends CloudKeyListQuery {
  page: number
  page_size: number
}

export function makeCloudKeyApi(client: AxiosInstance) {
  const base = '/credentials/cloud-keys'
  return {
    list: async (params: CloudKeyListParams) => {
      const r = await client.get<Envelope<PageResp<CloudKeySummary>>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<CloudKeySummary>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: CloudKeyCreateRequest) => {
      const r = await client.post<Envelope<CloudKeySummary>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: CloudKeyUpdateRequest) => {
      const r = await client.put<Envelope<CloudKeySummary>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
  }
}

export type CloudKeyApi = ReturnType<typeof makeCloudKeyApi>
