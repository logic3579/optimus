<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('credentials.action.edit') : $t('credentials.action.create')"
    :confirm-loading="saving"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('credentials.field.name')" name="name">
        <a-input v-model:value="form.name" :maxlength="128" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.description')" name="description">
        <a-textarea v-model:value="form.description" :rows="2" :maxlength="4096" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.username')" name="username">
        <a-input v-model:value="form.username" :maxlength="64" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.private_key')" name="private_key">
        <a-textarea
          v-model:value="form.private_key"
          :rows="8"
          :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
          class="mono"
        />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.passphrase')" name="passphrase">
        <a-input-password
          v-model:value="form.passphrase"
          :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { SshKeyApi } from '@/api/credentials/ssh-key'
import type { SshKeySummary } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: SshKeySummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const sshKeyApi = inject<SshKeyApi>('sshKeyApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  name: '',
  description: '',
  username: '',
  private_key: '',
  passphrase: '',
})
let initialSnapshot = { name: '', description: '', username: '' }

const rules = computed(() => ({
  name:        [{ required: true, max: 128, message: t('form.required') }],
  username:    [{ required: true, max: 64,  message: t('form.required') }],
  private_key: isEdit.value ? [] : [{ required: true, message: t('form.required') }],
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.name = props.initial.name
      form.description = props.initial.description
      form.username = props.initial.username
      form.private_key = ''
      form.passphrase = ''
      initialSnapshot = {
        name: props.initial.name,
        description: props.initial.description,
        username: props.initial.username,
      }
    } else {
      form.name = ''
      form.description = ''
      form.username = ''
      form.private_key = ''
      form.passphrase = ''
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
      const patch: Record<string, unknown> = formDiff(initialSnapshot, {
        name: form.name,
        description: form.description,
        username: form.username,
      })
      if (form.private_key) patch.private_key = form.private_key
      if (form.passphrase) patch.passphrase = form.passphrase
      if (Object.keys(patch).length > 0) {
        await sshKeyApi.update(props.initial.id, patch)
      }
      message.success(t('credentials.toast.updated'))
    } else {
      await sshKeyApi.create({
        name: form.name,
        description: form.description || undefined,
        username: form.username,
        private_key: form.private_key,
        passphrase: form.passphrase || undefined,
      })
      message.success(t('credentials.toast.created'))
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

<style scoped lang="scss">
.mono {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
}
</style>
