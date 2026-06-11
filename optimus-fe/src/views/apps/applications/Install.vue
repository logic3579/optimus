<template>
  <a-card>
    <PageHeader :title="t('apps.install.title')">
      <a-button @click="goBack">{{ t('common.button.back') }}</a-button>
    </PageHeader>

    <a-steps :current="current" class="u-mb-16">
      <a-step :title="t('apps.install.step.basicChart')" />
      <a-step :title="t('apps.install.step.values')" />
    </a-steps>

    <!-- Step 0: Basics + Chart picker. On Next we POST /apps/applications.
         If the user backs from step 1, the application row already exists;
         the basics card is disabled to prevent re-POSTing, and only the
         chart version (a step-1 concern handled via ValuesEditor) is
         freely editable. See `basicsLocked` below. -->
    <div v-show="current === 0">
      <h3 class="section-title">{{ t('apps.install.section.basics') }}</h3>
      <ApplicationFormBasic v-model="basic" :is-edit="basicsLocked" />

      <h3 class="section-title">{{ t('apps.install.section.chart') }}</h3>
      <ChartPickerStep
        v-model:repo-id="chart.repoId"
        v-model:chart-name="chart.name"
        v-model:version="chart.version"
      />
    </div>

    <!-- Step 1: Values editor for the install body. -->
    <div v-show="current === 1">
      <ValuesEditor
        v-model="valuesYaml"
        :repo-id="chart.repoId"
        :chart-name="chart.name"
        :chart-version="chart.version"
      />
    </div>

    <div class="footer u-mt-16">
      <a-space>
        <a-button v-if="current > 0" @click="onPrev">{{ t('common.button.previous') }}</a-button>
        <a-button
          v-if="current === 0"
          v-permission="'apps:application:write'"
          type="primary"
          :loading="submitting"
          :disabled="!canProceedToValues"
          @click="onCreateAndNext"
        >
          {{ t('common.button.next') }}
        </a-button>
        <a-button
          v-if="current === 1"
          v-permission="'apps:release:install'"
          type="primary"
          :loading="submitting"
          :disabled="!chart.version"
          @click="onSubmitInstall"
        >
          {{ t('apps.install.btn.submit') }}
        </a-button>
      </a-space>
    </div>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import ApplicationFormBasic, { type ApplicationFormModel } from './components/ApplicationFormBasic.vue'
import ChartPickerStep from './components/ChartPickerStep.vue'
import ValuesEditor from './components/ValuesEditor.vue'
import type { AppsApplicationApi } from '@/api/apps/application'
import type { AppsReleaseApi } from '@/api/apps/release'

/**
 * Install wizard — TWO-step variant.
 *
 * Step 0 (Basics + Chart picker): collects everything required to create
 *   the application row in the BE. POST /apps/applications fires on Next.
 *   This intentionally folds the original plan's "Step 1 (Basics)" and
 *   "Step 2 (Chart)" together because the BE DTO requires chart_repo_id +
 *   chart_name at create time — splitting them would force a phantom
 *   create with empty chart fields, which fails validation.
 *
 * Step 1 (Values): editor with Load-defaults + Format helpers. Submit
 *   fires POST /apps/applications/:id/release/install.
 *
 * Back-nav after the first POST: the basics card is locked
 * (ApplicationFormBasic respects :is-edit and disables the four
 * immutable fields). Chart picker stays editable because nothing has
 * been written to the BE for those fields outside the chart_name on
 * the application row, which the BE marks immutable on create anyway.
 */

const { t } = useI18n()
const router = useRouter()
const appApi = inject<AppsApplicationApi>('appsApplicationApi')!
const relApi = inject<AppsReleaseApi>('appsReleaseApi')!

const current = ref(0)
const submitting = ref(false)

const basic = reactive<ApplicationFormModel>({
  name: '',
  release_name: '',
  cluster_id: undefined,
  namespace: '',
  owner_user_id: undefined,
  tags: [],
  description: '',
})

const chart = reactive<{ repoId?: number; name?: string; version?: string }>({
  repoId: undefined,
  name: undefined,
  version: undefined,
})

const valuesYaml = ref<string>('')

// applicationId is set once step 0 succeeds; the basics card is locked
// thereafter so the user cannot edit the immutable fields and trigger a
// second POST.
const applicationId = ref<number | null>(null)
const basicsLocked = computed(() => applicationId.value !== null)

const canProceedToValues = computed(
  () =>
    !!basic.name &&
    !!basic.release_name &&
    !!basic.cluster_id &&
    !!basic.namespace &&
    !!chart.repoId &&
    !!chart.name,
)

function goBack() {
  void router.push('/apps/applications')
}

function onPrev() {
  if (current.value > 0) current.value -= 1
}

async function onCreateAndNext() {
  // Idempotent guard: if the row already exists (back-nav scenario), skip
  // the POST and just advance.
  if (applicationId.value !== null) {
    current.value = 1
    return
  }
  if (basic.cluster_id === undefined || !chart.repoId || !chart.name) {
    return
  }
  submitting.value = true
  try {
    const created = await appApi.create({
      name: basic.name,
      release_name: basic.release_name,
      cluster_id: basic.cluster_id,
      namespace: basic.namespace,
      chart_repo_id: chart.repoId,
      chart_name: chart.name,
      description: basic.description || undefined,
      tags: basic.tags.length > 0 ? basic.tags : undefined,
      owner_user_id: basic.owner_user_id,
    })
    applicationId.value = created.id
    current.value = 1
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    submitting.value = false
  }
}

async function onSubmitInstall() {
  if (applicationId.value === null || !chart.version) return
  submitting.value = true
  try {
    await relApi.install(applicationId.value, {
      chart_version: chart.version,
      values_yaml: valuesYaml.value,
    })
    message.success(t('common.message.installed'))
    void router.push(`/apps/applications/${applicationId.value}`)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    submitting.value = false
  }
}
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
