/* eslint-disable vue/require-prop-types */
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { permissionDirective } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'
import type { DeliveryProjectSummary } from '@/types/delivery'
import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { push, showError } = vi.hoisted(() => ({ push: vi.fn(), showError: vi.fn() }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('ant-design-vue', () => ({ message: { error: showError } }))

const row: DeliveryProjectSummary = { id: 4, name: 'Checkout', description: 'edge', environment_count: 3, owner_user_id: 8, created_at: '', updated_at: '' }
function api() { return { list: vi.fn().mockResolvedValue({ items: [row], total: 1, page: 1, page_size: 20 }), create: vi.fn().mockResolvedValue(row), update: vi.fn().mockResolvedValue(row), remove: vi.fn(), get: vi.fn(), listEnvironments: vi.fn(), bindEnvironment: vi.fn(), updateEnvironment: vi.fn(), unbindEnvironment: vi.fn() } }
const Table = defineComponent({ props: ['columns', 'dataSource'], setup(props, { slots }) { return () => h('div', (props.dataSource ?? []).flatMap((record: unknown) => (props.columns ?? []).map((column: unknown) => slots.bodyCell?.({ column, record })))) } })
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, PageHeader: { template: '<div><slot/></div>' },
  'a-button': { template: '<button><slot/></button>' }, 'a-space': { template: '<div><slot/></div>' },
  'a-popconfirm': { emits: ['confirm'], template: '<span><slot/><button class="confirm" @click="$emit(\'confirm\')">confirm</button></span>' },
  'a-table': Table, 'a-pagination': true, 'a-input-search': true,
  'a-modal': { template: '<div><slot/></div>' }, 'a-form': { template: '<form><slot/></form>' },
  'a-form-item': { template: '<div><slot/></div>' }, 'a-input': true, 'a-textarea': true, 'a-input-number': true,
}

describe('delivery project list', () => {
  beforeEach(() => { setActivePinia(createPinia()); push.mockClear(); showError.mockClear() })
  function mounted(permissions: string[], projectApi = api()) {
    useAuthStore().setPermissions(permissions)
    const wrapper = mount(List, { global: { provide: { deliveryProjectApi: projectApi }, stubs, directives: { permission: permissionDirective } } })
    return { wrapper, projectApi, vm: wrapper.vm as unknown as { formOpen:boolean;form:{name:string;description:string;owner_user_id?:number};table:{setPage(n:number):Promise<void>};openCreate():void;openEdit(row:DeliveryProjectSummary):void;save():Promise<void>;openDetail(row:DeliveryProjectSummary):void } }
  }

  it('suppresses fetch without read and exposes exact write/delete controls', async () => {
    const denied = mounted([]); await flushPromises(); expect(denied.projectApi.list).not.toHaveBeenCalled()
    const readOnly = mounted(['delivery:project:read']); await flushPromises()
    expect(readOnly.wrapper.find('[data-testid="create-project"]').exists()).toBe(false)
    expect(readOnly.wrapper.find('[data-testid="edit-4"]').exists()).toBe(false)
    expect(readOnly.wrapper.find('[data-testid="delete-4"]').exists()).toBe(false)
    const allowed = mounted(['delivery:project:read', 'delivery:project:write', 'delivery:project:delete']); await flushPromises()
    expect(allowed.wrapper.find('[data-testid="create-project"]').exists()).toBe(true)
    expect(allowed.wrapper.find('[data-testid="edit-4"]').exists()).toBe(true)
    expect(allowed.wrapper.find('[data-testid="delete-4"]').exists()).toBe(true)
  })

  it('uses useTable pagination, navigates, creates, edits, and deletes', async () => {
    const { wrapper, projectApi, vm } = mounted(['delivery:project:read', 'delivery:project:write', 'delivery:project:delete']); await flushPromises()
    await vm.table.setPage(2); expect(projectApi.list).toHaveBeenLastCalledWith({ page: 2, page_size: 20 })
    vm.openDetail(row); expect(push).toHaveBeenCalledWith('/delivery/projects/4')
    vm.openCreate(); Object.assign(vm.form, { name: ' Billing ', description: 'core' }); await vm.save()
    expect(projectApi.create).toHaveBeenCalledWith({ name: 'Billing', description: 'core' })
    vm.openEdit(row); vm.form.description = 'updated'; await vm.save()
    expect(projectApi.update).toHaveBeenCalledWith(4, { name: 'Checkout', description: 'updated', owner_user_id: 8 })
    await wrapper.find('.confirm').trigger('click'); await flushPromises(); expect(projectApi.remove).toHaveBeenCalledWith(4)
  })

  it('normalizes list errors', async () => {
    const projectApi = api(); projectApi.list.mockRejectedValueOnce(new Error('offline'))
    mounted(['delivery:project:read'], projectApi); await flushPromises()
    expect(showError).toHaveBeenCalledWith('network.error')
  })
})
