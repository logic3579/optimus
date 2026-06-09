import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { has, hasAll, hasAny } from '@/utils/permission'

export function usePermission() {
  const auth = useAuthStore()
  const set = computed(() => new Set(auth.permissions))
  return {
    has: (code: string) => has(set.value, code),
    hasAll: (codes: readonly string[]) => hasAll(set.value, codes),
    hasAny: (codes: readonly string[]) => hasAny(set.value, codes),
    set
  }
}
