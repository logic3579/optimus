import { describe, it, expect } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { vi } from 'vitest'
import HistoryTable from '../components/HistoryTable.vue'
import { useAuthStore } from '@/stores/auth'
import { permissionDirective } from '@/directives/permission'
import type { RevisionRow } from '@/types/apps'

vi.mock('@/hooks/useI18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

function rows(): RevisionRow[] {
  return [
    { revision: 1, status: 'superseded', chart_version: '1.0.0', app_version: '1.0', updated_at: '2026-01-01T00:00:00Z', description: 'initial' },
    { revision: 2, status: 'deployed',   chart_version: '1.1.0', app_version: '1.0', updated_at: '2026-02-01T00:00:00Z', description: 'bump'   },
  ]
}

/**
 * a-table stub that renders the bodyCell slot for the actions column so we
 * can assert against the rollback button without bringing in ant-design-vue.
 */
const aTableStub = {
  props: ['columns', 'dataSource'],
  template: `
    <table class="ant-table-stub">
      <tbody>
        <tr v-for="(record, i) in dataSource" :key="i" :data-rev="record.revision">
          <td v-for="col in columns" :key="col.key" :data-col="col.key">
            <slot name="bodyCell" :column="col" :record="record"/>
          </td>
        </tr>
      </tbody>
    </table>`,
}

const aPopconfirmStub = {
  emits: ['confirm'],
  template: `<span class="ant-popconfirm-stub" @click="$emit('confirm')"><slot/></span>`,
}

const aButtonStub = {
  props: ['disabled'],
  template: `<button class="ant-btn-stub" :disabled="disabled"><slot/></button>`,
}

const aTagStub = { template: '<span class="ant-tag-stub"><slot/></span>' }

const stubs = {
  'a-table': aTableStub,
  'a-popconfirm': aPopconfirmStub,
  'a-button': aButtonStub,
  'a-tag': aTagStub,
}

describe('HistoryTable', () => {
  it('disables rollback on the current revision and emits revision number otherwise', async () => {
    setActivePinia(createPinia())
    useAuthStore().setPermissions(['apps:release:rollback'])
    const wrapper = mount(HistoryTable, {
      props: { rows: rows(), currentRevision: 2 },
      global: { directives: { permission: permissionDirective }, stubs },
    })
    // Row 1 = older → rollback button should be enabled.
    const row1Btn = wrapper.find('tr[data-rev="1"] [data-col="actions"] button')
    expect(row1Btn.exists()).toBe(true)
    expect((row1Btn.element as HTMLButtonElement).disabled).toBe(false)

    // Row 2 = current → disabled.
    const row2Btn = wrapper.find('tr[data-rev="2"] [data-col="actions"] button')
    expect(row2Btn.exists()).toBe(true)
    expect((row2Btn.element as HTMLButtonElement).disabled).toBe(true)

    // The popconfirm wraps the rollback button; clicking it fires `confirm`,
    // which our stub re-emits and the component translates into `rollback`.
    await wrapper.find('tr[data-rev="1"] [data-col="actions"] .ant-popconfirm-stub').trigger('click')
    const ev = wrapper.emitted('rollback')
    expect(ev).toBeTruthy()
    expect(ev![0]).toEqual([1])
  })

  it('hides the rollback button when the viewer lacks apps:release:rollback', () => {
    setActivePinia(createPinia())
    useAuthStore().setPermissions([])
    const wrapper = mount(HistoryTable, {
      props: { rows: rows(), currentRevision: 2 },
      global: { directives: { permission: permissionDirective }, stubs },
    })
    // v-permission removes the element entirely when the perm is missing.
    expect(wrapper.findAll('[data-col="actions"] button')).toHaveLength(0)
  })
})
