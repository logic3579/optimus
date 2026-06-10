import type { AxiosInstance } from 'axios'
import type {
  Envelope, K8sListResponse, K8sListQuery,
  NetworkKind, NetworkSummary
} from '@/types/api'

/**
 * Network dispatcher — Service + Ingress share the URL shape:
 *   GET /k8s/clusters/:id/network/:kind
 *   GET /k8s/clusters/:id/network/:kind/:ns/:name
 */
export function makeK8sNetworkApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number, kind: NetworkKind, params: K8sListQuery = {}) => {
      const r = await client.get<Envelope<K8sListResponse<NetworkSummary>>>(
        `/k8s/clusters/${clusterId}/network/${kind}`,
        { params }
      )
      return r.data.data
    },
    get: async (clusterId: number, kind: NetworkKind, namespace: string, name: string) => {
      const r = await client.get<Envelope<NetworkSummary>>(
        `/k8s/clusters/${clusterId}/network/${kind}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
      )
      return r.data.data
    }
  }
}

export type K8sNetworkApi = ReturnType<typeof makeK8sNetworkApi>
