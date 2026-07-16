/* eslint-disable vue/require-prop-types */
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => true }) }))
vi.mock('ant-design-vue', () => ({ message: { error: vi.fn() } }))
const ASelect = defineComponent({ name: 'ASelect', props: ['dataTestid'], emits: ['change'], setup: (props, { emit }) => () => h('button', { class: props.dataTestid, onClick: () => emit('change', 'available') }) })
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': { template: '<div />' },
  'a-input-search': true, 'a-input-number': true, 'a-input': true, 'a-checkbox': true,
  'a-select': ASelect, 'a-pagination': true, 'a-tag': { template: '<span><slot/></span>' }, PageHeader: true,
}

describe('Assets database list', () => {
  it('loads API pagination and applies the database status filter', async () => {
    const api = { listDatabases: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const accountApi = { list: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accountApi }, stubs } })
    await vi.waitFor(() => expect(api.listDatabases).toHaveBeenCalledWith({ page: 1, size: 20 }))
    await wrapper.find('.status').trigger('click')
    await vi.waitFor(() => expect(api.listDatabases).toHaveBeenLastCalledWith({ page: 1, size: 20, status: 'available' }))
  })
})
