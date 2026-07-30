import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { DeliveryEventsApi } from '@/api/delivery/events'
import type { DeliveryRunApi } from '@/api/delivery/run'
import type { DeliveryEvent, DeliveryRun } from '@/types/delivery'

const RECONNECT_DELAY_MS = 1_000
const POLL_INTERVAL_MS = 5_000

export type DeliveryConnectionStatus = 'idle' | 'connecting' | 'open' | 'polling' | 'closed'

export const useDeliveryStore = defineStore('delivery', () => {
  const selectedRunId = ref<number | null>(null)
  const run = ref<DeliveryRun | null>(null)
  const events = ref<DeliveryEvent[]>([])
  const connectionStatus = ref<DeliveryConnectionStatus>('idle')
  const generation = ref(0)
  let controller: AbortController | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null

  function current(owner: number, runId: number) {
    return generation.value === owner && selectedRunId.value === runId
  }

  function stopTransport() {
    controller?.abort()
    controller = null
    if (reconnectTimer) clearTimeout(reconnectTimer)
    if (pollTimer) clearInterval(pollTimer)
    reconnectTimer = null
    pollTimer = null
  }

  function mergeEvent(owner: number, runId: number, event: DeliveryEvent) {
    if (!current(owner, runId) || event.run_id !== runId || events.value.some(item => item.id === event.id)) return
    events.value = [...events.value, event].sort((a, b) => a.id - b.id)
  }

  async function poll(owner: number, runId: number, runApi: DeliveryRunApi) {
    const snapshot = await runApi.get(runId)
    if (current(owner, runId)) run.value = snapshot
  }

  function startPolling(owner: number, runId: number, runApi: DeliveryRunApi) {
    if (!current(owner, runId)) return
    connectionStatus.value = 'polling'
    void poll(owner, runId, runApi).catch(() => undefined)
    pollTimer = setInterval(() => void poll(owner, runId, runApi).catch(() => undefined), POLL_INTERVAL_MS)
  }

  async function connect(owner: number, runId: number, runApi: DeliveryRunApi, eventApi: DeliveryEventsApi) {
    if (!current(owner, runId)) return
    controller = new AbortController()
    connectionStatus.value = 'connecting'
    try {
      connectionStatus.value = 'open'
      await eventApi.stream(runId, {
        cursor: events.value.at(-1)?.id,
        signal: controller.signal,
        onEvent: event => mergeEvent(owner, runId, event)
      })
      if (current(owner, runId) && !controller.signal.aborted) {
        reconnectTimer = setTimeout(() => void connect(owner, runId, runApi, eventApi), RECONNECT_DELAY_MS)
      }
    } catch (error) {
      if (current(owner, runId) && (error as { name?: string }).name !== 'AbortError') startPolling(owner, runId, runApi)
    }
  }

  async function selectRun(runId: number, runApi: DeliveryRunApi, eventApi: DeliveryEventsApi) {
    stopTransport()
    const owner = ++generation.value
    selectedRunId.value = runId
    run.value = null
    events.value = []
    connectionStatus.value = 'connecting'
    const snapshot = await runApi.get(runId)
    if (!current(owner, runId)) return
    run.value = snapshot
    void connect(owner, runId, runApi, eventApi)
  }

  function reset() {
    ++generation.value
    stopTransport()
    selectedRunId.value = null
    run.value = null
    events.value = []
    connectionStatus.value = 'idle'
  }

  return { selectedRunId, run, events, connectionStatus, generation, selectRun, reset }
})
