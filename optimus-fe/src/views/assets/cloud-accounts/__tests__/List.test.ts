/* eslint-disable vue/require-prop-types */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import List from '../List.vue'
import { permissionDirective } from '@/directives/permission'
import { useAssetsStore } from '@/stores/assets'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string, args?: { count?: number }) => args?.count === undefined ? key : `${key}:${args.count}` }) }))
const { success } = vi.hoisted(() => ({ success: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { success, error: vi.fn() } }))

const row = {
  id: 3, name: 'prod', provider: 'aws' as const, cloudkey_id: 8, cloudkey_name: 'key-prod',
  regions_count: 2, enabled: true, last_sync_at: '2026-07-16T10:00:00Z', last_sync_status: 'success',
  created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
}
function makeApi() {
  return {
    list: vi.fn().mockResolvedValue({ items: [row], total: 1 }),
    get: vi.fn().mockResolvedValue({ ...row, enabled_regions: ['eu-west-1'], description: '' }),
    create: vi.fn(), update: vi.fn(),
    remove: vi.fn().mockResolvedValue({ cascaded_resources_count: 4 }),
    triggerSync: vi.fn().mockResolvedValue({ queued: true, started_at: '2026-07-16T10:01:00Z' }),
  }
}
const ATable = defineComponent({
  props: ['columns', 'dataSource', 'loading'],
  setup(props, { slots }) {
    return () => h('div', { class: 'table-stub' }, (props.dataSource ?? []).map((record: unknown) =>
      h('div', { class: 'table-row' }, (props.columns ?? []).map((column: unknown) => slots.bodyCell?.({ column, record }))),
    ))
  },
})
const stubs = {
  'a-card': { template: '<div><slot/></div>' },
  'a-input-search': true, 'a-select': true, 'a-tag': { template: '<span><slot/></span>' },
  'a-space': { template: '<div><slot/></div>' }, 'a-pagination': true,
  'a-button': { template: '<button><slot/></button>' },
  'a-popconfirm': { emits: ['confirm'], template: '<span><slot/><button class="confirm-delete" @click="$emit(\'confirm\')">confirm</button></span>' },
  'a-table': ATable,
  PageHeader: { template: '<div><slot/></div>' },
  Form: true,
}

describe('CloudAccount List', () => {
  beforeEach(() => { setActivePinia(createPinia()); success.mockClear() })

  function mountList(api: ReturnType<typeof makeApi>, permissions: string[]) {
    useAuthStore().setPermissions(permissions)
    return mount(List, {
      global: {
        provide: { assetsAccountApi: api }, stubs,
        directives: { permission: permissionDirective },
      },
    })
  }

  it('loads and renders rows, then queues sync and tracks it in the assets store', async () => {
    vi.useFakeTimers()
    const api = makeApi()
    const wrapper = mountList(api, ['assets:account:write'])
    await vi.runAllTicks(); await nextTick(); await nextTick()
    expect(api.list).toHaveBeenCalledWith({ page: 1, size: 20 })
    expect(wrapper.text()).toContain('prod')

    await wrapper.find('[data-testid="sync-3"]').trigger('click')
    await vi.runAllTicks(); await nextTick()
    expect(api.triggerSync).toHaveBeenCalledWith(3)
    expect(useAssetsStore().syncInFlight[3]).toBe(true)
    expect(success).toHaveBeenCalledWith('assets.account.sync_queued')
    vi.advanceTimersByTime(30_000)
    expect(useAssetsStore().syncInFlight[3]).toBeUndefined()
    vi.useRealTimers()
  })

  it('reports delete cascade count and reloads the table', async () => {
    const api = makeApi()
    const wrapper = mountList(api, ['assets:account:delete'])
    await vi.waitFor(() => expect(wrapper.find('.confirm-delete').exists()).toBe(true))
    await wrapper.find('.confirm-delete').trigger('click')
    await nextTick(); await nextTick()
    expect(api.remove).toHaveBeenCalledWith(3)
    expect(success).toHaveBeenCalledWith('assets.account.cascaded_resources:4')
    expect(api.list).toHaveBeenCalledTimes(2)
  })

  it('hides write and delete actions without their respective permissions', async () => {
    const api = makeApi()
    const wrapper = mountList(api, [])
    await nextTick(); await nextTick()
    expect(wrapper.find('[data-testid="create-account"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sync-3"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="edit-3"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="delete-3"]').exists()).toBe(false)
  })
})
