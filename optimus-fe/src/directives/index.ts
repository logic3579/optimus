import type { App } from 'vue'
import { permissionDirective } from './permission'

export function installDirectives(app: App) {
  app.directive('permission', permissionDirective)
}
