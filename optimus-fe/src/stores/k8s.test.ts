import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useK8sStore } from './k8s'

describe('useK8sStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('starts with no cluster and empty namespaces', () => {
    const s = useK8sStore()
    expect(s.currentClusterId).toBeNull()
    expect(s.currentClusterName).toBe('')
    expect(s.namespaces).toEqual([])
    expect(s.currentNamespace).toBe('')
  })

  it('setCluster on a new id clears the namespace cache + selection', () => {
    const s = useK8sStore()
    s.setCluster(1, 'a')
    s.namespaces = ['default', 'kube-system']
    s.namespacesFetchedAt = Date.now()
    s.setCurrentNamespace('default')

    s.setCluster(2, 'b')
    expect(s.currentClusterId).toBe(2)
    expect(s.currentClusterName).toBe('b')
    expect(s.namespaces).toEqual([])
    expect(s.namespacesFetchedAt).toBe(0)
    expect(s.currentNamespace).toBe('')
  })

  it('setCluster on the same id only refreshes the display name', () => {
    const s = useK8sStore()
    s.setCluster(1, '')
    s.namespaces = ['default']
    s.namespacesFetchedAt = 12345
    s.setCurrentNamespace('default')

    s.setCluster(1, 'now-known')
    expect(s.currentClusterName).toBe('now-known')
    expect(s.namespaces).toEqual(['default'])
    expect(s.namespacesFetchedAt).toBe(12345)
    expect(s.currentNamespace).toBe('default')
  })

  it('ensureNamespaces respects the 5-minute TTL (no refetch within window, refetch after)', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-10T00:00:00Z'))
    const s = useK8sStore()
    s.setCluster(7, 'prod')

    const fetcher = vi.fn(async (_id: number) => ({
      items: [{ name: 'default' }, { name: 'kube-system' }]
    }))

    await s.ensureNamespaces(fetcher)
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(s.namespaces).toEqual(['default', 'kube-system'])

    // 4m 59s later — still cached.
    vi.setSystemTime(new Date('2026-06-10T00:04:59Z'))
    await s.ensureNamespaces(fetcher)
    expect(fetcher).toHaveBeenCalledTimes(1)

    // 5m 1s later — TTL expired, refetch fires.
    vi.setSystemTime(new Date('2026-06-10T00:05:01Z'))
    await s.ensureNamespaces(fetcher)
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('ensureNamespaces is a no-op when no cluster is selected', async () => {
    const s = useK8sStore()
    const fetcher = vi.fn()
    await s.ensureNamespaces(fetcher)
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('invalidateNamespaces forces the next ensureNamespaces to refetch', async () => {
    const s = useK8sStore()
    s.setCluster(1, 'x')
    const fetcher = vi.fn(async () => ({ items: [{ name: 'a' }] }))

    await s.ensureNamespaces(fetcher)
    await s.ensureNamespaces(fetcher)
    expect(fetcher).toHaveBeenCalledTimes(1)

    s.invalidateNamespaces()
    await s.ensureNamespaces(fetcher)
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('reset clears every field', () => {
    const s = useK8sStore()
    s.setCluster(9, 'z')
    s.namespaces = ['a']
    s.namespacesFetchedAt = 1
    s.setCurrentNamespace('a')
    s.reset()
    expect(s.currentClusterId).toBeNull()
    expect(s.currentClusterName).toBe('')
    expect(s.namespaces).toEqual([])
    expect(s.namespacesFetchedAt).toBe(0)
    expect(s.currentNamespace).toBe('')
  })
})
