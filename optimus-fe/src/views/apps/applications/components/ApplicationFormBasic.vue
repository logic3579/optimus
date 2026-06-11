<template>
  <a-form :model="model" layout="vertical">
    <a-row :gutter="16">
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.name')" :required="!isEdit" name="name">
          <a-input :value="model.name" :max-length="64" :disabled="isEdit" @update:value="onField('name', $event)" />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.release')" :required="!isEdit" name="release_name">
          <a-input
            :value="model.release_name"
            :max-length="53"
            :disabled="isEdit"
            @update:value="onField('release_name', $event)"
          />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.cluster')" :required="!isEdit" name="cluster_id">
          <a-select
            :value="model.cluster_id"
            :options="clusterOptions"
            :loading="loadingClusters"
            :disabled="isEdit"
            show-search
            option-filter-prop="label"
            @update:value="onField('cluster_id', $event)"
          />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.namespace')" :required="!isEdit" name="namespace">
          <a-input
            :value="model.namespace"
            :max-length="63"
            :disabled="isEdit"
            @update:value="onField('namespace', $event)"
          />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.owner')">
          <a-select
            :value="model.owner_user_id"
            :options="userOptions"
            :loading="loadingUsers"
            allow-clear
            show-search
            option-filter-prop="label"
            @update:value="onField('owner_user_id', $event)"
          />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.field.tags')">
          <a-select
            :value="model.tags"
            mode="tags"
            :token-separators="[',', ' ']"
            @update:value="onField('tags', $event)"
          />
        </a-form-item>
      </a-col>
      <a-col :span="24">
        <a-form-item :label="t('apps.application.field.description')">
          <a-textarea
            :value="model.description"
            :rows="3"
            :max-length="4096"
            @update:value="onField('description', $event)"
          />
        </a-form-item>
      </a-col>
    </a-row>
  </a-form>
</template>

<script setup lang="ts">
import { inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { ClusterApi } from '@/api/k8s/cluster'
import type { UserApi } from '@/api/user'

/**
 * ApplicationFormBasic — the shared "basics" panel rendered by both the
 * Install wizard (creating a new application) and the Detail page
 * (read-only via :is-edit="true").
 *
 * Spec §3.2 immutables after create: name, release_name, cluster_id,
 * namespace. They are disabled in edit mode but still rendered for context.
 *
 * `modelValue` is shaped to overlap with both ApplicationCreateRequest and
 * ApplicationUpdateRequest so the parent can spread it onto whichever DTO it
 * needs. Field names mirror the BE wire format.
 */
export interface ApplicationFormModel {
  name: string
  release_name: string
  cluster_id: number | undefined
  namespace: string
  owner_user_id: number | undefined
  tags: string[]
  description: string
}

const props = defineProps<{ modelValue: ApplicationFormModel; isEdit?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: ApplicationFormModel): void }>()

const { t } = useI18n()
const clusterApi = inject<ClusterApi>('clusterApi')!
const userApi = inject<UserApi>('userApi')!

// Local alias so the template doesn't have to drill through props.
const model = props.modelValue

const clusterOptions = ref<Array<{ label: string; value: number }>>([])
const userOptions = ref<Array<{ label: string; value: number }>>([])
const loadingClusters = ref(false)
const loadingUsers = ref(false)

function onField<K extends keyof ApplicationFormModel>(key: K, value: ApplicationFormModel[K]): void {
  // Emit a fresh copy so v-model's reactivity contract is honoured even
  // when the parent compares by reference (e.g. computed sources).
  emit('update:modelValue', { ...model, [key]: value })
}

onMounted(async () => {
  loadingClusters.value = true
  try {
    const r = await clusterApi.list({ page_size: 200 })
    clusterOptions.value = r.items.map((c) => ({ label: c.name, value: c.id }))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loadingClusters.value = false
  }

  loadingUsers.value = true
  try {
    const r = await userApi.list({ page: 1, page_size: 200 })
    userOptions.value = r.items.map((u) => ({
      label: u.display_name || u.username,
      value: u.id,
    }))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loadingUsers.value = false
  }
})
</script>
