import type { AxiosInstance } from 'axios'
import type {
  Envelope,
  Cluster, ClusterCreateRequest, ClusterUpdateRequest,
  ClusterListQuery, ClusterListResponse,
  PingResult
} from '@/types/api'

/** Cluster CRUD client. Maps 1:1 to /api/v1/k8s/clusters. */
export function makeClusterApi(client: AxiosInstance) {
  const base = '/k8s/clusters'
  return {
    list: async (params: ClusterListQuery = {}) => {
      const r = await client.get<Envelope<ClusterListResponse>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<Cluster>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: ClusterCreateRequest) => {
      const r = await client.post<Envelope<Cluster>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: ClusterUpdateRequest) => {
      const r = await client.put<Envelope<Cluster>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
    ping: async (id: number) => {
      const r = await client.post<Envelope<PingResult>>(`${base}/${id}/ping`)
      return r.data.data
    }
  }
}

export type ClusterApi = ReturnType<typeof makeClusterApi>
