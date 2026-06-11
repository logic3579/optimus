<template>
  <a-row :gutter="16">
    <a-col :span="8">
      <a-form-item :label="t('apps.install.field.repo')">
        <a-select
          :value="repoId"
          :options="repoOptions"
          :loading="loadingRepos"
          show-search
          option-filter-prop="label"
          allow-clear
          @update:value="onRepoChange"
        />
      </a-form-item>
    </a-col>
    <a-col :span="8">
      <a-form-item :label="t('apps.install.field.chart')">
        <a-select
          :value="chartName"
          :options="chartOptions"
          :disabled="!repoId"
          :loading="loadingCharts"
          show-search
          option-filter-prop="label"
          allow-clear
          @update:value="onChartChange"
        />
      </a-form-item>
    </a-col>
    <a-col :span="8">
      <a-form-item :label="t('apps.install.field.version')">
        <a-select
          :value="version"
          :options="versionOptions"
          :disabled="!chartName"
          :loading="loadingVersions"
          show-search
          option-filter-prop="label"
          allow-clear
          @update:value="onVersionChange"
        />
      </a-form-item>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { AppsRepoApi } from '@/api/apps/repo'
import type { ChartRepoSummary, ChartSummary, VersionSummary } from '@/types/apps'

/**
 * ChartPickerStep — three-level cascade: repository -> chart -> version.
 *
 * Parent owns selection via v-model:repoId / v-model:chartName / v-model:version.
 * Changing the parent invalidates downstream selections (chart resets when
 * repo changes, version resets when chart changes).
 */
const props = defineProps<{
  repoId?: number
  chartName?: string
  version?: string
  // Optional: when set, repo selector is locked (Upgrade step pins it).
  lockRepo?: boolean
}>()
const emit = defineEmits<{
  (e: 'update:repoId', v: number | undefined): void
  (e: 'update:chartName', v: string | undefined): void
  (e: 'update:version', v: string | undefined): void
}>()

const { t } = useI18n()
const repoApi = inject<AppsRepoApi>('appsRepoApi')!

const repos = ref<ChartRepoSummary[]>([])
const loadingRepos = ref(false)
const repoOptions = computed(() =>
  repos.value.map((r) => ({ label: `${r.name} (${r.type})`, value: r.id, disabled: props.lockRepo === true && r.id !== props.repoId })),
)

const charts = ref<ChartSummary[]>([])
const loadingCharts = ref(false)
const chartOptions = computed(() => charts.value.map((c) => ({ label: c.name, value: c.name })))

const versions = ref<VersionSummary[]>([])
const loadingVersions = ref(false)
const versionOptions = computed(() =>
  versions.value.map((v) => ({
    label: v.app_version ? `${v.version}  (app: ${v.app_version})` : v.version,
    value: v.version,
  })),
)

function onRepoChange(v: number | undefined): void {
  emit('update:repoId', v)
  // Cascade: clear downstream selections so stale chart/version don't leak.
  if (props.chartName) emit('update:chartName', undefined)
  if (props.version) emit('update:version', undefined)
}
function onChartChange(v: string | undefined): void {
  emit('update:chartName', v)
  if (props.version) emit('update:version', undefined)
}
function onVersionChange(v: string | undefined): void {
  emit('update:version', v)
}

onMounted(async () => {
  loadingRepos.value = true
  try {
    const r = await repoApi.list({ page_size: 200 })
    repos.value = r.items
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loadingRepos.value = false
  }
})

watch(
  () => props.repoId,
  async (id) => {
    charts.value = []
    versions.value = []
    if (!id) return
    loadingCharts.value = true
    try {
      const r = await repoApi.listCharts(id)
      charts.value = r.items
    } catch (e) {
      message.error(isBizError(e) ? e.message : t('network.error'))
    } finally {
      loadingCharts.value = false
    }
  },
  { immediate: true },
)

watch(
  () => [props.repoId, props.chartName] as const,
  async ([id, name]) => {
    versions.value = []
    if (!id || !name) return
    loadingVersions.value = true
    try {
      const r = await repoApi.listVersions(id, name)
      versions.value = r.items
    } catch (e) {
      message.error(isBizError(e) ? e.message : t('network.error'))
    } finally {
      loadingVersions.value = false
    }
  },
  { immediate: true },
)
</script>
