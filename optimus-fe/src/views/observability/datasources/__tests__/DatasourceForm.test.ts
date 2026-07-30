import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { DatasourceDetail, SaveDatasource } from '@/types/observability'
import DatasourceForm from '../components/DatasourceForm.vue'
import { validateDatasourceURL } from '../components/validation'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const credentials = [
  { id: 1, name: 'basic-one', auth_type: 'basic', created_at: '', updated_at: '' },
  { id: 2, name: 'bearer-one', auth_type: 'bearer', created_at: '', updated_at: '' },
] as const
const stubs = {
  'a-form': { template: '<form><slot/></form>' }, 'a-form-item': { template: '<div><slot/></div>' },
  'a-input': { template: '<input />' }, 'a-textarea': { template: '<textarea />' },
  'a-select': { template: '<div />' }, 'a-switch': { template: '<button />' },
  'a-radio-group': { template: '<div />' },
  'a-alert': { template: '<div data-testid="tls-warning"><slot/></div>' },
}
interface FormVM { form: SaveDatasource; caMode: 'keep' | 'replace' | 'clear'; credentialOptions: { value: number }[]; payload(): SaveDatasource; $nextTick(): Promise<void> }
function mounted(initial: DatasourceDetail | null = null) {
  const wrapper = mount(DatasourceForm, { props: { initial, credentials: [...credentials], clusters: [{ id: 7, name: 'prod' }] }, global: { stubs } })
  return { wrapper, vm: wrapper.vm as unknown as FormVM }
}

describe('DatasourceForm', () => {
  it('filters credentials by auth and clears it for none', async () => {
    const { vm } = mounted()
    vm.form.auth_type = 'basic'; await vm.$nextTick()
    expect(vm.credentialOptions.map(x => x.value)).toEqual([1])
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
  it('emits explicit credential and cluster clears on update', async () => {
    const { vm } = mounted({ id: 9, name: 'prom', base_url: 'https://metrics.test', auth_type: 'basic', http_credential: { id: 1, name: 'basic-one' }, cluster: { id: 7, name: 'prod' }, tls_skip_verify: false, has_custom_ca: false, description: '', created_at: '', updated_at: '' })
    vm.form.auth_type = 'none'; vm.form.cluster_id = undefined; await vm.$nextTick()
    expect(vm.payload()).toEqual(expect.objectContaining({ clear_http_credential: true, clear_cluster: true }))
  })
  it('keeps, replaces, or explicitly clears an existing custom CA', () => {
    const initial: DatasourceDetail = { id: 9, name: 'prom', base_url: 'https://metrics.test', auth_type: 'none', tls_skip_verify: false, has_custom_ca: true, description: '', created_at: '', updated_at: '' }
    const { vm } = mounted(initial)
    expect(vm.payload()).not.toHaveProperty('custom_ca_pem')
    expect(vm.payload()).not.toHaveProperty('clear_custom_ca')
    vm.caMode = 'replace'; vm.form.custom_ca_pem = '-----BEGIN CERTIFICATE-----\npublic\n-----END CERTIFICATE-----'
    expect(vm.payload()).toEqual(expect.objectContaining({ custom_ca_pem: expect.stringContaining('BEGIN CERTIFICATE') }))
    vm.caMode = 'clear'
    expect(vm.payload()).toEqual(expect.objectContaining({ clear_custom_ca: true }))
    expect(vm.payload()).not.toHaveProperty('custom_ca_pem')
  })
  it('does not send update clear flags when creating', () => {
    const { vm } = mounted()
    vm.form.name = 'prom'; vm.form.base_url = 'https://metrics.test'
    expect(vm.payload()).not.toEqual(expect.objectContaining({ clear_http_credential: true, clear_cluster: true, clear_custom_ca: true }))
    expect(Object.keys(vm.payload()).some((key: string) => key.startsWith('clear_'))).toBe(false)
  })
  it('requires a credential for Basic and Bearer authentication', () => {
    const { vm } = mounted()
    vm.form.name = 'prom'; vm.form.base_url = 'https://metrics.test'; vm.form.auth_type = 'basic'
    expect(() => vm.payload()).toThrowError(expect.objectContaining({ message_key: 'observability.datasource.credential_required' }))
    vm.form.http_credential_id = 1
    expect(vm.payload()).toEqual(expect.objectContaining({ auth_type: 'basic', http_credential_id: 1 }))
  })
  it('does not silently clear an existing credential while auth remains enabled', () => {
    const { vm } = mounted({ id: 9, name: 'prom', base_url: 'https://metrics.test', auth_type: 'basic', http_credential: { id: 1, name: 'basic-one' }, tls_skip_verify: false, has_custom_ca: false, description: '', created_at: '', updated_at: '' })
    vm.form.http_credential_id = undefined
    expect(() => vm.payload()).toThrowError(expect.objectContaining({ message_key: 'observability.datasource.credential_required' }))
  })
  it('requires PEM content when replacing an existing custom CA', () => {
    const { vm } = mounted({ id: 9, name: 'prom', base_url: 'https://metrics.test', auth_type: 'none', tls_skip_verify: false, has_custom_ca: true, description: '', created_at: '', updated_at: '' })
    vm.caMode = 'replace'
    expect(() => vm.payload()).toThrowError(expect.objectContaining({ message_key: 'observability.datasource.ca_required' }))
  })
  it('attaches a localization key to local URL validation failures', () => {
    const { vm } = mounted()
    vm.form.base_url = 'https://user:pass@metrics.test'
    expect(() => vm.payload()).toThrowError(expect.objectContaining({ message_key: 'observability.datasource.invalid_url' }))
  })
})
