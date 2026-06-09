<template>
  <a-card>
    <PageHeader :title="$t('system.roles.title')" />

    <div class="filter-row u-mb-16">
      <a-button v-permission="'system:role:write'" type="primary" @click="openCreate">
        {{ $t('system.roles.create') }}
      </a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="roles"
      :loading="loading"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'is_builtin'">
          {{ record.is_builtin ? $t('system.roles.builtin_yes') : $t('system.roles.builtin_no') }}
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'system:role:write'" @click="openEdit(record)">{{ $t('system.roles.action_edit') }}</a>
            <a v-permission="'system:role:write'" @click="openPermissions(record)">{{ $t('system.roles.action_permissions') }}</a>
            <a-popconfirm
              :title="$t('confirm.delete_title')"
              :description="$t('confirm.delete_desc')"
              :disabled="record.is_builtin"
              @confirm="remove(record)"
            >
              <a v-permission="'system:role:delete'" :class="{ 'a-disabled': record.is_builtin }">
                {{ $t('system.roles.action_delete') }}
              </a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <RoleEditModal v-model:open="editOpen" :initial="editing" @saved="reload" />
    <RolePermissionsModal v-model:open="permOpen" :role="permRole" @saved="reload" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import RoleEditModal from './components/RoleEditModal.vue'
import RolePermissionsModal from './components/RolePermissionsModal.vue'
import type { RoleApi } from '@/api/role'
import type { RoleSummary } from '@/types/api'

const { t } = useI18n()
const roleApi = inject<RoleApi>('roleApi')!

const roles = ref<RoleSummary[]>([])
const loading = ref(false)

const editOpen = ref(false)
const editing = ref<RoleSummary | null>(null)
const permOpen = ref(false)
const permRole = ref<RoleSummary | null>(null)

const columns = computed(() => [
  { key: 'code',        title: t('system.roles.col_code'),        dataIndex: 'code' },
  { key: 'name',        title: t('system.roles.col_name'),        dataIndex: 'name' },
  { key: 'description', title: t('system.roles.col_description'),dataIndex: 'description' },
  { key: 'is_builtin',  title: t('system.roles.col_is_builtin') },
  { key: 'created_at',  title: t('system.roles.col_created_at') },
  { key: 'actions',     title: t('system.roles.col_actions'), width: 240 }
])

async function reload() {
  loading.value = true
  try {
    roles.value = await roleApi.list()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  editOpen.value = true
}
function openEdit(r: RoleSummary) {
  editing.value = r
  editOpen.value = true
}
function openPermissions(r: RoleSummary) {
  permRole.value = r
  permOpen.value = true
}

async function remove(r: RoleSummary) {
  if (r.is_builtin) return
  try {
    await roleApi.remove(r.id)
    message.success(t('system.roles.delete_ok'))
    await reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

onMounted(reload)
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
