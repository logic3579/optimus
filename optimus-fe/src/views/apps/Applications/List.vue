<template>
  <a-card>
    <PageHeader :title="t('apps.application.list.title')">
      <a-button
        v-permission="'apps:application:write'"
        type="primary"
        @click="goInstall"
      >
        {{ t('apps.application.list.action.installNew') }}
      </a-button>
    </PageHeader>

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="t('apps.application.list.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="clusterFilter"
        :placeholder="t('apps.application.list.col.cluster')"
        :options="clusterOptions"
        :loading="loadingClusters"
        style="width: 200px;"
        allow-clear
        show-search
        option-filter-prop="label"
        @change="onClusterChange"
      />
      <a-input
        v-model:value="namespaceInput"
        :placeholder="t('apps.application.list.col.namespace')"
        style="width: 200px;"
        allow-clear
        @press-enter="onNamespaceSubmit"
        @change="onNamespaceChange"
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
        <template v-if="column.key === 'name'">
          <a @click="goDetail(record)">{{ record.name }}</a>
        </template>
        <template v-else-if="column.key === 'tags'">
          <a-tag v-for="tg in record.tags ?? []" :key="tg">{{ tg }}</a-tag>
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'apps:application:read'" @click="goDetail(record)">
              {{ t('apps.application.list.action.detail') }}
            </a>
            <a v-permission="'apps:release:upgrade'" @click="goUpgrade(record)">
              {{ t('apps.application.list.action.upgrade') }}
            </a>
            <a-popconfirm
              :title="t('apps.application.list.confirm.uninstall')"
              @confirm="uninstall(record)"
            >
              <a v-permission="'apps:release:uninstall'" class="danger">
                {{ t('apps.application.list.action.uninstall') }}
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
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { useAppsStore } from '@/stores/apps'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import type { AppsApplicationApi } from '@/api/apps/application'
import type { AppsReleaseApi } from '@/api/apps/release'
import type { ClusterApi } from '@/api/k8s/cluster'
import type { ApplicationSummary, ApplicationListQuery } from '@/types/apps'

/**
 * Applications List — P3 §8.5.
 *
 * Filters: name search, cluster (from useAppsStore filterClusterId), namespace
 * (session-only from store.filterNamespace). Selecting a cluster filter writes
 * back into the store so navigating away and back preserves the user's choice.
 * Per spec §8.5 no list data is cached in the store — every reload re-queries
 * the BE which itself proxies the live release status.
 */

const { t } = useI18n()
const router = useRouter()
const appApi = inject<AppsApplicationApi>('appsApplicationApi')!
const relApi = inject<AppsReleaseApi>('appsReleaseApi')!
const clusterApi = inject<ClusterApi>('clusterApi')!
const appsStore = useAppsStore()

type Filters = Pick<ApplicationListQuery, 'name' | 'cluster_id' | 'namespace'>

const searchInput = ref('')
const clusterFilter = ref<number | undefined>(appsStore.filterClusterId ?? undefined)
const namespaceInput = ref<string>(appsStore.filterNamespace)

const clusterOptions = ref<Array<{ label: string; value: number }>>([])
const loadingClusters = ref(false)

const table = useTable<ApplicationSummary, Filters>({
  defaultFilters: {
    cluster_id: appsStore.filterClusterId ?? undefined,
    namespace: appsStore.filterNamespace || undefined,
  },
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await appApi.list({
      page,
      page_size: pageSize,
      name: filters?.name || undefined,
      cluster_id: filters?.cluster_id,
      namespace: filters?.namespace || undefined,
    })
    return { items: r.items, total: r.total }
  },
})

const columns = computed(() => [
  { key: 'name',          title: t('apps.application.list.col.name') },
  { key: 'cluster',       title: t('apps.application.list.col.cluster'),     dataIndex: 'cluster_name' },
  { key: 'namespace',     title: t('apps.application.list.col.namespace'),   dataIndex: 'namespace' },
  { key: 'release',       title: t('apps.application.list.col.release'),     dataIndex: 'release_name' },
  { key: 'chart',         title: t('apps.application.list.col.chart'),       dataIndex: 'chart_name' },
  { key: 'owner',         title: t('apps.application.list.col.owner'),       dataIndex: 'owner_name' },
  { key: 'tags',          title: t('apps.application.field.tags') },
  { key: 'actions',       title: t('apps.application.list.col.actions'),     width: 220 },
])

function onSearch(v: string) {
  table.setFilters({ name: v || undefined })
}
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ name: undefined })
}
function onClusterChange(v: unknown) {
  const id = (v as number | undefined) ?? null
  appsStore.setClusterFilter(id)
  // setClusterFilter also clears namespace, mirror that in the local input.
  if (id === null || id !== appsStore.filterClusterId) {
    namespaceInput.value = ''
  }
  table.setFilters({ cluster_id: id ?? undefined, namespace: appsStore.filterNamespace || undefined })
}
function onNamespaceSubmit() {
  appsStore.setNamespaceFilter(namespaceInput.value)
  table.setFilters({ namespace: namespaceInput.value || undefined })
}
function onNamespaceChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') {
    appsStore.setNamespaceFilter('')
    table.setFilters({ namespace: undefined })
  }
}

function goInstall() {
  void router.push('/apps/applications/new')
}
function goDetail(r: ApplicationSummary) {
  void router.push(`/apps/applications/${r.id}`)
}
function goUpgrade(r: ApplicationSummary) {
  void router.push(`/apps/applications/${r.id}/upgrade`)
}

async function uninstall(r: ApplicationSummary) {
  try {
    await relApi.uninstall(r.id, {})
    message.success(t('common.message.done'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

onMounted(async () => {
  loadingClusters.value = true
  try {
    const r = await clusterApi.list({ page_size: 200 })
    clusterOptions.value = r.items.map((c) => ({ label: c.name, value: c.id }))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loadingClusters.value = false
  }
  await table.reload()
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
