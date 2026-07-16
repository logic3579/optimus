/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Form from '../Form.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', () => ({
  message: { success: vi.fn(), error: vi.fn() },
}))

function makeApis() {
  return {
    account: {
      list: vi.fn(), get: vi.fn(), remove: vi.fn(), triggerSync: vi.fn(),
      create: vi.fn().mockResolvedValue({ id: 1 }),
      update: vi.fn().mockResolvedValue({ id: 7 }),
    },
    cloudKey: {
      list: vi.fn().mockResolvedValue({
        items: [{ id: 11, name: 'aws-prod', provider: 'aws' }], total: 1,
      }),
    },
  }
}

const AModal = {
  props: ['open', 'title', 'confirmLoading'],
  emits: ['ok', 'cancel'],
  template: '<div v-if="open"><slot/><button class="modal-ok" @click="$emit(\'ok\')">ok</button></div>',
}
const AForm = defineComponent({
  props: ['model', 'rules'],
  setup(_, { expose, slots }) {
    expose({ validate: vi.fn().mockResolvedValue(undefined), resetFields: vi.fn() })
    return () => h('form', slots.default?.())
  },
})
const AInput = defineComponent({
  name: 'AInput', props: ['value'], emits: ['update:value'],
  setup(props, { emit }) {
    return () => h('input', { class: 'name-input', value: props.value,
      onInput: (event: Event) => emit('update:value', (event.target as HTMLInputElement).value) })
  },
})
const ATextarea = defineComponent({
  name: 'ATextarea', props: ['value'], emits: ['update:value'],
  setup(props, { emit }) {
    return () => h('textarea', { class: 'description-input', value: props.value,
      onInput: (event: Event) => emit('update:value', (event.target as HTMLTextAreaElement).value) })
  },
})
const ASelect = defineComponent({
  name: 'ASelect', props: ['value', 'options', 'disabled', 'mode'], emits: ['update:value'],
  setup(_, { slots }) { return () => h('div', { class: 'select-stub' }, slots.default?.()) },
})
const ASwitch = defineComponent({
  name: 'ASwitch', props: ['checked'], emits: ['update:checked'],
  setup() { return () => h('div', { class: 'switch-stub' }) },
})
const stubs = {
  'a-modal': AModal, 'a-form': AForm,
  'a-form-item': { template: '<div><slot/></div>' },
  'a-input': AInput, 'a-textarea': ATextarea, 'a-select': ASelect, 'a-switch': ASwitch,
}

describe('CloudAccount Form', () => {
  it('loads AWS cloud keys and sends the complete create payload', async () => {
    const apis = makeApis()
    const wrapper = mount(Form, {
      props: { open: true, editing: null },
      global: { provide: { assetsAccountApi: apis.account, cloudKeyApi: apis.cloudKey }, stubs },
    })
    await nextTick(); await nextTick()
    expect(apis.cloudKey.list).toHaveBeenCalledWith({ page: 1, page_size: 100, provider: 'aws' })

    await wrapper.find('.name-input').setValue('production')
    const selects = wrapper.findAllComponents(ASelect)
    selects[0]!.vm.$emit('update:value', 11)
    selects[1]!.vm.$emit('update:value', ['eu-central-1', 'us-east-1'])
    await wrapper.find('.description-input').setValue('primary account')
    await wrapper.find('.modal-ok').trigger('click')
    await nextTick(); await nextTick()

    expect(apis.account.create).toHaveBeenCalledWith({
      name: 'production', provider: 'aws', cloudkey_id: 11,
      enabled_regions: ['eu-central-1', 'us-east-1'], enabled: true,
      description: 'primary account',
    })
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('keeps credential binding immutable and sends only mutable edit fields', async () => {
    const apis = makeApis()
    const editing = {
      id: 7, name: 'old', provider: 'aws' as const, cloudkey_id: 11, cloudkey_name: 'aws-prod',
      regions_count: 1, enabled_regions: ['eu-west-1'], enabled: false, description: 'old desc',
      created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    }
    const wrapper = mount(Form, {
      props: { open: true, editing },
      global: { provide: { assetsAccountApi: apis.account, cloudKeyApi: apis.cloudKey }, stubs },
    })
    await nextTick(); await nextTick()
    expect(wrapper.findAllComponents(ASelect)[0]!.props('disabled')).toBe(true)
    await wrapper.find('.name-input').setValue('renamed')
    wrapper.findAllComponents(ASelect)[1]!.vm.$emit('update:value', ['ap-southeast-1'])
    wrapper.findComponent(ASwitch).vm.$emit('update:checked', true)
    await wrapper.find('.modal-ok').trigger('click')
    await nextTick(); await nextTick()

    expect(apis.account.update).toHaveBeenCalledWith(7, {
      name: 'renamed', enabled_regions: ['ap-southeast-1'], enabled: true, description: 'old desc',
    })
  })
})
