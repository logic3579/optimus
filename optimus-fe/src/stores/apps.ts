import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * useAppsStore — UI filter state for the P3 Applications surface.
 *
 * Holds only the list-page filter selections (cluster id + namespace) so the
 * user keeps their filter when navigating away to a detail page and back.
 * No application list is cached here per spec §8.5 — every page fetches
 * fresh from the BE, which itself queries the live release status.
 *
 * The cluster filter is persisted across reloads (matches P2's ClusterPicker
 * UX); the namespace filter is intentionally session-only so it does not
 * silently drift when clusters change off-screen.
 */
export const useAppsStore = defineStore('apps', () => {
  const filterClusterId = ref<number | null>(null)
  const filterNamespace = ref<string>('')

  function setClusterFilter(id: number | null) {
    if (id !== filterClusterId.value) {
      filterClusterId.value = id
      // Namespace is scoped to a cluster — clear it when the cluster changes.
      filterNamespace.value = ''
    }
  }
  function setNamespaceFilter(ns: string) {
    filterNamespace.value = ns
  }
  function reset() {
    filterClusterId.value = null
    filterNamespace.value = ''
  }

  return {
    // state
    filterClusterId,
    filterNamespace,
    // actions
    setClusterFilter,
    setNamespaceFilter,
    reset,
  }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['filterClusterId'],
  },
})
