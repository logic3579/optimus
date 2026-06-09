<template>
  <a-modal
    :open="open"
    :title="$t('system.roles.permissions_modal_title')"
    :confirm-loading="saving"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-tree
        v-if="treeData.length"
        v-model:checked-keys="checkedKeys"
        checkable
        :tree-data="treeData"
        :default-expand-all="true"
        :check-strictly="false"
      />
      <a-empty v-else />
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch, inject } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { RoleApi } from '@/api/role'
import type { PermissionApi } from '@/api/permission'
import type { RoleSummary, Permission } from '@/types/api'

const props = defineProps<{
  open: boolean
  role: RoleSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const roleApi = inject<RoleApi>('roleApi')!
const permissionApi = inject<PermissionApi>('permissionApi')!

const loading = ref(false)
const saving = ref(false)
const allPermissions = ref<Permission[]>([])
const checkedKeys = ref<string[]>([])

const CATEGORY_PREFIX = '__cat:'

interface PermTreeNode {
  title: string
  key: string
  children?: PermTreeNode[]
  selectable?: boolean
}

const treeData = computed<PermTreeNode[]>(() => {
  const byCategory = new Map<string, Permission[]>()
  for (const p of allPermissions.value) {
    const arr = byCategory.get(p.category) ?? []
    arr.push(p)
    byCategory.set(p.category, arr)
  }
  return Array.from(byCategory.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([category, perms]) => ({
      title: t(`perm.category.${category}`),
      key: `${CATEGORY_PREFIX}${category}`,
      selectable: false,
      children: perms
        .sort((a, b) => a.code.localeCompare(b.code))
        .map(p => ({
          title: `${t(`perm.${p.code.replace(/:/g, '.')}`)} (${p.code})`,
          key: p.code
        }))
    }))
})

watch(
  () => props.open,
  async (open) => {
    if (!open || !props.role) return
    loading.value = true
    try {
      const [perms, detail] = await Promise.all([
        permissionApi.list(),
        roleApi.get(props.role.id)
      ])
      allPermissions.value = perms
      checkedKeys.value = [...detail.permission_codes]
    } catch (e) {
      message.error(isBizError(e) ? e.message : t('network.error'))
    } finally {
      loading.value = false
    }
  }
)

async function onOk() {
  if (!props.role) return
  saving.value = true
  try {
    const codes = checkedKeys.value.filter(k => !k.startsWith(CATEGORY_PREFIX))
    await roleApi.setPermissions(props.role.id, { permission_codes: codes })
    message.success(t('system.roles.permissions_ok'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
