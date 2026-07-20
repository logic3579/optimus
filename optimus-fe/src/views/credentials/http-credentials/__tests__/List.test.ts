import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn(), error: vi.fn() } }))

const stubs = {
  'a-card': { template: '<div><slot/></div>' },
  'a-alert': { template: '<div><slot/></div>' },
  'a-input-search': { template: '<input />' },
  'a-select': { props: ['options'], template: '<div />' },
  'a-button': { template: '<button data-testid="create"><slot/></button>' },
  'a-table': { template: '<div><slot name="bodyCell" :column="{key:\'actions\'}" :record="{id:3,name:\'cred\'}"/></div>' },
  'a-space': { template: '<div><slot/></div>' },
  'a-popconfirm': { template: '<button data-testid="confirm" @click="$emit(\'confirm\')"><slot/></button>' },
  'a-pagination': { template: '<button data-testid="page" @click="$emit(\'change\',2,20)" />' },
  PageHeader: true,
  HttpCredentialEditModal: true,
}

function mounted(permissions: string[]) {
  const api = { list: vi.fn().mockResolvedValue({ items: [], total: 41 }), get: vi.fn(), remove: vi.fn().mockResolvedValue(undefined) }
  const wrapper = mount(List, {
    global: {
      plugins: [createPinia()], provide: { httpCredentialApi: api }, stubs,
      directives: { permission: { mounted(el: HTMLElement, binding: any) { if (!permissions.includes(binding.value)) el.style.display = 'none' } } },
      mocks: { $t: (key: string) => key },
    },
  })
  return { wrapper, api, vm: wrapper.vm as any }
}

describe('HTTP credential list', () => {
  beforeEach(() => setActivePinia(createPinia()))
  it('gates write/delete controls and confirms delete', async () => {
    const readOnly = mounted(['credentials:http:read']).wrapper
    expect(readOnly.find('[data-testid="create"]').isVisible()).toBe(false)
    readOnly.unmount()
    const { wrapper, api } = mounted(['credentials:http:write', 'credentials:http:delete'])
    expect(wrapper.find('[data-testid="create"]').exists()).toBe(true)
    await wrapper.find('[data-testid="confirm"]').trigger('click')
    expect(api.remove).toHaveBeenCalledWith(3)
  })

  it('loads and changes pagination without erasing API errors', async () => {
    const { wrapper, api, vm } = mounted(['credentials:http:read'])
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledWith(expect.objectContaining({ page: 1, page_size: 20 })))
    await wrapper.find('[data-testid="page"]').trigger('click')
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, page_size: 20 }))
    api.list.mockRejectedValueOnce({ message: 'upstream unavailable', code: 44102 })
    await vm.table.reload().catch(() => undefined)
    expect(vm.errorMessage).toBe('upstream unavailable')
  })
})
