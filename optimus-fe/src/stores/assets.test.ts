import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { useAssetsStore } from './assets'

describe('useAssetsStore', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('tracks independent manual syncs and clears only the requested account', () => {
    const store = useAssetsStore()

    store.markSyncStarted(7)
    store.markSyncStarted(9)
    expect(store.syncInFlight).toEqual({ 7: true, 9: true })

    store.clearSyncStarted(7)
    expect(store.syncInFlight).toEqual({ 9: true })
  })

  it('starts with no sync in flight', () => {
    expect(useAssetsStore().syncInFlight).toEqual({})
  })
})
