<template>
  <div class="workloads-page">
    <a-card>
      <template #title>
        <a-space wrap>
          <span>{{ $t('menu.k8s.workloads') }}</span>
          <a-select
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
        <a-tab-pane
          v-for="k in KINDS"
          :key="k"
          :tab="$t(`k8s.workload.${KIND_SINGULAR[k]}`)"
        >
          <a-alert
            v-if="truncated && currentKind === k"
            :message="$t('k8s.list.truncated', { limit: 500 })"
            type="info"
            show-icon
            style="margin-bottom: 8px;"
          />
          <a-table
            :columns="columnsByKind[k]"
            :data-source="currentKind === k ? items : []"
            :loading="loading && currentKind === k"
            :row-key="rowKey"
            :pagination="false"
            :custom-row="customRow"
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
      :namespace="drawerDetail?.namespace ?? ''"
      :name="drawerDetail?.name ?? ''"
      :detail="drawerDetail"
      :pod-containers="podContainers"
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
          <a-descriptions-item :label="$t('k8s.cluster.namespace')">
            {{ detailRecord(detail)?.namespace ?? '-' }}
          </a-descriptions-item>

          <template v-if="drawerKind === 'pod'">
            <a-descriptions-item label="Phase">
              {{ detailRecord(detail)?.phase ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="Ready">
              {{ detailRecord(detail)?.ready_containers ?? 0 }}/{{ detailRecord(detail)?.total_containers ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Restarts">
              {{ detailRecord(detail)?.restart_count ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Node">
              {{ detailRecord(detail)?.node_name ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="IP">
              {{ detailRecord(detail)?.pod_ip ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item
              v-if="detailRecord(detail)?.status_reason"
              label="Reason"
            >
              {{ detailRecord(detail)?.status_reason }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'deployment'">
            <a-descriptions-item label="Replicas">
              {{ detailRecord(detail)?.replicas_ready ?? 0 }}/{{ detailRecord(detail)?.replicas_desired ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Updated">
              {{ detailRecord(detail)?.replicas_updated ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Available">
              {{ detailRecord(detail)?.replicas_available ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Strategy">
              {{ detailRecord(detail)?.strategy ?? '-' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'statefulset'">
            <a-descriptions-item label="Replicas">
              {{ detailRecord(detail)?.ready_replicas ?? 0 }}/{{ detailRecord(detail)?.replicas ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Service">
              {{ detailRecord(detail)?.service_name ?? '-' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'daemonset'">
            <a-descriptions-item label="Desired">
              {{ detailRecord(detail)?.desired_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Current">
              {{ detailRecord(detail)?.current_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Ready">
              {{ detailRecord(detail)?.ready_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Available">
              {{ detailRecord(detail)?.available_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Misscheduled">
              {{ detailRecord(detail)?.misscheduled ?? 0 }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'job'">
            <a-descriptions-item label="Completions">
              {{ detailRecord(detail)?.completions ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Succeeded">
              {{ detailRecord(detail)?.succeeded ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Failed">
              {{ detailRecord(detail)?.failed ?? 0 }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'cronjob'">
            <a-descriptions-item label="Schedule">
              {{ detailRecord(detail)?.schedule ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="Active">
              {{ detailRecord(detail)?.active_jobs ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Suspended">
              {{ detailRecord(detail)?.suspended ? 'Yes' : 'No' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'replicaset'">
            <a-descriptions-item label="Replicas">
              {{ detailRecord(detail)?.ready_replicas ?? 0 }}/{{ detailRecord(detail)?.replicas ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item
              v-if="detailRecord(detail)?.owner_kind"
              label="Owner"
            >
              {{ detailRecord(detail)?.owner_kind }}/{{ detailRecord(detail)?.owner_name }}
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
import { columnsByKind } from './components/columns'
import type { WorkloadApi } from '@/api/k8s/workload'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type { PodSummary, WorkloadKind, WorkloadSummary } from '@/types/api'

dayjs.extend(relativeTime)

/**
 * Workloads page (Plan T22).
 *
 * Single page with 7 antd tabs (deployment/statefulset/daemonset/job/cronjob/
 * replicaset/pod). Each tab shares one `<a-table>`-per-pane bound to the
 * current kind. The page is cluster-scoped — landing here without a cluster
 * redirects to /k8s/clusters with a hint message.
 *
 * Namespace selector and tab switch both trigger an immediate reload. The
 * ClusterPicker bumps `?_r=<ts>` on cluster swap so list pages re-enter and
 * refetch; we watch that param too.
 */
const router = useRouter()
const k8s = useK8sStore()
const { t } = useI18n()

const workloadApi = inject<WorkloadApi>('workloadApi')!
const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!

const KINDS: WorkloadKind[] = [
  'deployments',
  'statefulsets',
  'daemonsets',
  'jobs',
  'cronjobs',
  'replicasets',
  'pods',
]

// Plural URL kind -> singular drawer kind, used by both the i18n tab label
// lookup and the ResourceDrawer's `kind` prop (singular is what the YAML +
// log endpoints expect).
const KIND_SINGULAR: Record<WorkloadKind, string> = {
  deployments: 'deployment',
  statefulsets: 'statefulset',
  daemonsets: 'daemonset',
  jobs: 'job',
  cronjobs: 'cronjob',
  replicasets: 'replicaset',
  pods: 'pod',
}

const currentKind = ref<WorkloadKind>('deployments')
const items = ref<WorkloadSummary[]>([])
const truncated = ref(false)
const loading = ref(false)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<WorkloadSummary | null>(null)

// Two-way binding helper — `useK8sStore().currentNamespace` is the source of
// truth, but the select needs a plain ref to mutate. We forward via setter.
const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKey(record: WorkloadSummary) {
  // Names are unique within a (kind, namespace) tuple; namespace+name avoids
  // duplicate-key warnings when a cluster-wide list returns same-named items
  // from different namespaces.
  return `${record.namespace}/${record.name}`
}

function detailRecord(detail: unknown): Record<string, never> | undefined {
  // The drawer slot exposes `detail` as `unknown`; this narrows it for the
  // template without leaking `any`. We accept that fields not present on a
  // given kind render as undefined (then masked to '-' by the template).
  return (detail ?? undefined) as Record<string, never> | undefined
}

const podContainers = computed<string[]>(() => {
  if (drawerKind.value !== 'pod') return []
  const d = drawerDetail.value as PodSummary | null
  if (!d) return []
  // PodSummary doesn't expose a container list; the page only knows how many
  // containers are ready. For v1 we hand LogViewer an empty list — it falls
  // back to the default container.
  return []
})

async function load(kind: WorkloadKind): Promise<void> {
  if (!k8s.currentClusterId) return
  loading.value = true
  try {
    const res = await workloadApi.list(k8s.currentClusterId, kind, {
      namespace: k8s.currentNamespace || undefined,
    })
    items.value = res.items
    truncated.value = res.truncated
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loading.value = false
  }
}

function customRow(record: WorkloadSummary) {
  return {
    onClick: () => void openDetail(record),
    style: { cursor: 'pointer' },
  }
}

async function openDetail(record: WorkloadSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  drawerKind.value = KIND_SINGULAR[currentKind.value]
  try {
    drawerDetail.value = await workloadApi.get(
      k8s.currentClusterId,
      currentKind.value,
      record.namespace,
      record.name,
    )
    drawerOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onNamespaceChange(): void {
  // `currentNamespace` setter is invoked via v-model; an explicit reload here
  // makes the dependency from selector -> table data unambiguous.
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
// ClusterPicker bumps ?_r=<ts> when the user swaps clusters; re-pull on each bump.
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
  void k8s.ensureNamespaces(cid => k8sNsApi.list(cid))
  void load(currentKind.value)
})
</script>

<style scoped lang="scss">
.workloads-page {
  // Tighten the in-card tab body — antd's default padding wastes vertical
  // space when the page is mostly a table.
  :deep(.ant-tabs-content-holder) {
    padding-top: 4px;
  }
}
</style>
