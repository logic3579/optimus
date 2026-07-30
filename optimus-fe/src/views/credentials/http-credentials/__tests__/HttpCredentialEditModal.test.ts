import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import type { HTTPCredentialSummary } from '@/api/credentials/http-credential'
import HttpCredentialEditModal from '../components/HttpCredentialEditModal.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn(), error: vi.fn() } }))

const stubs = {
  'a-modal': { template: '<div><slot/></div>' },
  'a-form': { template: '<form><slot/></form>', methods: { validate: vi.fn() } },
  'a-form-item': { template: '<label><slot/></label>' },
  'a-input': { template: '<input />' },
  'a-input-password': { template: '<input type="password" />' },
  'a-radio-group': { template: '<div><slot/></div>' },
  'a-radio-button': { template: '<button><slot/></button>' },
  'a-alert': { template: '<div><slot/></div>' },
}

interface ModalVM { form: { name: string; auth_type: string; username: string; secret: string }; rules: { username: { required?: boolean }[]; secret: { required?: boolean }[] }; errorMessage: string; onOk(): Promise<void> }
function mounted(initial: HTTPCredentialSummary | null = null) {
  const api = { create: vi.fn().mockResolvedValue({}), update: vi.fn().mockResolvedValue({}) }
  const wrapper = mount(HttpCredentialEditModal, {
    props: { open: true, initial },
    global: { provide: { httpCredentialApi: api }, stubs, mocks: { $t: (key: string) => key } },
  })
  return { wrapper, api, vm: wrapper.vm as unknown as ModalVM }
}

describe('HTTP credential modal', () => {
  it('requires Basic username and create secret, while update secret is optional', async () => {
    const create = mounted()
    expect(create.vm.rules.username.at(0)?.required).toBe(true)
    expect(create.vm.rules.secret.at(0)?.required).toBe(true)
    const edit = mounted({ id: 7, name: 'saved', auth_type: 'basic', username: 'reader', created_at: '', updated_at: '' })
    expect(edit.vm.rules.secret).toEqual([])
    expect(edit.vm.form.secret).toBe('')
    expect(edit.wrapper.text()).not.toContain('stored-secret')
  })

  it('hides and clears username for Bearer', async () => {
    const { vm, wrapper } = mounted()
    vm.form.username = 'reader'
    vm.form.auth_type = 'bearer'
    await nextTick()
    expect(vm.form.username).toBe('')
    expect(wrapper.find('[data-testid="username-field"]').exists()).toBe(false)
  })

  it('overwrites and clears secret after save and close while preserving API errors', async () => {
    const { vm, api, wrapper } = mounted()
    vm.form.name = 'prom'
    vm.form.auth_type = 'bearer'
    vm.form.secret = 'top-secret'
    await vm.onOk()
    expect(api.create).toHaveBeenCalledWith({ name: 'prom', auth_type: 'bearer', secret: 'top-secret' })
    expect(vm.form.secret).toBe('')
    expect(wrapper.emitted('saved')).toHaveLength(1)
    api.create.mockRejectedValueOnce({ message: 'duplicate name', code: 44002 })
    vm.form.secret = 'retry-secret'
    await vm.onOk()
    expect(vm.errorMessage).toBe('duplicate name')
    expect(vm.form.secret).toBe('')
  })
})
