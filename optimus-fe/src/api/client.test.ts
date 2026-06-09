import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import MockAdapter from 'axios-mock-adapter'
import { createApiClient, __resetRefreshState } from './client'
import { useAuthStore } from '@/stores/auth'

describe('axios client single-flight refresh', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    __resetRefreshState()
  })

  it('refreshes once when two concurrent requests both 401', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('expired-access', 'good-refresh')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    const mock = new MockAdapter(client)
    let refreshCalls = 0
    mock.onPost('/auth/refresh').reply(() => {
      refreshCalls++
      return [200, { code: 0, data: { access_token: 'new-access', refresh_token: 'new-refresh', expires_at: '2099-01-01T00:00:00Z' }, message: '' }]
    })

    let firstHits = 0
    mock.onGet('/me').reply(config => {
      const auth = (config.headers?.Authorization as string) ?? ''
      if (auth.includes('expired-access')) {
        firstHits++
        return [401, { code: 40102, data: null, message: 'expired', message_key: 'auth.expired' }]
      }
      return [200, { code: 0, data: { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }, message: '' }]
    })

    const [r1, r2] = await Promise.all([client.get('/me'), client.get('/me')])
    expect(r1.data.data.username).toBe('a')
    expect(r2.data.data.username).toBe('a')
    expect(refreshCalls).toBe(1)
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

    const mock = new MockAdapter(client)
    mock.onPost('/auth/refresh').reply(401, { code: 40101, data: null, message: 'bad refresh' })
    mock.onGet('/me').reply(401, { code: 40102, data: null, message: 'expired' })

    await expect(client.get('/me')).rejects.toBeTruthy()
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('does not refresh for /auth/refresh itself (avoid loop)', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('any', 'any')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

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
})
