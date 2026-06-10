import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { useLogStream } from './log'
import { useAuthStore } from '@/stores/auth'

/**
 * Build a Response whose body is a ReadableStream driven by an async
 * generator. Yielding strings encodes them via TextEncoder; yielding
 * Uint8Arrays passes them through. The stream closes when the generator
 * returns. `signal` (if provided) wires AbortError into the stream so the
 * caller's read() rejects when the fetch is aborted.
 */
function streamingResponse(
  chunks: () => AsyncGenerator<string | Uint8Array, void, void>,
  init: { status?: number; signal?: AbortSignal } = {}
): Response {
  const enc = new TextEncoder()
  const stream = new ReadableStream<Uint8Array>({
    async start(controller) {
      const onAbort = () => {
        try {
          controller.error(Object.assign(new Error('aborted'), { name: 'AbortError' }))
        } catch {
          /* already errored */
        }
      }
      if (init.signal) {
        if (init.signal.aborted) {
          onAbort()
          return
        }
        init.signal.addEventListener('abort', onAbort)
      }
      try {
        for await (const c of chunks()) {
          if (init.signal?.aborted) return
          controller.enqueue(typeof c === 'string' ? enc.encode(c) : c)
        }
        controller.close()
      } catch (e) {
        controller.error(e)
      }
    }
  })
  return new Response(stream, { status: init.status ?? 200 })
}

/**
 * Suspend until `predicate()` returns true or `timeoutMs` elapses. Used by
 * tests to wait for the async read-loop inside useLogStream to flush state
 * to its refs without sleeping arbitrary amounts.
 */
async function waitFor(predicate: () => boolean, timeoutMs = 1000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (predicate()) return
    await new Promise(r => setTimeout(r, 5))
  }
  throw new Error('waitFor: timeout')
}

describe('useLogStream', () => {
  let origFetch: typeof globalThis.fetch
  let scope: ReturnType<typeof effectScope>

  beforeEach(() => {
    setActivePinia(createPinia())
    scope = effectScope()
    origFetch = globalThis.fetch
  })

  afterEach(() => {
    scope.stop()
    globalThis.fetch = origFetch
    vi.restoreAllMocks()
  })

  it('parses SSE `data:` events into lines (one chunk per event)', async () => {
    globalThis.fetch = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) =>
      streamingResponse(async function* () {
        yield 'data: hello\n\n'
        yield 'data: world\n\n'
      }, { signal: init?.signal as AbortSignal | undefined })
    ) as typeof fetch

    const r = scope.run(() => useLogStream())!
    await r.open({ clusterId: 1, namespace: 'ns', pod: 'p', follow: false })
    expect(r.status.value).toBe('closed')
    expect(r.lines.value).toEqual(['hello', 'world'])
  })

  it('handles events split across read() boundaries (buf accumulator)', async () => {
    globalThis.fetch = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) =>
      streamingResponse(async function* () {
        // Split a single SSE event across multiple chunks; also split between
        // two consecutive events. The buf accumulator must stitch them.
        yield 'data: hel'
        yield 'lo\n\nda'
        yield 'ta: wo'
        yield 'rld\n\n'
      }, { signal: init?.signal as AbortSignal | undefined })
    ) as typeof fetch

    const r = scope.run(() => useLogStream())!
    await r.open({ clusterId: 1, namespace: 'ns', pod: 'p', follow: false })
    expect(r.lines.value).toEqual(['hello', 'world'])
  })

  it('skips `: keepalive` comment lines', async () => {
    globalThis.fetch = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) =>
      streamingResponse(async function* () {
        yield 'data: a\n\n'
        yield ': keepalive\n\n'
        yield 'data: b\n\n'
        yield ': keepalive\n\n'
      }, { signal: init?.signal as AbortSignal | undefined })
    ) as typeof fetch

    const r = scope.run(() => useLogStream())!
    await r.open({ clusterId: 1, namespace: 'ns', pod: 'p', follow: false })
    expect(r.lines.value).toEqual(['a', 'b'])
  })

  it('close() aborts the in-flight fetch and flips status to closed', async () => {
    // A never-ending stream — only the abort path can end it.
    let resolveStart!: () => void
    const started = new Promise<void>(res => { resolveStart = res })

    globalThis.fetch = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) =>
      streamingResponse(async function* () {
        resolveStart()
        yield 'data: tick\n\n'
        // Block forever — only AbortSignal can interrupt the underlying read.
        await new Promise<void>(() => { /* never resolves */ })
      }, { signal: init?.signal as AbortSignal | undefined })
    ) as typeof fetch

    const r = scope.run(() => useLogStream())!
    const opened = r.open({ clusterId: 1, namespace: 'ns', pod: 'p', follow: true })
    await started
    await waitFor(() => r.lines.value.length >= 1)
    expect(r.status.value).toBe('open')

    r.close()
    await opened
    await nextTick()
    expect(r.status.value).toBe('closed')
    expect(r.lines.value).toEqual(['tick'])
  })

  it('retries once on 401 after refreshAccessTokenShared resolves', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('expired', 'refresh-tok')
    const refreshSpy = vi.spyOn(auth, 'refreshAccessTokenShared').mockResolvedValue({
      access_token: 'fresh', refresh_token: 'refresh-tok', expires_at: '2099-01-01T00:00:00Z'
    })

    let call = 0
    globalThis.fetch = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      call++
      if (call === 1) return new Response('', { status: 401 })
      return streamingResponse(async function* () {
        yield 'data: ok\n\n'
      }, { signal: init?.signal as AbortSignal | undefined })
    }) as typeof fetch

    const r = scope.run(() => useLogStream())!
    await r.open({ clusterId: 1, namespace: 'ns', pod: 'p', follow: false })
    expect(refreshSpy).toHaveBeenCalledTimes(1)
    expect(call).toBe(2)
    expect(r.lines.value).toEqual(['ok'])
    expect(r.status.value).toBe('closed')
  })
})
