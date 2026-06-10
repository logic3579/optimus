import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  SshKeySummary,
  SshKeyCreateRequest, SshKeyUpdateRequest, SshKeyListQuery,
} from '@/types/api'

export interface SshKeyListParams extends SshKeyListQuery {
  page: number
  page_size: number
}

export function makeSshKeyApi(client: AxiosInstance) {
  const base = '/credentials/ssh-keys'
  return {
    list: async (params: SshKeyListParams) => {
      const r = await client.get<Envelope<PageResp<SshKeySummary>>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<SshKeySummary>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: SshKeyCreateRequest) => {
      const r = await client.post<Envelope<SshKeySummary>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: SshKeyUpdateRequest) => {
      const r = await client.put<Envelope<SshKeySummary>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
  }
}

export type SshKeyApi = ReturnType<typeof makeSshKeyApi>
