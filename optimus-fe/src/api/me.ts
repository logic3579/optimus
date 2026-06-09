import type { AxiosInstance } from 'axios'
import type {
  ChangePasswordRequest, Envelope, MeMenuNode, MeUser, UpdateMeRequest
} from '@/types/api'

export function makeMeApi(client: AxiosInstance) {
  return {
    get: async () => (await client.get<Envelope<MeUser>>('/me')).data.data,
    update: async (body: UpdateMeRequest) =>
      (await client.put<Envelope<MeUser>>('/me', body)).data.data,
    changePassword: async (body: ChangePasswordRequest) => {
      await client.put<Envelope<null>>('/me/password', body)
    },
    menus: async () => (await client.get<Envelope<MeMenuNode[]>>('/me/menus')).data.data,
    permissions: async () => (await client.get<Envelope<string[]>>('/me/permissions')).data.data
  }
}

export type MeApi = ReturnType<typeof makeMeApi>
