<template>
  <a-modal :open="open" :title="t('delivery.environment.create')" @ok="submit" @cancel="$emit('close')">
    <a-form layout="vertical">
      <a-form-item :label="t('delivery.environment.key')" required><a-input v-model:value="form.environment_key" :maxlength="128" /></a-form-item>
      <a-form-item :label="t('delivery.environment.name')" required><a-input v-model:value="form.display_name" :maxlength="128" /></a-form-item>
      <a-form-item :label="t('delivery.environment.application')" required>
        <a-select v-model:value="form.application_id" :options="applicationOptions" show-search option-filter-prop="label" @change="inspectApplication" />
      </a-form-item>
      <a-alert v-if="validationError" type="error" :message="validationError" />
      <dl v-if="selected">
        <dt>{{ t('apps.application.list.col.cluster') }}</dt><dd>{{ selected.cluster_name }}</dd>
        <dt>{{ t('apps.application.list.col.namespace') }}</dt><dd>{{ selected.namespace }}</dd>
        <dt>{{ t('apps.application.list.col.release') }}</dt><dd>{{ selected.release_name }}</dd>
        <dt>{{ t('apps.application.list.col.chart') }}</dt><dd>{{ selected.chart_name }}</dd>
      </dl>
    </a-form>
  </a-modal>
</template>
<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import type { AppsApplicationApi } from '@/api/apps/application'
import type { DeliveryProjectApi } from '@/api/delivery/project'
import { useI18n } from '@/hooks/useI18n'
import type { ApplicationDetail, ApplicationSummary } from '@/types/apps'
import { isBizError } from '@/utils/http-error'

const props = defineProps<{ open:boolean;projectId:number;boundApplicationIds:number[] }>()
const emit = defineEmits<{close:[];saved:[]}>()
const apps = inject<AppsApplicationApi>('appsApplicationApi')!
const projects = inject<DeliveryProjectApi>('deliveryProjectApi')!
const { t } = useI18n()
const applications = ref<ApplicationSummary[]>([]), selected = ref<ApplicationDetail|null>(null), validationError = ref('')
const form = reactive({ environment_key:'', display_name:'', application_id:undefined as number|undefined })
const applicationOptions = computed(() => applications.value.map(app => ({ value:app.id, label:app.name, disabled:props.boundApplicationIds.includes(app.id) })))

watch(() => props.open, async open => { if(!open)return;Object.assign(form,{environment_key:'',display_name:'',application_id:undefined});selected.value=null;validationError.value='';const page=await apps.list({page:1,page_size:100});applications.value=page.items })
async function inspectApplication(id:number){if(props.boundApplicationIds.includes(id)){validationError.value=t('delivery.environment.duplicate');return}selected.value=await apps.get(id);validationError.value=selected.value.status!=='deployed'?t('delivery.environment.not_installed'):''}
async function submit(){if(!form.environment_key.trim()||!form.display_name.trim()||!form.application_id||validationError.value)return;try{await projects.bindEnvironment(props.projectId,{environment_key:form.environment_key.trim(),display_name:form.display_name.trim(),application_id:form.application_id});emit('saved')}catch(error){validationError.value=isBizError(error)?error.message:t('network.error')}}
defineExpose({form,selected,validationError,inspectApplication,submit})
</script>
