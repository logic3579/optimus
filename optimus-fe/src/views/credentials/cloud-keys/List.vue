<template>
  <a-card>
    <PageHeader :title="$t('credentials.cloud_keys.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="$t('credentials.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="providerInput"
        style="width: 160px;"
        :options="providerOptions"
        @change="onProviderChange"
      />
      <a-button v-permission="'credentials:cloud_key:write'" type="primary" @click="openCreate">
        {{ $t('credentials.action.create') }}
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
        <template v-if="column.key === 'updated_at'">
          {{ formatTime(record.updated_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'credentials:cloud_key:write'" @click="openEdit(record)">
              {{ $t('credentials.action.edit') }}
            </a>
            <a-popconfirm
              :title="$t('credentials.action.confirm_delete')"
              @confirm="remove(record)"
            >
              <a v-permission="'credentials:cloud_key:delete'" class="danger">
                {{ $t('credentials.action.delete') }}
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

    <CloudKeyEditModal v-model:open="editOpen" :initial="editing" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import CloudKeyEditModal from './components/CloudKeyEditModal.vue'
import type { CloudKeyApi } from '@/api/credentials/cloud-key'
import type { CloudKeySummary, CloudKeyListQuery, CloudProvider } from '@/types/api'

const { t } = useI18n()
const cloudKeyApi = inject<CloudKeyApi>('cloudKeyApi')!

const searchInput = ref('')
const providerInput = ref<'' | CloudProvider>('')

const providerOptions = computed(() => [
  { value: '',      label: t('credentials.filter_provider_all') },
  { value: 'aws',   label: 'AWS' },
  { value: 'gcp',   label: 'GCP' },
  { value: 'azure', label: 'Azure' },
])

const table = useTable<CloudKeySummary, CloudKeyListQuery>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await cloudKeyApi.list({
      page,
      page_size: pageSize,
      q: filters?.q || undefined,
      provider: filters?.provider || undefined,
    })
    return { items: r.items, total: r.total }
  },
})

const columns = computed(() => [
  { key: 'name',        title: t('credentials.field.name'),        dataIndex: 'name' },
  { key: 'description', title: t('credentials.field.description'), dataIndex: 'description' },
  { key: 'provider',    title: t('credentials.cloud_keys.col_provider'), dataIndex: 'provider' },
  { key: 'region',      title: t('credentials.cloud_keys.col_region'),   dataIndex: 'region' },
  { key: 'updated_at',  title: 'Updated' },
  { key: 'actions',     title: '', width: 160 },
])

const editOpen = ref(false)
const editing = ref<CloudKeySummary | null>(null)

function onSearch(v: string) { table.setFilters({ q: v || undefined }) }
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ q: undefined })
}
function onProviderChange(v: '' | CloudProvider) {
  table.setFilters({ provider: v || undefined })
}

function openCreate() {
  editing.value = null
  editOpen.value = true
}
async function openEdit(r: CloudKeySummary) {
  try {
    editing.value = await cloudKeyApi.get(r.id)
    editOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function remove(r: CloudKeySummary) {
  try {
    await cloudKeyApi.remove(r.id)
    message.success(t('credentials.toast.deleted'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onSaved() { table.reload() }

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

onMounted(() => { table.reload() })
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
}
.danger {
  color: var(--ant-color-error, #ff4d4f);
}
</style>
