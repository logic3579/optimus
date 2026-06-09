<template>
  <a-card>
    <PageHeader :title="$t('system.audit_logs.title')" />

    <div class="filter-row u-mb-16">
      <a-input
        v-model:value="actionInput"
        :placeholder="$t('system.audit_logs.filter_action')"
        style="width: 200px;"
        allow-clear
      />
      <a-input-number
        v-model:value="userIdInput"
        :placeholder="$t('system.audit_logs.filter_user_id')"
        style="width: 140px;"
      />
      <a-range-picker
        v-model:value="rangeInput"
        show-time
        :placeholder="[$t('system.audit_logs.filter_range'), '']"
      />
      <a-button type="primary" @click="applyFilters">{{ $t('system.audit_logs.filter_search') }}</a-button>
      <a-button @click="resetFilters">{{ $t('system.audit_logs.filter_reset') }}</a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      :expanded-row-render="expandedRowRender"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
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
import { computed, h, inject, onMounted, ref } from 'vue'
import type { Dayjs } from 'dayjs'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import PageHeader from '@/components/PageHeader.vue'
import type { AuditApi } from '@/api/audit'
import type { AuditLogEntry, AuditListQuery } from '@/types/api'

const { t } = useI18n()
const auditApi = inject<AuditApi>('auditApi')!

const actionInput = ref('')
const userIdInput = ref<number | null>(null)
const rangeInput = ref<[Dayjs, Dayjs] | null>(null)

type AuditFilters = AuditListQuery

const table = useTable<AuditLogEntry, AuditFilters>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await auditApi.list({
      page,
      page_size: pageSize,
      action: filters?.action || undefined,
      user_id: filters?.user_id,
      start: filters?.start,
      end: filters?.end
    })
    return { items: r.items, total: r.total }
  }
})

const columns = computed(() => [
  { key: 'created_at',  title: t('system.audit_logs.col_created_at'), width: 180 },
  { key: 'action',      title: t('system.audit_logs.col_action'),     dataIndex: 'action' },
  { key: 'user_id',     title: t('system.audit_logs.col_user_id'),    dataIndex: 'user_id', width: 100 },
  { key: 'target_type', title: t('system.audit_logs.col_target_type'),dataIndex: 'target_type', width: 140 },
  { key: 'target_id',   title: t('system.audit_logs.col_target_id'),  dataIndex: 'target_id', width: 140 },
  { key: 'ip',          title: t('system.audit_logs.col_ip'),         dataIndex: 'ip', width: 140 }
])

function expandedRowRender({ record }: { record: AuditLogEntry }) {
  const payload = record.payload === null || record.payload === undefined
    ? ''
    : JSON.stringify(record.payload, null, 2)
  return h('pre', {
    style: 'margin: 0; font-family: monospace; max-height: 400px; overflow: auto; background: #fafafa; padding: 12px; border-radius: 4px;'
  }, payload)
}

function applyFilters() {
  table.setFilters({
    action: actionInput.value || undefined,
    user_id: userIdInput.value ?? undefined,
    start: rangeInput.value?.[0] ? rangeInput.value[0].toISOString() : undefined,
    end: rangeInput.value?.[1] ? rangeInput.value[1].toISOString() : undefined
  })
}
function resetFilters() {
  actionInput.value = ''
  userIdInput.value = null
  rangeInput.value = null
  table.setFilters({ action: undefined, user_id: undefined, start: undefined, end: undefined })
}

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

onMounted(() => { table.reload() })
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}
</style>
