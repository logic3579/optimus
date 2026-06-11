<template>
  <a-card v-if="detail">
    <PageHeader :title="t('apps.upgrade.title', { name: detail.name })">
      <a-button @click="goBack">{{ t('common.button.back') }}</a-button>
    </PageHeader>

    <a-descriptions :column="2" size="middle" class="u-mb-16">
      <a-descriptions-item :label="t('apps.application.list.col.cluster')">
        {{ detail.cluster_name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.namespace')">
        {{ detail.namespace }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.release')">
        {{ detail.release_name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart')">
        {{ detail.chart_name }}
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.upgrade.currentVersion')">
        <a-tag v-if="currentVersion">{{ currentVersion }}</a-tag>
        <span v-else>—</span>
      </a-descriptions-item>
    </a-descriptions>

    <!-- Chart picker pinned to the application's repo (lockRepo). The user
         picks a target version; the chart name itself stays fixed because
         the BE forbids cross-chart upgrades. -->
    <h3 class="section-title">{{ t('apps.install.section.chart') }}</h3>
    <ChartPickerStep
      v-model:repo-id="chart.repoId"
      v-model:chart-name="chart.name"
      v-model:version="chart.version"
      :lock-repo="true"
    />

    <h3 class="section-title">{{ t('apps.install.step.values') }}</h3>
    <ValuesEditor
      v-model="valuesYaml"
      :repo-id="chart.repoId"
      :chart-name="chart.name"
      :chart-version="chart.version"
    />

    <div class="footer u-mt-16">
      <a-space>
        <a-button @click="goBack">{{ t('common.cancel') }}</a-button>
        <a-button
          v-permission="'apps:release:upgrade'"
          type="primary"
          :loading="submitting"
          :disabled="!chart.version"
          @click="onSubmit"
        >
          {{ t('apps.upgrade.btn.submit') }}
        </a-button>
      </a-space>
    </div>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import ChartPickerStep from './components/ChartPickerStep.vue'
import ValuesEditor from './components/ValuesEditor.vue'
import type { AppsApplicationApi } from '@/api/apps/application'
import type { AppsReleaseApi } from '@/api/apps/release'
import type { ApplicationDetail } from '@/types/apps'

/**
 * Upgrade page — /apps/applications/:id/upgrade.
 *
 * Pre-fills the chart picker with the application's current repo + chart;
 * the repo is locked because cross-repo upgrades are out of P3 scope. The
 * version is intentionally left blank so the user has to consciously pick
 * a target version (preventing accidental same-version upgrades).
 *
 * Submit calls /apps/applications/:id/release/upgrade with chart_version +
 * values_yaml. chart_repo_id is omitted from the body when unchanged
 * (currently always omitted since the picker locks the repo).
 */

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appApi = inject<AppsApplicationApi>('appsApplicationApi')!
const relApi = inject<AppsReleaseApi>('appsReleaseApi')!

const detail = ref<ApplicationDetail | null>(null)
const submitting = ref(false)
const valuesYaml = ref<string>('')

const chart = reactive<{ repoId?: number; name?: string; version?: string }>({
  repoId: undefined,
  name: undefined,
  version: undefined,
})

const appId = computed<number>(() => Number(route.params.id))
const currentVersion = computed(() => detail.value?.chart_version || '')

function goBack() {
  void router.push(`/apps/applications/${appId.value}`)
}

async function onSubmit() {
  if (!chart.version) return
  submitting.value = true
  try {
    // chart_repo_id only sent when the picker allows it to change. Today
    // the picker is locked, so we never override the repo on upgrade —
    // omit the field entirely so the BE keeps the existing repo.
    await relApi.upgrade(appId.value, {
      chart_version: chart.version,
      values_yaml: valuesYaml.value,
    })
    message.success(t('common.message.upgraded'))
    void router.push(`/apps/applications/${appId.value}`)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  try {
    const d = await appApi.get(appId.value)
    detail.value = d
    chart.repoId = d.chart_repo_id
    chart.name = d.chart_name
    // chart.version intentionally left blank.
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
})
</script>

<style scoped lang="scss">
.section-title {
  margin: 16px 0 12px;
  font-size: 16px;
  font-weight: 600;
}
.footer {
  display: flex;
  justify-content: flex-end;
}
</style>
