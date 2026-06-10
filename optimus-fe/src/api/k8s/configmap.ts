import type { AxiosInstance } from 'axios'
import type {
  Envelope, K8sListResponse, K8sListQuery,
  ConfigMapSummary, ConfigMapDetail
} from '@/types/api'

/**
 * ConfigMap list + detail.
 *   GET /k8s/clusters/:id/config/configmaps
 *   GET /k8s/clusters/:id/config/configmaps/:ns/:name
 */
export function makeConfigMapApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number, params: K8sListQuery = {}) => {
      const r = await client.get<Envelope<K8sListResponse<ConfigMapSummary>>>(
        `/k8s/clusters/${clusterId}/config/configmaps`,
        { params }
      )
      return r.data.data
    },
    get: async (clusterId: number, namespace: string, name: string) => {
      const r = await client.get<Envelope<ConfigMapDetail>>(
        `/k8s/clusters/${clusterId}/config/configmaps/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
      )
      return r.data.data
    }
  }
}

export type ConfigMapApi = ReturnType<typeof makeConfigMapApi>
