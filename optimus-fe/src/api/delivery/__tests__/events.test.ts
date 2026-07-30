import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { makeDeliveryEventsApi, parseDeliveryEventStream } from '../events'
import { useAuthStore } from '@/stores/auth'
import type { DeliveryEvent } from '@/types/delivery'

function bodyFrom(chunks: Uint8Array[]) {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      chunks.forEach(chunk => controller.enqueue(chunk))
      controller.close()
    }
  })
}

const event = (id = 1): DeliveryEvent => ({
  id, run_id: 7, event_type: 'stage.succeeded', actor_type: 'system',
  occurred_at: '2026-07-30T00:00:00Z', metadata: { reason: '发布✅' }
})

describe('delivery event stream', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('parses split UTF-8 chunks, multiline frames, comments, and ids', async () => {
    const json = JSON.stringify(event())
    const split = json.indexOf(',"event_type"') + 1
    const source = `: heartbeat\r\n\r\nid: 1\r\nevent: delivery\r\ndata: ${json.slice(0, split)}\r\ndata: ${json.slice(split)}\r\n\r\n`
    const encoded = new TextEncoder().encode(source)
    const marker = encoded.indexOf(0xe2)
    const chunks = [encoded.slice(0, marker + 1), encoded.slice(marker + 1, marker + 2), encoded.slice(marker + 2)]
    const received: DeliveryEvent[] = []
    expect(await parseDeliveryEventStream(bodyFrom(chunks), item => received.push(item))).toBe(1)
    expect(received).toEqual([event()])
  })

  it('sends authorization and reconnect cursor without EventSource', async () => {
    useAuthStore().setActiveTokens('access-token', 'refresh-token')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(bodyFrom([
      new TextEncoder().encode(`id: 4\nevent: delivery\ndata: ${JSON.stringify(event(4))}\n\n`)
    ]), { status: 200 }))
    const signal = new AbortController().signal
    const received: DeliveryEvent[] = []
    await makeDeliveryEventsApi('/api/v1').stream(7, { cursor: 3, signal, onEvent: item => received.push(item) })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/delivery/runs/7/events', {
      headers: { Authorization: 'Bearer access-token', 'Last-Event-ID': '3' }, signal
    })
    expect(received[0]?.id).toBe(4)
    expect('EventSource' in fetchMock.mock).toBe(false)
  })

  it('passes abort through the fetch signal', async () => {
    const controller = new AbortController()
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, init) => {
      expect(init?.signal).toBe(controller.signal)
      throw new DOMException('aborted', 'AbortError')
    })
    controller.abort()
    await expect(makeDeliveryEventsApi('/api/v1').stream(7, {
      signal: controller.signal, onEvent: () => undefined
    })).rejects.toMatchObject({ name: 'AbortError' })
  })
})
