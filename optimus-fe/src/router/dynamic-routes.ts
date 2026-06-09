import type { Component } from 'vue'
import type { RouteRecordRaw, Router } from 'vue-router'
import type { MeMenuNode } from '@/types/api'

type Loader = () => Promise<unknown>

export function flattenMenusToRoutes(
  tree: MeMenuNode[],
  resolve: (component: string) => Loader | undefined,
  warn: (msg: string) => void = m => console.warn(m)
): RouteRecordRaw[] {
  const out: RouteRecordRaw[] = []
  const walk = (nodes: MeMenuNode[]) => {
    for (const n of nodes) {
      if (n.component) {
        const loader = resolve(n.component)
        if (!loader) {
          warn(`[router] dropped menu '${n.code}': component '${n.component}' not found in glob`)
        } else {
          out.push({
            path: n.path,
            name: n.code,
            component: loader as unknown as Component,
            meta: {
              permission: n.permission_code ?? undefined,
              icon: n.icon,
              menuName: n.name
            }
          })
        }
      }
      if (n.children?.length) walk(n.children)
    }
  }
  walk(tree)
  return out
}

export function buildViewResolver(): (component: string) => Loader | undefined {
  const map = import.meta.glob('/src/views/**/*.vue')
  return (component: string) => {
    const key = `/src/views/${component}.vue`
    return map[key] as Loader | undefined
  }
}

export function registerDynamicRoutes(router: Router, tree: MeMenuNode[]) {
  const resolve = buildViewResolver()
  const routes = flattenMenusToRoutes(tree, resolve)
  for (const r of routes) {
    router.addRoute('root', r)
  }
}
