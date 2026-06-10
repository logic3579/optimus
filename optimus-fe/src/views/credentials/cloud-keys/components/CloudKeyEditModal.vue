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
      <a-form-item :label="$t('credentials.field.provider')" name="provider">
        <a-radio-group v-model:value="form.provider">
          <a-radio-button value="aws">AWS</a-radio-button>
          <a-radio-button value="gcp">GCP</a-radio-button>
          <a-radio-button value="azure">Azure</a-radio-button>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="$t('credentials.field.region')" name="region">
        <a-input v-model:value="form.region" :maxlength="32" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.access_key_id')" name="access_key_id">
        <a-input
          v-model:value="form.access_key_id"
          :maxlength="256"
          :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
        />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.secret_access_key')" name="secret_access_key">
        <a-input-password
          v-model:value="form.secret_access_key"
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
import type { CloudKeyApi } from '@/api/credentials/cloud-key'
import type { CloudKeySummary, CloudProvider } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: CloudKeySummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const cloudKeyApi = inject<CloudKeyApi>('cloudKeyApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<{
  name: string
  description: string
  provider: CloudProvider
  region: string
  access_key_id: string
  secret_access_key: string
}>({
  name: '',
  description: '',
  provider: 'aws',
  region: '',
  access_key_id: '',
  secret_access_key: '',
})
let initialSnapshot: { name: string; description: string; provider: CloudProvider; region: string } = {
  name: '', description: '', provider: 'aws', region: '',
}

const rules = computed(() => ({
  name:              [{ required: true, max: 128, message: t('form.required') }],
  provider:          [{ required: true, message: t('form.required') }],
  access_key_id:     isEdit.value ? [] : [{ required: true, message: t('form.required') }],
  secret_access_key: isEdit.value ? [] : [{ required: true, message: t('form.required') }],
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.name = props.initial.name
      form.description = props.initial.description
      form.provider = props.initial.provider
      form.region = props.initial.region
      form.access_key_id = ''
      form.secret_access_key = ''
      initialSnapshot = {
        name: props.initial.name,
        description: props.initial.description,
        provider: props.initial.provider,
        region: props.initial.region,
      }
    } else {
      form.name = ''
      form.description = ''
      form.provider = 'aws'
      form.region = ''
      form.access_key_id = ''
      form.secret_access_key = ''
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
        provider: form.provider,
        region: form.region,
      })
      if (form.access_key_id) patch.access_key_id = form.access_key_id
      if (form.secret_access_key) patch.secret_access_key = form.secret_access_key
      if (Object.keys(patch).length > 0) {
        await cloudKeyApi.update(props.initial.id, patch)
      }
      message.success(t('credentials.toast.updated'))
    } else {
      await cloudKeyApi.create({
        name: form.name,
        description: form.description || undefined,
        provider: form.provider,
        region: form.region || undefined,
        access_key_id: form.access_key_id,
        secret_access_key: form.secret_access_key,
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
