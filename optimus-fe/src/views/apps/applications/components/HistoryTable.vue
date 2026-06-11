<template>
  <a-table
    :columns="columns"
    :data-source="rows"
    :pagination="false"
    row-key="revision"
    size="middle"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'status'">
        <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
      </template>
      <template v-else-if="column.key === 'updated_at'">
        {{ formatTime(record.updated_at) }}
      </template>
      <template v-else-if="column.key === 'actions'">
        <a-popconfirm
          :title="t('apps.application.detail.confirm.rollback')"
          @confirm="emit('rollback', record.revision)"
        >
          <a-button
            v-permission="'apps:release:rollback'"
            size="small"
            :disabled="record.revision === currentRevision"
          >
            {{ t('apps.application.detail.btn.rollback') }}
          </a-button>
        </a-popconfirm>
      </template>
    </template>
  </a-table>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from '@/hooks/useI18n'
import type { RevisionRow } from '@/types/apps'

/**
 * HistoryTable — renders Helm revision history with a rollback action.
 *
 * Parent owns the rows + the current revision (so the current row's rollback
 * button is disabled). Rollback dispatch is emitted as a number, parent
 * decides whether to call /release/rollback and refresh.
 */
defineProps<{ rows: RevisionRow[]; currentRevision?: number }>()
const emit = defineEmits<{ (e: 'rollback', revision: number): void }>()

const { t } = useI18n()

const columns = computed(() => [
  { key: 'revision',      title: '#',                                       dataIndex: 'revision', width: 64 },
  { key: 'status',        title: t('apps.application.history.col.status') },
  { key: 'chart_version', title: t('apps.application.history.col.chart_version'), dataIndex: 'chart_version' },
  { key: 'app_version',   title: t('apps.application.history.col.app_version'),   dataIndex: 'app_version' },
  { key: 'updated_at',    title: t('apps.application.history.col.updated_at') },
  { key: 'description',   title: t('apps.application.history.col.description'),   dataIndex: 'description' },
  { key: 'actions',       title: t('apps.application.history.col.actions') },
])

function statusColor(s: string): string {
  switch (s) {
    case 'deployed':
      return 'green'
    case 'failed':
    case 'unknown':
      return 'red'
    case 'pending-install':
    case 'pending-upgrade':
    case 'pending-rollback':
      return 'orange'
    case 'uninstalled':
      return 'default'
    default:
      return 'blue'
  }
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}
</script>
