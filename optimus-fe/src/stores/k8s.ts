import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * P2 §7.2 — Frontend caches the namespace list for 5 minutes per cluster
 * to avoid pounding apiserver every time the namespace dropdown opens.
 */
const NS_CACHE_TTL_MS = 5 * 60 * 1000

/**
 * useK8sStore holds the currently-selected cluster and the per-cluster
 * namespace cache for the P2 k8s management surface. The cluster id/name
 * are persisted across reloads so the page the user lands on shows the
 * same cluster they last picked.
 *
 * The namespace cache is *not* persisted — its TTL is short and the
 * apiserver list is cheap to refetch on load.
 */
export const useK8sStore = defineStore('k8s', () => {
  const currentClusterId = ref<number | null>(null)
  const currentClusterName = ref<string>('')
  const namespaces = ref<string[]>([])
  const namespacesFetchedAt = ref<number>(0)
  const currentNamespace = ref<string>('')

  /**
   * Switch the active cluster. If the cluster id actually changes this also
   * clears the namespace cache + current namespace selection — namespaces
   * are scoped to a cluster and must not bleed across switches.
   */
  function setCluster(id: number, name: string) {
    if (id !== currentClusterId.value) {
      currentClusterId.value = id
      currentClusterName.value = name
      namespaces.value = []
      namespacesFetchedAt.value = 0
      currentNamespace.value = ''
    } else {
      // Same id, but maybe the display name was missing on first load
      // (e.g. id restored from localStorage before /clusters resolved).
      currentClusterName.value = name
    }
  }

  /**
   * Fetch + cache namespaces for the current cluster. The caller supplies
   * the fetcher (the API module is injected at app boot, not at store
   * construction, so the store stays free of axios coupling and testable
   * without a provided HTTP client).
   *
   * No-op when:
   *  - no cluster is selected
   *  - the cache is still fresh (within `NS_CACHE_TTL_MS`)
   */
  async function ensureNamespaces(
    fetcher: (clusterId: number) => Promise<{ items: { name: string }[] }>
  ) {
    if (!currentClusterId.value) return
    if (Date.now() - namespacesFetchedAt.value < NS_CACHE_TTL_MS) return
    const res = await fetcher(currentClusterId.value)
    namespaces.value = res.items.map(n => n.name)
    namespacesFetchedAt.value = Date.now()
  }

  /** Force the next `ensureNamespaces` call to refetch (post-mutation hook). */
  function invalidateNamespaces() {
    namespacesFetchedAt.value = 0
  }

  function setCurrentNamespace(ns: string) {
    currentNamespace.value = ns
  }

  function reset() {
    currentClusterId.value = null
    currentClusterName.value = ''
    namespaces.value = []
    namespacesFetchedAt.value = 0
    currentNamespace.value = ''
  }

  return {
    // state
    currentClusterId,
    currentClusterName,
    namespaces,
    namespacesFetchedAt,
    currentNamespace,
    // actions
    setCluster,
    ensureNamespaces,
    invalidateNamespaces,
    setCurrentNamespace,
    reset
  }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['currentClusterId', 'currentClusterName', 'currentNamespace']
  }
})
