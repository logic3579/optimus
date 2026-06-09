import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { usePermission } from '@/hooks/usePermission'
import { registerDynamicRoutes } from './dynamic-routes'
import type { MeApi } from '@/api/me'

export function installGuards(router: Router, meApi: MeApi) {
  router.beforeEach(async to => {
    if (to.meta?.public) return true

    const auth = useAuthStore()
    if (!auth.accessToken) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }

    if (!auth.userLoaded) {
      try {
        const [user, menus, perms] = await Promise.all([meApi.get(), meApi.menus(), meApi.permissions()])
        auth.setUser(user)
        auth.setPermissions(perms)
        useMenuStore().setTree(menus)
        registerDynamicRoutes(router, menus)
        return { ...to, replace: true }
      } catch {
        auth.reset()
        useMenuStore().reset()
        return { name: 'login', query: { redirect: to.fullPath } }
      }
    }

    const perm = to.meta?.permission as string | undefined
    if (perm && !usePermission().has(perm)) {
      return { name: 'forbidden' }
    }
    return true
  })
}
