import type { AxiosInstance } from 'axios'
import type { Envelope, K8sListResponse, NamespaceSummary } from '@/types/api'

/** Cluster-scoped Namespace list. GET /k8s/clusters/:id/namespaces. */
export function makeK8sNamespaceApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number) => {
      const r = await client.get<Envelope<K8sListResponse<NamespaceSummary>>>(
        `/k8s/clusters/${clusterId}/namespaces`
      )
      return r.data.data
    }
  }
}

export type K8sNamespaceApi = ReturnType<typeof makeK8sNamespaceApi>
