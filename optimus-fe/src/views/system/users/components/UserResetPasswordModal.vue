<template>
  <a-modal
    :open="open"
    :title="$t('system.users.reset_password_title')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('system.users.reset_password_new')" name="password">
        <a-input-password v-model:value="form.password" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { UserApi } from '@/api/user'
import type { UserDetail } from '@/types/api'

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
const formRef = ref<FormInstance>()
const saving = ref(false)
const form = reactive({ password: '' })

const rules = {
  password: [{ required: true, min: 8, message: t('form.min_length', { n: 8 }) }]
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    form.password = ''
    formRef.value?.resetFields()
  }
)

async function onOk() {
  if (!props.user) return
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    await userApi.setPassword(props.user.id, { password: form.password })
    message.success(t('system.users.password_ok'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
