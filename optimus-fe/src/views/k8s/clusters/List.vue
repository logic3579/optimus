<template>
  <a-card>
    <PageHeader :title="$t('k8s.cluster.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="$t('k8s.cluster.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-input
        v-model:value="tagInput"
        :placeholder="$t('k8s.cluster.filter_tag')"
        style="width: 200px;"
        allow-clear
        @press-enter="onTagSubmit"
        @change="onTagChange"
      />
      <a-button
        :loading="refreshingAll"
        @click="refreshAllHealth"
      >
        <template #icon><reload-outlined /></template>
        {{ $t('k8s.cluster.refresh_all_health') }}
      </a-button>
      <a-button
        v-permission="'k8s:cluster:write'"
        type="primary"
        @click="openCreate"
      >
        {{ $t('k8s.cluster.action.create') }}
      </a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'tags'">
          <a-tag v-for="tg in record.tags ?? []" :key="tg">{{ tg }}</a-tag>
        </template>
        <template v-else-if="column.key === 'health'">
          <a-tooltip :title="healthTooltip(record)">
            <span :class="['health-dot', dotClass(record.last_health_ok)]" />
            <span class="health-label">{{ healthLabel(record.last_health_ok) }}</span>
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'k8s:cluster:write'" @click="openEdit(record)">
              {{ $t('k8s.cluster.action.edit') }}
            </a>
            <a
              v-permission="'k8s:cluster:write'"
              :class="{ disabled: pinging[record.id] }"
              @click="ping(record)"
            >
              {{ $t('k8s.cluster.ping') }}
            </a>
            <a-popconfirm
              :title="$t('k8s.cluster.action.confirm_delete')"
              @confirm="remove(record)"
            >
              <a v-permission="'k8s:cluster:write'" class="danger">
                {{ $t('k8s.cluster.action.delete') }}
              </a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="table.setPage"
      @show-size-change="(_: number, size: number) => table.setPageSize(size)"
    />

    <ClusterForm v-model:open="editOpen" :initial="editing" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import ClusterForm from './components/ClusterForm.vue'
import type { ClusterApi } from '@/api/k8s/cluster'
import type { Cluster, ClusterListQuery } from '@/types/api'

const { t } = useI18n()
const clusterApi = inject<ClusterApi>('clusterApi')!

const searchInput = ref('')
const tagInput = ref('')

type Filters = Pick<ClusterListQuery, 'search' | 'tag'>

const table = useTable<Cluster, Filters>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await clusterApi.list({
      page,
      page_size: pageSize,
      search: filters?.search || undefined,
      tag: filters?.tag || undefined,
    })
    return { items: r.items, total: r.total }
  },
})

const columns = computed(() => [
  { key: 'name',             title: t('k8s.cluster.name'),        dataIndex: 'name' },
  { key: 'kubeconfig_name',  title: t('k8s.cluster.kubeconfig'),  dataIndex: 'kubeconfig_name' },
  { key: 'context',          title: t('k8s.cluster.context'),     dataIndex: 'context' },
  { key: 'tags',             title: t('k8s.cluster.tags'),        dataIndex: 'tags' },
  { key: 'health',           title: t('k8s.cluster.col_health'),  width: 140 },
  { key: 'description',      title: t('k8s.cluster.description'), dataIndex: 'description', ellipsis: true },
  { key: 'created_at',       title: t('k8s.cluster.col_created_at') },
  { key: 'actions',          title: '', width: 200 },
])

const editOpen = ref(false)
const editing = ref<Cluster | null>(null)
const pinging = reactive<Record<number, boolean>>({})
const refreshingAll = ref(false)

function onSearch(v: string) { table.setFilters({ search: v || undefined }) }
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ search: undefined })
}
function onTagSubmit() { table.setFilters({ tag: tagInput.value || undefined }) }
function onTagChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ tag: undefined })
}

function openCreate() {
  editing.value = null
  editOpen.value = true
}
async function openEdit(r: Cluster) {
  try {
    editing.value = await clusterApi.get(r.id)
    editOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function remove(r: Cluster) {
  try {
    await clusterApi.remove(r.id)
    message.success(t('k8s.cluster.toast.deleted'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function ping(r: Cluster) {
  if (pinging[r.id]) return
  pinging[r.id] = true
  try {
    const res = await clusterApi.ping(r.id)
    if (res.ok) {
      message.success(t('k8s.cluster.toast.ping_ok', { version: res.server_version ?? 'unknown' }))
    } else {
      message.error(res.message || t('k8s.cluster.toast.ping_fail'))
    }
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    pinging[r.id] = false
  }
}

async function refreshAllHealth() {
  if (refreshingAll.value) return
  refreshingAll.value = true
  try {
    // Sequential fan-out into the existing ping endpoint. We swallow per-row
    // errors so one unreachable cluster doesn't abort the rest.
    await Promise.all(
      table.items.value.map(async c => {
        try { await clusterApi.ping(c.id) } catch { /* ignored */ }
      })
    )
    message.success(t('k8s.cluster.toast.refresh_all_done'))
    await table.reload()
  } finally {
    refreshingAll.value = false
  }
}

function onSaved() { table.reload() }

function dotClass(ok: boolean | null | undefined): string {
  if (ok === true) return 'ok'
  if (ok === false) return 'fail'
  return 'unknown'
}
function healthLabel(ok: boolean | null | undefined): string {
  if (ok === true) return t('k8s.cluster.health.ok')
  if (ok === false) return t('k8s.cluster.health.fail')
  return t('k8s.cluster.health.unknown')
}
function healthTooltip(r: Cluster): string {
  const parts: string[] = []
  if (r.last_health_at) parts.push(formatTime(r.last_health_at))
  if (r.last_health_msg) parts.push(r.last_health_msg)
  return parts.join(' · ')
}

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

onMounted(() => { table.reload() })
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}
.health-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;

  &.ok      { background: #52c41a; }
  &.fail    { background: #ff4d4f; }
  &.unknown { background: #d9d9d9; }
}
.health-label {
  vertical-align: middle;
}
.danger {
  color: var(--ant-color-error, #ff4d4f);
}
.disabled {
  opacity: 0.5;
  pointer-events: none;
}
</style>
