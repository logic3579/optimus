import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import MockAdapter from 'axios-mock-adapter'
import { createApiClient, __resetRefreshState } from './client'
import { useAuthStore } from '@/stores/auth'

// Helper: stub global.fetch so that POSTs to `/auth/refresh` resolve with the
// given reply. Returns the call count + restore fn. The single-flight state
// now lives in the auth store and `_doRefresh` uses raw fetch (not axios), so
// axios-mock-adapter no longer sees /auth/refresh.
type FetchReply = { status: number; body: unknown; delayMs?: number }
function stubRefreshFetch(reply: () => FetchReply) {
  const orig = globalThis.fetch
  let calls = 0
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    if (url.includes('/auth/refresh')) {
      calls++
      const r = reply()
      if (r.delayMs) await new Promise(res => setTimeout(res, r.delayMs))
      return new Response(JSON.stringify(r.body), {
        status: r.status,
        headers: { 'Content-Type': 'application/json' }
      })
    }
    throw new Error(`unexpected fetch to ${url}`)
  }) as typeof fetch
  return {
    get calls() { return calls },
    restore: () => { globalThis.fetch = orig }
  }
}

describe('axios client single-flight refresh', () => {
  let fetchStub: { restore: () => void } | null = null

  beforeEach(() => {
    setActivePinia(createPinia())
    __resetRefreshState()
  })

  afterEach(() => {
    fetchStub?.restore()
    fetchStub = null
  })

  it('refreshes once when two concurrent requests both 401', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('expired-access', 'good-refresh')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    fetchStub = stubRefreshFetch(() => ({
      status: 200,
      body: { code: 0, data: { access_token: 'new-access', refresh_token: 'new-refresh', expires_at: '2099-01-01T00:00:00Z' }, message: '' },
      delayMs: 10
    }))

    const mock = new MockAdapter(client)
    let firstHits = 0
    mock.onGet('/me').reply(config => {
      const authz = (config.headers?.Authorization as string) ?? ''
      if (authz.includes('expired-access')) {
        firstHits++
        return [401, { code: 40102, data: null, message: 'expired', message_key: 'auth.expired' }]
      }
      return [200, { code: 0, data: { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }, message: '' }]
    })

    const [r1, r2] = await Promise.all([client.get('/me'), client.get('/me')])
    expect(r1.data.data.username).toBe('a')
    expect(r2.data.data.username).toBe('a')
    expect((fetchStub as unknown as { calls: number }).calls).toBe(1)
    expect(firstHits).toBeGreaterThanOrEqual(2)
    expect(auth.accessToken).toBe('new-access')
    expect(auth.refreshToken).toBe('new-refresh')
    expect(onLogout).not.toHaveBeenCalled()
  })

  it('logs out and rejects when refresh itself returns 401', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('expired-access', 'bad-refresh')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    fetchStub = stubRefreshFetch(() => ({
      status: 401,
      body: { code: 40101, data: null, message: 'bad refresh' }
    }))

    const mock = new MockAdapter(client)
    mock.onGet('/me').reply(401, { code: 40102, data: null, message: 'expired' })

    await expect(client.get('/me')).rejects.toBeTruthy()
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('does not refresh for /auth/refresh itself (avoid loop)', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('any', 'any')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    // Direct POST to /auth/refresh goes through axios (still mocked here),
    // not the store's _doRefresh — so we test that path with the adapter.
    const mock = new MockAdapter(client)
    let calls = 0
    mock.onPost('/auth/refresh').reply(() => { calls++; return [401, { code: 40101, data: null, message: 'bad' }] })

    await expect(client.post('/auth/refresh', { refresh_token: 'x' })).rejects.toBeTruthy()
    expect(calls).toBe(1)
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('attaches Authorization + Accept-Language headers', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('access-x', null)
    const client = createApiClient({ baseURL: '/api/v1', onLogout: () => {}, getLocale: () => 'en-US' })

    const mock = new MockAdapter(client)
    let seen: Record<string, string> = {}
    mock.onGet('/me').reply(config => {
      seen = { ...config.headers } as Record<string, string>
      return [200, { code: 0, data: { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }, message: '' }]
    })

    await client.get('/me')
    expect(seen['Authorization']).toBe('Bearer access-x')
    expect(seen['Accept-Language']).toBe('en-US')
  })

  it('shares the in-flight refresh promise across concurrent callers (store-level)', async () => {
    // Direct test against the store's refreshAccessTokenShared API — exercises
    // the same single-flight slot that the upcoming useLogStream (P2 T19/T21)
    // will join from a fetch-based SSE transport. We stub `fetch` (not
    // `_doRefresh`) because Pinia's setup-style action references are
    // closure-bound and not swappable via `store._doRefresh = ...`.
    const store = useAuthStore()
    store.setActiveTokens('expired-access', 'good-refresh')
    fetchStub = stubRefreshFetch(() => ({
      status: 200,
      body: { code: 0, data: { access_token: 'a', refresh_token: 'r', expires_at: '2099-01-01T00:00:00Z' }, message: '' },
      delayMs: 20
    }))

    const [a, b, c] = await Promise.all([
      store.refreshAccessTokenShared(),
      store.refreshAccessTokenShared(),
      store.refreshAccessTokenShared()
    ])
    expect((fetchStub as unknown as { calls: number }).calls).toBe(1)
    expect(a).toEqual(b)
    expect(b).toEqual(c)
    expect(a.access_token).toBe('a')
  })
})
