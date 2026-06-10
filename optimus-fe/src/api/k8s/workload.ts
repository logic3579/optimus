import type { AxiosInstance } from 'axios'
import type {
  Envelope, K8sListResponse, K8sListQuery,
  WorkloadKind, WorkloadSummary
} from '@/types/api'

/**
 * Workloads dispatcher — all 7 kinds share one endpoint shape:
 *   GET /k8s/clusters/:id/workloads/:kind
 *   GET /k8s/clusters/:id/workloads/:kind/:ns/:name
 *
 * The FE callers pass a kind string; the BE switches on it. Return types
 * are the discriminated WorkloadSummary union — callers narrow as needed
 * (e.g. the workloads page already knows which kind it requested).
 */
export function makeWorkloadApi(client: AxiosInstance) {
  return {
    list: async (clusterId: number, kind: WorkloadKind, params: K8sListQuery = {}) => {
      const r = await client.get<Envelope<K8sListResponse<WorkloadSummary>>>(
        `/k8s/clusters/${clusterId}/workloads/${kind}`,
        { params }
      )
      return r.data.data
    },
    get: async (clusterId: number, kind: WorkloadKind, namespace: string, name: string) => {
      const r = await client.get<Envelope<WorkloadSummary>>(
        `/k8s/clusters/${clusterId}/workloads/${kind}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
      )
      return r.data.data
    }
  }
}

export type WorkloadApi = ReturnType<typeof makeWorkloadApi>
