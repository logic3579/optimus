<template>
  <div class="config-page">
    <a-card>
      <template #title>
        <a-space wrap>
          <span>{{ $t('menu.k8s.config') }}</span>
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
        <a-tab-pane key="configmaps" :tab="$t('k8s.config.configmap')">
          <a-alert
            v-if="truncated && currentKind === 'configmaps'"
            :message="$t('k8s.list.truncated', { limit: 500 })"
            type="info"
            show-icon
            style="margin-bottom: 8px;"
          />
          <a-table
            :columns="configmapColumns"
            :data-source="currentKind === 'configmaps' ? (items as ConfigMapSummary[]) : []"
            :loading="loading && currentKind === 'configmaps'"
            :row-key="rowKey"
            :pagination="false"
            :custom-row="customRow"
            size="small"
          />
        </a-tab-pane>
        <a-tab-pane key="secrets" :tab="$t('k8s.config.secret')">
          <a-alert
            v-if="truncated && currentKind === 'secrets'"
            :message="$t('k8s.list.truncated', { limit: 500 })"
            type="info"
            show-icon
            style="margin-bottom: 8px;"
          />
          <a-table
            :columns="secretColumns"
            :data-source="currentKind === 'secrets' ? (items as SecretSummary[]) : []"
            :loading="loading && currentKind === 'secrets'"
            :row-key="rowKey"
            :pagination="false"
            :custom-row="customRow"
            size="small"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'actions'">
                <!-- Reveal is a separate, perm-gated affordance. Stop propagation
                     so clicking it doesn't ALSO open the resource drawer. -->
                <a
                  v-permission="'k8s:secret:reveal'"
                  @click.stop="reveal(record as SecretSummary)"
                >
                  {{ $t('k8s.secret.reveal') }}
                </a>
              </template>
            </template>
          </a-table>
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

          <template v-if="drawerKind === 'configmap'">
            <a-descriptions-item label="DataCount">
              {{ asConfigMap(detail)?.data_count ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item
              label="Keys"
              :span="2"
            >
              <a-tag
                v-for="k in asConfigMap(detail)?.data_keys ?? []"
                :key="k"
              >
                {{ k }}
              </a-tag>
              <span v-if="!(asConfigMap(detail)?.data_keys?.length)">-</span>
            </a-descriptions-item>
            <a-descriptions-item
              v-if="(asConfigMap(detail)?.binary_keys ?? []).length"
              label="BinaryKeys"
              :span="2"
            >
              <a-tag
                v-for="k in asConfigMap(detail)?.binary_keys ?? []"
                :key="k"
                color="orange"
              >
                {{ k }}
              </a-tag>
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'secret'">
            <a-descriptions-item label="Type">
              {{ asSecret(detail)?.type ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="DataCount">
              {{ asSecret(detail)?.data_count ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item
              label="Keys"
              :span="2"
            >
              <a-tag
                v-for="k in asSecret(detail)?.data_keys ?? []"
                :key="k"
              >
                {{ k }}
              </a-tag>
              <span v-if="!(asSecret(detail)?.data_keys?.length)">-</span>
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

    <a-modal
      v-model:open="revealOpen"
      :title="$t('k8s.secret.reveal') + ': ' + revealingName"
      :footer="null"
      width="600px"
    >
      <a-table
        :columns="[
          { title: 'Key', dataIndex: 'key' },
          { title: 'Value', dataIndex: 'value' },
        ]"
        :data-source="revealedRows"
        :pagination="false"
        size="small"
        row-key="key"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'value'">
            <code v-if="typeof record.value === 'string'">{{ record.value }}</code>
            <a-tag v-else color="orange">
              {{ $t('k8s.secret.binary_b64') }}: {{ record.value.value }}
            </a-tag>
          </template>
        </template>
      </a-table>
    </a-modal>
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
import { configmapColumns, secretBaseColumns } from './components/columns'
import type { ConfigMapApi } from '@/api/k8s/configmap'
import type { SecretApi } from '@/api/k8s/secret'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type {
  ConfigMapDetail,
  ConfigMapSummary,
  SecretDetail,
  SecretSummary,
} from '@/types/api'

dayjs.extend(relativeTime)

/**
 * Config + Secret page (Plan T23). Two tabs. Both kinds are namespaced. The
 * Secret tab gets an extra perm-gated Reveal action which decodes data into a
 * separate modal — the YAML tab (already in ResourceDrawer) keeps showing the
 * raw base64 with its own warning banner.
 */
type ConfigKind = 'configmaps' | 'secrets'

const router = useRouter()
const k8s = useK8sStore()
const { t } = useI18n()

const configMapApi = inject<ConfigMapApi>('configMapApi')!
const secretApi = inject<SecretApi>('secretApi')!
const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!

const currentKind = ref<ConfigKind>('configmaps')
const items = ref<ConfigMapSummary[] | SecretSummary[]>([])
const truncated = ref(false)
const loading = ref(false)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<ConfigMapDetail | SecretDetail | null>(null)

// Secret reveal modal state. Kept on the page (not in a child component) so
// the perm-gated Reveal action and its modal share the same scope and the
// modal closes naturally when the tab unmounts.
const revealOpen = ref(false)
const revealingName = ref('')
const revealedRows = ref<Array<{ key: string; value: string | { value: string; base64: true } }>>([])

const secretColumns = [
  ...secretBaseColumns,
  { title: t('common.actions'), key: 'actions', width: 110 },
]

const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKey(record: ConfigMapSummary | SecretSummary) {
  return `${record.namespace}/${record.name}`
}

function detailRecord(detail: unknown): { name?: string; namespace?: string; age?: string } | undefined {
  return (detail ?? undefined) as { name?: string; namespace?: string; age?: string } | undefined
}

function asConfigMap(detail: unknown): ConfigMapDetail | undefined {
  return (detail ?? undefined) as ConfigMapDetail | undefined
}

function asSecret(detail: unknown): SecretDetail | undefined {
  return (detail ?? undefined) as SecretDetail | undefined
}

async function load(kind: ConfigKind): Promise<void> {
  if (!k8s.currentClusterId) return
  loading.value = true
  try {
    if (kind === 'configmaps') {
      const res = await configMapApi.list(k8s.currentClusterId, {
        namespace: k8s.currentNamespace || undefined,
      })
      items.value = res.items
      truncated.value = res.truncated
    } else {
      const res = await secretApi.list(k8s.currentClusterId, {
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

function customRow(record: ConfigMapSummary | SecretSummary) {
  return {
    onClick: () => void openDetail(record),
    style: { cursor: 'pointer' },
  }
}

async function openDetail(record: ConfigMapSummary | SecretSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  try {
    if (currentKind.value === 'configmaps') {
      drawerKind.value = 'configmap'
      drawerDetail.value = await configMapApi.get(k8s.currentClusterId, record.namespace, record.name)
    } else {
      drawerKind.value = 'secret'
      drawerDetail.value = await secretApi.get(k8s.currentClusterId, record.namespace, record.name)
    }
    drawerOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function reveal(record: SecretSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  try {
    const res = await secretApi.data(k8s.currentClusterId, record.namespace, record.name)
    revealedRows.value = Object.entries(res.data).map(([k, v]) => ({ key: k, value: v }))
    revealingName.value = record.name
    revealOpen.value = true
  } catch (e: unknown) {
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
.config-page {
  :deep(.ant-tabs-content-holder) {
    padding-top: 4px;
  }
}
</style>
