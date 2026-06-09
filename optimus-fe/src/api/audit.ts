import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  AuditLogEntry, AuditListQuery
} from '@/types/api'

export interface AuditListParams extends AuditListQuery {
  page: number
  page_size: number
}

export function makeAuditApi(client: AxiosInstance) {
  return {
    list: async (params: AuditListParams) => {
      const r = await client.get<Envelope<PageResp<AuditLogEntry>>>('/audit-logs', { params })
      return r.data.data
    }
  }
}

export type AuditApi = ReturnType<typeof makeAuditApi>
