const SKEW_SECONDS = 5

export interface JwtPayload {
  sub?: string
  exp?: number
  iat?: number
  jti?: string
  [k: string]: unknown
}

export function decodeJwtPayload(token: string | null | undefined): JwtPayload | null {
  if (!token) return null
  const parts = token.split('.')
  if (parts.length < 2 || !parts[1]) return null
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = b64 + '==='.slice((b64.length + 3) % 4)
    const raw = atob(padded)
    return JSON.parse(raw) as JwtPayload
  } catch {
    return null
  }
}

export function isAccessTokenExpired(token: string | null | undefined): boolean {
  if (!token) return true
  const p = decodeJwtPayload(token)
  if (!p || typeof p.exp !== 'number') return true
  const nowSec = Math.floor(Date.now() / 1000)
  return p.exp <= nowSec + SKEW_SECONDS
}
