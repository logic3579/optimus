import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { MeMenuNode } from '@/types/api'

export const useMenuStore = defineStore('menu', () => {
  const tree = ref<MeMenuNode[]>([])

  function setTree(t: MeMenuNode[]) {
    tree.value = t
  }
  function reset() {
    tree.value = []
  }

  return { tree, setTree, reset }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.sessionStorage : undefined,
    pick: ['tree']
  }
})
