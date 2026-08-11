import { describe, expect, it } from 'vitest'
import { comparePermissionCategories } from './order'

describe('permission category order', () => {
  it('matches the top-level menu order and puts unknown categories last', () => {
    const categories = ['system', 'delivery', 'unknown-z', 'k8s', 'assets', 'credentials', 'apps', 'observability', 'unknown-a']
    expect(categories.sort(comparePermissionCategories)).toEqual([
      'assets', 'k8s', 'apps', 'delivery', 'observability', 'credentials', 'system', 'unknown-a', 'unknown-z'
    ])
  })
})
