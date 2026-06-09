import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from './auth'
import type { MeUser } from '@/types/api'

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts empty', () => {
    const s = useAuthStore()
    expect(s.accessToken).toBeNull()
    expect(s.refreshToken).toBeNull()
    expect(s.user).toBeNull()
    expect(s.permissions).toEqual([])
    expect(s.userLoaded).toBe(false)
  })

  it('setActiveTokens populates both tokens', () => {
    const s = useAuthStore()
    s.setActiveTokens('access', 'refresh')
    expect(s.accessToken).toBe('access')
    expect(s.refreshToken).toBe('refresh')
  })

  it('setUser flips userLoaded computed', () => {
    const s = useAuthStore()
    const u: MeUser = { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }
    s.setUser(u)
    expect(s.userLoaded).toBe(true)
    expect(s.user?.username).toBe('a')
  })

  it('setPermissions stores codes', () => {
    const s = useAuthStore()
    s.setPermissions(['system:user:read', 'system:role:read'])
    expect(s.permissions).toEqual(['system:user:read', 'system:role:read'])
  })

  it('reset clears everything', () => {
    const s = useAuthStore()
    s.setActiveTokens('a', 'b')
    s.setUser({ id: 1, username: 'a', email: 'a', display_name: '', avatar_url: '', status: 'enabled' })
    s.setPermissions(['x'])
    s.reset()
    expect(s.accessToken).toBeNull()
    expect(s.refreshToken).toBeNull()
    expect(s.user).toBeNull()
    expect(s.permissions).toEqual([])
  })
})
