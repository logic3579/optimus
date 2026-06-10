import type { AxiosInstance } from 'axios'
import type { Envelope, K8sListResponse, NodeSummary } from '@/types/api'

/** Cluster-scoped Node list + detail. GET /k8s/clusters/:id/nodes[/:name]. */
export function makeK8sNodeApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number) => {
      const r = await client.get<Envelope<K8sListResponse<NodeSummary>>>(
        `/k8s/clusters/${clusterId}/nodes`
      )
      return r.data.data
    },
    get: async (clusterId: number, name: string) => {
      const r = await client.get<Envelope<NodeSummary>>(
        `/k8s/clusters/${clusterId}/nodes/${encodeURIComponent(name)}`
      )
      return r.data.data
    }
  }
}

export type K8sNodeApi = ReturnType<typeof makeK8sNodeApi>
