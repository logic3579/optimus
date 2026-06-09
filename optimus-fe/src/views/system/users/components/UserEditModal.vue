<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('system.users.edit') : $t('system.users.create')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item v-if="!isEdit" :label="$t('system.users.form_username')" name="username">
        <a-input v-model:value="form.username" />
      </a-form-item>
      <a-form-item :label="$t('system.users.form_email')" name="email">
        <a-input v-model:value="form.email" />
      </a-form-item>
      <a-form-item v-if="!isEdit" :label="$t('system.users.form_password')" name="password">
        <a-input-password v-model:value="form.password" />
      </a-form-item>
      <a-form-item :label="$t('system.users.form_display_name')" name="display_name">
        <a-input v-model:value="form.display_name" />
      </a-form-item>
      <a-form-item :label="$t('system.users.form_avatar_url')" name="avatar_url">
        <a-input v-model:value="form.avatar_url" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { UserApi } from '@/api/user'
import type { UserDetail } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: UserDetail | null  // null/undefined → create mode
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  username: '',
  email: '',
  password: '',
  display_name: '',
  avatar_url: ''
})
let initialSnapshot = { email: '', display_name: '', avatar_url: '' }

const rules = computed(() => ({
  username: [{ required: true, min: 3, max: 64, message: t('form.required') }],
  email: [{ required: true, type: 'email', message: t('form.invalid_email') }],
  password: isEdit.value ? [] : [{ required: true, min: 8, message: t('form.min_length', { n: 8 }) }],
  display_name: [{ max: 128 }]
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.username = props.initial.username
      form.email = props.initial.email
      form.password = ''
      form.display_name = props.initial.display_name
      form.avatar_url = props.initial.avatar_url
      initialSnapshot = {
        email: props.initial.email,
        display_name: props.initial.display_name,
        avatar_url: props.initial.avatar_url
      }
    } else {
      form.username = ''
      form.email = ''
      form.password = ''
      form.display_name = ''
      form.avatar_url = ''
    }
  },
  { immediate: true }
)

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch = formDiff(initialSnapshot, {
        email: form.email,
        display_name: form.display_name,
        avatar_url: form.avatar_url
      })
      if (Object.keys(patch).length > 0) {
        await userApi.update(props.initial.id, patch)
      }
      message.success(t('system.users.update_ok'))
    } else {
      await userApi.create({
        username: form.username,
        email: form.email,
        password: form.password,
        display_name: form.display_name || undefined
      })
      message.success(t('system.users.create_ok'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
