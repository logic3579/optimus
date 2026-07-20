<template>
  <a-card v-if="canRead">
    <PageHeader title="Data sources" />
    <a-button v-if="canWrite" type="primary" @click="openCreate">Create</a-button>
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon />
    <a-alert v-if="testError" type="error" :message="testError" show-icon />
    <div v-if="testResult" data-testid="test-result">{{ testResult.reachable ? 'Reachable' : 'Unreachable' }}<span v-if="testResult.version"> · {{ testResult.version }}</span></div>
    <a-table :columns="columns" :data-source="table.items.value" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'cluster'">{{ record.cluster?.name || '—' }}</template>
        <template v-else-if="column.key === 'tls'">{{ record.tls_skip_verify ? 'Danger' : '—' }}</template>
        <template v-else-if="column.key === 'updated_at'">{{ record.updated_at }}</template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-if="canWrite" @click="openEdit(record)">Edit</a>
            <a v-if="canTest" @click="testDatasource(record)">Test</a>
            <a-popconfirm v-if="canDelete" title="Confirm deletion" @confirm="remove(record)"><a>Delete</a></a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>
    <a-pagination :current="table.page.value" :page-size="table.pageSize.value" :total="table.total.value" @change="table.setPage" />
    <a-modal v-model:open="formOpen" title="Data source" @ok="save">
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
const canTest = canWrite
const errorMessage = ref(''); const testError = ref(''); const testResult = ref<{ reachable: boolean; version?: string }>()
function localized(error: unknown) { if (typeof error === 'object' && error && 'message_key' in error && typeof error.message_key === 'string') return t(error.message_key); return t('network.error') }
const table = useTable<DatasourceSummary, { q?: string }>({ fetcher: async ({ page, pageSize, filters }) => { const result = await api.list({ page, page_size: pageSize, q: filters?.q }); return { items: result.items, total: result.total } } })
const columns = [{ key: 'name', title: 'Name', dataIndex: 'name' }, { key: 'base_url', title: 'Base URL', dataIndex: 'base_url' }, { key: 'auth_type', title: 'Auth', dataIndex: 'auth_type' }, { key: 'cluster', title: 'Cluster' }, { key: 'tls', title: 'TLS warning' }, { key: 'updated_at', title: 'Updated' }, { key: 'actions', title: 'Actions' }]
const credentials = ref<HTTPCredentialSummary[]>([]); const clusters = ref<NamedRef[]>([])
const formOpen = ref(false); const editing = ref<DatasourceDetail | null>(null); const formRef = ref<{ payload(): SaveDatasource }>()
function openCreate() { editing.value = null; formOpen.value = true }
async function openEdit(row: DatasourceSummary) { editing.value = await api.get(row.id); formOpen.value = true }
async function save() { try { const body = formRef.value!.payload(); if (editing.value) await api.update(editing.value.id, body); else await api.create(body); formOpen.value = false; await table.reload() } catch (error) { errorMessage.value = localized(error) } }
async function remove(row: Pick<DatasourceSummary, 'id'>) { try { await api.remove(row.id); message.success(t('observability.datasource.deleted')); errorMessage.value = ''; await table.reload() } catch (error) { errorMessage.value = localized(error) } }
async function testDatasource(row: Pick<DatasourceSummary, 'id'>) { testError.value = ''; testResult.value = undefined; try { const value = await api.test(row.id); testResult.value = { reachable: value.reachable, ...(value.version ? { version: value.version } : {}) } } catch (error) { testError.value = localized(error) } }
onMounted(async () => { void table.reload().catch(error => { errorMessage.value = localized(error) }); const [credentialPage, clusterPage] = await Promise.all([credentialApi.list({ page: 1, page_size: 100 }), clusterApi.list({ page: 1, page_size: 100 })]); credentials.value = credentialPage.items; clusters.value = clusterPage.items.map(item => ({ id: item.id, name: item.name })) })
defineExpose({ auth, table, canRead, canWrite, canDelete, canTest, errorMessage, testError, testResult, remove, testDatasource })
</script>
