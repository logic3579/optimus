<template>
  <a-card>
    <PageHeader :title="t('assets.resource.subnet.title')">
      <a-button @click="router.back()">{{ t('common.button.back') }}</a-button>
    </PageHeader>
    <div class="filter-row u-mb-16">
      <a-input-search v-model:value="searchInput" allow-clear :placeholder="t('assets.resource.subnet.subnet_id')" style="width: 260px" @search="onSearch" @change="onSearchInputChange" />
      <a-checkbox :checked="includeDeleted" @change="onIncludeDeletedChange">{{ t('assets.resource.common.include_deleted') }}</a-checkbox>
    </div>
    <a-table :columns="columns" :data-source="table.items.value" :loading="table.loading.value" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'deleted'"><a-tag v-if="record.deleted" color="red">{{ t('assets.resource.common.deleted_badge') }}</a-tag></template>
        <template v-else>{{ record[column.dataIndex] ?? '—' }}</template>
      </template>
    </a-table>
    <a-pagination class="u-mt-16" :current="table.page.value" :page-size="table.pageSize.value" :total="table.total.value" show-size-changer @change="onPageChange" @show-size-change="onPageSizeChange" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRoute, useRouter } from 'vue-router'
import type { AssetsResourceApi, SubnetListParams } from '@/api/assets/resource'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import type { SubnetSummary } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const searchInput = ref('')
const includeDeleted = ref(false)
function routeRowID(): number | undefined {
  const raw = route.params.id
  if (typeof raw !== 'string' || !/^[1-9]\d*$/.test(raw)) return undefined
  const id = Number(raw)
  return Number.isSafeInteger(id) ? id : undefined
}
const vpcRowID = routeRowID()
const columns = computed(() => [
  { key: 'subnet_id', title: t('assets.resource.subnet.subnet_id'), dataIndex: 'subnet_id' },
  { key: 'name', title: t('assets.resource.instance.name'), dataIndex: 'name' },
  { key: 'cidr', title: t('assets.resource.subnet.cidr'), dataIndex: 'cidr_block' },
  { key: 'az', title: t('assets.resource.subnet.az'), dataIndex: 'availability_zone' },
  { key: 'vpc_id', title: t('assets.resource.vpc.vpc_id'), dataIndex: 'vpc_id' },
  { key: 'last_seen_at', title: t('assets.resource.common.last_seen_at'), dataIndex: 'last_seen_at' },
  { key: 'deleted', title: '' },
])
const table = useTable<SubnetSummary, SubnetListParams>({ fetcher: async ({ page, pageSize, filters }) => {
  if (vpcRowID === undefined) throw new Error('invalid VPC row ID')
  const result = await api.listSubnets(vpcRowID, { page, size: pageSize, ...filters }); return { items: result.items, total: result.total }
} })
function showLoadError(error: unknown) { message.error(isBizError(error) ? error.message : t('network.error')) }
async function runTableAction(action: () => Promise<unknown>) { try { await action() } catch (error) { showLoadError(error) } }
function clean(value: string) { return value.trim() || undefined }
async function onSearch(value: string) { await runTableAction(() => table.setFilters({ q: clean(value) })) }
async function onSearchInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ q: undefined })) }
async function onIncludeDeletedChange(event: { target?: { checked?: boolean } }) { includeDeleted.value = Boolean(event.target?.checked); await runTableAction(() => table.setFilters({ include_deleted: includeDeleted.value || undefined })) }
async function onPageChange(page: number) { await runTableAction(() => table.setPage(page)) }
async function onPageSizeChange(_: number, size: number) { await runTableAction(() => table.setPageSize(size)) }
onMounted(() => { void runTableAction(() => table.reload()) })
</script>

<style scoped lang="scss">
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
</style>
