/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { describe, it, expect, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Form from '../Form.vue'

vi.mock('@/hooks/useI18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

// ant-design-vue: stub only the bits Form.vue actually imports as JS values
// (message, FormInstance). The components themselves are template-side stubs.
vi.mock('ant-design-vue', () => ({
  message: { success: vi.fn(), error: vi.fn() },
}))

function makeRepoApi() {
  return {
    list: vi.fn(),
    get: vi.fn(),
    create: vi.fn().mockResolvedValue({ id: 1 }),
    update: vi.fn().mockResolvedValue(undefined),
    delete: vi.fn(),
    listCharts: vi.fn(),
    listChartVersions: vi.fn(),
    getDefaultValues: vi.fn(),
  }
}

/**
 * a-modal stub — exposes the open prop and re-emits @ok/@cancel as clicks on
 * a couple of hard-coded buttons. We forward the default slot so the inner
 * form renders inside the test DOM.
 */
const aModalStub = {
  props: ['open', 'title', 'confirmLoading', 'width'],
  emits: ['ok', 'cancel', 'update:open'],
  template: `
    <div v-if="open" class="ant-modal-stub">
      <slot/>
      <button class="modal-ok" @click="$emit('ok')">OK</button>
      <button class="modal-cancel" @click="$emit('cancel')">Cancel</button>
    </div>`,
}

const aFormStub = defineComponent({
  name: 'AFormStub',
  props: ['model', 'rules'],
  setup(_, { expose, slots }) {
    expose({
      validate: vi.fn().mockResolvedValue(undefined),
      resetFields: vi.fn(),
    })
    return () => h('form', { class: 'ant-form-stub' }, slots.default?.())
  },
})

const aFormItemStub = { template: '<div class="ant-form-item-stub"><slot/></div>' }

const aInputStub = defineComponent({
  props: ['value', 'maxLength', 'placeholder', 'autocomplete'],
  emits: ['update:value'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        class: 'ant-input-stub',
        value: props.value,
        onInput: (e: Event) => emit('update:value', (e.target as HTMLInputElement).value),
      })
  },
})

const aInputPasswordStub = defineComponent({
  props: ['value', 'maxLength', 'placeholder', 'autocomplete'],
  emits: ['update:value'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        type: 'password',
        class: 'ant-input-password-stub',
        value: props.value,
        onInput: (e: Event) => emit('update:value', (e.target as HTMLInputElement).value),
      })
  },
})

const aRadioGroupStub = defineComponent({
  props: ['value', 'disabled'],
  emits: ['update:value'],
  setup(_, { slots }) { return () => h('div', { class: 'ant-radio-group-stub' }, slots.default?.()) },
})
const aRadioStub = {
  props: ['value'],
  template: '<label class="ant-radio-stub"><slot/></label>',
}
const aTextareaStub = defineComponent({
  props: ['value', 'rows', 'maxLength'],
  emits: ['update:value'],
  setup(props, { emit }) {
    return () =>
      h('textarea', {
        class: 'ant-textarea-stub',
        value: props.value,
        onInput: (e: Event) => emit('update:value', (e.target as HTMLTextAreaElement).value),
      })
  },
})

const aButtonStub = {
  props: ['size', 'danger', 'type'],
  template: '<button class="ant-btn-stub"><slot/></button>',
}
const aTypographyTextStub = { template: '<span class="ant-typography-stub"><slot/></span>' }

const stubs = {
  'a-modal': aModalStub,
  'a-form': aFormStub,
  'a-form-item': aFormItemStub,
  'a-input': aInputStub,
  'a-input-password': aInputPasswordStub,
  'a-radio-group': aRadioGroupStub,
  'a-radio': aRadioStub,
  'a-textarea': aTextareaStub,
  'a-button': aButtonStub,
  'a-typography-text': aTypographyTextStub,
}

describe('ChartRepos Form.vue', () => {
  it('create flow — sends a body with the entered fields and emits saved', async () => {
    const repoApi = makeRepoApi()
    const wrapper = mount(Form, {
      props: { open: true, original: null },
      global: { provide: { appsRepoApi: repoApi }, stubs },
    })
    // Fill name + url (type defaults to 'http').
    const inputs = wrapper.findAll('input.ant-input-stub')
    expect(inputs.length).toBeGreaterThanOrEqual(3)
    await inputs[0]!.setValue('stable')
    await inputs[1]!.setValue('https://charts.example.com')

    await wrapper.find('.modal-ok').trigger('click')
    await nextTick(); await nextTick()
    expect(repoApi.create).toHaveBeenCalledTimes(1)
    const body = repoApi.create.mock.calls[0]![0]
    expect(body.name).toBe('stable')
    expect(body.type).toBe('http')
    expect(body.url).toBe('https://charts.example.com')
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('edit flow — clear-password button maps to {password: null} on the wire', async () => {
    const repoApi = makeRepoApi()
    const original = {
      id: 7,
      name: 'stable',
      type: 'http' as const,
      url: 'https://charts.example.com',
      username: 'u',
      has_password: true,
      description: 'd',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }
    const wrapper = mount(Form, {
      props: { open: true, original },
      global: { provide: { appsRepoApi: repoApi }, stubs },
    })
    await nextTick()
    // The clear-password button is the only <a-button> rendered inside the
    // password form item (it only appears when has_password && isEdit).
    const clearBtn = wrapper.findAll('button.ant-btn-stub').find(b => /clearPassword/i.test(b.text()))
    expect(clearBtn).toBeTruthy()
    await clearBtn!.trigger('click')
    await nextTick()
    await wrapper.find('.modal-ok').trigger('click')
    await nextTick(); await nextTick()
    expect(repoApi.update).toHaveBeenCalledTimes(1)
    const [id, body] = repoApi.update.mock.calls[0]!
    expect(id).toBe(7)
    expect(body.password).toBe(null)
  })
})
