import { h } from 'vue'
import { RouterView, type RouteRecordRaw } from 'vue-router'

// Passthrough component used as the `root` parent so dynamic routes can be
// registered as its children while App.vue picks the actual layout based on
// `route.meta.layout`.
const RouterViewPassthrough = { render: () => h(RouterView) }

export const staticRoutes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/auth/Login.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/403',
    name: 'forbidden',
    component: () => import('@/views/errors/403.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/404',
    name: 'notfound',
    component: () => import('@/views/errors/404.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/500',
    name: 'serverError',
    component: () => import('@/views/errors/500.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/',
    name: 'root',
    component: RouterViewPassthrough,
    redirect: '/dashboard',
    children: [
      {
        path: 'profile',
        name: 'profile',
        component: () => import('@/views/profile/Index.vue')
      },
      // P3 application sub-routes — Detail/Install/Upgrade are reachable from
      // the list page but are not menu nodes themselves. Registered here so
      // their perm meta gates the route guard the same way menu routes do.
      {
        path: 'apps/applications/new',
        name: 'apps.applications.new',
        component: () => import('@/views/apps/Applications/Install.vue'),
        meta: { permission: 'apps:application:write' }
      },
      {
        path: 'apps/applications/:id(\\d+)',
        name: 'apps.applications.detail',
        component: () => import('@/views/apps/Applications/Detail.vue'),
        meta: { permission: 'apps:application:read' }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/404'
  }
]
