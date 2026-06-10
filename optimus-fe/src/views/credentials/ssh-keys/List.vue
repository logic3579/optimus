<template>
  <a-card>
    <PageHeader :title="$t('credentials.ssh_keys.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="$t('credentials.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-input
        v-model:value="usernameInput"
        :placeholder="$t('credentials.filter_username')"
        style="width: 180px;"
        allow-clear
        @press-enter="onUsernameSubmit"
        @change="onUsernameChange"
      />
      <a-button v-permission="'credentials:ssh_key:write'" type="primary" @click="openCreate">
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
            <a v-permission="'credentials:ssh_key:write'" @click="openEdit(record)">
              {{ $t('credentials.action.edit') }}
            </a>
            <a-popconfirm
              :title="$t('credentials.action.confirm_delete')"
              @confirm="remove(record)"
            >
              <a v-permission="'credentials:ssh_key:delete'" class="danger">
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

    <SshKeyEditModal v-model:open="editOpen" :initial="editing" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import SshKeyEditModal from './components/SshKeyEditModal.vue'
import type { SshKeyApi } from '@/api/credentials/ssh-key'
import type { SshKeySummary, SshKeyListQuery } from '@/types/api'

const { t } = useI18n()
const sshKeyApi = inject<SshKeyApi>('sshKeyApi')!

const searchInput = ref('')
const usernameInput = ref('')

const table = useTable<SshKeySummary, SshKeyListQuery>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await sshKeyApi.list({
      page,
      page_size: pageSize,
      q: filters?.q || undefined,
      username: filters?.username || undefined,
    })
    return { items: r.items, total: r.total }
  },
})

const columns = computed(() => [
  { key: 'name',        title: t('credentials.field.name'),         dataIndex: 'name' },
  { key: 'description', title: t('credentials.field.description'),  dataIndex: 'description' },
  { key: 'username',    title: t('credentials.ssh_keys.col_username'), dataIndex: 'username' },
  { key: 'updated_at',  title: 'Updated' },
  { key: 'actions',     title: '', width: 160 },
])

const editOpen = ref(false)
const editing = ref<SshKeySummary | null>(null)

function onSearch(v: string) { table.setFilters({ q: v || undefined }) }
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ q: undefined })
}
function onUsernameSubmit() { table.setFilters({ username: usernameInput.value || undefined }) }
function onUsernameChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ username: undefined })
}

function openCreate() {
  editing.value = null
  editOpen.value = true
}
async function openEdit(r: SshKeySummary) {
  try {
    editing.value = await sshKeyApi.get(r.id)
    editOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function remove(r: SshKeySummary) {
  try {
    await sshKeyApi.remove(r.id)
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
