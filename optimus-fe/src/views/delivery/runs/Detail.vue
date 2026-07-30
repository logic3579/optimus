<template>
  <a-card v-if="run">
    <PageHeader :title="`${t('delivery.run.title')} #${run.id}`">
      <a-space>
        <a-button v-permission="'delivery:run:cancel'" data-testid="cancel-run" :disabled="!canCancel" @click="cancel">{{t('delivery.run.cancel')}}</a-button>
        <a-button v-permission="'delivery:run:create'" data-testid="reconcile-run" :disabled="!canReconcile" @click="reconcile">{{t('delivery.run.reconcile')}}</a-button>
        <a-button v-permission="'delivery:run:create'" data-testid="retry-run" :disabled="!canRetry" @click="retry">{{t('delivery.run.retry')}}</a-button>
      </a-space>
    </PageHeader>
    <a-descriptions bordered>
      <a-descriptions-item :label="t('delivery.run.state')"><a-tag>{{stateLabel(run.state)}}</a-tag></a-descriptions-item>
      <a-descriptions-item :label="t('delivery.run.version')">{{run.chart_name}} {{run.chart_version}}</a-descriptions-item>
      <a-descriptions-item :label="t('delivery.run.digest')"><code>{{run.chart_digest}}</code></a-descriptions-item>
      <a-descriptions-item v-if="run.error_message_key" :label="t('common.error')">{{t(run.error_message_key)}}<span v-if="run.correlation_id"> · {{run.correlation_id}}</span></a-descriptions-item>
    </a-descriptions>
    <a-timeline data-testid="stage-timeline">
      <a-timeline-item v-for="stage in run.stages" :key="stage.id">
        <a-space><strong>{{stage.order}}. {{stage.environment_name}}</strong><a-tag>{{stateLabel(stage.state)}}</a-tag></a-space>
        <div>{{stage.namespace}} / {{stage.release_name}}</div>
        <div v-if="stage.approval_required">{{t('delivery.approval.required')}}</div>
        <div v-if="stage.result_revision">{{t('delivery.run.revision')}}: {{stage.result_revision}}</div>
        <div v-if="stage.result_digest">{{t('delivery.run.digest')}}: <code>{{stage.result_digest}}</code></div>
        <div v-if="stage.error_message_key">{{t(stage.error_message_key)}}<span v-if="stage.correlation_id"> · {{stage.correlation_id}}</span></div>
        <router-link v-if="showRecovery(stage.state)" :to="`/apps/applications/${stage.application_id}`">{{t('delivery.run.p3_recovery')}}</router-link>
      </a-timeline-item>
    </a-timeline>
    <a-list data-testid="event-timeline" :data-source="events"><template #renderItem="{item}"><a-list-item><span>{{item.event_type}}</span><span v-if="item.old_state||item.new_state"> · {{item.old_state??'—'}} → {{item.new_state??'—'}}</span><span v-if="item.correlation_id"> · {{item.correlation_id}}</span></a-list-item></template></a-list>
  </a-card>
</template>
<script setup lang="ts">
import{computed,inject,onBeforeUnmount,onMounted,watch}from'vue';import{useRoute,useRouter}from'vue-router';import type{DeliveryEventsApi}from'@/api/delivery/events';import type{DeliveryRunApi}from'@/api/delivery/run';import PageHeader from'@/components/PageHeader.vue';import{useI18n}from'@/hooks/useI18n';import{useDeliveryStore}from'@/stores/delivery';import type{RunState,StageState}from'@/types/delivery';import{newIdempotencyKey}from'@/views/delivery/projects/components/idempotency'
const route=useRoute(),router=useRouter(),runApi=inject<DeliveryRunApi>('deliveryRunApi')!,eventApi=inject<DeliveryEventsApi>('deliveryEventsApi')!,store=useDeliveryStore(),{t}=useI18n(),run=computed(()=>store.run),events=computed(()=>store.events),runId=computed(()=>Number(route.params.id)),canCancel=computed(()=>!!run.value&&['queued','running','waiting_approval'].includes(run.value.state)),canReconcile=computed(()=>run.value?.state==='outcome_unknown'),canRetry=computed(()=>!!run.value&&['failed','rejected','canceled','timed_out'].includes(run.value.state))
function stateLabel(state:RunState|StageState){return t(`delivery.state.${state}`)}function showRecovery(state:StageState){return['failed','timed_out','outcome_unknown'].includes(state)}
async function select(id=runId.value){if(Number.isInteger(id)&&id>0)await store.selectRun(id,runApi,eventApi)}async function cancel(){if(!canCancel.value||!run.value)return;store.run=await runApi.cancel(run.value.id)}async function reconcile(){if(!canReconcile.value||!run.value)return;store.run=await runApi.reconcile(run.value.id)}async function retry(){if(!canRetry.value||!run.value)return;const next=await runApi.retry(run.value.id,newIdempotencyKey());await router.push(`/delivery/runs/${next.id}`)}
onMounted(()=>void select());watch(runId,(next,previous)=>{if(next!==previous)void select(next)});onBeforeUnmount(()=>store.reset());defineExpose({run,events,canCancel,canReconcile,canRetry,stateLabel,showRecovery,select,cancel,reconcile,retry})
</script>
