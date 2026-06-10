<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('k8s.cluster.action.edit') : $t('k8s.cluster.action.create')"
    :confirm-loading="saving"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('k8s.cluster.name')" name="name">
        <a-input v-model:value="form.name" :maxlength="128" />
      </a-form-item>

      <a-form-item :label="$t('k8s.cluster.kubeconfig')" name="kubeconfig_id">
        <a-select
          v-model:value="form.kubeconfig_id"
          :loading="loadingKubeconfigs"
          :options="kubeconfigOptions"
          :placeholder="$t('k8s.cluster.kubeconfig')"
          show-search
          option-filter-prop="label"
        />
      </a-form-item>

      <a-form-item :label="$t('k8s.cluster.context')" name="context">
        <a-input v-model:value="form.context" :maxlength="128" />
        <div class="hint">{{ $t('k8s.cluster.context_hint') }}</div>
      </a-form-item>

      <a-form-item :label="$t('k8s.cluster.description')" name="description">
        <a-textarea v-model:value="form.description" :rows="2" :maxlength="4096" />
      </a-form-item>

      <a-form-item :label="$t('k8s.cluster.tags')" name="tags">
        <a-select
          v-model:value="form.tags"
          mode="tags"
          :placeholder="$t('k8s.cluster.tags_placeholder')"
          :token-separators="[',', ' ']"
          style="width: 100%;"
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
import type { ClusterApi } from '@/api/k8s/cluster'
import type { KubeconfigApi } from '@/api/credentials/kubeconfig'
import type { Cluster } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: Cluster | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const clusterApi = inject<ClusterApi>('clusterApi')!
const kubeconfigApi = inject<KubeconfigApi>('kubeconfigApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

interface FormShape {
  name: string
  kubeconfig_id: number | undefined
  context: string
  description: string
  tags: string[]
}

const form = reactive<FormShape>({
  name: '',
  kubeconfig_id: undefined,
  context: '',
  description: '',
  tags: [],
})

// initialSnapshot keys mirror `form` for clean diffing on edit. tags is
// reference-compared by formDiff; we always replace the array on input so
// edits show up correctly.
interface Snapshot {
  name: string
  kubeconfig_id: number | undefined
  context: string
  description: string
  tags: string[]
}
let initialSnapshot: Snapshot = {
  name: '',
  kubeconfig_id: undefined,
  context: '',
  description: '',
  tags: [],
}

const rules = computed(() => ({
  name:          [{ required: true, max: 128, message: t('form.required') }],
  kubeconfig_id: [{ required: true, message: t('form.required'), type: 'number' as const }],
  context:       [{ required: true, max: 128, message: t('form.required') }],
}))

const loadingKubeconfigs = ref(false)
const kubeconfigOptions = ref<{ value: number; label: string }[]>([])

async function loadKubeconfigs() {
  loadingKubeconfigs.value = true
  try {
    const r = await kubeconfigApi.list({ page: 1, page_size: 200 })
    kubeconfigOptions.value = r.items.map(k => ({ value: k.id, label: k.name }))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loadingKubeconfigs.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    void loadKubeconfigs()
    if (props.initial) {
      form.name = props.initial.name
      form.kubeconfig_id = props.initial.kubeconfig_id
      form.context = props.initial.context
      form.description = props.initial.description ?? ''
      form.tags = [...(props.initial.tags ?? [])]
      initialSnapshot = {
        name: form.name,
        kubeconfig_id: form.kubeconfig_id,
        context: form.context,
        description: form.description,
        tags: form.tags,
      }
    } else {
      form.name = ''
      form.kubeconfig_id = undefined
      form.context = ''
      form.description = ''
      form.tags = []
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
  if (form.kubeconfig_id === undefined) return
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch = formDiff(initialSnapshot as unknown as Record<string, unknown>, {
        name: form.name,
        kubeconfig_id: form.kubeconfig_id,
        context: form.context,
        description: form.description,
        tags: form.tags,
      })
      if (Object.keys(patch).length > 0) {
        await clusterApi.update(props.initial.id, patch)
      }
      message.success(t('k8s.cluster.toast.updated'))
    } else {
      await clusterApi.create({
        name: form.name,
        kubeconfig_id: form.kubeconfig_id,
        context: form.context,
        description: form.description || undefined,
        tags: form.tags.length > 0 ? form.tags : undefined,
      })
      message.success(t('k8s.cluster.toast.created'))
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
.hint {
  color: var(--ant-color-text-tertiary, #999);
  font-size: 12px;
  margin-top: 4px;
}
</style>
