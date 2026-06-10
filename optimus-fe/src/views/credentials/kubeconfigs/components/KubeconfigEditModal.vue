<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('credentials.action.edit') : $t('credentials.action.create')"
    :confirm-loading="saving"
    width="720px"
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
      <a-form-item :label="$t('credentials.field.default_namespace')" name="default_namespace">
        <a-input v-model:value="form.default_namespace" :maxlength="64" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.kubeconfig')" name="kubeconfig">
        <a-textarea
          v-model:value="form.kubeconfig"
          :rows="12"
          :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
          class="mono"
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
import type { KubeconfigApi } from '@/api/credentials/kubeconfig'
import type { KubeconfigSummary } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: KubeconfigSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const kubeconfigApi = inject<KubeconfigApi>('kubeconfigApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  name: '',
  description: '',
  default_namespace: '',
  kubeconfig: '',
})
let initialSnapshot = { name: '', description: '', default_namespace: '' }

const rules = computed(() => ({
  name:       [{ required: true, max: 128, message: t('form.required') }],
  kubeconfig: isEdit.value ? [] : [{ required: true, message: t('form.required') }],
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.name = props.initial.name
      form.description = props.initial.description
      form.default_namespace = props.initial.default_namespace
      form.kubeconfig = ''
      initialSnapshot = {
        name: props.initial.name,
        description: props.initial.description,
        default_namespace: props.initial.default_namespace,
      }
    } else {
      form.name = ''
      form.description = ''
      form.default_namespace = ''
      form.kubeconfig = ''
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
        default_namespace: form.default_namespace,
      })
      if (form.kubeconfig) patch.kubeconfig = form.kubeconfig
      if (Object.keys(patch).length > 0) {
        await kubeconfigApi.update(props.initial.id, patch)
      }
      message.success(t('credentials.toast.updated'))
    } else {
      await kubeconfigApi.create({
        name: form.name,
        description: form.description || undefined,
        default_namespace: form.default_namespace || undefined,
        kubeconfig: form.kubeconfig,
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
