/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { showError } = vi.hoisted(() => ({ showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { error: showError } }))

const ATable = defineComponent({
  props: ['columns', 'dataSource'],
  setup(props, { slots }) {
    return () => h('div', (props.dataSource ?? []).flatMap((record: unknown) =>
      (props.columns ?? []).map((column: unknown) => slots.bodyCell?.({ column, record }))))
  },
})
const AInputNumber = defineComponent({
  name: 'AInputNumber', emits: ['change'],
  setup: (_, { emit }) => () => h('div', [
    h('button', { class: 'account', onClick: () => emit('change', 9) }),
    h('button', { class: 'account-invalid', onClick: () => emit('change', Number.MAX_SAFE_INTEGER + 2) }),
  ]),
})
const ASelect = defineComponent({
  name: 'ASelect', props: ['dataTestid'], emits: ['change'],
  setup: (props, { emit }) => () => h('button', {
    class: props.dataTestid,
    onClick: () => emit('change', props.dataTestid === 'resource-type' ? 'network' : 'failed'),
  }),
})
const ADatePicker = defineComponent({
  name: 'ADatePicker', emits: ['change'],
  setup: (_, { emit }) => () => h('button', {
    class: 'started-after',
    onClick: () => emit('change', { toDate: () => new Date('2026-07-15T12:34:56.000Z') }, '2026-07-15 12:34:56'),
  }),
})
const APagination = defineComponent({ name: 'APagination', emits: ['change', 'showSizeChange'], setup: (_, { emit }) => () => h('button', { class: 'page', onClick: () => emit('change', 2) }) })
const ATooltip = defineComponent({ name: 'ATooltip', props: ['title'], setup: (_, { slots }) => () => h('span', { class: 'tooltip' }, slots.default?.()) })
const ATag = defineComponent({ name: 'ATag', props: ['color'], setup: (_, { slots }) => () => h('span', { class: 'status-tag' }, slots.default?.()) })
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': ATable,
  'a-input-number': AInputNumber, 'a-select': ASelect, 'a-date-picker': ADatePicker,
  'a-pagination': APagination, 'a-tag': ATag,
  'a-tooltip': ATooltip, PageHeader: true,
}

const row = {
  id: 1, cloud_account_id: 9, cloud_account_name: 'prod', region: 'eu-west-1',
  resource_type: 'network' as const, started_at: '2026-07-15T12:00:00Z',
  finished_at: '2026-07-15T12:00:01.500Z', status: 'failed' as const,
  items_seen: 4, items_softdeleted: 0, error: 'internal detail must stay in tooltip',
  error_code: 50001, trigger: 'cron' as const,
}

describe('Assets sync-runs list', () => {
  it('loads API pagination, renders status and keeps error text inside a tooltip', async () => {
    const api = { listRuns: vi.fn().mockResolvedValue({ items: [row], total: 1 }) }
    const wrapper = mount(List, { global: { provide: { assetsSyncApi: api }, stubs } })
    await vi.waitFor(() => expect(api.listRuns).toHaveBeenCalledWith({ page: 1, size: 20 }))
    await vi.waitFor(() => expect(wrapper.findComponent(ATooltip).exists()).toBe(true))
    expect(wrapper.text()).toContain('assets.sync.run_status.failed')
    expect(wrapper.findComponent(ATag).props('color')).toBe('red')
    expect(wrapper.text()).toContain('50001')
    expect(wrapper.text()).not.toContain(row.error)
    expect(wrapper.findComponent(ATooltip).props('title')).toBe(row.error)
    expect(wrapper.find('.error-code').element.tagName).toBe('BUTTON')
    expect(wrapper.find('.error-code').attributes('aria-label')).toBe('assets.sync.error')
  })

  it('sends account, resource, status and RFC3339 started-after filters, then paginates', async () => {
    const api = { listRuns: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsSyncApi: api }, stubs } })
    await vi.waitFor(() => expect(api.listRuns).toHaveBeenCalledTimes(1))
    await wrapper.find('.account').trigger('click')
    await wrapper.find('.resource-type').trigger('click')
    await wrapper.find('.status').trigger('click')
    await wrapper.find('.started-after').trigger('click')
    await vi.waitFor(() => expect(api.listRuns).toHaveBeenLastCalledWith({
      page: 1, size: 20, account_id: 9, resource_type: 'network', status: 'failed',
      started_after: '2026-07-15T12:34:56.000Z',
    }))
    await wrapper.find('.page').trigger('click')
    await vi.waitFor(() => expect(api.listRuns).toHaveBeenLastCalledWith({
      page: 2, size: 20, account_id: 9, resource_type: 'network', status: 'failed',
      started_after: '2026-07-15T12:34:56.000Z',
    }))
  })

  it('catches initial, filter and pagination failures locally', async () => {
    const api = { listRuns: vi.fn().mockRejectedValue(new Error('offline')) }
    const wrapper = mount(List, { global: { provide: { assetsSyncApi: api }, stubs } })
    await vi.waitFor(() => expect(showError).toHaveBeenCalledWith('network.error'))
    showError.mockClear()
    await wrapper.find('.account').trigger('click'); await nextTick()
    await vi.waitFor(() => expect(showError).toHaveBeenCalledWith('network.error'))
    showError.mockClear()
    await wrapper.find('.page').trigger('click'); await nextTick()
    await vi.waitFor(() => expect(showError).toHaveBeenCalledWith('network.error'))
  })

  it('rejects unsafe account IDs instead of querying a rounded account', async () => {
    const api = { listRuns: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsSyncApi: api }, stubs } })
    await vi.waitFor(() => expect(api.listRuns).toHaveBeenCalledTimes(1))
    await wrapper.find('.account-invalid').trigger('click')
    await vi.waitFor(() => expect(api.listRuns).toHaveBeenLastCalledWith({ page: 1, size: 20, account_id: undefined }))
  })
})
