import type { AxiosInstance } from 'axios'
import { describe, expect, it, vi } from 'vitest'
import { makeDeliveryApprovalApi } from '../approval'
import { makeDeliveryPipelineApi } from '../pipeline'
import { makeDeliveryProjectApi } from '../project'
import { makeDeliveryRunApi } from '../run'

function client() {
  const ok = vi.fn().mockResolvedValue({ data: { data: {} } })
  return { get: ok, post: vi.fn(ok), put: vi.fn(ok), delete: vi.fn(ok) } as unknown as AxiosInstance
}

describe('delivery APIs', () => {
  it('uses exact project and environment routes', async () => {
    const c = client(); const api = makeDeliveryProjectApi(c)
    await api.list({ q: 'edge', page_size: 10 }); await api.get(3)
    await api.create({ name: 'Edge', description: '' }); await api.update(3, { description: 'x' }); await api.remove(3)
    await api.listEnvironments(3); await api.bindEnvironment(3, { environment_key: 'prod', display_name: 'Production', application_id: 9 })
    await api.updateEnvironment(3, 7, { display_name: 'Prod' }); await api.unbindEnvironment(3, 7)
    expect(c.get).toHaveBeenCalledWith('/delivery/projects', { params: { q: 'edge', page_size: 10 } })
    expect(c.put).toHaveBeenCalledWith('/delivery/projects/3/environments/7', { display_name: 'Prod' })
  })

  it('serializes pipeline durations and artifact lookup without extra fields', async () => {
    const c = client(); const api = makeDeliveryPipelineApi(c)
    const stages = [{ environment_id: 7, approval_required: true, timeout: '10m' }]
    const artifact = { chart_repo_id: 4, chart_name: 'edge', chart_version: '1.2.3' }
    await api.get(3); await api.publish(3, stages); await api.listArtifacts(3); await api.resolveArtifact(3, artifact)
    expect(c.put).toHaveBeenCalledWith('/delivery/projects/3/pipeline', { stages })
    expect(c.get).toHaveBeenCalledWith('/delivery/projects/3/artifacts')
    expect(c.post).toHaveBeenCalledWith('/delivery/projects/3/artifacts/resolve', artifact)
  })

  it('sends idempotency keys and exposes no values field', async () => {
    const c = client(); const api = makeDeliveryRunApi(c)
    const artifact = { chart_repo_id: 4, chart_name: 'edge', chart_version: '1.2.3' }
    await api.list(3, { page: 2 }); await api.get(8); await api.create(3, artifact, 'create-key')
    await api.cancel(8); await api.reconcile(8); await api.retry(8, 'retry-key')
    expect(c.post).toHaveBeenCalledWith('/delivery/projects/3/runs', artifact, { headers: { 'Idempotency-Key': 'create-key' } })
    expect(c.post).toHaveBeenCalledWith('/delivery/runs/8/retry', undefined, { headers: { 'Idempotency-Key': 'retry-key' } })
    expect(JSON.stringify(artifact)).not.toContain('values')
  })

  it('uses exact approval paths and comment bodies', async () => {
    const c = client(); const api = makeDeliveryApprovalApi(c)
    await api.listPending(); await api.approve(12, 'ship it'); await api.reject(13, 'hold')
    expect(c.get).toHaveBeenCalledWith('/delivery/approvals/pending')
    expect(c.post).toHaveBeenNthCalledWith(1, '/delivery/run-stages/12/approve', { comment: 'ship it' })
    expect(c.post).toHaveBeenNthCalledWith(2, '/delivery/run-stages/13/reject', { comment: 'hold' })
  })
})
