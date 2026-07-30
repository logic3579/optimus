import { useAuthStore } from '@/stores/auth'
import type { DeliveryEvent } from '@/types/delivery'

export interface DeliveryEventStreamOptions {
  cursor?: number
  signal: AbortSignal
  onEvent: (event: DeliveryEvent) => void
}

interface ParsedFrame {
  id?: number
  event?: string
  data?: string
}

function parseFrame(raw: string): ParsedFrame {
  const frame: ParsedFrame = {}
  const data: string[] = []
  for (const line of raw.split(/\r?\n/)) {
    if (!line || line.startsWith(':')) continue
    const separator = line.indexOf(':')
    const field = separator < 0 ? line : line.slice(0, separator)
    let value = separator < 0 ? '' : line.slice(separator + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'id' && /^\d+$/.test(value)) frame.id = Number(value)
    if (field === 'event') frame.event = value
    if (field === 'data') data.push(value)
  }
  if (data.length) frame.data = data.join('\n')
  return frame
}

export async function parseDeliveryEventStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: DeliveryEvent) => void
): Promise<number> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let cursor = 0

  const consume = (raw: string) => {
    const frame = parseFrame(raw)
    if (!frame.data || (frame.event && frame.event !== 'delivery')) return
    const event = JSON.parse(frame.data) as DeliveryEvent
    if (!Number.isSafeInteger(event.id) || event.id <= 0 || (frame.id !== undefined && frame.id !== event.id)) {
      throw new Error('invalid delivery event id')
    }
    cursor = Math.max(cursor, event.id)
    onEvent(event)
  }

  for (;;) {
    const chunk = await reader.read()
    buffer += decoder.decode(chunk.value, { stream: !chunk.done })
    let boundary = buffer.search(/\r?\n\r?\n/)
    while (boundary >= 0) {
      const raw = buffer.slice(0, boundary)
      const match = buffer.slice(boundary).match(/^\r?\n\r?\n/)
      buffer = buffer.slice(boundary + (match?.[0].length ?? 2))
      consume(raw)
      boundary = buffer.search(/\r?\n\r?\n/)
    }
    if (chunk.done) break
  }
  if (buffer.trim()) consume(buffer)
  return cursor
}

export function makeDeliveryEventsApi(baseURL = (import.meta.env.VITE_API_BASE_URL ?? '/api/v1') as string) {
  async function stream(runId: number, options: DeliveryEventStreamOptions, retried = false): Promise<number> {
    const auth = useAuthStore()
    const headers: Record<string, string> = { Authorization: `Bearer ${auth.accessToken ?? ''}` }
    if (options.cursor) headers['Last-Event-ID'] = String(options.cursor)
    const response = await fetch(`${baseURL}/delivery/runs/${runId}/events`, {
      headers,
      signal: options.signal
    })
    if (response.status === 401 && !retried) {
      await auth.refreshAccessTokenShared()
      return stream(runId, options, true)
    }
    if (!response.ok || !response.body) throw new Error(`delivery event stream unavailable (${response.status})`)
    return parseDeliveryEventStream(response.body, options.onEvent)
  }
  return { stream }
}

export type DeliveryEventsApi = ReturnType<typeof makeDeliveryEventsApi>
