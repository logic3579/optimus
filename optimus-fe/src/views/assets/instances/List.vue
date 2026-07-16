<template>
  <a-card>
    <PageHeader :title="t('assets.resource.instance.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        allow-clear
        :placeholder="t('assets.resource.instance.name')"
        style="width: 220px"
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-if="canReadAccounts"
        v-model:value="accountID"
        data-testid="account"
        allow-clear
        show-search
        option-filter-prop="label"
        :placeholder="t('assets.account.title')"
        :options="accountOptions"
        :loading="loadingAccounts"
        style="width: 150px"
        @change="onAccountChange"
      />
      <a-input-number
        v-else
        v-model:value="accountID"
        data-testid="account-id"
        :min="1"
        :precision="0"
        :placeholder="t('assets.account.title')"
        style="width: 150px"
        @change="onAccountChange"
      />
      <a-input
        v-model:value="regionInput"
        data-testid="region"
        allow-clear
        :placeholder="t('assets.account.regions')"
        style="width: 150px"
        @press-enter="onRegionSubmit"
        @change="onRegionInputChange"
      />
      <a-select
        v-model:value="stateFilter"
        data-testid="state"
        allow-clear
        :placeholder="t('assets.resource.instance.state')"
        :options="stateOptions"
        style="width: 150px"
        @change="onStateChange"
      />
      <a-input
        v-model:value="vpcInput"
        data-testid="vpc"
        allow-clear
        :placeholder="t('assets.resource.instance.vpc')"
        style="width: 170px"
        @press-enter="onVPCSubmit"
        @change="onVPCInputChange"
      />
      <a-checkbox :checked="includeDeleted" @change="onIncludeDeletedChange">
        {{ t('assets.resource.common.include_deleted') }}
      </a-checkbox>
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'state'">
          <a-tag :color="stateColor(record.state)">{{ record.state }}</a-tag>
        </template>
        <template v-else-if="column.key === 'tags'">{{ formatTags(record.tags) }}</template>
        <template v-else-if="column.key === 'deleted'">
          <a-tag v-if="record.deleted" color="red">{{ t('assets.resource.common.deleted_badge') }}</a-tag>
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

import type { AssetsAccountApi } from '@/api/assets/account'
import type { AssetsResourceApi, InstanceListParams } from '@/api/assets/resource'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { useTable } from '@/hooks/useTable'
import type { InstanceSummary } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const { t } = useI18n()
const { has } = usePermission()
const canReadAccounts = has('assets:account:read')
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const searchInput = ref('')
const accountID = ref<number>()
const regionInput = ref('')
const stateFilter = ref<string>()
const vpcInput = ref('')
const includeDeleted = ref(false)
const accountOptions = ref<Array<{ label: string; value: number }>>([])
const loadingAccounts = ref(false)

const stateOptions = ['pending', 'running', 'stopping', 'stopped', 'terminated'].map(value => ({ value, label: value }))
const columns = computed(() => [
  { key: 'account', title: t('assets.account.title'), dataIndex: 'cloud_account_name' },
  { key: 'region', title: t('assets.account.regions'), dataIndex: 'region' },
  { key: 'instance_id', title: t('assets.resource.instance.instance_id'), dataIndex: 'instance_id' },
  { key: 'name', title: t('assets.resource.instance.name'), dataIndex: 'name' },
  { key: 'type', title: t('assets.resource.instance.type'), dataIndex: 'instance_type' },
  { key: 'state', title: t('assets.resource.instance.state') },
  { key: 'private_ip', title: t('assets.resource.instance.private_ip'), dataIndex: 'private_ip' },
  { key: 'public_ip', title: t('assets.resource.instance.public_ip'), dataIndex: 'public_ip' },
  { key: 'vpc', title: t('assets.resource.instance.vpc'), dataIndex: 'vpc_id' },
  { key: 'az', title: t('assets.resource.instance.az'), dataIndex: 'availability_zone' },
  { key: 'tags', title: t('assets.resource.common.tags') },
  { key: 'deleted', title: '' },
])
const table = useTable<InstanceSummary, InstanceListParams>({
  fetcher: async ({ page, pageSize, filters }) => {
    const result = await api.listInstances({ page, size: pageSize, ...filters })
    return { items: result.items, total: result.total }
  },
})

function showLoadError(error: unknown) { message.error(isBizError(error) ? error.message : t('network.error')) }
async function runTableAction(action: () => Promise<unknown>) { try { await action() } catch (error) { showLoadError(error) } }
function clean(value: string) { return value.trim() || undefined }
async function onSearch(value: string) { await runTableAction(() => table.setFilters({ q: clean(value) })) }
async function onSearchInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ q: undefined })) }
async function onAccountChange(value: number | null | undefined) { await runTableAction(() => table.setFilters({ account_id: value ?? undefined })) }
async function onRegionSubmit() { await runTableAction(() => table.setFilters({ region: clean(regionInput.value) })) }
async function onRegionInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ region: undefined })) }
async function onStateChange(value: string | undefined) { await runTableAction(() => table.setFilters({ state: value })) }
async function onVPCSubmit() { await runTableAction(() => table.setFilters({ vpc_id: clean(vpcInput.value) })) }
async function onVPCInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ vpc_id: undefined })) }
async function onIncludeDeletedChange(event: { target?: { checked?: boolean } }) { includeDeleted.value = Boolean(event.target?.checked); await runTableAction(() => table.setFilters({ include_deleted: includeDeleted.value || undefined })) }
async function onPageChange(page: number) { await runTableAction(() => table.setPage(page)) }
async function onPageSizeChange(_: number, size: number) { await runTableAction(() => table.setPageSize(size)) }
function stateColor(state: string) { return ({ running: 'green', stopped: 'default', pending: 'gold', terminated: 'red', stopping: 'orange' } as Record<string, string>)[state] ?? 'default' }
function formatTags(tags: Record<string, string> | undefined) {
  return tags && Object.keys(tags).length > 0
    ? Object.entries(tags).map(([key, value]) => `${key}=${value}`).join(', ')
    : '—'
}

async function loadAccounts() {
  loadingAccounts.value = true
  try {
    const result = await accountApi.list({ page: 1, size: 100 })
    accountOptions.value = result.items.map(account => ({ label: account.name, value: account.id }))
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
