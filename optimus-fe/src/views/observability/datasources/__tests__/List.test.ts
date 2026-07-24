import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
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
  const api = { list: vi.fn().mockResolvedValue({ items: [], total: 0 }), get: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), test: vi.fn() }
  const wrapper = mount(List, { global: { plugins: [createPinia()], stubs, provide: { observabilityDatasourceApi: api, httpCredentialApi: { list: vi.fn().mockResolvedValue({ items: [] }) }, clusterApi: { list: vi.fn().mockResolvedValue({ items: [] }) } } } })
  const vm = wrapper.vm as unknown as { auth: { permissions: string[] }; canRead: boolean; canWrite: boolean; canDelete: boolean; canTest: boolean; errorMessage: string; testError: string; testResult?: { reachable: boolean; version?: string }; remove(row: { id: number; name?: string }): Promise<void>; testDatasource(row: { id: number }): Promise<void> }
  const auth = vm.auth
  auth.permissions = permissions
  return { wrapper, vm, api }
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
})
