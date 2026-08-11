import { describe, expect, it } from 'vitest'

import { staticRoutes } from './static-routes'

describe('static asset routes', () => {
  it('registers the numeric VPC detail route with resource read permission', () => {
    const root = staticRoutes.find((route) => route.name === 'root')
    const detail = root?.children?.find((route) => route.name === 'assets.vpcs.detail')
    expect(detail?.path).toBe('assets/vpcs/:id(\\d+)')
    expect(detail?.meta?.permission).toBe('assets:resource:read')
  })

  it('keeps the catch-all behind auth bootstrap instead of redirecting early', () => {
    const catchall = staticRoutes.find((route) => route.name === 'catchall')
    expect(catchall?.redirect).toBeUndefined()
    expect(catchall?.component).toBeDefined()
    expect(catchall?.meta?.public).not.toBe(true)
  })
})
