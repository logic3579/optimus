/* eslint-disable vue/one-component-per-file */
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import Detail from '../Detail.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { showError } = vi.hoisted(() => ({ showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { error: showError } }))
let routeID: string | string[] = '12'
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { get id() { return routeID } } }),
  useRouter: () => ({ back: vi.fn() }),
}))

const AInputSearch = defineComponent({ name: 'AInputSearch', emits: ['search'], setup: (_, { emit }) => () => h('button', { class: 'search', onClick: () => emit('search', 'private') }) })
const APagination = defineComponent({ name: 'APagination', emits: ['change'], setup: (_, { emit }) => () => h('button', { class: 'page', onClick: () => emit('change', 2) }) })
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': { template: '<div />' },
  'a-input-search': AInputSearch, 'a-checkbox': true, 'a-pagination': APagination,
  'a-button': true, 'a-tag': true,
  PageHeader: true,
}

describe('Assets VPC detail', () => {
  beforeEach(() => { routeID = '12'; showError.mockClear() })

  it('loads and paginates/filter subnets using the validated VPC row ID', async () => {
    const api = { listSubnets: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(Detail, { global: { provide: { assetsResourceApi: api }, stubs } })
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenCalledWith(12, { page: 1, size: 20 }))
    await wrapper.find('.search').trigger('click')
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenLastCalledWith(12, { page: 1, size: 20, q: 'private' }))
    await wrapper.find('.page').trigger('click')
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenLastCalledWith(12, { page: 2, size: 20, q: 'private' }))
  })

  it('rejects invalid IDs without calling the API', async () => {
    routeID = '12oops'
    const api = { listSubnets: vi.fn() }
    mount(Detail, { global: { provide: { assetsResourceApi: api }, stubs } })
    await vi.waitFor(() => expect(showError).toHaveBeenCalledWith('network.error'))
    expect(api.listSubnets).not.toHaveBeenCalled()
  })
})
