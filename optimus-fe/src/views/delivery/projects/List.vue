<template>
  <a-card v-if="canRead">
    <PageHeader :title="t('delivery.project.title')">
      <a-button v-permission="'delivery:project:write'" data-testid="create-project" type="primary" @click="openCreate">
        {{ t('common.create') }}
      </a-button>
    </PageHeader>

    <a-input-search
      v-model:value="searchInput"
      allow-clear
      :maxlength="128"
      :placeholder="t('common.search')"
      style="width: 280px"
      @search="onSearch"
      @change="onSearchChange"
    />
    <a-table :columns="columns" :data-source="table.items.value" :loading="table.loading.value" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <a data-testid="project-link" @click="openDetail(record)">{{ record.name }}</a>
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a-button v-permission="'delivery:project:write'" :data-testid="`edit-${record.id}`" type="link" @click="openEdit(record)">
              {{ t('common.edit') }}
            </a-button>
            <a-popconfirm :title="t('confirm.delete_title')" @confirm="remove(record)">
              <a v-permission="'delivery:project:delete'" :data-testid="`delete-${record.id}`" class="danger">{{ t('common.delete') }}</a>
            </a-popconfirm>
          </a-space>
        </template>
        <template v-else>{{ record[column.dataIndex] }}</template>
      </template>
    </a-table>
    <a-pagination
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="onPageChange"
      @show-size-change="onPageSizeChange"
    />

    <a-modal v-model:open="formOpen" :title="editing ? t('common.edit') : t('common.create')" @ok="save" @cancel="formOpen = false">
      <a-form layout="vertical">
        <a-form-item :label="t('delivery.project.name')" required>
          <a-input v-model:value="form.name" :maxlength="128" />
        </a-form-item>
        <a-form-item :label="t('delivery.project.description')">
          <a-textarea v-model:value="form.description" :maxlength="4096" />
        </a-form-item>
        <a-form-item :label="t('delivery.project.owner')">
          <a-input-number v-model:value="form.owner_user_id" :min="1" style="width: 100%" />
        </a-form-item>
      </a-form>
    </a-modal>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import type { DeliveryProjectApi } from '@/api/delivery/project'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { useTable } from '@/hooks/useTable'
import type { DeliveryProjectSummary } from '@/types/delivery'
import { isBizError } from '@/utils/http-error'

const api = inject<DeliveryProjectApi>('deliveryProjectApi')!
const router = useRouter()
const { t } = useI18n()
const permission = usePermission()
const canRead = computed(() => permission.has('delivery:project:read'))
const searchInput = ref('')
const formOpen = ref(false)
const editing = ref<DeliveryProjectSummary | null>(null)
const form = reactive<{ name: string; description: string; owner_user_id?: number }>({ name: '', description: '' })
const columns = computed(() => [
  { key: 'name', title: t('delivery.project.name'), dataIndex: 'name' },
  { key: 'description', title: t('delivery.project.description'), dataIndex: 'description' },
  { key: 'environment_count', title: t('delivery.project.environment_count'), dataIndex: 'environment_count' },
  { key: 'owner_user_id', title: t('delivery.project.owner'), dataIndex: 'owner_user_id' },
  { key: 'actions', title: t('common.actions') },
])
const table = useTable<DeliveryProjectSummary, { q?: string }>({
  fetcher: async ({ page, pageSize, filters }) => {
    const result = await api.list({ page, page_size: pageSize, q: filters?.q || undefined })
    return { items: result.items, total: result.total }
  }
})

function showError(error: unknown) { message.error(isBizError(error) ? error.message : t('network.error')) }
async function action(work: () => Promise<unknown>) { try { await work() } catch (error) { showError(error) } }
function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', description: '', owner_user_id: undefined })
  formOpen.value = true
}
function openEdit(project: DeliveryProjectSummary) {
  editing.value = project
  Object.assign(form, { name: project.name, description: project.description, owner_user_id: project.owner_user_id })
  formOpen.value = true
}
async function save() {
  const name = form.name.trim()
  if (!name) return
  await action(async () => {
    if (editing.value) await api.update(editing.value.id, { name, description: form.description, owner_user_id: form.owner_user_id ?? 0 })
    else await api.create({ name, description: form.description, ...(form.owner_user_id ? { owner_user_id: form.owner_user_id } : {}) })
    formOpen.value = false
    await table.reload()
  })
}
async function remove(project: DeliveryProjectSummary) { await action(async () => { await api.remove(project.id); await table.reload() }) }
function openDetail(project: DeliveryProjectSummary) { void router.push(`/delivery/projects/${project.id}`) }
async function onSearch(value: string) { await action(() => table.setFilters({ q: value.trim() || undefined })) }
async function onSearchChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await action(() => table.setFilters({ q: undefined })) }
async function onPageChange(page: number) { await action(() => table.setPage(page)) }
async function onPageSizeChange(_: number, size: number) { await action(() => table.setPageSize(size)) }

onMounted(() => { if (canRead.value) void action(() => table.reload()) })
defineExpose({ canRead, table, formOpen, editing, form, openCreate, openEdit, save, remove, openDetail })
</script>

<style scoped>
.danger { color: var(--ant-color-error, #ff4d4f); }
</style>
