<template>
  <a-modal
    :open="open"
    :title="isEdit ? t('apps.repo.form.title.edit') : t('apps.repo.form.title.create')"
    :confirm-loading="submitting"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="t('apps.repo.field.name')" name="name">
        <a-input v-model:value="form.name" :max-length="64" />
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.type')" name="type">
        <a-radio-group v-model:value="form.type" :disabled="isEdit">
          <a-radio value="http">HTTP</a-radio>
          <a-radio value="oci">OCI</a-radio>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.url')" name="url">
        <a-input v-model:value="form.url" :max-length="2048" />
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.username')">
        <a-input v-model:value="form.username" :max-length="255" />
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.password')">
        <a-input-password
          v-model:value="form.password"
          :placeholder="isEdit ? t('apps.repo.form.placeholder.passwordEdit') : ''"
          autocomplete="new-password"
        />
        <a-button
          v-if="isEdit && original?.has_password"
          size="small"
          danger
          type="link"
          @click="form.password = CLEAR_SENTINEL"
        >
          {{ t('apps.repo.form.btn.clearPassword') }}
        </a-button>
        <a-typography-text v-if="form.password === CLEAR_SENTINEL" type="warning" class="cleared-hint">
          {{ t('apps.repo.form.btn.clearPasswordHint') }}
        </a-typography-text>
      </a-form-item>
      <a-form-item :label="t('apps.repo.field.description')">
        <a-textarea v-model:value="form.description" :rows="3" :max-length="4096" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { AppsRepoApi } from '@/api/apps/repo'
import type {
  ChartRepoSummary, ChartRepoDetail,
  ChartRepoCreateRequest, ChartRepoUpdateRequest,
} from '@/types/apps'

// Sentinel value used to flag "the user explicitly cleared the password".
// Translates to JSON null on the wire so the BE differentiates
// keep (omit) / replace (string) / clear (null).
const CLEAR_SENTINEL = '__CLEAR__'

const props = defineProps<{
  open: boolean
  original?: ChartRepoSummary | ChartRepoDetail | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const repoApi = inject<AppsRepoApi>('appsRepoApi')!

const isEdit = computed(() => !!props.original)
const submitting = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  name: '',
  type: 'http' as 'http' | 'oci',
  url: '',
  username: '',
  password: '',
  description: '',
})

const rules = computed(() => ({
  name: [{ required: true, max: 64, message: t('form.required') }],
  type: [{ required: true, message: t('form.required') }],
  url:  [{ required: true, max: 2048, message: t('form.required') }],
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.original) {
      form.name = props.original.name
      form.type = props.original.type
      form.url = props.original.url
      form.username = props.original.username
      form.password = ''
      form.description = props.original.description
    } else {
      form.name = ''
      form.type = 'http'
      form.url = ''
      form.username = ''
      form.password = ''
      form.description = ''
    }
  },
  { immediate: true },
)

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  submitting.value = true
  try {
    if (props.original) {
      const body: ChartRepoUpdateRequest = {
        name: form.name,
        url: form.url,
        username: form.username,
        description: form.description,
      }
      // Sentinel → JSON null clears the stored secret; empty string omits the key.
      if (form.password === CLEAR_SENTINEL) {
        body.password = null
      } else if (form.password !== '') {
        body.password = form.password
      }
      await repoApi.update(props.original.id, body)
      message.success(t('apps.repo.toast.updated'))
    } else {
      const body: ChartRepoCreateRequest = {
        name: form.name,
        type: form.type,
        url: form.url,
        username: form.username || undefined,
        password: form.password || undefined,
        description: form.description || undefined,
      }
      await repoApi.create(body)
      message.success(t('apps.repo.toast.created'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped lang="scss">
.cleared-hint {
  display: block;
  margin-top: 4px;
  font-size: 12px;
}
</style>
