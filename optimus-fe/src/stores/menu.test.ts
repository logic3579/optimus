import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useMenuStore } from './menu'
import type { MeMenuNode } from '@/types/api'

describe('useMenuStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts with empty tree', () => {
    expect(useMenuStore().tree).toEqual([])
  })

  it('setTree replaces the tree', () => {
    const tree: MeMenuNode[] = [{
      id: 1, code: 'dashboard', name: 'menu.dashboard',
      path: '/dashboard', component: 'dashboard/Index',
      icon: 'dashboard', sort_order: 0, hidden: false
    }]
    const s = useMenuStore()
    s.setTree(tree)
    expect(s.tree).toEqual(tree)
  })

  it('reset clears the tree', () => {
    const s = useMenuStore()
    s.setTree([{ id: 1, code: 'x', name: 'x', path: '/x', component: 'x', icon: '', sort_order: 0, hidden: false }])
    s.reset()
    expect(s.tree).toEqual([])
  })
})
