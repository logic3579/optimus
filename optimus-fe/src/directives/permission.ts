import type { Directive, DirectiveBinding } from 'vue'
import { useAuthStore } from '@/stores/auth'

type Arg = 'any' | undefined
type Value = string | readonly string[]

function check(value: Value, arg: Arg, perms: ReadonlySet<string>): boolean {
  if (typeof value === 'string') return perms.has(value)
  if (arg === 'any') return value.some(c => perms.has(c))
  return value.every(c => perms.has(c))
}

function apply(el: HTMLElement, binding: DirectiveBinding<Value>) {
  const auth = useAuthStore()
  const perms = new Set(auth.permissions)
  if (!check(binding.value, binding.arg as Arg, perms)) {
    el.parentNode?.removeChild(el)
  }
}

export const permissionDirective: Directive<HTMLElement, Value> = {
  mounted: apply,
  updated: apply
}
