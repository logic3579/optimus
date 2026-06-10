import axios, { type AxiosInstance, type InternalAxiosRequestConfig, AxiosHeaders } from 'axios'
import { useAuthStore } from '@/stores/auth'
import { parseEnvelopeError } from '@/utils/http-error'

export interface ClientOptions {
  baseURL: string
  onLogout: () => void
  getLocale?: () => string
}

/**
 * Test helper: clears the auth store's single-flight refresh slot.
 * Kept for backwards compatibility with `client.test.ts`; the actual state
 * now lives in `useAuthStore` so the SSE path can join the same in-flight
 * promise.
 */
export function __resetRefreshState() {
  // Best-effort: only callable when a pinia instance is active (tests).
  try {
    useAuthStore()._resetRefreshState()
  } catch {
    /* no active pinia — nothing to reset */
  }
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
          const auth = useAuthStore()
          const pair = await auth.refreshAccessTokenShared()
          original.__retried = true
          const headers = AxiosHeaders.from(original.headers as never)
          headers.set('Authorization', `Bearer ${pair.access_token}`)
          original.headers = headers
          return client.request(original)
        } catch (refreshErr) {
          // The shared refresh promise rejects with the same error instance
          // for every awaiter. Tag it so concurrent callers don't double-fire
          // onLogout when the underlying /auth/refresh itself 401'd.
          const tag = refreshErr as { status?: number; __logoutFired?: boolean }
          if (tag?.status === 401) {
            if (!tag.__logoutFired) {
              tag.__logoutFired = true
              opts.onLogout()
            }
          }
          throw refreshErr
        }
      }

      // Direct call to /auth/refresh (not through the shared single-flight)
      // that returned 401 — still treat as session-dead.
      if (status === 401 && isRefreshCall) {
        opts.onLogout()
      }

      throw error
    }
  )

  return client
}
