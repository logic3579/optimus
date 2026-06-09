/* eslint-disable vue/one-component-per-file */
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { permissionDirective } from './permission'

function makeApp(template: string) {
  return defineComponent({
    directives: { permission: permissionDirective },
    setup() { return () => h('div', { innerHTML: template }) }
  })
}

describe('v-permission', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('removes element when single perm missing', () => {
    useAuthStore().setPermissions(['system:role:read'])
    const Cmp = defineComponent({
      directives: { permission: permissionDirective },
      template: '<div><a-button v-permission="\'system:user:write\'" class="target">x</a-button></div>'
    })
    const wrapper = mount(Cmp, { global: { stubs: { 'a-button': { template: '<button class="target"><slot/></button>' } } } })
    expect(wrapper.find('.target').exists()).toBe(false)
  })

  it('keeps element when single perm present', () => {
    useAuthStore().setPermissions(['system:user:write'])
    const Cmp = defineComponent({
      directives: { permission: permissionDirective },
      template: '<div><span v-permission="\'system:user:write\'" class="target">x</span></div>'
    })
    expect(mount(Cmp).find('.target').exists()).toBe(true)
  })

  it('array form requires ALL perms (intersection)', () => {
    useAuthStore().setPermissions(['a'])
    const All = defineComponent({
      directives: { permission: permissionDirective },
      template: '<div><span v-permission="[\'a\', \'b\']" class="target">x</span></div>'
    })
    expect(mount(All).find('.target').exists()).toBe(false)

    useAuthStore().setPermissions(['a', 'b'])
    expect(mount(All).find('.target').exists()).toBe(true)
  })

  it('v-permission:any requires AT LEAST ONE (union)', () => {
    useAuthStore().setPermissions(['c'])
    const Any = defineComponent({
      directives: { permission: permissionDirective },
      template: '<div><span v-permission:any="[\'a\', \'b\']" class="target">x</span></div>'
    })
    expect(mount(Any).find('.target').exists()).toBe(false)

    useAuthStore().setPermissions(['a'])
    expect(mount(Any).find('.target').exists()).toBe(true)
  })
})

// Suppress unused import warning
void makeApp
