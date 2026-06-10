<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import YamlViewer from './YamlViewer.vue'
import LogViewer from './LogViewer.vue'
import type { YamlApi } from '@/api/k8s/yaml'

/**
 * Shared drawer for any k8s resource. Tabs:
 *   - Overview   — `overview` slot (consumer-provided)
 *   - YAML       — lazy-loaded via the injected `yamlApi`
 *   - Events     — `events` slot
 *   - Logs       — only when `kind === 'pod'`
 *
 * The YAML cache is keyed on `${kind}:${namespace}:${name}` so reopening the
 * drawer for a different resource refetches; reopening for the same resource
 * reuses the cached text.
 */
const props = defineProps<{
  open: boolean
  clusterId: number
  kind: string
  namespace?: string
  name: string
  detail: unknown
  podContainers?: string[]
}>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const yamlApi = inject<YamlApi>('yamlApi')
if (!yamlApi) {
  // Surface a clear error during development rather than NPE in loadYaml().
  throw new Error('ResourceDrawer requires `yamlApi` to be provided at the app level')
}
const api: YamlApi = yamlApi

type TabKey = 'overview' | 'yaml' | 'events' | 'logs'
const activeTab = ref<TabKey>('overview')
const yamlText = ref<string>('')
const yamlLoading = ref<boolean>(false)
const yamlError = ref<string>('')

async function loadYaml(): Promise<void> {
  if (yamlText.value) return
  yamlLoading.value = true
  yamlError.value = ''
  try {
    yamlText.value = await api.get(props.clusterId, {
      kind: props.kind,
      namespace: props.namespace,
      name: props.name,
    })
  } catch (e) {
    yamlError.value = (e as Error)?.message ?? String(e)
  } finally {
    yamlLoading.value = false
  }
}

watch(activeTab, async (v) => {
  if (v === 'yaml') await loadYaml()
})

// Reset cached YAML + tab whenever the drawer points at a new resource.
watch(
  () => `${props.kind}:${props.namespace ?? ''}:${props.name}`,
  () => {
    yamlText.value = ''
    yamlError.value = ''
    activeTab.value = 'overview'
  }
)

const showSecretWarning = computed(() => props.kind === 'secret')
</script>

<template>
  <a-drawer
    :open="open"
    :title="name"
    width="60%"
    :destroy-on-close="true"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <a-tabs v-model:active-key="activeTab">
      <a-tab-pane key="overview" :tab="$t('k8s.tab.overview')">
        <slot name="overview" :detail="detail" />
      </a-tab-pane>
      <a-tab-pane key="yaml" :tab="$t('k8s.tab.yaml')">
        <a-alert
          v-if="showSecretWarning"
          :message="$t('k8s.secret.yaml_warning')"
          type="warning"
          show-icon
          style="margin-bottom: 8px"
        />
        <a-alert
          v-if="yamlError"
          :message="yamlError"
          type="error"
          show-icon
          style="margin-bottom: 8px"
        />
        <a-spin :spinning="yamlLoading">
          <YamlViewer v-model="yamlText" />
        </a-spin>
      </a-tab-pane>
      <a-tab-pane key="events" :tab="$t('k8s.tab.events')">
        <slot name="events" />
      </a-tab-pane>
      <a-tab-pane v-if="kind === 'pod'" key="logs" :tab="$t('k8s.tab.logs')">
        <LogViewer
          :cluster-id="clusterId"
          :namespace="namespace ?? ''"
          :pod="name"
          :containers="podContainers ?? []"
        />
      </a-tab-pane>
    </a-tabs>
  </a-drawer>
</template>
