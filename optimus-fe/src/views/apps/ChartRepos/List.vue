<template>
  <a-card>
    <PageHeader :title="t('apps.repo.list.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="t('apps.repo.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="typeFilter"
        :placeholder="t('apps.repo.form.field.type')"
        style="width: 140px;"
        allow-clear
        :options="typeOptions"
        @change="onTypeChange"
      />
      <a-button v-permission="'apps:repo:write'" type="primary" @click="openCreate">
        {{ t('apps.repo.action.create') }}
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
        <template v-if="column.key === 'type'">
          <a-tag :color="record.type === 'oci' ? 'purple' : 'blue'">{{ record.type.toUpperCase() }}</a-tag>
        </template>
        <template v-else-if="column.key === 'has_password'">
          <a-tag :color="record.has_password ? 'green' : 'default'">
            {{ record.has_password ? t('apps.repo.form.field.hasPassword') : '—' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'updated_at'">
          {{ formatTime(record.updated_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'apps:repo:write'" @click="openEdit(record)">
              {{ t('apps.repo.action.edit') }}
            </a>
            <a-popconfirm
              :title="t('apps.repo.action.confirm_delete')"
              @confirm="remove(record)"
            >
              <a v-permission="'apps:repo:delete'" class="danger">
                {{ t('apps.repo.action.delete') }}
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

    <Form v-model:open="formOpen" :original="formOriginal" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import Form from './Form.vue'
import type { AppsRepoApi } from '@/api/apps/repo'
import type {
  ChartRepoSummary, ChartRepoListQuery,
} from '@/types/apps'

const { t } = useI18n()
const repoApi = inject<AppsRepoApi>('appsRepoApi')!

const searchInput = ref('')
const typeFilter = ref<'oci' | 'http' | undefined>(undefined)

const typeOptions = [
  { label: 'HTTP', value: 'http' },
  { label: 'OCI', value: 'oci' },
]

const table = useTable<ChartRepoSummary, ChartRepoListQuery>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await repoApi.list({
      page,
      page_size: pageSize,
      name: filters?.name || undefined,
      type: filters?.type,
    })
    return { items: r.items, total: r.total }
  },
})

const columns = computed(() => [
  { key: 'name',         title: t('apps.repo.field.name'),        dataIndex: 'name' },
  { key: 'type',         title: t('apps.repo.form.field.type') },
  { key: 'url',          title: t('apps.repo.form.field.url'),    dataIndex: 'url' },
  { key: 'username',     title: t('apps.repo.form.field.username'), dataIndex: 'username' },
  { key: 'has_password', title: t('apps.repo.form.field.hasPassword') },
  { key: 'description',  title: t('apps.repo.field.description'), dataIndex: 'description' },
  { key: 'updated_at',   title: t('apps.repo.field.updated_at') },
  { key: 'actions',      title: t('apps.repo.field.actions'), width: 160 },
])

const formOpen = ref(false)
const formOriginal = ref<ChartRepoSummary | null>(null)

function onSearch(v: string) {
  table.setFilters({ name: v || undefined })
}
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ name: undefined })
}
function onTypeChange(v: unknown) {
  table.setFilters({ type: (v as 'oci' | 'http' | undefined) || undefined })
}

function openCreate() {
  formOriginal.value = null
  formOpen.value = true
}
async function openEdit(r: ChartRepoSummary) {
  try {
    formOriginal.value = await repoApi.get(r.id)
    formOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function remove(r: ChartRepoSummary) {
  try {
    await repoApi.remove(r.id)
    message.success(t('apps.repo.toast.deleted'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onSaved() {
  table.reload()
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

onMounted(() => {
  table.reload()
})
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
