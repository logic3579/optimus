import type { AxiosInstance } from 'axios'

import type { Envelope } from '@/types/api'
import type {
  CloudAccountDetail,
  CloudAccountSummary,
  CreateCloudAccountRequest,
  ListResponse,
  UpdateCloudAccountRequest,
} from '@/types/assets'

export interface AssetsAccountListParams {
  q?: string
  provider?: string
  enabled?: boolean
  include_deleted?: boolean
  page?: number
  size?: number
}

export function makeAssetsAccountApi(client: AxiosInstance) {
  const base = '/assets/cloud-accounts'

  return {
    list: async (params: AssetsAccountListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<CloudAccountSummary>>>(base, { params })
      return response.data.data
    },
    get: async (id: number) => {
      const response = await client.get<Envelope<CloudAccountDetail>>(`${base}/${id}`)
      return response.data.data
    },
    create: async (request: CreateCloudAccountRequest) => {
      const response = await client.post<Envelope<CloudAccountDetail>>(base, request)
      return response.data.data
    },
    update: async (id: number, request: UpdateCloudAccountRequest) => {
      const response = await client.put<Envelope<CloudAccountDetail>>(`${base}/${id}`, request)
      return response.data.data
    },
    remove: async (id: number) => {
      const response = await client.delete<Envelope<{ cascaded_resources_count: number }>>(`${base}/${id}`)
      return response.data.data
    },
    triggerSync: async (id: number) => {
      const response = await client.post<Envelope<{ queued: boolean; started_at: string }>>(`${base}/${id}/sync`)
      return response.data.data
    },
  }
}

export type AssetsAccountApi = ReturnType<typeof makeAssetsAccountApi>
