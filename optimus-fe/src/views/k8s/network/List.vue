<template>
  <div class="network-page">
    <a-card>
      <template #title>
        <a-space wrap>
          <span>{{ $t('menu.k8s.network') }}</span>
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
          :tab="$t(`k8s.network.${KIND_SINGULAR[k]}`)"
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

          <template v-if="drawerKind === 'service'">
            <a-descriptions-item label="Type">
              {{ asService(detail)?.type ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="ClusterIP">
              {{ asService(detail)?.cluster_ip ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item
              label="Ports"
              :span="2"
            >
              <a-tag
                v-for="p in asService(detail)?.ports ?? []"
                :key="`${p.port}/${p.protocol}`"
                color="blue"
              >
                {{ p.port }}/{{ p.protocol }}
              </a-tag>
              <span v-if="!(asService(detail)?.ports?.length)">-</span>
            </a-descriptions-item>
            <a-descriptions-item
              v-if="(asService(detail)?.external_ips ?? []).length"
              label="ExternalIPs"
              :span="2"
            >
              {{ (asService(detail)?.external_ips ?? []).join(', ') }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'ingress'">
            <a-descriptions-item label="Class">
              {{ asIngress(detail)?.ingress_class ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item
              label="Hosts"
              :span="2"
            >
              <a-tag
                v-for="h in asIngress(detail)?.hosts ?? []"
                :key="h"
                color="geekblue"
              >
                {{ h }}
              </a-tag>
              <span v-if="!(asIngress(detail)?.hosts?.length)">-</span>
            </a-descriptions-item>
            <a-descriptions-item
              v-if="(asIngress(detail)?.load_balancer_ips ?? []).length"
              label="LoadBalancerIPs"
              :span="2"
            >
              {{ (asIngress(detail)?.load_balancer_ips ?? []).join(', ') }}
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
import type { K8sNetworkApi } from '@/api/k8s/network'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type {
  NetworkKind,
  NetworkSummary,
  ServiceSummary,
  IngressSummary,
} from '@/types/api'

dayjs.extend(relativeTime)

/**
 * Network page (Plan T23). Two antd tabs (services, ingresses) sharing one
 * cluster-required guard and one namespace selector. Both kinds are namespaced.
 * Row click opens ResourceDrawer keyed by singular kind (service/ingress) so
 * the YAML/Events tabs talk to the right BE endpoint.
 */
const router = useRouter()
const k8s = useK8sStore()
const { t } = useI18n()

const networkApi = inject<K8sNetworkApi>('k8sNetworkApi')!
const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!

const KINDS: NetworkKind[] = ['services', 'ingresses']

const KIND_SINGULAR: Record<NetworkKind, string> = {
  services: 'service',
  ingresses: 'ingress',
}

const currentKind = ref<NetworkKind>('services')
const items = ref<NetworkSummary[]>([])
const truncated = ref(false)
const loading = ref(false)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<NetworkSummary | null>(null)

const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKey(record: NetworkSummary) {
  return `${record.namespace}/${record.name}`
}

function detailRecord(detail: unknown): { name?: string; namespace?: string; age?: string } | undefined {
  return (detail ?? undefined) as { name?: string; namespace?: string; age?: string } | undefined
}

function asService(detail: unknown): ServiceSummary | undefined {
  return (detail ?? undefined) as ServiceSummary | undefined
}

function asIngress(detail: unknown): IngressSummary | undefined {
  return (detail ?? undefined) as IngressSummary | undefined
}

async function load(kind: NetworkKind): Promise<void> {
  if (!k8s.currentClusterId) return
  loading.value = true
  try {
    const res = await networkApi.list(k8s.currentClusterId, kind, {
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

function customRow(record: NetworkSummary) {
  return {
    onClick: () => void openDetail(record),
    style: { cursor: 'pointer' },
  }
}

async function openDetail(record: NetworkSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  drawerKind.value = KIND_SINGULAR[currentKind.value]
  try {
    drawerDetail.value = await networkApi.get(
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
  void k8s.ensureNamespaces(cid => k8sNsApi.list(cid))
  void load(currentKind.value)
})
</script>

<style scoped lang="scss">
.network-page {
  :deep(.ant-tabs-content-holder) {
    padding-top: 4px;
  }
}
</style>
