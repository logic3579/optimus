<template>
  <a-card v-if="canRead">
    <PageHeader :title="t('menu.observability.datasources')" />
    <a-button v-if="canWrite" type="primary" @click="openCreate">{{ t('common.create') }}</a-button>
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon />
    <a-alert v-if="testError" type="error" :message="testError" show-icon />
    <div v-if="testResult" data-testid="test-result">{{ testResult.reachable ? t('observability_ui.test.reachable') : t('observability_ui.test.unreachable') }}<span v-if="testResult.version"> · {{ testResult.version }}</span></div>
    <a-table :columns="columns" :data-source="table.items.value" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'cluster'">{{ record.cluster?.name || '—' }}</template>
        <template v-else-if="column.key === 'tls'">{{ record.tls_skip_verify ? t('observability_ui.tls_danger') : '—' }}</template>
        <template v-else-if="column.key === 'updated_at'">{{ record.updated_at }}</template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-if="canWrite" @click="openEdit(record)">{{ t('common.edit') }}</a>
            <a v-if="canTest" @click="testDatasource(record)">{{ t('common.test') }}</a>
            <a-popconfirm v-if="canDelete" :title="t('confirm.delete_title')" @confirm="remove(record)"><a>{{ t('common.delete') }}</a></a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>
    <a-pagination :current="table.page.value" :page-size="table.pageSize.value" :total="table.total.value" @change="table.setPage" />
    <a-modal v-if="canWrite" v-model:open="formOpen" :title="t('observability_ui.forms.data_source')" @ok="save">
      <DatasourceForm ref="formRef" :initial="editing" :credentials="credentials" :clusters="clusters" />
    </a-modal>
  </a-card>
</template>
<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { useTable } from '@/hooks/useTable'
import PageHeader from '@/components/PageHeader.vue'
import DatasourceForm from './components/DatasourceForm.vue'
import type { ObservabilityDatasourceApi } from '@/api/observability/datasource'
import type { HTTPCredentialApi, HTTPCredentialSummary } from '@/api/credentials/http-credential'
import type { ClusterApi } from '@/api/k8s/cluster'
import type { DatasourceDetail, DatasourceSummary, NamedRef, SaveDatasource } from '@/types/observability'

const api = inject<ObservabilityDatasourceApi>('observabilityDatasourceApi')!
const credentialApi = inject<HTTPCredentialApi>('httpCredentialApi')!
const clusterApi = inject<ClusterApi>('clusterApi')!
const auth = useAuthStore(); const permission = usePermission(); const { t } = useI18n()
const canRead = computed(() => permission.has('observability:datasource:read'))
const canWrite = computed(() => permission.has('observability:datasource:write'))
const canDelete = computed(() => permission.has('observability:datasource:delete'))
const canTest = computed(() => permission.has('observability:datasource:write'))
const canReadCredentials = computed(() => permission.has('credentials:http:read'))
const canReadClusters = computed(() => permission.has('k8s:cluster:read'))
const errorMessage = ref(''); const testError = ref(''); const testResult = ref<{ reachable: boolean; version?: string }>()
function localized(error: unknown) { if (typeof error === 'object' && error && 'message_key' in error && typeof error.message_key === 'string') return t(error.message_key); return t('network.error') }
const table = useTable<DatasourceSummary, { q?: string }>({ fetcher: async ({ page, pageSize, filters }) => { const result = await api.list({ page, page_size: pageSize, q: filters?.q }); return { items: result.items, total: result.total } } })
const columns = computed(() => [{ key: 'name', title: t('observability_ui.forms.name'), dataIndex: 'name' }, { key: 'base_url', title: t('observability_ui.forms.base_url'), dataIndex: 'base_url' }, { key: 'auth_type', title: t('observability_ui.credentials.auth_type'), dataIndex: 'auth_type' }, { key: 'cluster', title: t('observability_ui.forms.cluster') }, { key: 'tls', title: t('observability_ui.forms.tls') }, { key: 'updated_at', title: t('credentials.field.updated_at') }, { key: 'actions', title: t('common.actions') }])
const credentials = ref<HTTPCredentialSummary[]>([]); const clusters = ref<NamedRef[]>([])
const formOpen = ref(false); const editing = ref<DatasourceDetail | null>(null); const formRef = ref<{ payload(): SaveDatasource }>()
async function loadReferences() {
  const tasks: Promise<void>[] = []
  if (canReadCredentials.value) {
    tasks.push(credentialApi.list({ page: 1, page_size: 100 })
      .then(page => { credentials.value = page.items })
      .catch(error => { errorMessage.value = localized(error) }))
  } else {
    credentials.value = []
  }
  if (canReadClusters.value) {
    tasks.push(clusterApi.list({ page: 1, page_size: 100 })
      .then(page => { clusters.value = page.items.map(item => ({ id: item.id, name: item.name })) })
      .catch(error => { errorMessage.value = localized(error) }))
  } else {
    clusters.value = []
  }
  await Promise.all(tasks)
}
async function openCreate() {
  editing.value = null
  await loadReferences()
  formOpen.value = true
}
async function openEdit(row: DatasourceSummary) {
  try {
    const [detail] = await Promise.all([api.get(row.id), loadReferences()])
    editing.value = detail
    formOpen.value = true
  } catch (error) {
    errorMessage.value = localized(error)
  }
}
async function save() { try { const body = formRef.value!.payload(); if (editing.value) await api.update(editing.value.id, body); else await api.create(body); formOpen.value = false; await table.reload() } catch (error) { errorMessage.value = localized(error) } }
async function remove(row: Pick<DatasourceSummary, 'id'>) { try { await api.remove(row.id); message.success(t('observability.datasource.deleted')); errorMessage.value = ''; await table.reload() } catch (error) { errorMessage.value = localized(error) } }
async function testDatasource(row: Pick<DatasourceSummary, 'id'>) { testError.value = ''; testResult.value = undefined; try { const value = await api.test(row.id); testResult.value = { reachable: value.reachable, ...(value.version ? { version: value.version } : {}) } } catch (error) { testError.value = localized(error) } }
onMounted(() => {
  if (canRead.value) void table.reload().catch(error => { errorMessage.value = localized(error) })
})
defineExpose({ auth, table, canRead, canWrite, canDelete, canTest, errorMessage, testError, testResult, remove, testDatasource, openCreate, openEdit })
</script>
