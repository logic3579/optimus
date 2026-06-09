import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { usePermission } from './usePermission'

describe('usePermission', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('has() returns true when permission is in the store', () => {
    useAuthStore().setPermissions(['system:user:read', 'system:user:write'])
    const p = usePermission()
    expect(p.has('system:user:read')).toBe(true)
    expect(p.has('system:role:delete')).toBe(false)
  })

  it('hasAll / hasAny work like their utils', () => {
    useAuthStore().setPermissions(['a', 'b'])
    const p = usePermission()
    expect(p.hasAll(['a', 'b'])).toBe(true)
    expect(p.hasAll(['a', 'c'])).toBe(false)
    expect(p.hasAny(['c', 'b'])).toBe(true)
    expect(p.hasAny(['c'])).toBe(false)
  })
})
