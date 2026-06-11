import { describe, it, expect, vi } from 'vitest'
import { makeAppsReleaseApi } from '../release'

/**
 * Smoke tests for the helm release API factory.
 *
 * Each method must:
 *   - hit /apps/applications/:id/release[/...] with the right verb + body
 *   - unwrap the envelope (return r.data.data, not r.data)
 *
 * We stub the axios instance with vi.fn() to capture both shape.
 */
describe('makeAppsReleaseApi', () => {
  function envelope<T>(data: T) {
    return { data: { code: 0, data, message: 'ok' } }
  }

  function makeClient() {
    return {
      get: vi.fn(),
      post: vi.fn(),
    }
  }

  it('status() GETs /apps/applications/:id/release and unwraps envelope', async () => {
    const client = makeClient()
    client.get.mockResolvedValue(envelope({
      status: 'deployed',
      revision: 3,
      chart_version: '1.2.3',
      app_version: '1.0',
      last_deployed_at: '2026-01-01T00:00:00Z',
    }))
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = makeAppsReleaseApi(client as any)
    const r = await api.status(7)
    expect(client.get).toHaveBeenCalledWith('/apps/applications/7/release')
    expect(r.status).toBe('deployed')
    expect(r.revision).toBe(3)
  })

  it('history() GETs /history and unwraps the items array', async () => {
    const client = makeClient()
    client.get.mockResolvedValue(envelope({
      items: [
        { revision: 1, status: 'superseded', chart_version: '1.0.0', app_version: '1.0', updated_at: '', description: 'initial' },
        { revision: 2, status: 'deployed',   chart_version: '1.1.0', app_version: '1.0', updated_at: '', description: 'bump'   },
      ],
    }))
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = makeAppsReleaseApi(client as any)
    const r = await api.history(7)
    expect(client.get).toHaveBeenCalledWith('/apps/applications/7/release/history')
    expect(r.items).toHaveLength(2)
    expect(r.items[1]!.status).toBe('deployed')
  })

  it('install() POSTs body to /install', async () => {
    const client = makeClient()
    client.post.mockResolvedValue(envelope({ revision: 1, status: 'deployed' }))
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = makeAppsReleaseApi(client as any)
    const body = { chart_version: '1.0.0', values_yaml: 'replicaCount: 2\n' }
    const r = await api.install(7, body)
    expect(client.post).toHaveBeenCalledWith('/apps/applications/7/release/install', body)
    expect(r.revision).toBe(1)
  })

  it('upgrade() POSTs body to /upgrade', async () => {
    const client = makeClient()
    client.post.mockResolvedValue(envelope({ revision: 2, status: 'deployed' }))
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = makeAppsReleaseApi(client as any)
    const body = { values_yaml: 'replicaCount: 3\n', chart_version: '1.2.0' }
    const r = await api.upgrade(7, body)
    expect(client.post).toHaveBeenCalledWith('/apps/applications/7/release/upgrade', body)
    expect(r.revision).toBe(2)
  })

  it('rollback() POSTs revision to /rollback', async () => {
    const client = makeClient()
    client.post.mockResolvedValue(envelope({ revision: 3, status: 'deployed' }))
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = makeAppsReleaseApi(client as any)
    await api.rollback(7, { revision: 1 })
    expect(client.post).toHaveBeenCalledWith('/apps/applications/7/release/rollback', { revision: 1 })
  })

  it('uninstall() POSTs to /uninstall and ignores any payload', async () => {
    const client = makeClient()
    client.post.mockResolvedValue(envelope(null))
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = makeAppsReleaseApi(client as any)
    await api.uninstall(7)
    expect(client.post).toHaveBeenCalledWith('/apps/applications/7/release/uninstall', {})
  })
})
