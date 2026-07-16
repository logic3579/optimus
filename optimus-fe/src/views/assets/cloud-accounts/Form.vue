<template>
  <a-modal
    :open="open"
    :title="editing ? t('assets.account.actions.edit') : t('assets.account.actions.create')"
    :confirm-loading="submitting"
    width="640px"
    @ok="onOk"
    @cancel="emit('close')"
  >
    <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
      <a-form-item :label="t('assets.account.name')" name="name">
        <a-input v-model:value="form.name" :maxlength="128" />
      </a-form-item>
      <a-form-item :label="t('assets.account.cloudkey')" name="cloudkey_id">
        <a-select
          v-model:value="form.cloudkey_id"
          :disabled="isEdit"
          :loading="loadingCloudKeys"
          :options="cloudKeyOptions"
        />
      </a-form-item>
      <a-form-item :label="t('assets.account.regions')" name="enabled_regions">
        <a-select
          v-model:value="form.enabled_regions"
          mode="multiple"
          :options="regionOptions"
        />
      </a-form-item>
      <a-form-item :label="t('assets.account.enabled')">
        <a-switch v-model:checked="form.enabled" />
      </a-form-item>
      <a-form-item :label="t('assets.account.description')">
        <a-textarea v-model:value="form.description" :rows="3" :maxlength="2000" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import type { AssetsAccountApi } from '@/api/assets/account'
import type { CloudKeyApi } from '@/api/credentials/cloud-key'
import { useI18n } from '@/hooks/useI18n'
import type { CloudAccountDetail, CreateCloudAccountRequest, UpdateCloudAccountRequest } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const AWS_REGIONS = [
  'us-east-1', 'us-east-2', 'us-west-1', 'us-west-2',
  'ap-southeast-1', 'ap-southeast-2', 'ap-northeast-1', 'ap-northeast-2',
  'ap-south-1', 'eu-west-1', 'eu-west-2', 'eu-central-1', 'sa-east-1',
]

const props = defineProps<{ open: boolean; editing?: CloudAccountDetail | null }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'saved'): void }>()
const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const cloudKeyApi = inject<CloudKeyApi>('cloudKeyApi')!
const { t } = useI18n()

const isEdit = computed(() => props.editing != null)
const formRef = ref<FormInstance>()
const submitting = ref(false)
const loadingCloudKeys = ref(false)
const cloudKeys = ref<{ id: number; name: string }[]>([])
const form = reactive<CreateCloudAccountRequest>({
  name: '', provider: 'aws', cloudkey_id: 0, enabled_regions: [], enabled: true, description: '',
})
const cloudKeyOptions = computed(() => cloudKeys.value.map(key => ({ value: key.id, label: key.name })))
const regionOptions = AWS_REGIONS.map(region => ({ value: region, label: region }))
const rules = computed(() => ({
  name: [{ required: true, max: 128, message: t('form.required') }],
  cloudkey_id: [{ required: true, type: 'number' as const, min: 1, message: t('form.required') }],
  enabled_regions: [{ required: true, type: 'array' as const, min: 1, message: t('form.required') }],
}))

function resetForm() {
  formRef.value?.resetFields()
  const account = props.editing
  form.name = account?.name ?? ''
  form.provider = 'aws'
  form.cloudkey_id = account?.cloudkey_id ?? 0
  form.enabled_regions = [...(account?.enabled_regions ?? [])]
  form.enabled = account?.enabled ?? true
  form.description = account?.description ?? ''
}

async function loadCloudKeys() {
  loadingCloudKeys.value = true
  try {
    const result = await cloudKeyApi.list({ page: 1, page_size: 100, provider: 'aws' })
    cloudKeys.value = result.items.map(key => ({ id: key.id, name: key.name }))
  } catch (error) {
    message.error(isBizError(error) ? error.message : t('network.error'))
  } finally {
    loadingCloudKeys.value = false
  }
}

watch(() => props.open, open => {
  if (!open) return
  resetForm()
  void loadCloudKeys()
}, { immediate: true })
watch(() => props.editing, () => { if (props.open) resetForm() })

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  submitting.value = true
  try {
    if (props.editing) {
      const request: UpdateCloudAccountRequest = {
        name: form.name,
        enabled_regions: [...form.enabled_regions],
        enabled: form.enabled,
        description: form.description,
      }
      await accountApi.update(props.editing.id, request)
    } else {
      await accountApi.create({
        name: form.name,
        provider: 'aws',
        cloudkey_id: form.cloudkey_id,
        enabled_regions: [...form.enabled_regions],
        enabled: form.enabled,
        description: form.description,
      })
    }
    message.success(t('common.message.done'))
    emit('saved')
  } catch (error) {
    message.error(isBizError(error) ? error.message : t('network.error'))
  } finally {
    submitting.value = false
  }
}
</script>
