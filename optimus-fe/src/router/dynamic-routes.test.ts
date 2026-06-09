import { describe, it, expect } from 'vitest'
import { flattenMenusToRoutes } from './dynamic-routes'
import type { MeMenuNode } from '@/types/api'

const tree: MeMenuNode[] = [
  {
    id: 1, code: 'dashboard', name: 'menu.dashboard',
    path: '/dashboard', component: 'dashboard/Index',
    icon: 'dashboard', sort_order: 0, hidden: false
  },
  {
    id: 2, code: 'system', name: 'menu.system_group',
    path: '/system', component: '', icon: 'setting', sort_order: 1, hidden: false,
    children: [
      {
        id: 3, code: 'system.users', name: 'menu.system.users',
        path: '/system/users', component: 'system/users/List',
        icon: '', permission_code: 'system:user:read',
        sort_order: 0, hidden: false
      }
    ]
  }
]

describe('flattenMenusToRoutes', () => {
  it('skips group nodes (empty component) and flattens leaves', () => {
    const components = new Map<string, () => Promise<unknown>>([
      ['dashboard/Index', async () => ({ default: {} })],
      ['system/users/List', async () => ({ default: {} })]
    ])
    const routes = flattenMenusToRoutes(tree, p => components.get(p))
    expect(routes.map(r => r.path).sort()).toEqual(['/dashboard', '/system/users'])
    const usersRoute = routes.find(r => r.path === '/system/users')!
    expect(usersRoute.name).toBe('system.users')
    expect(usersRoute.meta?.permission).toBe('system:user:read')
  })

  it('skips nodes whose component path has no loader (with warn)', () => {
    const components = new Map<string, () => Promise<unknown>>([
      ['dashboard/Index', async () => ({ default: {} })]
    ])
    const warns: string[] = []
    const routes = flattenMenusToRoutes(tree, p => components.get(p), msg => warns.push(msg))
    expect(routes.map(r => r.path)).toEqual(['/dashboard'])
    expect(warns.some(w => w.includes('system/users/List'))).toBe(true)
  })
})
