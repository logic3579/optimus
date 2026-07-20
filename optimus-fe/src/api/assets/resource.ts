import type { AxiosInstance } from 'axios'

import type { Envelope } from '@/types/api'
import type {
  DatabaseSummary,
  InstanceSummary,
  ListResponse,
  SubnetSummary,
  VPCSummary,
} from '@/types/assets'

export interface ResourceListParams {
  account_id?: number
  region?: string
  q?: string
  include_deleted?: boolean
  page?: number
  size?: number
}

export interface InstanceListParams extends ResourceListParams {
  state?: string
  vpc_id?: string
}

export interface DatabaseListParams extends ResourceListParams {
  engine?: string
  status?: string
}

export interface SubnetListParams {
  q?: string
  include_deleted?: boolean
  page?: number
  size?: number
}

export function makeAssetsResourceApi(client: AxiosInstance) {
  return {
    listInstances: async (params: InstanceListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<InstanceSummary>>>('/assets/instances', { params })
      return response.data.data
    },
    listVPCs: async (params: ResourceListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<VPCSummary>>>('/assets/vpcs', { params })
      return response.data.data
    },
    listSubnets: async (vpcRowID: number, params?: SubnetListParams) => {
      const path = `/assets/vpcs/${vpcRowID}/subnets`
      const response = params
        ? await client.get<Envelope<ListResponse<SubnetSummary>>>(path, { params })
        : await client.get<Envelope<ListResponse<SubnetSummary>>>(path)
      return response.data.data
    },
    listDatabases: async (params: DatabaseListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<DatabaseSummary>>>('/assets/databases', { params })
      return response.data.data
    },
  }
}

export type AssetsResourceApi = ReturnType<typeof makeAssetsResourceApi>
