import { describe, expect, it, vi } from 'vitest'
import type { Panel } from '@/types/observability'
import { computeRange, createRefreshController, groupPanelQueries } from '../dashboard-utils'
const panel = (id: number, datasource_id: number): Panel => ({ id, dashboard_id: 1, datasource_id, title: `p${id}`, panel_type: 'time_series', promql: 'up', unit: '', legend: '', sort_order: id, width: 12, created_at: '', updated_at: '' })
describe('dashboard query orchestration', () => {
  it('groups one batch per datasource and maps ref ids', () => {
    const groups = groupPanelQueries([panel(1, 2), panel(2, 2), panel(3, 7)])
    expect([...groups.keys()]).toEqual([2, 7]); expect(groups.get(2)?.map(x => x.ref_id)).toEqual(['panel-1', 'panel-2'])
  })
  it('owns one timer/generation, pauses hidden tabs, cancels and cleans up', async () => {
    const abort = vi.fn(); const run = vi.fn(async () => undefined); const controller = createRefreshController(run, () => false, () => ({ abort, signal: {} as AbortSignal }))
    await controller.refresh(); await controller.refresh(); expect(abort).toHaveBeenCalledOnce(); expect(run).toHaveBeenCalledTimes(2)
    controller.setHidden(true); await controller.refresh(); expect(run).toHaveBeenCalledTimes(2)
    controller.dispose(); expect(abort).toHaveBeenCalledTimes(2)
  })
  it('computes exact saved range boundaries and step', () => {
    expect(computeRange('15m', new Date('2026-01-01T01:00:00Z'))).toEqual({ start: '2026-01-01T00:45:00.000Z', end: '2026-01-01T01:00:00.000Z', step: '15s' })
    expect(computeRange('7d', new Date('2026-01-08T00:00:00Z')).step).toBe('10m')
  })
  it('rejects stale generations even when abort is ignored', async () => {
    const commits: number[] = []; let release!: () => void
    const first = new Promise<void>(resolve => { release = resolve })
    const controller = createRefreshController(async (_signal, generation, current) => { if (generation === 1) await first; if (current()) commits.push(generation) }, () => false)
    const old = controller.refresh(); await controller.refresh(); release(); await old
    expect(commits).toEqual([2])
  })
})
