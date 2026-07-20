/* eslint-disable @typescript-eslint/no-explicit-any */
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DatasourceForm, { validateDatasourceURL } from '../components/DatasourceForm.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const credentials = [
  { id: 1, name: 'basic-one', auth_type: 'basic', created_at: '', updated_at: '' },
  { id: 2, name: 'bearer-one', auth_type: 'bearer', created_at: '', updated_at: '' },
] as const
const stubs = {
  'a-form': { template: '<form><slot/></form>' }, 'a-form-item': { template: '<div><slot/></div>' },
  'a-input': { template: '<input />' }, 'a-textarea': { template: '<textarea />' },
  'a-select': { template: '<div />' }, 'a-switch': { template: '<button />' },
  'a-alert': { template: '<div data-testid="tls-warning"><slot/></div>' },
}
function mounted(initial: any = null) {
  const wrapper = mount(DatasourceForm, { props: { initial, credentials: [...credentials], clusters: [{ id: 7, name: 'prod' }] }, global: { stubs } })
  return { wrapper, vm: wrapper.vm as any }
}

describe('DatasourceForm', () => {
  it('filters credentials by auth and clears it for none', async () => {
    const { vm } = mounted()
    vm.form.auth_type = 'basic'; await vm.$nextTick()
    expect(vm.credentialOptions.map((x: any) => x.value)).toEqual([1])
    vm.form.http_credential_id = 1; vm.form.auth_type = 'none'; await vm.$nextTick()
    expect(vm.form.http_credential_id).toBeUndefined()
  })
  it('supports optional cluster and public CA without a secret field', () => {
    const { wrapper, vm } = mounted()
    expect(vm.form.cluster_id).toBeUndefined()
    expect('custom_ca_pem' in vm.form).toBe(true)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
  })
  it('shows a danger warning only when TLS verification is skipped', async () => {
    const { wrapper, vm } = mounted()
    expect(wrapper.find('[data-testid="tls-warning"]').exists()).toBe(false)
    vm.form.tls_skip_verify = true; await vm.$nextTick()
    expect(wrapper.find('[data-testid="tls-warning"]').exists()).toBe(true)
  })
  it.each(['ftp://metrics.test', 'https://u:p@metrics.test', 'https://metrics.test/x?q=1', 'https://metrics.test/x#f'])('rejects unsafe URL %s', value => {
    expect(validateDatasourceURL(value)).toBe(false)
  })
  it('accepts normalized HTTP(S) base URLs', () => expect(validateDatasourceURL('https://metrics.test/prometheus')).toBe(true))
})
