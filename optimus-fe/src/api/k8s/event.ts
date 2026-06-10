import type { AxiosInstance } from 'axios'
import type { Envelope, K8sListResponse, EventSummary, K8sListQuery } from '@/types/api'

/** Cluster-scoped Event list. GET /k8s/clusters/:id/events?namespace=. */
export function makeK8sEventApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number, params: K8sListQuery = {}) => {
      const r = await client.get<Envelope<K8sListResponse<EventSummary>>>(
        `/k8s/clusters/${clusterId}/events`,
        { params }
      )
      return r.data.data
    }
  }
}

export type K8sEventApi = ReturnType<typeof makeK8sEventApi>
