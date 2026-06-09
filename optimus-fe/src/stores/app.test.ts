import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAppStore } from './app'

describe('useAppStore', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('defaults locale=zh-CN, theme=light, sidebarCollapsed=false', () => {
    const s = useAppStore()
    expect(s.locale).toBe('zh-CN')
    expect(s.theme).toBe('light')
    expect(s.sidebarCollapsed).toBe(false)
  })

  it('mutators flip values', () => {
    const s = useAppStore()
    s.setLocale('en-US')
    s.setTheme('dark')
    s.toggleSidebar()
    expect(s.locale).toBe('en-US')
    expect(s.theme).toBe('dark')
    expect(s.sidebarCollapsed).toBe(true)
  })
})
