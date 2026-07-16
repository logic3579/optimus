import { describe, expect, it } from 'vitest'

import { staticRoutes } from './static-routes'

describe('static asset routes', () => {
  it('registers the numeric VPC detail route with resource read permission', () => {
    const root = staticRoutes.find((route) => route.name === 'root')
    const detail = root?.children?.find((route) => route.name === 'assets.vpcs.detail')
    expect(detail?.path).toBe('assets/vpcs/:id(\\d+)')
    expect(detail?.meta?.permission).toBe('assets:resource:read')
  })
})
