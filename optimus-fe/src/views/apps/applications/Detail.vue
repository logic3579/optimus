<template>
  <a-card v-if="detail">
    <PageHeader :title="`${detail.name}`">
      <a-button @click="goBack">{{ t('common.button.back') }}</a-button>
      <a-button
        v-permission="'apps:release:upgrade'"
        type="primary"
        @click="goUpgrade"
      >
        {{ t('apps.application.list.action.upgrade') }}
      </a-button>
    </PageHeader>

    <a-descriptions
      :title="t('apps.application.detail.section.basic')"
      bordered
      :column="2"
      size="middle"
    >
      <a-descriptions-item :label="t('apps.application.list.col.name')">
        {{ detail.name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.release')">
        {{ detail.release_name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.cluster')">
        {{ detail.cluster_name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.namespace')">
        {{ detail.namespace }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart')">
        {{ detail.chart_name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart_version')">
        {{ detail.chart_version || '—' }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.app_version')">
        {{ detail.app_version || '—' }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.owner')">
        {{ detail.owner_name || '—' }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.field.tags')" :span="2">
        <a-tag v-for="tg in detail.tags ?? []" :key="tg">{{ tg }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.field.description')" :span="2">
        {{ detail.description || '—' }}
      </a-descriptions-item>
      <a-descriptions-item
        v-if="status"
        :label="t('apps.application.list.showLiveStatus')"
        :span="2"
      >
        <a-tag :color="statusColor(status.status)">{{ status.status }}</a-tag>
        <span class="status-extra">
          revision #{{ status.revision }} ·
          chart {{ status.chart_version }}
          <template v-if="status.app_version"> (app {{ status.app_version }})</template>
          · {{ formatTime(status.last_deployed_at) }}
        </span>
      </a-descriptions-item>
    </a-descriptions>

    <h3 class="section-title">{{ t('apps.application.detail.section.history') }}</h3>
    <HistoryTable
      :rows="history"
      :current-revision="status?.revision"
      @rollback="onRollback"
    />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import HistoryTable from './components/HistoryTable.vue'
import type { AppsApplicationApi } from '@/api/apps/application'
import type { AppsReleaseApi } from '@/api/apps/release'
import type { ApplicationDetail, ReleaseStatus, RevisionRow } from '@/types/apps'

/**
 * Applications Detail — P3 §8.6.
 *
 * Three reads run in parallel on mount: application metadata + live release
 * status + revision history. Rollback dispatches a confirm-flow via the
 * HistoryTable component and refreshes status + history (but not the row
 * itself; metadata is immutable).
 */

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appApi = inject<AppsApplicationApi>('appsApplicationApi')!
const relApi = inject<AppsReleaseApi>('appsReleaseApi')!

const detail = ref<ApplicationDetail | null>(null)
const status = ref<ReleaseStatus | null>(null)
const history = ref<RevisionRow[]>([])

const appId = computed<number>(() => Number(route.params.id))

function goBack() {
  void router.push('/apps/applications')
}
function goUpgrade() {
  void router.push(`/apps/applications/${appId.value}/upgrade`)
}

async function loadStatus() {
  try {
    status.value = await relApi.status(appId.value)
  } catch (e) {
    // No release yet → BE returns 42202 release_not_found; that is a normal
    // state for an application that has never been installed.
    if (isBizError(e) && e.code === 42202) {
      status.value = null
    } else {
      message.error(isBizError(e) ? e.message : t('network.error'))
    }
  }
}

async function loadHistory() {
  try {
    const r = await relApi.history(appId.value)
    history.value = r.items
  } catch (e) {
    if (isBizError(e) && e.code === 42202) {
      history.value = []
    } else {
      message.error(isBizError(e) ? e.message : t('network.error'))
    }
  }
}

async function loadDetail() {
  try {
    detail.value = await appApi.get(appId.value)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function onRollback(revision: number) {
  try {
    await relApi.rollback(appId.value, { revision })
    message.success(t('common.message.done'))
    await Promise.all([loadStatus(), loadHistory()])
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function statusColor(s: string): string {
  switch (s) {
    case 'deployed':
      return 'green'
    case 'failed':
    case 'unknown':
      return 'red'
    case 'pending':
    case 'pending-install':
    case 'pending-upgrade':
    case 'pending-rollback':
      return 'orange'
    default:
      return 'blue'
  }
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

onMounted(async () => {
  await Promise.all([loadDetail(), loadStatus(), loadHistory()])
})
</script>

<style scoped lang="scss">
.section-title {
  margin: 24px 0 12px;
  font-size: 16px;
  font-weight: 600;
}
.status-extra {
  margin-left: 12px;
  color: var(--ant-color-text-secondary, rgba(0, 0, 0, 0.45));
}
</style>
