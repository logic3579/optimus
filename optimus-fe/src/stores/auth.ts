import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { MeUser } from '@/types/api'

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string | null>(null)
  const refreshToken = ref<string | null>(null)
  const user = ref<MeUser | null>(null)
  const permissions = ref<string[]>([])

  const userLoaded = computed(() => user.value !== null)

  function setActiveTokens(access: string | null, refresh: string | null) {
    accessToken.value = access
    refreshToken.value = refresh
  }
  function setUser(u: MeUser | null) {
    user.value = u
  }
  function setPermissions(codes: string[]) {
    permissions.value = codes
  }
  function reset() {
    accessToken.value = null
    refreshToken.value = null
    user.value = null
    permissions.value = []
  }

  return {
    accessToken,
    refreshToken,
    user,
    permissions,
    userLoaded,
    setActiveTokens,
    setUser,
    setPermissions,
    reset
  }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['accessToken', 'refreshToken', 'user', 'permissions']
  }
})
