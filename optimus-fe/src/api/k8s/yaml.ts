import type { AxiosInstance } from 'axios'

export interface YamlGetParams {
  /** Lowercase singular kind — see BE KindPerm map. e.g. "deployment", "configmap". */
  kind: string
  /** Required for namespaced kinds. */
  namespace?: string
  name: string
}

/**
 * Universal YAML endpoint. The BE responds with text/yaml (not the JSON
 * envelope), so we set `responseType: 'text'`. The envelope-check axios
 * interceptor short-circuits on non-object payloads, leaving the string
 * untouched. Permission is dispatched server-side based on `kind`.
 *
 *   GET /k8s/clusters/:id/yaml?kind=<k>&namespace=<ns>&name=<n>
 */
export function makeYamlApi(client: AxiosInstance) {
  return {
    get: async (clusterId: number, params: YamlGetParams): Promise<string> => {
      const r = await client.get<string>(
        `/k8s/clusters/${clusterId}/yaml`,
        { params, responseType: 'text', transformResponse: [(d) => d] }
      )
      return r.data
    }
  }
}

export type YamlApi = ReturnType<typeof makeYamlApi>
