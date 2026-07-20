<template>
  <a-card>
    <PageHeader :title="t('assets.sync.title')" />

    <div class="filter-row u-mb-16">
      <a-input-number
        v-model:value="accountID"
        :min="1"
        :precision="0"
        :placeholder="t('assets.sync.filter.account')"
        style="width: 160px"
        @change="onAccountChange"
      />
      <a-select
        v-model:value="resourceType"
        data-testid="resource-type"
        allow-clear
        :placeholder="t('assets.sync.filter.resource_type')"
        :options="resourceTypeOptions"
        style="width: 170px"
        @change="onResourceTypeChange"
      />
      <a-select
        v-model:value="status"
        data-testid="status"
        allow-clear
        :placeholder="t('assets.sync.filter.status')"
        :options="statusOptions"
        style="width: 150px"
        @change="onStatusChange"
      />
      <a-date-picker
        show-time
        allow-clear
        :placeholder="t('assets.sync.filter.started_after')"
        @change="onStartedAfterChange"
      />
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'started_at'">{{ formatTime(record.started_at) }}</template>
        <template v-else-if="column.key === 'finished_at'">{{ record.finished_at ? formatTime(record.finished_at) : '—' }}</template>
        <template v-else-if="column.key === 'duration'">{{ formatDuration(record) }}</template>
        <template v-else-if="column.key === 'resource_type'">{{ t(`assets.sync.resource_types.${record.resource_type}`) }}</template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="statusTagColor(record.status)">{{ t(`assets.sync.run_status.${record.status}`) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'trigger'">{{ t(`assets.sync.triggers.${record.trigger}`) }}</template>
        <template v-else-if="column.key === 'error'">
          <a-tooltip v-if="record.error" :title="record.error">
            <button type="button" class="error-code" :aria-label="t('assets.sync.error')">
              {{ record.error_code ?? '—' }}
            </button>
          </a-tooltip>
          <template v-else>—</template>
        </template>
        <template v-else>{{ record[column.dataIndex] ?? '—' }}</template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="onPageChange"
      @show-size-change="onPageSizeChange"
    />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'

import type { AssetsSyncApi, SyncRunListParams } from '@/api/assets/sync'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import type { SyncRunResourceType, SyncRunStatus, SyncRunSummary } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const { t } = useI18n()
const api = inject<AssetsSyncApi>('assetsSyncApi')!
const accountID = ref<number>()
const resourceType = ref<SyncRunResourceType>()
const status = ref<SyncRunStatus>()

const statusColor: Record<SyncRunStatus, string> = {
  running: 'blue', success: 'green', failed: 'red', skipped: 'gold',
}
const resourceTypeOptions = computed(() => (['instance', 'network', 'database'] as const).map(value => ({
  value, label: t(`assets.sync.resource_types.${value}`),
})))
const statusOptions = computed(() => (['running', 'success', 'failed', 'skipped'] as const).map(value => ({
  value, label: t(`assets.sync.run_status.${value}`),
})))
const columns = computed(() => [
  { key: 'started_at', title: t('assets.sync.started_at') },
  { key: 'finished_at', title: t('assets.sync.finished_at') },
  { key: 'duration', title: t('assets.sync.duration') },
  { key: 'account', title: t('assets.account.title'), dataIndex: 'cloud_account_name' },
  { key: 'region', title: t('assets.account.regions'), dataIndex: 'region' },
  { key: 'resource_type', title: t('assets.sync.resource_type') },
  { key: 'status', title: t('assets.sync.status') },
  { key: 'items_seen', title: t('assets.sync.items_seen'), dataIndex: 'items_seen' },
  { key: 'items_softdeleted', title: t('assets.sync.items_softdeleted'), dataIndex: 'items_softdeleted' },
  { key: 'trigger', title: t('assets.sync.trigger') },
  { key: 'error', title: t('assets.sync.error') },
])
const table = useTable<SyncRunSummary, SyncRunListParams>({
  fetcher: async ({ page, pageSize, filters }) => {
    const result = await api.listRuns({ ...filters, page, size: pageSize })
    return { items: result.items, total: result.total }
  },
})

function showLoadError(error: unknown) { message.error(isBizError(error) ? error.message : t('network.error')) }
async function runTableAction(action: () => Promise<unknown>) {
  try { await action() } catch (error) { showLoadError(error) }
}
async function onAccountChange(value: number | null) {
  const validID = value !== null && Number.isSafeInteger(value) && value > 0 ? value : undefined
  if (value !== null && validID === undefined) accountID.value = undefined
  await runTableAction(() => table.setFilters({ account_id: validID }))
}
async function onResourceTypeChange(value: SyncRunResourceType | undefined) {
  await runTableAction(() => table.setFilters({ resource_type: value }))
}
async function onStatusChange(value: SyncRunStatus | undefined) {
  await runTableAction(() => table.setFilters({ status: value }))
}
async function onStartedAfterChange(value: { toDate?: () => Date } | null) {
  const date = value?.toDate?.()
  const startedAfter = date && Number.isFinite(date.getTime()) ? date.toISOString() : undefined
  await runTableAction(() => table.setFilters({ started_after: startedAfter }))
}
async function onPageChange(page: number) { await runTableAction(() => table.setPage(page)) }
async function onPageSizeChange(_: number, size: number) { await runTableAction(() => table.setPageSize(size)) }
function formatTime(value: string) {
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : '—'
}
function statusTagColor(value: SyncRunStatus) { return statusColor[value] }
function formatDuration(run: SyncRunSummary) {
  if (!run.finished_at) return '—'
  const milliseconds = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  return Number.isFinite(milliseconds) && milliseconds >= 0 ? `${(milliseconds / 1000).toFixed(1)}s` : '—'
}

onMounted(() => { void runTableAction(() => table.reload()) })
</script>

<style scoped lang="scss">
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
.error-code {
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: help;
  text-decoration: underline dotted;
}
</style>
