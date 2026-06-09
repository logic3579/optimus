import type { RouteRecordRaw } from 'vue-router'

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
    component: () => import('@/layouts/DefaultLayout.vue'),
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
