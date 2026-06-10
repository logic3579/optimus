import type { AxiosInstance } from 'axios'
import type {
  Envelope, K8sListResponse, K8sListQuery,
  SecretSummary, SecretDetail, SecretDataResponse
} from '@/types/api'

/**
 * Secret list/get + separate /data endpoint for reveal. The reveal route is
 * gated by a distinct k8s:secret:reveal permission on the BE — list/get only
 * carry names and types.
 *
 *   GET /k8s/clusters/:id/secrets
 *   GET /k8s/clusters/:id/secrets/:ns/:name
 *   GET /k8s/clusters/:id/secrets/:ns/:name/data   (reveal)
 */
export function makeSecretApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number, params: K8sListQuery = {}) => {
      const r = await client.get<Envelope<K8sListResponse<SecretSummary>>>(
        `/k8s/clusters/${clusterId}/secrets`,
        { params }
      )
      return r.data.data
    },
    get: async (clusterId: number, namespace: string, name: string) => {
      const r = await client.get<Envelope<SecretDetail>>(
        `/k8s/clusters/${clusterId}/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
      )
      return r.data.data
    },
    data: async (clusterId: number, namespace: string, name: string) => {
      const r = await client.get<Envelope<SecretDataResponse>>(
        `/k8s/clusters/${clusterId}/secrets/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/data`
      )
      return r.data.data
    }
  }
}

export type SecretApi = ReturnType<typeof makeSecretApi>
