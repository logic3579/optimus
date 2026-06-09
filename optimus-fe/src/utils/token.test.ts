import { describe, it, expect } from 'vitest'
import { decodeJwtPayload, isAccessTokenExpired } from './token'

describe('decodeJwtPayload', () => {
  it('returns null for empty / malformed tokens', () => {
    expect(decodeJwtPayload('')).toBeNull()
    expect(decodeJwtPayload('garbage')).toBeNull()
    expect(decodeJwtPayload('a.b')).toBeNull()
  })

  it('decodes a valid JWT payload (HS256, base64url)', () => {
    // header.payload.signature — payload = {"sub":"1","exp":2000000000}
    const token = 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIiwiZXhwIjoyMDAwMDAwMDAwfQ.sig'
    const payload = decodeJwtPayload(token)
    expect(payload).toEqual({ sub: '1', exp: 2_000_000_000 })
  })
})

describe('isAccessTokenExpired', () => {
  it('returns true for null/empty', () => {
    expect(isAccessTokenExpired(null)).toBe(true)
    expect(isAccessTokenExpired('')).toBe(true)
  })

  it('returns true when exp is in the past (with 5s skew)', () => {
    const past = Math.floor(Date.now() / 1000) - 60
    const token = makeToken({ exp: past })
    expect(isAccessTokenExpired(token)).toBe(true)
  })

  it('returns false when exp is comfortably in the future', () => {
    const future = Math.floor(Date.now() / 1000) + 3600
    const token = makeToken({ exp: future })
    expect(isAccessTokenExpired(token)).toBe(false)
  })

  it('returns true within the 5s grace skew window', () => {
    const nearFuture = Math.floor(Date.now() / 1000) + 3
    const token = makeToken({ exp: nearFuture })
    expect(isAccessTokenExpired(token)).toBe(true)
  })
})

function makeToken(payload: Record<string, unknown>): string {
  const b64url = (s: string) => btoa(s).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  return `eyJhbGciOiJIUzI1NiJ9.${b64url(JSON.stringify(payload))}.sig`
}
