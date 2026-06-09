import axios, { type AxiosInstance, type InternalAxiosRequestConfig, AxiosHeaders } from 'axios'
import { useAuthStore } from '@/stores/auth'
import type { Envelope, TokenPair } from '@/types/api'
import { parseEnvelopeError } from '@/utils/http-error'

export interface ClientOptions {
  baseURL: string
  onLogout: () => void
  getLocale?: () => string
}

let refreshing: Promise<TokenPair> | null = null

export function __resetRefreshState() {
  refreshing = null
}

interface RetriableConfig extends InternalAxiosRequestConfig {
  __retried?: boolean
}

export function createApiClient(opts: ClientOptions): AxiosInstance {
  const client = axios.create({ baseURL: opts.baseURL, timeout: 30_000 })

  client.interceptors.request.use(config => {
    const auth = useAuthStore()
    const headers = AxiosHeaders.from(config.headers as never)
    if (auth.accessToken) {
      headers.set('Authorization', `Bearer ${auth.accessToken}`)
    }
    if (opts.getLocale) {
      headers.set('Accept-Language', opts.getLocale())
    }
    config.headers = headers
    return config
  })

  client.interceptors.response.use(
    response => {
      // Successful HTTP but non-zero envelope code -> throw BizError so callers .catch
      const body = response.data
      const bizErr = parseEnvelopeError(body)
      if (bizErr) throw bizErr
      return response
    },
    async error => {
      const status = error?.response?.status
      const original = error?.config as RetriableConfig | undefined
      const url = original?.url ?? ''
      const isRefreshCall = url.includes('/auth/refresh')

      if (status === 401 && !isRefreshCall && original && !original.__retried) {
        try {
          const pair = await ensureFreshAccess(client)
          original.__retried = true
          const headers = AxiosHeaders.from(original.headers as never)
          headers.set('Authorization', `Bearer ${pair.access_token}`)
          original.headers = headers
          return client.request(original)
        } catch (refreshErr) {
          // If the refresh attempt itself produced a 401, the isRefreshCall
          // branch below already called onLogout — don't double-fire.
          const refreshStatus = (refreshErr as { response?: { status?: number } })?.response?.status
          if (refreshStatus !== 401) {
            opts.onLogout()
          }
          throw refreshErr
        }
      }

      if (status === 401 && isRefreshCall) {
        opts.onLogout()
      }

      throw error
    }
  )

  async function ensureFreshAccess(c: AxiosInstance): Promise<TokenPair> {
    if (!refreshing) {
      refreshing = doRefresh(c).finally(() => { refreshing = null })
    }
    return refreshing
  }

  async function doRefresh(c: AxiosInstance): Promise<TokenPair> {
    const auth = useAuthStore()
    const rt = auth.refreshToken
    if (!rt) throw new Error('no refresh token')
    const resp = await c.post<Envelope<TokenPair>>('/auth/refresh', { refresh_token: rt })
    const pair = resp.data.data
    auth.setActiveTokens(pair.access_token, pair.refresh_token)
    return pair
  }

  return client
}
