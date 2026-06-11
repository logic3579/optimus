/* eslint-disable vue/require-prop-types */
import { describe, it, expect, vi } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import ValuesEditor from '../components/ValuesEditor.vue'

// Stub CodeMirror — the real one needs a DOM editor view that jsdom can't
// fully drive. We replace it with a textarea that proxies model-value updates.
vi.mock('vue-codemirror', () => ({
  Codemirror: defineComponent({
    name: 'CodemirrorStub',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    setup(props, { emit }) {
      return () =>
        h('textarea', {
          class: 'cm-stub',
          value: props.modelValue,
          onInput: (e: Event) =>
            emit('update:modelValue', (e.target as HTMLTextAreaElement).value),
        })
    },
  }),
}))
vi.mock('@codemirror/lang-yaml',     () => ({ yaml: () => ({}) }))
vi.mock('@codemirror/theme-one-dark', () => ({ oneDark: {} }))
vi.mock('@codemirror/view',           () => ({ EditorView: { lineWrapping: {} } }))
vi.mock('ant-design-vue', () => ({
  message: { warning: vi.fn(), error: vi.fn(), success: vi.fn() },
  Modal: { confirm: ({ onOk }: { onOk?: () => void }) => { onOk?.() } },
}))
// Pretty much every view file imports useI18n via this wrapper. Replace it
// with an identity-style stub so we don't need to bootstrap vue-i18n.
vi.mock('@/hooks/useI18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

function makeRepoApi() {
  return {
    list: vi.fn(),
    get: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    listCharts: vi.fn(),
    listChartVersions: vi.fn(),
    getDefaultValues: vi.fn(),
  }
}

function stubButton() {
  return {
    inheritAttrs: false,
    props: ['loading'],
    emits: ['click'],
    template: '<button :data-loading="loading" @click="$emit(\'click\', $event)"><slot/></button>',
  }
}

const globalStubs = {
  'a-button': stubButton(),
  'a-typography-text': { template: '<span class="ant-typography"><slot/></span>' },
}

describe('ValuesEditor', () => {
  it('emits update:modelValue when CodeMirror buffer changes', async () => {
    const repoApi = makeRepoApi()
    const wrapper = mount(ValuesEditor, {
      props: { modelValue: '', repoId: 1, chartName: 'demo', chartVersion: '1.0.0' },
      global: {
        provide: { appsRepoApi: repoApi },
        stubs: globalStubs,
      },
    })
    await wrapper.find('textarea.cm-stub').setValue('replicaCount: 2\n')
    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted![0]).toEqual(['replicaCount: 2\n'])
  })

  it('Load defaults — calls repoApi.getDefaultValues + emits the fetched yaml', async () => {
    const repoApi = makeRepoApi()
    repoApi.getDefaultValues.mockResolvedValue({ values_yaml: 'image: nginx\n' })
    const wrapper = mount(ValuesEditor, {
      props: { modelValue: '', repoId: 5, chartName: 'demo', chartVersion: '1.0.0' },
      global: {
        provide: { appsRepoApi: repoApi },
        stubs: globalStubs,
      },
    })
    // First button is "load defaults" per template order.
    await wrapper.findAll('button')[0]!.trigger('click')
    // Wait for the async resolver to flush.
    await nextTick(); await nextTick()
    expect(repoApi.getDefaultValues).toHaveBeenCalledWith(5, 'demo', '1.0.0')
    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted?.some(e => e[0] === 'image: nginx\n')).toBe(true)
  })

  it('Format — runs the buffer through js-yaml and re-emits canonical YAML', async () => {
    const repoApi = makeRepoApi()
    const wrapper = mount(ValuesEditor, {
      props: { modelValue: 'replicaCount:    5\nimage:\n  repo:   nginx\n', repoId: 1, chartName: 'demo', chartVersion: '1.0.0' },
      global: {
        provide: { appsRepoApi: repoApi },
        stubs: globalStubs,
      },
    })
    // Second button is "format".
    await wrapper.findAll('button')[1]!.trigger('click')
    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    // Should round-trip cleanly with 2-space indent (no quadruple spaces).
    const formatted = emitted!.at(-1)![0] as string
    expect(formatted).toMatch(/replicaCount: 5/)
    expect(formatted).not.toMatch(/replicaCount:    5/)
  })
})
