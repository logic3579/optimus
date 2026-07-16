import type { AxiosInstance } from 'axios'

import type { Envelope } from '@/types/api'
import type { ListResponse, SyncRunResourceType, SyncRunStatus, SyncRunSummary } from '@/types/assets'

export interface SyncRunListParams {
  account_id?: number
  resource_type?: SyncRunResourceType
  status?: SyncRunStatus
  started_after?: string
  page?: number
  size?: number
}

export function makeAssetsSyncApi(client: AxiosInstance) {
  return {
    listRuns: async (params: SyncRunListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<SyncRunSummary>>>('/assets/sync-runs', { params })
      return response.data.data
    },
  }
}

export type AssetsSyncApi = ReturnType<typeof makeAssetsSyncApi>
