import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeliveryEventsApi, DeliveryEventStreamOptions } from '@/api/delivery/events'
import type { DeliveryRunApi } from '@/api/delivery/run'
import type { DeliveryEvent, DeliveryRun } from '@/types/delivery'
import { useDeliveryStore } from './delivery'

const run = (id: number): DeliveryRun => ({
  id, project_id: 1, pipeline_id: 2, pipeline_version: 1, chart_repo_id: 3,
  chart_name: 'app', chart_version: '1.0.0', chart_digest: 'sha256:x',
  initiator_user_id: 4, state: 'running', created_at: '', updated_at: '', stages: []
})
const event = (id: number, runId: number): DeliveryEvent => ({
  id, run_id: runId, event_type: 'run.running', actor_type: 'system', occurred_at: '', metadata: {}
})

describe('delivery store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useRealTimers()
  })

  it('rejects old-generation snapshots and events and deduplicates current events', async () => {
    const resolvers = new Map<number, (value: DeliveryRun) => void>()
    const get = vi.fn((id: number) => new Promise<DeliveryRun>(resolve => resolvers.set(id, resolve)))
    const streams = new Map<number, DeliveryEventStreamOptions>()
    const stream = vi.fn((id: number, options: DeliveryEventStreamOptions) => {
      streams.set(id, options)
      return new Promise<number>(() => undefined)
    })
    const store = useDeliveryStore()
    const first = store.selectRun(1, { get } as unknown as DeliveryRunApi, { stream } as DeliveryEventsApi)
    const second = store.selectRun(2, { get } as unknown as DeliveryRunApi, { stream } as DeliveryEventsApi)
    resolvers.get(2)?.(run(2)); await second; await Promise.resolve()
    streams.get(2)?.onEvent(event(5, 2)); streams.get(2)?.onEvent(event(5, 2))
    resolvers.get(1)?.(run(1)); await first
    streams.get(1)?.onEvent(event(6, 1))
    expect(store.selectedRunId).toBe(2)
    expect(store.run?.id).toBe(2)
    expect(store.events.map(item => item.id)).toEqual([5])
  })

  it('reconnects with the latest cursor after a clean stream close', async () => {
    vi.useFakeTimers()
    const stream = vi.fn(async (_id: number, options: DeliveryEventStreamOptions) => {
      if (stream.mock.calls.length === 1) options.onEvent(event(9, 2))
      return 9
    })
    const api = { get: vi.fn().mockResolvedValue(run(2)) } as unknown as DeliveryRunApi
    const store = useDeliveryStore()
    await store.selectRun(2, api, { stream } as DeliveryEventsApi)
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(1_000)
    expect(stream).toHaveBeenCalledTimes(2)
    expect(stream.mock.calls[1]?.[1].cursor).toBe(9)
    store.reset()
  })

  it('falls back to polling and reset aborts the active generation', async () => {
    vi.useFakeTimers()
    const get = vi.fn().mockResolvedValue(run(3))
    const stream = vi.fn().mockRejectedValue(new Error('offline'))
    const store = useDeliveryStore()
    await store.selectRun(3, { get } as unknown as DeliveryRunApi, { stream } as DeliveryEventsApi)
    await Promise.resolve(); await Promise.resolve()
    expect(store.connectionStatus).toBe('polling')
    await vi.advanceTimersByTimeAsync(5_000)
    expect(get.mock.calls.length).toBeGreaterThanOrEqual(3)
    const previousGeneration = store.generation
    store.reset()
    expect(store.generation).toBe(previousGeneration + 1)
    expect(store.selectedRunId).toBeNull()
    expect(store.connectionStatus).toBe('idle')
  })
})
