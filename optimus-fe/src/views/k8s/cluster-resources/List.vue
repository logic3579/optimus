<template>
  <div class="cluster-resources-page">
    <a-card>
      <template #title>
        <a-space wrap>
          <span>{{ $t('menu.k8s.cluster_resources') }}</span>
          <!-- Namespace selector is only meaningful on the Events tab; for
               Namespace + Node it would be confusing (a namespace can't filter
               a list of namespaces). -->
          <a-select
            v-if="currentKind === 'events'"
            v-model:value="namespaceModel"
            :placeholder="$t('k8s.cluster.namespace_all')"
            style="width: 220px;"
            allow-clear
            @change="onNamespaceChange"
          >
            <a-select-option value="">{{ $t('k8s.cluster.namespace_all') }}</a-select-option>
            <a-select-option
              v-for="n in k8s.namespaces"
              :key="n"
              :value="n"
            >
              {{ n }}
            </a-select-option>
          </a-select>
          <a-button
            :loading="loading"
            @click="load(currentKind)"
          >
            <template #icon>
              <reload-outlined />
            </template>
            {{ $t('common.refresh') }}
          </a-button>
        </a-space>
      </template>

      <a-tabs v-model:active-key="currentKind">
        <a-tab-pane key="namespaces" :tab="$t('k8s.cluster_resource.namespace')">
          <a-alert
            v-if="truncated && currentKind === 'namespaces'"
            :message="$t('k8s.list.truncated', { limit: 500 })"
            type="info"
            show-icon
            style="margin-bottom: 8px;"
          />
          <a-table
            :columns="namespaceColumns"
            :data-source="currentKind === 'namespaces' ? (items as NamespaceSummary[]) : []"
            :loading="loading && currentKind === 'namespaces'"
            :row-key="rowKeyName"
            :pagination="false"
            :custom-row="customRowNamespace"
            size="small"
          />
        </a-tab-pane>
        <a-tab-pane key="nodes" :tab="$t('k8s.cluster_resource.node')">
          <a-alert
            v-if="truncated && currentKind === 'nodes'"
            :message="$t('k8s.list.truncated', { limit: 500 })"
            type="info"
            show-icon
            style="margin-bottom: 8px;"
          />
          <a-table
            :columns="nodeColumns"
            :data-source="currentKind === 'nodes' ? (items as NodeSummary[]) : []"
            :loading="loading && currentKind === 'nodes'"
            :row-key="rowKeyName"
            :pagination="false"
            :custom-row="customRowNode"
            size="small"
          />
        </a-tab-pane>
        <a-tab-pane key="events" :tab="$t('k8s.cluster_resource.event')">
          <a-alert
            v-if="truncated && currentKind === 'events'"
            :message="$t('k8s.list.truncated', { limit: 500 })"
            type="info"
            show-icon
            style="margin-bottom: 8px;"
          />
          <a-table
            :columns="eventColumns"
            :data-source="currentKind === 'events' ? (items as EventSummary[]) : []"
            :loading="loading && currentKind === 'events'"
            :row-key="rowKeyEvent"
            :pagination="false"
            size="small"
          />
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <ResourceDrawer
      v-if="drawerOpen && k8s.currentClusterId !== null"
      v-model:open="drawerOpen"
      :cluster-id="k8s.currentClusterId"
      :kind="drawerKind"
      :namespace="''"
      :name="drawerDetail?.name ?? ''"
      :detail="drawerDetail"
    >
      <template #overview="{ detail }">
        <a-descriptions
          :column="2"
          bordered
          size="small"
        >
          <a-descriptions-item :label="$t('k8s.cluster.name')">
            {{ detailRecord(detail)?.name ?? '-' }}
          </a-descriptions-item>

          <template v-if="drawerKind === 'namespace'">
            <a-descriptions-item label="Phase">
              {{ asNamespace(detail)?.phase ?? '-' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'node'">
            <a-descriptions-item label="Ready">
              {{ asNode(detail)?.ready ? 'Yes' : 'No' }}
            </a-descriptions-item>
            <a-descriptions-item label="Schedulable">
              {{ asNode(detail)?.schedulable ? 'Yes' : 'No' }}
            </a-descriptions-item>
            <a-descriptions-item label="Roles">
              {{ (asNode(detail)?.roles ?? []).join(', ') || '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="KubeletVersion">
              {{ asNode(detail)?.kubelet_version ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="CPU">
              {{ asNode(detail)?.cpu_capacity ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="Mem">
              {{ asNode(detail)?.mem_capacity ?? '-' }}
            </a-descriptions-item>
          </template>

          <a-descriptions-item
            v-if="detailRecord(detail)?.age"
            label="Age"
          >
            {{ formatAge(detailRecord(detail)?.age) }}
          </a-descriptions-item>
        </a-descriptions>
      </template>
      <template #events>
        <a-empty :description="$t('placeholder.coming_soon')" />
      </template>
    </ResourceDrawer>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeMount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

import { useI18n } from '@/hooks/useI18n'
import { useK8sStore } from '@/stores/k8s'
import { isBizError } from '@/utils/http-error'
import ResourceDrawer from '@/components/k8s/ResourceDrawer.vue'
import { eventColumns, namespaceColumns, nodeColumns } from './components/columns'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type { K8sNodeApi } from '@/api/k8s/node'
import type { K8sEventApi } from '@/api/k8s/event'
import type {
  EventSummary,
  NamespaceSummary,
  NodeSummary,
} from '@/types/api'

dayjs.extend(relativeTime)

/**
 * Cluster-resources page (Plan T23). Three tabs: Namespaces, Nodes, Events.
 *
 * - Namespace + Node are cluster-scoped — no namespace selector.
 * - Events are technically namespaced but we expose an OPTIONAL filter (blank
 *   = cluster-wide). The selector hides itself for the other two tabs.
 * - Events tab has no drawer; the row IS the detail.
 */
type ClusterResourceKind = 'namespaces' | 'nodes' | 'events'

const router = useRouter()
const k8s = useK8sStore()
const { t } = useI18n()

const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!
const k8sNodeApi = inject<K8sNodeApi>('k8sNodeApi')!
const k8sEventApi = inject<K8sEventApi>('k8sEventApi')!

const currentKind = ref<ClusterResourceKind>('namespaces')
const items = ref<NamespaceSummary[] | NodeSummary[] | EventSummary[]>([])
const truncated = ref(false)
const loading = ref(false)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<NamespaceSummary | NodeSummary | null>(null)

const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKeyName(record: { name: string }) {
  return record.name
}

function rowKeyEvent(record: EventSummary, index?: number) {
  // Events don't have unique names — multiple Pods can emit the same
  // (reason, involved_kind, involved_name) tuple. Index keeps each row
  // distinct without warnings.
  return `${record.namespace ?? ''}/${record.involved_kind}/${record.involved_name}/${record.reason}/${index ?? 0}`
}

function detailRecord(detail: unknown): { name?: string; age?: string } | undefined {
  return (detail ?? undefined) as { name?: string; age?: string } | undefined
}

function asNamespace(detail: unknown): NamespaceSummary | undefined {
  return (detail ?? undefined) as NamespaceSummary | undefined
}

function asNode(detail: unknown): NodeSummary | undefined {
  return (detail ?? undefined) as NodeSummary | undefined
}

async function load(kind: ClusterResourceKind): Promise<void> {
  if (!k8s.currentClusterId) return
  loading.value = true
  try {
    if (kind === 'namespaces') {
      const res = await k8sNsApi.list(k8s.currentClusterId)
      items.value = res.items
      truncated.value = res.truncated
    } else if (kind === 'nodes') {
      const res = await k8sNodeApi.list(k8s.currentClusterId)
      items.value = res.items
      truncated.value = res.truncated
    } else {
      const res = await k8sEventApi.list(k8s.currentClusterId, {
        namespace: k8s.currentNamespace || undefined,
      })
      items.value = res.items
      truncated.value = res.truncated
    }
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loading.value = false
  }
}

function customRowNamespace(record: NamespaceSummary) {
  return {
    onClick: () => void openNamespace(record),
    style: { cursor: 'pointer' },
  }
}

function customRowNode(record: NodeSummary) {
  return {
    onClick: () => void openNode(record),
    style: { cursor: 'pointer' },
  }
}

function openNamespace(record: NamespaceSummary): void {
  drawerKind.value = 'namespace'
  drawerDetail.value = record
  drawerOpen.value = true
}

async function openNode(record: NodeSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  drawerKind.value = 'node'
  try {
    drawerDetail.value = await k8sNodeApi.get(k8s.currentClusterId, record.name)
    drawerOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onNamespaceChange(): void {
  void load(currentKind.value)
}

function formatAge(iso?: string): string {
  if (!iso) return '-'
  return dayjs(iso).fromNow(true)
}

watch(currentKind, kind => void load(kind))
watch(
  () => k8s.currentClusterId,
  (id) => {
    if (id !== null) void load(currentKind.value)
  },
)
watch(
  () => router.currentRoute.value.query._r,
  () => {
    if (k8s.currentClusterId !== null) void load(currentKind.value)
  },
)

onBeforeMount(() => {
  if (k8s.currentClusterId === null) {
    message.info(t('k8s.cluster.no_cluster_selected'))
    void router.replace('/k8s/clusters')
    return
  }
  // Namespace cache is needed for the Events tab filter.
  void k8s.ensureNamespaces(cid => k8sNsApi.list(cid))
  void load(currentKind.value)
})
</script>

<style scoped lang="scss">
.cluster-resources-page {
  :deep(.ant-tabs-content-holder) {
    padding-top: 4px;
  }
}
</style>
