<template>
  <a-card>
    <PageHeader :title="t('assets.resource.database.title')" />
    <div class="filter-row u-mb-16">
      <a-input-search v-model:value="searchInput" allow-clear :placeholder="t('assets.resource.database.db_instance_id')" style="width: 220px" @search="onSearch" @change="onSearchInputChange" />
      <a-select v-if="canReadAccounts" v-model:value="accountID" data-testid="account" allow-clear show-search option-filter-prop="label" :placeholder="t('assets.account.title')" :options="accountOptions" :loading="loadingAccounts" style="width: 150px" @change="onAccountChange" />
      <a-input-number v-else v-model:value="accountID" data-testid="account-id" :min="1" :precision="0" :placeholder="t('assets.account.title')" style="width: 150px" @change="onAccountChange" />
      <a-input v-model:value="regionInput" data-testid="region" allow-clear :placeholder="t('assets.account.regions')" style="width: 150px" @press-enter="onRegionSubmit" @change="onRegionInputChange" />
      <a-input v-model:value="engineInput" data-testid="engine" allow-clear :placeholder="t('assets.resource.database.engine')" style="width: 140px" @press-enter="onEngineSubmit" @change="onEngineInputChange" />
      <a-input v-model:value="statusInput" data-testid="status" allow-clear :placeholder="t('assets.resource.database.status')" style="width: 150px" @press-enter="onStatusSubmit" @change="onStatusInputChange" />
      <a-checkbox :checked="includeDeleted" @change="onIncludeDeletedChange">{{ t('assets.resource.common.include_deleted') }}</a-checkbox>
    </div>
    <a-table :columns="columns" :data-source="table.items.value" :loading="table.loading.value" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'engine'">{{ record.engine }} {{ record.engine_version }}</template>
        <template v-else-if="column.key === 'status'"><a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag></template>
        <template v-else-if="column.key === 'multi_az' || column.key === 'publicly_accessible'"><a-tag :color="record[column.key] ? 'green' : 'default'">{{ record[column.key] ? '✓' : '—' }}</a-tag></template>
        <template v-else-if="column.key === 'deleted'"><a-tag v-if="record.deleted" color="red">{{ t('assets.resource.common.deleted_badge') }}</a-tag></template>
        <template v-else>{{ record[column.dataIndex] ?? '—' }}</template>
      </template>
    </a-table>
    <a-pagination class="u-mt-16" :current="table.page.value" :page-size="table.pageSize.value" :total="table.total.value" show-size-changer @change="onPageChange" @show-size-change="onPageSizeChange" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { AssetsAccountApi } from '@/api/assets/account'
import type { AssetsResourceApi, DatabaseListParams } from '@/api/assets/resource'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { useTable } from '@/hooks/useTable'
import type { DatabaseSummary } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const { t } = useI18n()
const { has } = usePermission()
const canReadAccounts = has('assets:account:read')
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const searchInput = ref('')
const accountID = ref<number>()
const regionInput = ref('')
const engineInput = ref('')
const statusInput = ref('')
const includeDeleted = ref(false)
const accountOptions = ref<Array<{ label: string; value: number }>>([])
const loadingAccounts = ref(false)
const columns = computed(() => [
  { key: 'account', title: t('assets.account.title'), dataIndex: 'cloud_account_name' },
  { key: 'region', title: t('assets.account.regions'), dataIndex: 'region' },
  { key: 'db_instance_id', title: t('assets.resource.database.db_instance_id'), dataIndex: 'db_instance_id' },
  { key: 'engine', title: t('assets.resource.database.engine') },
  { key: 'class', title: t('assets.resource.database.class'), dataIndex: 'instance_class' },
  { key: 'status', title: t('assets.resource.database.status') },
  { key: 'endpoint', title: t('assets.resource.database.endpoint'), dataIndex: 'endpoint' },
  { key: 'port', title: t('assets.resource.database.port'), dataIndex: 'port' },
  { key: 'multi_az', title: t('assets.resource.database.multi_az') },
  { key: 'publicly_accessible', title: t('assets.resource.database.publicly_accessible') },
  { key: 'storage_gb', title: t('assets.resource.database.storage_gb'), dataIndex: 'storage_gb' },
  { key: 'deleted', title: '' },
])
const table = useTable<DatabaseSummary, DatabaseListParams>({ fetcher: async ({ page, pageSize, filters }) => {
  const result = await api.listDatabases({ page, size: pageSize, ...filters }); return { items: result.items, total: result.total }
} })
function showLoadError(error: unknown) { message.error(isBizError(error) ? error.message : t('network.error')) }
async function runTableAction(action: () => Promise<unknown>) { try { await action() } catch (error) { showLoadError(error) } }
function clean(value: string) { return value.trim() || undefined }
async function onSearch(value: string) { await runTableAction(() => table.setFilters({ q: clean(value) })) }
async function onSearchInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ q: undefined })) }
async function onAccountChange(value: number | null | undefined) { await runTableAction(() => table.setFilters({ account_id: value ?? undefined })) }
async function onRegionSubmit() { await runTableAction(() => table.setFilters({ region: clean(regionInput.value) })) }
async function onRegionInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ region: undefined })) }
async function onEngineSubmit() { await runTableAction(() => table.setFilters({ engine: clean(engineInput.value) })) }
async function onEngineInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ engine: undefined })) }
async function onStatusSubmit() { await runTableAction(() => table.setFilters({ status: clean(statusInput.value) })) }
async function onStatusInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ status: undefined })) }
async function onIncludeDeletedChange(event: { target?: { checked?: boolean } }) { includeDeleted.value = Boolean(event.target?.checked); await runTableAction(() => table.setFilters({ include_deleted: includeDeleted.value || undefined })) }
async function onPageChange(page: number) { await runTableAction(() => table.setPage(page)) }
async function onPageSizeChange(_: number, size: number) { await runTableAction(() => table.setPageSize(size)) }
function statusColor(status: string) { return ({ available: 'green', creating: 'blue', deleting: 'red', failed: 'red', stopped: 'default', stopping: 'orange' } as Record<string, string>)[status] ?? 'gold' }
async function loadAccounts() {
  loadingAccounts.value = true
  try {
    const accounts: Array<{ id: number; name: string }> = []
    let page = 1
    let total = 0
    do {
      const result = await accountApi.list({ page, size: 100 })
      accounts.push(...result.items)
      total = result.total
      page += 1
      if (result.items.length === 0) break
    } while (accounts.length < total)
    accountOptions.value = accounts.map(account => ({ label: account.name, value: account.id }))
  } catch (error) { showLoadError(error) } finally { loadingAccounts.value = false }
}
onMounted(() => {
  if (canReadAccounts) void loadAccounts()
  void runTableAction(() => table.reload())
})
</script>

<style scoped lang="scss">
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
</style>
