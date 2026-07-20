import type { AxiosInstance } from 'axios'
import { describe, expect, it, vi } from 'vitest'

import { makeAssetsAccountApi } from '../account'
import { makeAssetsResourceApi } from '../resource'
import { makeAssetsSyncApi } from '../sync'

function envelope<T>(data: T) {
  return { data: { code: 0, data, message: 'ok' } }
}

function makeClient() {
  const methods = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  }
  return { methods, client: methods as unknown as AxiosInstance }
}

describe('assets API factories', () => {
  it('uses cloud-account paths, forwards snake_case queries, and unwraps envelopes', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsAccountApi(client)
    const params = { q: 'prod', include_deleted: true, page: 2, size: 50 }
    const result = { items: [], total: 0 }
    methods.get.mockResolvedValueOnce(envelope(result))

    await expect(api.list(params)).resolves.toBe(result)
    expect(methods.get).toHaveBeenCalledWith('/assets/cloud-accounts', { params })

    methods.get.mockResolvedValueOnce(envelope({ id: 7 }))
    await api.get(7)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/cloud-accounts/7')

    const create = {
      name: 'prod', provider: 'aws' as const, cloudkey_id: 3, enabled_regions: ['eu-west-1'],
    }
    methods.post.mockResolvedValueOnce(envelope({ id: 7 }))
    await api.create(create)
    expect(methods.post).toHaveBeenCalledWith('/assets/cloud-accounts', create)

    const update = { enabled_regions: ['eu-central-1'], enabled: false }
    methods.put.mockResolvedValueOnce(envelope({ id: 7 }))
    await api.update(7, update)
    expect(methods.put).toHaveBeenCalledWith('/assets/cloud-accounts/7', update)

    methods.delete.mockResolvedValueOnce(envelope({ cascaded_resources_count: 4 }))
    await expect(api.remove(7)).resolves.toEqual({ cascaded_resources_count: 4 })
    expect(methods.delete).toHaveBeenCalledWith('/assets/cloud-accounts/7')

    methods.post.mockResolvedValueOnce(envelope({ queued: true, started_at: '2026-07-16T00:00:00Z' }))
    await expect(api.triggerSync(7)).resolves.toMatchObject({ queued: true })
    expect(methods.post).toHaveBeenLastCalledWith('/assets/cloud-accounts/7/sync')
  })

  it('uses resource paths and forwards each resource query unchanged', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsResourceApi(client)
    methods.get.mockResolvedValue(envelope({ items: [], total: 0 }))

    const instances = { account_id: 2, region: 'eu-west-1', vpc_id: 'vpc-1', include_deleted: true }
    await api.listInstances(instances)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/instances', { params: instances })

    const vpcs = { q: 'core', page: 3, size: 10 }
    await api.listVPCs(vpcs)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs', { params: vpcs })

    await api.listSubnets(9)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs/9/subnets')

    const databases = { engine: 'postgres', status: 'available', page: 1, size: 20 }
    await api.listDatabases(databases)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/databases', { params: databases })
  })

  it('uses the sync-runs path and preserves started_after', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsSyncApi(client)
    const params = {
      account_id: 2,
      resource_type: 'network' as const,
      status: 'failed' as const,
      started_after: '2026-07-01T00:00:00Z',
      page: 2,
      size: 25,
    }
    const result = { items: [], total: 0 }
    methods.get.mockResolvedValueOnce(envelope(result))

    await expect(api.listRuns(params)).resolves.toBe(result)
    expect(methods.get).toHaveBeenCalledWith('/assets/sync-runs', { params })
  })
})
