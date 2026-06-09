import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { SupportedLocale } from '@/locales'

export type Theme = 'light' | 'dark'

export const useAppStore = defineStore('app', () => {
  const locale = ref<SupportedLocale>('zh-CN')
  const theme = ref<Theme>('light')
  const sidebarCollapsed = ref(false)

  function setLocale(l: SupportedLocale) {
    locale.value = l
  }
  function setTheme(t: Theme) {
    theme.value = t
  }
  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  return { locale, theme, sidebarCollapsed, setLocale, setTheme, toggleSidebar }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['locale', 'theme', 'sidebarCollapsed']
  }
})
