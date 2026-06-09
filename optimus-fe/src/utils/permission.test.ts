import { describe, it, expect } from 'vitest'
import { has, hasAll, hasAny } from './permission'

const perms = new Set(['system:user:read', 'system:user:write'])

describe('has', () => {
  it('returns true when the single permission is present', () => {
    expect(has(perms, 'system:user:read')).toBe(true)
  })
  it('returns false when absent', () => {
    expect(has(perms, 'system:role:delete')).toBe(false)
  })
})

describe('hasAll', () => {
  it('returns true only when every code is present', () => {
    expect(hasAll(perms, ['system:user:read', 'system:user:write'])).toBe(true)
    expect(hasAll(perms, ['system:user:read', 'system:role:read'])).toBe(false)
  })
  it('vacuously true for empty list', () => {
    expect(hasAll(perms, [])).toBe(true)
  })
})

describe('hasAny', () => {
  it('returns true if at least one code is present', () => {
    expect(hasAny(perms, ['system:role:read', 'system:user:read'])).toBe(true)
    expect(hasAny(perms, ['system:role:read'])).toBe(false)
  })
  it('vacuously false for empty list', () => {
    expect(hasAny(perms, [])).toBe(false)
  })
})
