/* eslint-disable vue/require-prop-types */
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => true }) }))
vi.mock('ant-design-vue', () => ({ message: { error: vi.fn() } }))
const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

const ATable = defineComponent({
  props: ['dataSource', 'customRow'],
  setup(props) { return () => props.dataSource?.length
    ? h('div', { class: 'row', ...props.customRow?.(props.dataSource[0]) })
    : h('span') },
})
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': ATable,
  'a-input-search': true, 'a-select': true, 'a-input': true, 'a-checkbox': true,
  'a-input-number': true,
  'a-pagination': true, 'a-tag': { template: '<span><slot/></span>' }, PageHeader: true,
}

describe('Assets VPC list', () => {
  it('loads API pagination and routes a row to the static detail route', async () => {
    const api = { listVPCs: vi.fn().mockResolvedValue({ items: [{ id: 19, vpc_id: 'vpc-1' }], total: 1 }) }
    const accountApi = { list: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accountApi }, stubs } })
    await vi.waitFor(() => expect(api.listVPCs).toHaveBeenCalledWith({ page: 1, size: 20 }))
    await vi.waitFor(() => expect(wrapper.find('.row').exists()).toBe(true))
    await wrapper.find('.row').trigger('click')
    expect(push).toHaveBeenCalledWith('/assets/vpcs/19')
    push.mockClear()
    await wrapper.find('.row').trigger('keydown', { key: 'Enter' })
    expect(push).toHaveBeenCalledWith('/assets/vpcs/19')
    expect(wrapper.find('.row').attributes('role')).toBe('link')
    expect(wrapper.find('.row').attributes('tabindex')).toBe('0')
  })
})
