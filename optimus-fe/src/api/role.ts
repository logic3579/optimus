import type { AxiosInstance } from 'axios'
import type {
  Envelope,
  RoleSummary, RoleDetail,
  RoleCreateRequest, RoleUpdateRequest, RoleSetPermissionsRequest
} from '@/types/api'

export function makeRoleApi(client: AxiosInstance) {
  return {
    list: async () => {
      const r = await client.get<Envelope<RoleSummary[]>>('/roles')
      return r.data.data
    },
    create: async (body: RoleCreateRequest) => {
      const r = await client.post<Envelope<RoleDetail>>('/roles', body)
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<RoleDetail>>(`/roles/${id}`)
      return r.data.data
    },
    update: async (id: number, body: RoleUpdateRequest) => {
      const r = await client.put<Envelope<RoleDetail>>(`/roles/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`/roles/${id}`)
    },
    setPermissions: async (id: number, body: RoleSetPermissionsRequest) => {
      await client.put<Envelope<null>>(`/roles/${id}/permissions`, body)
    }
  }
}

export type RoleApi = ReturnType<typeof makeRoleApi>
