import { ref, onScopeDispose } from 'vue'
import { useAuthStore } from '@/stores/auth'

/**
 * Pod log SSE stream client. The BE handler at
 *   GET /k8s/clusters/:id/pods/:ns/:name/log
 * emits `data: <line>\n\n` events plus periodic `: keepalive\n\n` comment
 * lines. We can't use the native EventSource API because it doesn't support
 * custom Authorization headers — so we hand-parse SSE on top of fetch +
 * ReadableStream.
 *
 * Behaviour:
 *   - lines.value accumulates parsed `data:` payloads (no event-id / type).
 *   - keepalive comment lines (those starting with ":") are skipped.
 *   - On 401 we call `useAuthStore().refreshAccessTokenShared()` and retry
 *     once. If refresh itself fails, status flips to 'error'.
 *   - `close()` aborts the in-flight fetch via AbortController.
 *   - `onScopeDispose(close)` ensures the stream is torn down when the
 *     owning component unmounts even if the caller forgot.
 */
export interface LogStreamOpts {
  clusterId: number
  namespace: string
  pod: string
  container?: string
  follow?: boolean
  tailLines?: number
  previous?: boolean
}

export type LogStreamStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error'

export function useLogStream() {
  const lines = ref<string[]>([])
  const status = ref<LogStreamStatus>('idle')
  const errorMsg = ref<string>('')
  let ctl: AbortController | null = null
  // Tracks whether we've already retried after a 401 → refresh cycle so the
  // recursion can't loop more than once per `open()`.
  let retriedAfterAuth = false

  function buildUrl(opts: LogStreamOpts): string {
    const base = (import.meta.env.VITE_API_BASE_URL ?? '/api/v1') as string
    const params = new URLSearchParams()
    if (opts.container) params.set('container', opts.container)
    params.set('follow', String(opts.follow ?? true))
    params.set('tailLines', String(opts.tailLines ?? 200))
    params.set('previous', String(opts.previous ?? false))
    return `${base}/k8s/clusters/${opts.clusterId}/pods/${encodeURIComponent(opts.namespace)}/${encodeURIComponent(opts.pod)}/log?${params.toString()}`
  }

  async function open(opts: LogStreamOpts): Promise<void> {
    // Reset state for a fresh open(), but not on the recursive 401 retry
    // (that path keeps `lines` intact across the refresh boundary).
    if (!retriedAfterAuth) {
      close()
      lines.value = []
      errorMsg.value = ''
    }
    ctl = new AbortController()
    status.value = 'connecting'

    const auth = useAuthStore()
    let res: Response
    try {
      res = await fetch(buildUrl(opts), {
        headers: { Authorization: `Bearer ${auth.accessToken ?? ''}` },
        signal: ctl.signal
      })
    } catch (e) {
      if ((e as { name?: string })?.name === 'AbortError') {
        status.value = 'closed'
        return
      }
      status.value = 'error'
      errorMsg.value = String(e)
      return
    }

    if (res.status === 401 && !retriedAfterAuth) {
      try {
        await auth.refreshAccessTokenShared()
        retriedAfterAuth = true
        return open(opts)
      } catch {
        status.value = 'error'
        errorMsg.value = 'unauthorized'
        return
      } finally {
        retriedAfterAuth = false
      }
    }
    if (!res.ok || !res.body) {
      status.value = 'error'
      errorMsg.value = `http ${res.status}`
      return
    }

    status.value = 'open'
    const reader = res.body.getReader()
    const dec = new TextDecoder()
    let buf = ''
    // Loop until the server closes, the caller aborts, or a read errors.
    // SSE events are terminated by `\n\n`; we accumulate partial events
    // across chunks in `buf`.
    for (;;) {
      let chunk: ReadableStreamReadResult<Uint8Array>
      try {
        chunk = await reader.read()
      } catch (e) {
        if ((e as { name?: string })?.name === 'AbortError') {
          status.value = 'closed'
          return
        }
        status.value = 'error'
        errorMsg.value = String(e)
        return
      }
      if (chunk.done) break
      buf += dec.decode(chunk.value, { stream: true })
      let nl: number
      while ((nl = buf.indexOf('\n\n')) >= 0) {
        const ev = buf.slice(0, nl)
        buf = buf.slice(nl + 2)
        // Comment lines start with ':' (keepalive). Skip them.
        if (ev.startsWith(':')) continue
        if (ev.startsWith('data: ')) {
          lines.value.push(ev.slice(6))
        } else if (ev.startsWith('data:')) {
          // Tolerate the no-space form just in case.
          lines.value.push(ev.slice(5))
        }
      }
    }
    status.value = 'closed'
  }

  function close(): void {
    if (ctl) {
      ctl.abort()
      ctl = null
    }
  }

  onScopeDispose(close)

  return { lines, status, errorMsg, open, close }
}
