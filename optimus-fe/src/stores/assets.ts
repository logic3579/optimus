import { defineStore } from 'pinia'

export const useAssetsStore = defineStore('assets', {
  state: () => ({
    syncInFlight: {} as Record<number, boolean>,
  }),
  actions: {
    markSyncStarted(accountID: number) {
      this.syncInFlight[accountID] = true
    },
    clearSyncStarted(accountID: number) {
      delete this.syncInFlight[accountID]
    },
  },
})
