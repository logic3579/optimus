/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, inject, provide } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import Install from '../Install.vue'

vi.mock('@/hooks/useI18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('ant-design-vue', () => ({
  message: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

const inputStub = defineComponent({
  props: ['value', 'disabled'],
  emits: ['update:value'],
  setup(props, { emit }) {
    const getFormModel = inject<() => Record<string, unknown>>('test-form-model')!
    return () => h('input', {
      class: 'input-stub',
      disabled: props.disabled,
      value: props.value,
      onInput: (event: Event) => {
        emit('update:value', (event.target as HTMLInputElement).value)
        formModelAtControlUpdate = { ...getFormModel() }
      },
    })
  },
})

const selectStub = defineComponent({
  props: ['value', 'options', 'disabled'],
  emits: ['update:value'],
  setup(props, { emit }) {
    const getFormModel = inject<() => Record<string, unknown>>('test-form-model')!
    return () => h('select', {
      class: 'select-stub',
      disabled: props.disabled,
      value: props.value,
      onChange: (event: Event) => {
        emit('update:value', Number((event.target as HTMLSelectElement).value))
        formModelAtSelectUpdate = { ...getFormModel() }
      },
    }, (props.options ?? []).map((option: { label: string; value: number }) =>
      h('option', { value: option.value }, option.label),
    ))
  },
})

const chartPickerStub = defineComponent({
  emits: ['update:repoId', 'update:chartName', 'update:version'],
  setup(_, { emit }) {
    return () => h('button', {
      class: 'pick-chart',
      onClick: () => {
        emit('update:repoId', 9)
        emit('update:chartName', 'kafka')
        emit('update:version', '32.4.3')
      },
    }, 'pick chart')
  },
})

const buttonStub = defineComponent({
  props: ['disabled', 'loading'],
  emits: ['click'],
  setup(props, { emit, slots }) {
    return () => h('button', {
      class: 'button-stub',
      disabled: props.disabled,
      onClick: () => emit('click'),
    }, slots.default?.())
  },
})

let formModelAtControlUpdate: Record<string, unknown> = {}
let formModelAtSelectUpdate: Record<string, unknown> = {}
const formStub = defineComponent({
  props: ['model'],
  setup(props, { slots }) {
    provide('test-form-model', () => props.model as Record<string, unknown>)
    return () => h('form', slots.default?.())
  },
})

const passthrough = { template: '<div><slot/></div>' }

describe('application install wizard', () => {
  it('keeps all basic fields reactive and creates the application on Next', async () => {
    formModelAtControlUpdate = {}
    formModelAtSelectUpdate = {}
    const create = vi.fn().mockResolvedValue({ id: 17 })
    const wrapper = mount(Install, {
      global: {
        provide: {
          appsApplicationApi: { create },
          appsReleaseApi: { install: vi.fn() },
          clusterApi: { list: vi.fn().mockResolvedValue({ items: [{ id: 1, name: 'smoke-colima' }] }) },
          userApi: { list: vi.fn().mockResolvedValue({ items: [] }) },
        },
        directives: { permission: {} },
        stubs: {
          PageHeader: passthrough,
          ChartPickerStep: chartPickerStub,
          ValuesEditor: true,
          'a-card': passthrough,
          'a-steps': passthrough,
          'a-step': true,
          'a-row': passthrough,
          'a-col': passthrough,
          'a-form': formStub,
          'a-form-item': passthrough,
          'a-space': passthrough,
          'a-input': inputStub,
          'a-textarea': inputStub,
          'a-select': selectStub,
          'a-button': buttonStub,
        },
      },
    })

    await flushPromises()
    const inputs = wrapper.findAll('input.input-stub')
    await inputs[0]!.setValue('smoke-kafka')
    await inputs[1]!.setValue('smoke-kafka')
    await wrapper.find('select.select-stub').setValue('1')
    expect(formModelAtSelectUpdate.cluster_id).toBe(1)
    await inputs[2]!.setValue('optimus-smoke')
    expect(formModelAtControlUpdate).toMatchObject({
      name: 'smoke-kafka',
      release_name: 'smoke-kafka',
      cluster_id: 1,
      namespace: 'optimus-smoke',
    })
    await wrapper.find('button.pick-chart').trigger('click')

    const next = wrapper.findAll('button.button-stub').find((button) => button.text() === 'common.button.next')
    expect(next).toBeDefined()
    expect(next!.attributes('disabled')).toBeUndefined()

    await next!.trigger('click')
    await flushPromises()
    expect(create).toHaveBeenCalledWith({
      name: 'smoke-kafka',
      release_name: 'smoke-kafka',
      cluster_id: 1,
      namespace: 'optimus-smoke',
      chart_repo_id: 9,
      chart_name: 'kafka',
      description: undefined,
      tags: undefined,
      owner_user_id: undefined,
    })
  })
})
