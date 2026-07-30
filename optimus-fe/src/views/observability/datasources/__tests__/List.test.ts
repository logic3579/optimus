import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key === 'observability_ui.test.reachable' ? 'Reachable' : key }) }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-alert': { template: '<div><slot/></div>' },
  'a-input-search': { template: '<input />' }, 'a-button': { template: '<button><slot/></button>' },
  'a-table': { template: '<div><slot name="bodyCell" :column="{key:\'actions\'}" :record="{id:3,name:\'prom\'}"/></div>' },
  'a-space': { template: '<div><slot/></div>' },
  'a-modal': { template: '<div><slot/></div>' },
  'a-popconfirm': { template: '<button data-testid="delete" @click="$emit(\'confirm\')"><slot/></button>' },
  'a-pagination': true, PageHeader: true, DatasourceForm: true,
}
function mounted(permissions: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setPermissions(permissions)
  const api = { list: vi.fn().mockResolvedValue({ items: [], total: 0 }), get: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), test: vi.fn() }
  const credentialApi = { list: vi.fn().mockResolvedValue({ items: [] }) }
  const clusterApi = { list: vi.fn().mockResolvedValue({ items: [] }) }
  const wrapper = mount(List, { global: { plugins: [pinia], stubs, provide: { observabilityDatasourceApi: api, httpCredentialApi: credentialApi, clusterApi } } })
  const vm = wrapper.vm as unknown as { canRead: boolean; canWrite: boolean; canDelete: boolean; canTest: boolean; errorMessage: string; testError: string; testResult?: { reachable: boolean; version?: string }; remove(row: { id: number; name?: string }): Promise<void>; testDatasource(row: { id: number }): Promise<void>; openCreate(): Promise<void> }
  return { wrapper, vm, api, credentialApi, clusterApi }
}
describe('Datasource list', () => {
  beforeEach(() => setActivePinia(createPinia()))
  it('uses CRUD permissions and confirms deletion', async () => {
    const { vm, api } = mounted(['observability:datasource:read', 'observability:datasource:write', 'observability:datasource:delete'])
    expect(vm.canRead).toBe(true); expect(vm.canWrite).toBe(true); expect(vm.canDelete).toBe(true)
    await vm.remove({ id: 3, name: 'prom' })
    expect(api.remove).toHaveBeenCalledWith(3)
  })
  it('preserves localized reference-conflict deletion errors', async () => {
    const { vm, api } = mounted(['observability:datasource:delete'])
    api.remove.mockRejectedValueOnce({ message_key: 'observability.datasource.in_use', message: 'raw' })
    await vm.remove({ id: 3 })
    expect(vm.errorMessage).toBe('observability.datasource.in_use')
  })
  it('requires write permission to test and displays only outcome/version', async () => {
    const denied = mounted([]).vm
    expect(denied.canTest).toBe(false)
    const { vm, api, wrapper } = mounted(['observability:datasource:read', 'observability:datasource:write'])
    api.test.mockResolvedValueOnce({ reachable: true, version: '2.52.0', secret: 'never' })
    await vm.testDatasource({ id: 3 })
    expect(vm.testResult).toEqual({ reachable: true, version: '2.52.0' })
    expect(wrapper.text()).toContain('Reachable · 2.52.0')
    expect(wrapper.text()).not.toContain('never')
  })
  it('localizes test failures', async () => {
    const { vm, api } = mounted(['observability:datasource:write'])
    api.test.mockRejectedValueOnce({ message_key: 'observability.test.timeout', message: 'raw upstream' })
    await vm.testDatasource({ id: 3 })
    expect(vm.testError).toBe('observability.test.timeout')
  })
  it('does not load reference APIs for a datasource-only reader', async () => {
    const { api, credentialApi, clusterApi } = mounted(['observability:datasource:read'])
    await flushPromises()
    expect(api.list).toHaveBeenCalledTimes(1)
    expect(credentialApi.list).not.toHaveBeenCalled()
    expect(clusterApi.list).not.toHaveBeenCalled()
  })
  it('loads only permitted references when opening the form', async () => {
    const { vm, credentialApi, clusterApi } = mounted(['observability:datasource:write', 'credentials:http:read'])
    await vm.openCreate()
    expect(credentialApi.list).toHaveBeenCalledWith({ page: 1, page_size: 100 })
    expect(clusterApi.list).not.toHaveBeenCalled()
  })
  it('localizes reference-load failures without an unhandled rejection', async () => {
    const { vm, credentialApi } = mounted(['observability:datasource:write', 'credentials:http:read'])
    credentialApi.list.mockRejectedValueOnce({ message_key: 'credentials.http.list_failed' })
    await expect(vm.openCreate()).resolves.toBeUndefined()
    expect(vm.errorMessage).toBe('credentials.http.list_failed')
  })
  it('does not load the datasource table without read permission', async () => {
    const { api } = mounted(['observability:datasource:write'])
    await flushPromises()
    expect(api.list).not.toHaveBeenCalled()
  })
})
