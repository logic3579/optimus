<template>
  <a-card v-permission="'credentials:http:read'">
    <PageHeader title="HTTP credentials" />
    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="$t('credentials.search_placeholder')"
        allow-clear
        style="width: 280px"
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select v-model:value="authType" :options="authOptions" style="width: 160px" @change="onAuthTypeChange" />
      <a-button v-permission="'credentials:http:write'" type="primary" @click="openCreate">
        {{ $t('credentials.action.create') }}
      </a-button>
    </div>
    <a-alert v-if="errorMessage" class="u-mb-16" type="error" :message="errorMessage" show-icon />
    <a-table :columns="columns" :data-source="table.items.value" :loading="table.loading.value" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'auth_type'">{{ record.auth_type === 'basic' ? 'Basic' : 'Bearer' }}</template>
        <template v-else-if="column.key === 'username'">{{ record.auth_type === 'basic' ? record.username || '—' : '—' }}</template>
        <template v-else-if="column.key === 'updated_at'">{{ formatTime(record.updated_at) }}</template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'credentials:http:write'" @click="openEdit(record)">{{ $t('credentials.action.edit') }}</a>
            <a-popconfirm :title="$t('credentials.action.confirm_delete')" @confirm="remove(record)">
              <a v-permission="'credentials:http:delete'" class="danger">{{ $t('credentials.action.delete') }}</a>
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
    <HttpCredentialEditModal v-model:open="editOpen" :initial="editing" @saved="table.reload" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import PageHeader from '@/components/PageHeader.vue'
import HttpCredentialEditModal from './components/HttpCredentialEditModal.vue'
import type { HTTPCredentialApi, HTTPCredentialListParams, HTTPCredentialSummary, HTTPAuthType } from '@/api/credentials/http-credential'

const api = inject<HTTPCredentialApi>('httpCredentialApi')!
const { t } = useI18n()
const searchInput = ref('')
const authType = ref<'' | HTTPAuthType>('')
const errorMessage = ref('')
const authOptions = computed(() => [
  { value: '', label: 'All' }, { value: 'basic', label: 'Basic' }, { value: 'bearer', label: 'Bearer' },
])
function errorText(error: unknown) {
  if (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string') return error.message
  return t('network.error')
}
const table = useTable<HTTPCredentialSummary, Pick<HTTPCredentialListParams, 'q' | 'auth_type'>>({
  fetcher: async ({ page, pageSize, filters }) => {
    try {
      const result = await api.list({ page, page_size: pageSize, q: filters?.q, auth_type: filters?.auth_type })
      errorMessage.value = ''
      return { items: result.items, total: result.total }
    } catch (error) {
      errorMessage.value = errorText(error)
      throw error
    }
  },
})
const columns = computed(() => [
  { key: 'name', title: t('credentials.field.name'), dataIndex: 'name' },
  { key: 'auth_type', title: 'Authentication type' },
  { key: 'username', title: t('credentials.field.username') },
  { key: 'updated_at', title: 'Updated' },
  { key: 'actions', title: '', width: 160 },
])
const editOpen = ref(false)
const editing = ref<HTTPCredentialSummary | null>(null)
function onSearch(value: string) { void table.setFilters({ q: value || undefined }) }
function onSearchInputChange(event: Event) {
  if ((event.target as HTMLInputElement | null)?.value === '') void table.setFilters({ q: undefined })
}
function onAuthTypeChange(value: '' | HTTPAuthType) { void table.setFilters({ auth_type: value || undefined }) }
function openCreate() { editing.value = null; editOpen.value = true }
async function openEdit(row: HTTPCredentialSummary) {
  try { editing.value = await api.get(row.id); editOpen.value = true; errorMessage.value = '' }
  catch (error) { errorMessage.value = errorText(error) }
}
async function remove(row: HTTPCredentialSummary) {
  try { await api.remove(row.id); message.success(t('credentials.toast.deleted')); await table.reload() }
  catch (error) { errorMessage.value = errorText(error) }
}
function formatTime(value: string) { const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : date.toLocaleString() }
onMounted(() => { void table.reload().catch(() => undefined) })
defineExpose({ table, errorMessage, remove, openEdit })
</script>

<style scoped lang="scss">
.filter-row { display: flex; gap: 12px; align-items: center; }
.danger { color: var(--ant-color-error, #ff4d4f); }
</style>
