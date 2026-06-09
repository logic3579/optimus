<template>
  <a-card>
    <PageHeader :title="$t('system.users.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="$t('system.users.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="statusInput"
        style="width: 140px;"
        :options="statusOptions"
        @change="onStatusChange"
      />
      <a-button v-permission="'system:user:write'" type="primary" @click="openCreate">
        {{ $t('system.users.create') }}
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
        <template v-if="column.key === 'status'">
          <a-badge
            :status="record.status === 'enabled' ? 'success' : 'default'"
            :text="record.status === 'enabled' ? $t('system.users.status_enabled') : $t('system.users.status_disabled')"
          />
        </template>
        <template v-else-if="column.key === 'last_login_at'">
          {{ record.last_login_at ? formatTime(record.last_login_at) : '—' }}
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'system:user:write'" @click="openEdit(record)">{{ $t('system.users.action_edit') }}</a>
            <a v-permission="'system:user:write'" @click="openRoles(record)">{{ $t('system.users.action_roles') }}</a>
            <a v-permission="'system:user:reset_pass'" @click="openResetPassword(record)">{{ $t('system.users.action_reset_password') }}</a>
            <a-popconfirm
              :title="record.status === 'enabled' ? $t('confirm.disable_user') : $t('confirm.enable_user')"
              @confirm="toggleStatus(record)"
            >
              <a v-permission="'system:user:write'">
                {{ record.status === 'enabled' ? $t('system.users.action_disable') : $t('system.users.action_enable') }}
              </a>
            </a-popconfirm>
            <a-popconfirm
              :title="$t('confirm.delete_title')"
              :description="$t('confirm.delete_desc')"
              @confirm="remove(record)"
            >
              <a v-permission="'system:user:delete'" :class="{ 'a-disabled': record.username === 'admin' }">
                {{ $t('system.users.action_delete') }}
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

    <UserEditModal v-model:open="editOpen" :initial="editingDetail" @saved="onSaved" />
    <UserRolesModal v-model:open="rolesOpen" :user="rolesUser" @saved="onSaved" />
    <UserResetPasswordModal v-model:open="resetOpen" :user="resetUser" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import UserEditModal from './components/UserEditModal.vue'
import UserRolesModal from './components/UserRolesModal.vue'
import UserResetPasswordModal from './components/UserResetPasswordModal.vue'
import type { UserApi } from '@/api/user'
import type { UserSummary, UserDetail, UserListQuery } from '@/types/api'

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!

type UserFilters = UserListQuery

const searchInput = ref('')
const statusInput = ref<'' | 'enabled' | 'disabled'>('')
const statusOptions = computed(() => [
  { value: '', label: t('system.users.filter_status_all') },
  { value: 'enabled', label: t('system.users.status_enabled') },
  { value: 'disabled', label: t('system.users.status_disabled') }
])

const table = useTable<UserSummary, UserFilters>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await userApi.list({
      page,
      page_size: pageSize,
      search: filters?.search || undefined,
      status: filters?.status || undefined
    })
    return { items: r.items, total: r.total }
  }
})

const columns = computed(() => [
  { key: 'username',     title: t('system.users.col_username'),     dataIndex: 'username' },
  { key: 'email',        title: t('system.users.col_email'),        dataIndex: 'email' },
  { key: 'display_name', title: t('system.users.col_display_name'), dataIndex: 'display_name' },
  { key: 'status',       title: t('system.users.col_status') },
  { key: 'last_login_at',title: t('system.users.col_last_login') },
  { key: 'created_at',   title: t('system.users.col_created_at') },
  { key: 'actions',      title: t('system.users.col_actions'), width: 360 }
])

const editOpen = ref(false)
const editingDetail = ref<UserDetail | null>(null)
const rolesOpen = ref(false)
const rolesUser = ref<UserDetail | null>(null)
const resetOpen = ref(false)
const resetUser = ref<UserDetail | null>(null)

function onSearch(v: string) { table.setFilters({ search: v || undefined }) }
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ search: undefined })
}
function onStatusChange(v: '' | 'enabled' | 'disabled') {
  table.setFilters({ status: v || undefined })
}

function openCreate() {
  editingDetail.value = null
  editOpen.value = true
}
async function openEdit(r: UserSummary) {
  try {
    editingDetail.value = await userApi.get(r.id)
    editOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}
async function openRoles(r: UserSummary) {
  try {
    rolesUser.value = await userApi.get(r.id)
    rolesOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}
function openResetPassword(r: UserSummary) {
  resetUser.value = { ...(r as unknown as UserDetail) }
  resetOpen.value = true
}

async function toggleStatus(r: UserSummary) {
  const next = r.status === 'enabled' ? 'disabled' : 'enabled'
  try {
    await userApi.setStatus(r.id, { status: next })
    message.success(t('system.users.status_ok'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function remove(r: UserSummary) {
  if (r.username === 'admin') return
  try {
    await userApi.remove(r.id)
    message.success(t('system.users.delete_ok'))
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
.a-disabled {
  color: #ccc;
  pointer-events: none;
}
</style>
