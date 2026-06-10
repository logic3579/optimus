import { createApp } from 'vue'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import '@/assets/styles/utilities.scss'

import App from './App.vue'
import { createAppPinia } from '@/stores'
import { createAppRouter } from '@/router'
import { installGuards } from '@/router/guards'
import { i18n } from '@/locales'
import { installDirectives } from '@/directives'
import { createApiClient } from '@/api/client'
import { makeAuthApi } from '@/api/auth'
import { makeMeApi } from '@/api/me'
import { makeUserApi } from '@/api/user'
import { makeRoleApi } from '@/api/role'
import { makeMenuApi } from '@/api/menu'
import { makePermissionApi } from '@/api/permission'
import { makeAuditApi } from '@/api/audit'
import { makeSshKeyApi } from '@/api/credentials/ssh-key'
import { makeKubeconfigApi } from '@/api/credentials/kubeconfig'
import { makeCloudKeyApi } from '@/api/credentials/cloud-key'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { useAppStore } from '@/stores/app'

const app = createApp(App)
const pinia = createAppPinia()
app.use(pinia)
app.use(Antd)
app.use(i18n)

const router = createAppRouter()

const client = createApiClient({
  baseURL: import.meta.env.VITE_API_BASE_URL,
  onLogout: () => {
    useAuthStore().reset()
    useMenuStore().reset()
    router.push('/login')
  },
  getLocale: () => useAppStore().locale
})

const authApi = makeAuthApi(client)
const meApi = makeMeApi(client)
app.provide('authApi', authApi)
app.provide('meApi', meApi)
app.provide('userApi', makeUserApi(client))
app.provide('roleApi', makeRoleApi(client))
app.provide('menuApi', makeMenuApi(client))
app.provide('permissionApi', makePermissionApi(client))
app.provide('auditApi', makeAuditApi(client))
app.provide('sshKeyApi', makeSshKeyApi(client))
app.provide('kubeconfigApi', makeKubeconfigApi(client))
app.provide('cloudKeyApi', makeCloudKeyApi(client))

installGuards(router, meApi)
app.use(router)
installDirectives(app)

app.mount('#app')
