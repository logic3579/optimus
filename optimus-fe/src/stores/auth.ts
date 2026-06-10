import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Envelope, MeUser, TokenPair } from '@/types/api'

/**
 * Error thrown by `_doRefresh` when the refresh call itself fails.
 * Callers (e.g. the axios interceptor in `api/client.ts`) inspect `status`
 * to decide whether to fire `onLogout`. The `__logoutFired` flag is set by
 * the first caller to act on a 401 so that concurrent awaiters sharing the
 * same rejected promise don't double-fire side effects.
 */
export class RefreshError extends Error {
  status: number
  __logoutFired?: boolean
  constructor(status: number, message: string) {
    super(message)
    this.name = 'RefreshError'
    this.status = status
  }
}

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string | null>(null)
  const refreshToken = ref<string | null>(null)
  const user = ref<MeUser | null>(null)
  const permissions = ref<string[]>([])

  const userLoaded = computed(() => user.value !== null)

  // Single-flight refresh promise shared across concurrent callers (axios
  // 401 interceptor + the upcoming SSE useLogStream). Lives in the store
  // rather than the client closure so the fetch-based SSE path can join the
  // same in-flight refresh as axios.
  let refreshing: Promise<TokenPair> | null = null

  function setActiveTokens(access: string | null, refresh: string | null) {
    accessToken.value = access
    refreshToken.value = refresh
  }
  function setUser(u: MeUser | null) {
    user.value = u
  }
  function setPermissions(codes: string[]) {
    permissions.value = codes
  }
  function reset() {
    accessToken.value = null
    refreshToken.value = null
    user.value = null
    permissions.value = []
  }

  /**
   * Low-level refresh primitive. Uses raw `fetch` (no axios) so this never
   * recurses through `api/client.ts` interceptors. On non-OK responses
   * throws a `RefreshError` carrying the HTTP status; on success persists
   * the new pair via `setActiveTokens` and returns it.
   */
  async function _doRefresh(): Promise<TokenPair> {
    const rt = refreshToken.value
    if (!rt) throw new RefreshError(0, 'no refresh token')
    const baseURL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
    const resp = await fetch(`${baseURL}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: rt })
    })
    if (!resp.ok) {
      throw new RefreshError(resp.status, `refresh failed: ${resp.status}`)
    }
    const body = (await resp.json()) as Envelope<TokenPair>
    if (body.code !== 0 || !body.data) {
      throw new RefreshError(resp.status, body.message || 'refresh envelope error')
    }
    const pair = body.data
    setActiveTokens(pair.access_token, pair.refresh_token)
    return pair
  }

  /**
   * Single-flight wrapper. Concurrent callers receive the same in-flight
   * promise; the slot clears when the call settles so the next 401 burst
   * triggers a fresh refresh.
   */
  async function refreshAccessTokenShared(): Promise<TokenPair> {
    if (!refreshing) {
      refreshing = _doRefresh().finally(() => { refreshing = null })
    }
    return refreshing
  }

  function _resetRefreshState() {
    refreshing = null
  }

  return {
    accessToken,
    refreshToken,
    user,
    permissions,
    userLoaded,
    setActiveTokens,
    setUser,
    setPermissions,
    reset,
    _doRefresh,
    refreshAccessTokenShared,
    _resetRefreshState
  }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['accessToken', 'refreshToken', 'user', 'permissions']
  }
})
