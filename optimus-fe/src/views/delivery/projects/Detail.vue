<template>
  <a-card>
    <PageHeader :title="project?.name ?? t('delivery.project.title')" />
    <a-tabs>
      <a-tab-pane v-if="canProjectRead" key="environments" :tab="t('delivery.environment.title')">
        <a-button v-permission="'delivery:project:write'" data-testid="add-environment" @click="environmentOpen=true">{{t('common.create')}}</a-button>
        <a-table :columns="environmentColumns" :data-source="project?.environments??[]" :pagination="false" row-key="id">
          <template #bodyCell="{column,record}"><template v-if="column.key==='actions'"><a-popconfirm :title="t('confirm.delete_title')" @confirm="unbind(record.id)"><a v-permission="'delivery:project:write'">{{t('common.delete')}}</a></a-popconfirm></template></template>
        </a-table>
      </a-tab-pane>
      <a-tab-pane v-if="canPipelineRead" key="pipeline" :tab="t('delivery.pipeline.title')">
        <PipelineForm :project-id="projectId" :environments="project?.environments??[]" :current="pipeline" @published="pipeline=$event" />
      </a-tab-pane>
    </a-tabs>
    <EnvironmentForm :open="environmentOpen" :project-id="projectId" :bound-application-ids="project?.environments.map(x=>x.application_id)??[]" @close="environmentOpen=false" @saved="environmentSaved" />
  </a-card>
</template>
<script setup lang="ts">
import { computed,inject,onMounted,ref } from'vue';import{useRoute}from'vue-router';import type{DeliveryProjectApi}from'@/api/delivery/project';import type{DeliveryPipelineApi}from'@/api/delivery/pipeline';import PageHeader from'@/components/PageHeader.vue';import{useI18n}from'@/hooks/useI18n';import{usePermission}from'@/hooks/usePermission';import type{DeliveryPipeline,DeliveryProjectDetail}from'@/types/delivery';import EnvironmentForm from'./components/EnvironmentForm.vue';import PipelineForm from'./components/PipelineForm.vue'
const route=useRoute(),projects=inject<DeliveryProjectApi>('deliveryProjectApi')!,pipelines=inject<DeliveryPipelineApi>('deliveryPipelineApi')!,permission=usePermission(),{t}=useI18n(),projectId=Number(route.params.id),project=ref<DeliveryProjectDetail|null>(null),pipeline=ref<DeliveryPipeline|null>(null),environmentOpen=ref(false),canProjectRead=computed(()=>permission.has('delivery:project:read')),canPipelineRead=computed(()=>permission.has('delivery:pipeline:read')),environmentColumns=computed(()=>[{key:'display_name',title:t('delivery.environment.name'),dataIndex:'display_name'},{key:'application_name',title:t('delivery.environment.application'),dataIndex:'application_name'},{key:'cluster_id',title:t('apps.application.list.col.cluster'),dataIndex:'cluster_id'},{key:'namespace',title:t('apps.application.list.col.namespace'),dataIndex:'namespace'},{key:'release_name',title:t('apps.application.list.col.release'),dataIndex:'release_name'},{key:'chart_name',title:t('apps.application.list.col.chart'),dataIndex:'chart_name'},{key:'actions',title:t('common.actions')}])
async function loadProject(){project.value=await projects.get(projectId)}async function loadPipeline(){try{pipeline.value=await pipelines.get(projectId)}catch{pipeline.value=null}}async function unbind(id:number){await projects.unbindEnvironment(projectId,id);await loadProject()}async function environmentSaved(){environmentOpen.value=false;await loadProject()}onMounted(()=>{if(canProjectRead.value)void loadProject();if(canPipelineRead.value)void loadPipeline()});defineExpose({project,pipeline,environmentOpen,canProjectRead,canPipelineRead,loadProject,loadPipeline,unbind,environmentSaved})
</script>
