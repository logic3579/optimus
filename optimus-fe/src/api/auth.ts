import type { AxiosInstance } from 'axios'
import type { Envelope, LoginRequest, LogoutRequest, RefreshRequest, TokenPair } from '@/types/api'

export function makeAuthApi(client: AxiosInstance) {
  return {
    login: async (body: LoginRequest) => {
      const r = await client.post<Envelope<TokenPair>>('/auth/login', body)
      return r.data.data
    },
    refresh: async (body: RefreshRequest) => {
      const r = await client.post<Envelope<TokenPair>>('/auth/refresh', body)
      return r.data.data
    },
    logout: async (body: LogoutRequest) => {
      await client.post<Envelope<null>>('/auth/logout', body)
    }
  }
}

export type AuthApi = ReturnType<typeof makeAuthApi>
