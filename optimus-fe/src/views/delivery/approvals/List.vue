<template>
  <a-card v-if="canRead">
    <PageHeader :title="t('delivery.approval.title')" />
    <a-table :columns="columns" :data-source="items" :loading="loading" :pagination="false" row-key="id">
      <template #bodyCell="{column,record}">
        <template v-if="column.key==='run'"><router-link :to="`/delivery/runs/${record.run_id}`">#{{record.run_id}}</router-link></template>
        <template v-else-if="column.key==='artifact'">{{record.chart_name}} {{record.chart_version}}<br><code>{{record.chart_digest}}</code></template>
        <template v-else-if="column.key==='actions'"><a-space><a-button v-permission="'delivery:approval:decide'" :disabled="record.initiator_user_id===userId" @click="open(record,'approve')">{{t('delivery.approval.approve')}}</a-button><a-button v-permission="'delivery:approval:decide'" danger :disabled="record.initiator_user_id===userId" @click="open(record,'reject')">{{t('delivery.approval.reject')}}</a-button></a-space></template>
      </template>
    </a-table>
    <a-modal :open="!!selected" :title="t(`delivery.approval.${decision}`)" @ok="submit" @cancel="close"><a-textarea v-model:value="comment" :maxlength="512" :rows="4" /><a-alert v-if="validationError" type="error" :message="validationError" /></a-modal>
  </a-card>
</template>
<script setup lang="ts">
import{computed,inject,onMounted,ref}from'vue';import{Modal}from'ant-design-vue';import type{DeliveryApprovalApi}from'@/api/delivery/approval';import PageHeader from'@/components/PageHeader.vue';import{useI18n}from'@/hooks/useI18n';import{usePermission}from'@/hooks/usePermission';import{useAuthStore}from'@/stores/auth';import type{PendingDeliveryApproval}from'@/types/delivery'
const api=inject<DeliveryApprovalApi>('deliveryApprovalApi')!,auth=useAuthStore(),permission=usePermission(),{t}=useI18n(),canRead=computed(()=>permission.has('delivery:approval:read')),userId=computed(()=>auth.user?.id??0),items=ref<PendingDeliveryApproval[]>([]),loading=ref(false),selected=ref<PendingDeliveryApproval|null>(null),decision=ref<'approve'|'reject'>('approve'),comment=ref(''),validationError=ref(''),columns=computed(()=>[{key:'project',title:t('delivery.project.title'),dataIndex:'project_name'},{key:'environment',title:t('delivery.environment.title'),dataIndex:'environment_name'},{key:'artifact',title:t('delivery.run.artifact')},{key:'initiator',title:t('delivery.run.initiator'),dataIndex:'initiator_user_id'},{key:'requested_at',title:t('delivery.approval.requested_at'),dataIndex:'requested_at'},{key:'run',title:t('delivery.run.title')},{key:'actions',title:t('common.actions')}])
async function load(){if(!canRead.value)return;loading.value=true;try{items.value=await api.listPending()}finally{loading.value=false}}function open(row:PendingDeliveryApproval,next:'approve'|'reject'){if(row.initiator_user_id===userId.value)return;selected.value=row;decision.value=next;comment.value='';validationError.value=''}function close(){selected.value=null;comment.value='';validationError.value=''}
async function confirm(){return new Promise<boolean>(resolve=>Modal.confirm({title:t(`delivery.approval.${decision.value}_confirm`),onOk:()=>resolve(true),onCancel:()=>resolve(false)}))}async function submit(){const text=comment.value.trim();if(!text||text.length>512){validationError.value=t('delivery.approval.comment_required');return}if(!selected.value||!await confirm())return;try{await api[decision.value](selected.value.run_stage_id,text);close()}finally{await load()}}
onMounted(()=>void load());defineExpose({canRead,userId,items,selected,decision,comment,validationError,load,open,close,submit})
</script>
