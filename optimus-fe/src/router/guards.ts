import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { usePermission } from '@/hooks/usePermission'
import { registerDynamicRoutes } from './dynamic-routes'
import type { MeApi } from '@/api/me'

export function installGuards(router: Router, meApi: MeApi) {
  let dynamicRoutesReady = false
  let removeDynamicRoutes: Array<() => void> = []

  router.beforeEach(async to => {
    if (to.meta?.public) return true

    const auth = useAuthStore()
    if (!auth.accessToken) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }

    // Dynamic routes exist only in memory. Bootstrap them on the first
    // protected navigation after every page load, even when persisted auth
    // state already contains the user. Bootstrap again after logout resets
    // that user state so a different account receives its own menu routes.
    if (!dynamicRoutesReady || !auth.userLoaded) {
      try {
        const [user, menus, perms] = await Promise.all([meApi.get(), meApi.menus(), meApi.permissions()])
        auth.setUser(user)
        auth.setPermissions(perms)
        useMenuStore().setTree(menus)
        removeDynamicRoutes.forEach(remove => remove())
        removeDynamicRoutes = registerDynamicRoutes(router, menus)
        dynamicRoutesReady = true
        // Re-resolve by URL so a route initially matched by the catch-all can
        // land on the dynamic route that was just registered.
        return { path: to.fullPath, replace: true }
      } catch {
        dynamicRoutesReady = false
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
