<template>
  <a-modal
    :open="open"
    :title="$t('system.users.roles_modal_title')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-checkbox-group v-model:value="selected" style="display: flex; flex-direction: column; gap: 8px;">
        <a-checkbox v-for="r in roles" :key="r.id" :value="r.id">
          {{ r.name }} <span style="color:#999;">({{ r.code }})</span>
        </a-checkbox>
      </a-checkbox-group>
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch, inject } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { UserApi } from '@/api/user'
import type { RoleApi } from '@/api/role'
import type { UserDetail, RoleSummary } from '@/types/api'

const props = defineProps<{
  open: boolean
  user: UserDetail | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!
const roleApi = inject<RoleApi>('roleApi')!

const roles = ref<RoleSummary[]>([])
const selected = ref<number[]>([])
const loading = ref(false)
const saving = ref(false)

watch(
  () => props.open,
  async (open) => {
    if (!open || !props.user) return
    loading.value = true
    try {
      roles.value = await roleApi.list()
      selected.value = props.user.roles.map(r => r.id)
    } catch (e) {
      message.error(isBizError(e) ? e.message : t('network.error'))
    } finally {
      loading.value = false
    }
  }
)

async function onOk() {
  if (!props.user) return
  saving.value = true
  try {
    await userApi.setRoles(props.user.id, { role_ids: selected.value })
    message.success(t('system.users.roles_ok'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
