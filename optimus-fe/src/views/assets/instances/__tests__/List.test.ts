/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { permissionState } = vi.hoisted(() => ({ permissionState: { canReadAccounts: true } }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => permissionState.canReadAccounts }) }))
const { showError } = vi.hoisted(() => ({ showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { error: showError } }))

const AInputSearch = defineComponent({ name: 'AInputSearch', emits: ['search', 'change'], setup: (_, { emit }) => () => h('button', { class: 'search', onClick: () => emit('search', 'web') }) })
const AInput = defineComponent({ name: 'AInput', props: ['dataTestid'], emits: ['pressEnter', 'change'], setup: (props, { emit }) => () => h('button', { class: props.dataTestid, onClick: () => emit('pressEnter') }) })
const ASelect = defineComponent({ name: 'ASelect', props: ['dataTestid'], emits: ['change'], setup: (props, { emit }) => () => h('button', { class: props.dataTestid, onClick: () => emit('change', props.dataTestid === 'account' ? 7 : 'running') }) })
const AInputNumber = defineComponent({ name: 'AInputNumber', emits: ['change'], setup: (_, { emit }) => () => h('button', { class: 'account-id', onClick: () => emit('change', 9) }) })
const ACheckbox = defineComponent({ name: 'ACheckbox', emits: ['change'], setup: (_, { emit }) => () => h('button', { class: 'deleted', onClick: () => emit('change', { target: { checked: true } }) }) })
const APagination = defineComponent({ name: 'APagination', emits: ['change', 'showSizeChange'], setup: (_, { emit }) => () => h('button', { class: 'page', onClick: () => emit('change', 3) }) })

const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': { template: '<div />' },
  'a-input-search': AInputSearch, 'a-input': AInput,
  'a-select': ASelect, 'a-input-number': AInputNumber, 'a-checkbox': ACheckbox, 'a-pagination': APagination,
  'a-tag': { template: '<span><slot/></span>' }, PageHeader: true,
}

describe('Assets instance list', () => {
  const accountApi = { list: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
  beforeEach(() => {
    permissionState.canReadAccounts = true
    accountApi.list.mockClear()
    showError.mockClear()
  })
  it('loads with API pagination and sends search, account, state and deleted filters', async () => {
    const api = { listInstances: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accountApi }, stubs } })
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenCalledWith({ page: 1, size: 20 }))

    await wrapper.find('.search').trigger('click')
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenLastCalledWith({ page: 1, size: 20, q: 'web' }))
    await wrapper.find('.account').trigger('click')
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenLastCalledWith({ page: 1, size: 20, q: 'web', account_id: 7 }))
    await wrapper.find('.state').trigger('click')
    await wrapper.find('.deleted').trigger('click')
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenLastCalledWith({ page: 1, size: 20, q: 'web', account_id: 7, state: 'running', include_deleted: true }))
    await wrapper.find('.page').trigger('click')
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenLastCalledWith({ page: 3, size: 20, q: 'web', account_id: 7, state: 'running', include_deleted: true }))
  })

  it('catches initial and filter load failures locally', async () => {
    const api = { listInstances: vi.fn().mockRejectedValue(new Error('offline')) }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accountApi }, stubs } })
    await vi.waitFor(() => expect(showError).toHaveBeenCalledWith('network.error'))
    showError.mockClear()
    await wrapper.find('.search').trigger('click'); await nextTick()
    await vi.waitFor(() => expect(showError).toHaveBeenCalledWith('network.error'))
  })

  it('does not call the account-read API for a resource-only role', async () => {
    permissionState.canReadAccounts = false
    const api = { listInstances: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accountApi }, stubs } })
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenCalledWith({ page: 1, size: 20 }))
    expect(accountApi.list).not.toHaveBeenCalled()
    await wrapper.find('.account-id').trigger('click')
    await vi.waitFor(() => expect(api.listInstances).toHaveBeenLastCalledWith({ page: 1, size: 20, account_id: 9 }))
  })
})
