import { createRouter, createWebHistory, type Router } from 'vue-router'
import { staticRoutes } from './static-routes'

export function createAppRouter(): Router {
  return createRouter({
    history: createWebHistory(),
    routes: staticRoutes
  })
}
