import type { AxiosInstance } from 'axios'
import type { Envelope, Permission } from '@/types/api'

export function makePermissionApi(client: AxiosInstance) {
  return {
    list: async () => {
      const r = await client.get<Envelope<Permission[]>>('/permissions')
      return r.data.data
    }
  }
}

export type PermissionApi = ReturnType<typeof makePermissionApi>
