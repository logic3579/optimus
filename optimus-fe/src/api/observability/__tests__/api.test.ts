import type { AxiosInstance } from 'axios'
import { describe, expect, it, vi } from 'vitest'
import type { InstantBatch, RangeBatch, SaveDashboard, SaveDatasource } from '@/types/observability'
import { makeObservabilityDashboardApi } from '../dashboard'
import { makeObservabilityDatasourceApi } from '../datasource'
import { makeObservabilityQueryApi } from '../query'

const client = () => {
  const ok = vi.fn().mockResolvedValue({ data: { data: {} } })
  return { get: ok, post: vi.fn(ok), put: vi.fn(ok), delete: vi.fn(ok) } as unknown as AxiosInstance
}
const datasource: SaveDatasource = { name: 'p', base_url: 'https://p.test', auth_type: 'none', tls_skip_verify: false, description: '' }
const dashboard: SaveDashboard = { name: 'd', description: '', refresh_interval_s: 30, time_range: '1h', panels: [] }

describe('observability APIs', () => {
  it('uses exact datasource routes', async () => {
    const c = client(); const a = makeObservabilityDatasourceApi(c)
    await a.list({ q: 'x' }); await a.create(datasource); await a.get(3); await a.update(3, datasource); await a.remove(3); await a.test(3); await a.labels(3); await a.labelValues(3, 'pod/name')
    expect(c.get).toHaveBeenCalledWith('/observability/datasources', { params: { q: 'x' } }); expect(c.post).toHaveBeenCalledWith('/observability/datasources', datasource)
  })
  it('posts exact query batches', async () => {
    const c = client(); const a = makeObservabilityQueryApi(c)
    const q: InstantBatch = { datasource_id: 3, enrich_assets: false, queries: [{ ref_id: 'cpu', promql: 'up' }] }
    const r: RangeBatch = { ...q, start: 'a', end: 'b', step: '1m' }
    await a.instant(q); await a.range(r)
    expect(c.post).toHaveBeenNthCalledWith(1, '/observability/query', q, { signal: undefined }); expect(c.post).toHaveBeenNthCalledWith(2, '/observability/query-range', r, { signal: undefined })
  })
  it('uses exact aggregate and builtin routes', async () => {
    const c = client(); const a = makeObservabilityDashboardApi(c)
    await a.list({ q: 'x' }); await a.create(dashboard); await a.get(2); await a.update(2, dashboard); await a.remove(2); await a.listBuiltins(); await a.getBuiltin('kubernetes/nodes')
    expect(c.get).toHaveBeenCalledWith('/observability/dashboards', { params: { q: 'x' } }); expect(c.get).toHaveBeenCalledWith('/observability/builtin-dashboards/kubernetes%2Fnodes')
  })
})
