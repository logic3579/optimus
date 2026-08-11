import { describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'

import type { MeApi } from '@/api/me'
import type { MeMenuNode, MeUser } from '@/types/api'
import { useAuthStore } from '@/stores/auth'
import { installGuards } from './guards'
import { staticRoutes } from './static-routes'

const user = { id: 1, username: 'admin' } as MeUser
const dashboardMenu: MeMenuNode = {
  id: 1,
  code: 'dashboard',
  name: 'menu.dashboard',
  path: '/dashboard',
  component: 'dashboard/Index',
  icon: 'dashboard',
  sort_order: 0,
  hidden: false
}

function setup() {
  setActivePinia(createPinia())
  const router = createRouter({ history: createMemoryHistory(), routes: staticRoutes })
  const meApi = {
    get: vi.fn().mockResolvedValue(user),
    menus: vi.fn().mockResolvedValue([dashboardMenu]),
    permissions: vi.fn().mockResolvedValue([])
  } as unknown as MeApi
  installGuards(router, meApi)
  return { router, meApi, auth: useAuthStore() }
}

describe('router guards', () => {
  it('sends an unauthenticated root visit to login', async () => {
    const { router, meApi } = setup()

    await router.push('/')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/dashboard')
    expect(meApi.get).not.toHaveBeenCalled()
  })

  it('bootstraps menus before resolving the post-login dashboard', async () => {
    const { router, meApi, auth } = setup()
    auth.setActiveTokens('access', 'refresh')

    await router.push('/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(meApi.get).toHaveBeenCalledOnce()
    expect(meApi.menus).toHaveBeenCalledOnce()
    expect(meApi.permissions).toHaveBeenCalledOnce()
  })

  it('restores dynamic routes after a reload with persisted user state', async () => {
    const { router, meApi, auth } = setup()
    auth.setActiveTokens('access', 'refresh')
    auth.setUser(user)

    await router.push('/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(meApi.menus).toHaveBeenCalledOnce()
  })
})
