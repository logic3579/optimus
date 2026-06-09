import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  UserSummary, UserDetail,
  UserCreateRequest, UserUpdateRequest,
  UserSetRolesRequest, UserSetStatusRequest, UserSetPasswordRequest,
  UserListQuery
} from '@/types/api'

export interface UserListParams extends UserListQuery {
  page: number
  page_size: number
}

export function makeUserApi(client: AxiosInstance) {
  return {
    list: async (params: UserListParams) => {
      const r = await client.get<Envelope<PageResp<UserSummary>>>('/users', { params })
      return r.data.data
    },
    create: async (body: UserCreateRequest) => {
      const r = await client.post<Envelope<UserDetail>>('/users', body)
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<UserDetail>>(`/users/${id}`)
      return r.data.data
    },
    update: async (id: number, body: UserUpdateRequest) => {
      const r = await client.put<Envelope<UserDetail>>(`/users/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`/users/${id}`)
    },
    setRoles: async (id: number, body: UserSetRolesRequest) => {
      await client.put<Envelope<null>>(`/users/${id}/roles`, body)
    },
    setStatus: async (id: number, body: UserSetStatusRequest) => {
      await client.put<Envelope<null>>(`/users/${id}/status`, body)
    },
    setPassword: async (id: number, body: UserSetPasswordRequest) => {
      await client.put<Envelope<null>>(`/users/${id}/password`, body)
    }
  }
}

export type UserApi = ReturnType<typeof makeUserApi>
