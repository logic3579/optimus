<template>
  <a-card>
    <PageHeader :title="t('assets.account.title')">
      <a-button
        v-permission="'assets:account:write'"
        data-testid="create-account"
        type="primary"
        @click="openCreate"
      >
        {{ t('assets.account.actions.create') }}
      </a-button>
    </PageHeader>

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        allow-clear
        style="width: 280px;"
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="enabledFilter"
        allow-clear
        style="width: 140px;"
        :options="enabledOptions"
        @change="onEnabledChange"
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
        <template v-if="column.key === 'enabled'">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '✓' : '—' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'last_sync_at'">
          {{ record.last_sync_at ? formatTime(record.last_sync_at) : '—' }}
        </template>
        <template v-else-if="column.key === 'last_sync_status'">
          <a-tag v-if="record.last_sync_status" :color="statusColor(record.last_sync_status)">
            {{ t(`assets.sync.run_status.${record.last_sync_status}`) }}
          </a-tag>
          <template v-else>—</template>
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a
              v-permission="'assets:account:write'"
              :data-testid="`sync-${record.id}`"
              :class="{ disabled: store.syncInFlight[record.id] }"
              @click="!store.syncInFlight[record.id] && syncNow(record)"
            >{{ t('assets.account.actions.sync_now') }}</a>
            <a
              v-permission="'assets:account:write'"
              :data-testid="`edit-${record.id}`"
              @click="openEdit(record)"
            >{{ t('assets.account.actions.edit') }}</a>
            <a-popconfirm :title="t('assets.account.delete_confirm', { name: record.name })" @confirm="remove(record)">
              <a
                v-permission="'assets:account:delete'"
                :data-testid="`delete-${record.id}`"
                class="danger"
              >{{ t('assets.account.actions.delete') }}</a>
            </a-popconfirm>
          </a-space>
        </template>
        <template v-else>{{ record[column.dataIndex] }}</template>
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

    <Form :open="formOpen" :editing="editing" @close="formOpen = false" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { AssetsAccountApi, AssetsAccountListParams } from '@/api/assets/account'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { useAssetsStore } from '@/stores/assets'
import type { CloudAccountDetail, CloudAccountSummary } from '@/types/assets'
import { isBizError } from '@/utils/http-error'
import Form from './Form.vue'

const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const { t } = useI18n()
const store = useAssetsStore()
const searchInput = ref('')
const enabledFilter = ref<boolean | undefined>()
const formOpen = ref(false)
const editing = ref<CloudAccountDetail | null>(null)

const enabledOptions = computed(() => [
  { value: true, label: t('assets.account.enabled') },
  { value: false, label: t('system.users.status_disabled') },
])
const table = useTable<CloudAccountSummary, AssetsAccountListParams>({
  fetcher: async ({ page, pageSize, filters }) => {
    const result = await accountApi.list({
      page, size: pageSize, q: filters?.q || undefined, enabled: filters?.enabled,
    })
    return { items: result.items, total: result.total }
  },
})
const columns = computed(() => [
  { key: 'name', title: t('assets.account.name'), dataIndex: 'name' },
  { key: 'cloudkey_name', title: t('assets.account.cloudkey'), dataIndex: 'cloudkey_name' },
  { key: 'regions_count', title: t('assets.account.regions'), dataIndex: 'regions_count' },
  { key: 'enabled', title: t('assets.account.enabled') },
  { key: 'last_sync_at', title: t('assets.account.last_sync_at') },
  { key: 'last_sync_status', title: t('assets.account.last_sync_status') },
  { key: 'actions', title: '', width: 240 },
])

function onSearch(value: string) { void table.setFilters({ q: value || undefined }) }
function onSearchInputChange(event: Event) {
  const target = event.target as HTMLInputElement | null
  if (target?.value === '') void table.setFilters({ q: undefined })
}
function onEnabledChange(value: boolean | undefined) { void table.setFilters({ enabled: value }) }
function openCreate() { editing.value = null; formOpen.value = true }
async function openEdit(account: CloudAccountSummary) {
  try {
    editing.value = await accountApi.get(account.id)
    formOpen.value = true
  } catch (error) {
    message.error(isBizError(error) ? error.message : t('network.error'))
  }
}
async function onSaved() { formOpen.value = false; await table.reload() }

async function syncNow(account: CloudAccountSummary) {
  store.markSyncStarted(account.id)
  try {
    await accountApi.triggerSync(account.id)
    message.success(t('assets.account.sync_queued'))
  } catch (error) {
    message.error(isBizError(error) ? error.message : t('network.error'))
  } finally {
    window.setTimeout(() => store.clearSyncStarted(account.id), 30_000)
  }
}

async function remove(account: CloudAccountSummary) {
  try {
    const result = await accountApi.remove(account.id)
    message.success(t('assets.account.cascaded_resources', { count: result.cascaded_resources_count }))
    await table.reload()
  } catch (error) {
    message.error(isBizError(error) ? error.message : t('network.error'))
  }
}
function statusColor(status: string) {
  return ({ success: 'green', failed: 'red', running: 'blue', skipped: 'gold' } as Record<string, string>)[status] ?? 'default'
}
function formatTime(value: string) { return new Date(value).toLocaleString() }

onMounted(() => { void table.reload() })
</script>

<style scoped lang="scss">
.filter-row { display: flex; gap: 12px; align-items: center; }
.danger { color: var(--ant-color-error, #ff4d4f); }
.disabled { color: rgba(0, 0, 0, 0.25); cursor: not-allowed; }
</style>
