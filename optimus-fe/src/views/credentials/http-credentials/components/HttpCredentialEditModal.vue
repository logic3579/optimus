<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('credentials.action.edit') : $t('credentials.action.create')"
    :confirm-loading="saving"
    width="560px"
    @ok="onOk"
    @cancel="close"
  >
    <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
      <a-form-item :label="$t('credentials.field.name')" name="name">
        <a-input v-model:value="form.name" :maxlength="128" />
      </a-form-item>
      <a-form-item label="Authentication type" name="auth_type">
        <a-radio-group v-model:value="form.auth_type" :disabled="isEdit">
          <a-radio-button value="basic">Basic</a-radio-button>
          <a-radio-button value="bearer">Bearer</a-radio-button>
        </a-radio-group>
      </a-form-item>
      <a-form-item
        v-if="form.auth_type === 'basic'"
        data-testid="username-field"
        :label="$t('credentials.field.username')"
        name="username"
      >
        <a-input v-model:value="form.username" :maxlength="256" />
      </a-form-item>
      <a-form-item :label="$t('auth.password')" name="secret">
        <a-input-password
          v-model:value="form.secret"
          autocomplete="new-password"
          :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
        />
      </a-form-item>
    </a-form>
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon />
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import type { HTTPCredentialApi, HTTPCredentialSummary, HTTPAuthType } from '@/api/credentials/http-credential'

const props = defineProps<{ open: boolean; initial?: HTTPCredentialSummary | null }>()
const emit = defineEmits<{ (e: 'update:open', value: boolean): void; (e: 'saved'): void }>()
const api = inject<HTTPCredentialApi>('httpCredentialApi')!
const { t } = useI18n()
const formRef = ref<FormInstance>()
const saving = ref(false)
const errorMessage = ref('')
const isEdit = computed(() => !!props.initial)
const form = reactive<{ name: string; auth_type: HTTPAuthType; username: string; secret: string }>({
  name: '', auth_type: 'basic', username: '', secret: '',
})
const rules = computed(() => ({
  name: [{ required: true, max: 128, message: t('form.required') }],
  auth_type: [{ required: true, message: t('form.required') }],
  username: form.auth_type === 'basic' ? [{ required: true, max: 256, message: t('form.required') }] : [],
  secret: isEdit.value ? [] : [{ required: true, message: t('form.required') }],
}))

function clearSecret() {
  if (form.secret) form.secret = '\0'.repeat(form.secret.length)
  form.secret = ''
}
function reset() {
  clearSecret()
  errorMessage.value = ''
  form.name = props.initial?.name ?? ''
  form.auth_type = props.initial?.auth_type ?? 'basic'
  form.username = props.initial?.auth_type === 'basic' ? props.initial.username ?? '' : ''
}
function close() {
  clearSecret()
  emit('update:open', false)
}
function errorText(error: unknown): string {
  if (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string') return error.message
  return t('network.error')
}
watch(() => props.open, open => { if (open) reset(); else clearSecret() }, { immediate: true })
watch(() => form.auth_type, authType => { if (authType === 'bearer') form.username = '' })

async function onOk() {
  try { await formRef.value?.validate() } catch { return }
  const secret = form.secret
  saving.value = true
  errorMessage.value = ''
  try {
    if (props.initial) {
      const body: { name?: string; username?: string; secret?: string } = {}
      if (form.name !== props.initial.name) body.name = form.name
      if (form.auth_type === 'basic' && form.username !== (props.initial.username ?? '')) body.username = form.username
      if (secret) body.secret = secret
      if (Object.keys(body).length) await api.update(props.initial.id, body)
      message.success(t('credentials.toast.updated'))
    } else {
      await api.create({
        name: form.name,
        auth_type: form.auth_type,
        ...(form.auth_type === 'basic' ? { username: form.username } : {}),
        secret,
      })
      message.success(t('credentials.toast.created'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (error) {
    errorMessage.value = errorText(error)
  } finally {
    clearSecret()
    saving.value = false
  }
}

defineExpose({ form, rules, errorMessage, onOk, close } satisfies Record<string, unknown>)
</script>
