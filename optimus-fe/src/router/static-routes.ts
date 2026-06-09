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
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/404'
  }
]
