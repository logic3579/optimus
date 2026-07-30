<template>
  <a-alert v-if="error" type="error" :message="error" />
  <a-select v-model:value="selectedVersion" data-testid="artifact-version" :options="options" :loading="loading" />
  <a-button data-testid="create-run" type="primary" :disabled="!selectedArtifact || submitting" :loading="submitting" @click="submit">{{t('delivery.run.create')}}</a-button>
</template>
<script setup lang="ts">
import{computed,h,inject,onMounted,ref,watch}from'vue';import{Modal}from'ant-design-vue';import{useRouter}from'vue-router';import type{DeliveryPipelineApi}from'@/api/delivery/pipeline';import type{DeliveryRunApi}from'@/api/delivery/run';import{useI18n}from'@/hooks/useI18n';import type{DeliveryArtifactVersion,DeliveryEnvironment,DeliveryPipeline}from'@/types/delivery';import{newIdempotencyKey}from'./idempotency'
const props=defineProps<{projectId:number;environments:DeliveryEnvironment[];pipeline:DeliveryPipeline|null}>(),pipelineApi=inject<DeliveryPipelineApi>('deliveryPipelineApi')!,runApi=inject<DeliveryRunApi>('deliveryRunApi')!,router=useRouter(),{t}=useI18n(),artifacts=ref<DeliveryArtifactVersion[]>([]),selectedVersion=ref<string>(),loading=ref(false),submitting=ref(false),error=ref(''),intentKey=ref('')
const selectedArtifact=computed(()=>artifacts.value.find(x=>x.version===selectedVersion.value)),options=computed(()=>artifacts.value.map(x=>({label:x.version,value:x.version})))
watch(selectedVersion,()=>{intentKey.value=''})
async function load(){loading.value=true;try{artifacts.value=await pipelineApi.listArtifacts(props.projectId);selectedVersion.value=artifacts.value[0]?.version}catch(e){error.value=e instanceof Error?e.message:t('common.error')}finally{loading.value=false}}
function confirmation(artifact:{chart_name:string;version:string;digest:string}){const envById=new Map(props.environments.map(x=>[x.id,x]));const stages=props.pipeline?.stages??[];return h('div',[h('p',`${artifact.chart_name} ${artifact.version}`),h('p',artifact.digest),h('ol',stages.map(stage=>h('li',`${envById.get(stage.environment_id)?.display_name??stage.environment_id}${stage.approval_required?' · approval':''}`)))])}
async function confirm(content:ReturnType<typeof h>){return new Promise<boolean>(resolve=>Modal.confirm({title:t('delivery.run.confirm'),content,onOk:()=>resolve(true),onCancel:()=>resolve(false)}))}
async function submit(){if(submitting.value||!selectedArtifact.value)return;submitting.value=true;error.value='';try{const input={chart_repo_id:selectedArtifact.value.chart_repo_id,chart_name:selectedArtifact.value.chart_name,chart_version:selectedArtifact.value.version};const resolved=await pipelineApi.resolveArtifact(props.projectId,input);if(!await confirm(confirmation(resolved)))return;if(!intentKey.value)intentKey.value=newIdempotencyKey();const run=await runApi.create(props.projectId,input,intentKey.value);intentKey.value='';await router.push(`/delivery/runs/${run.id}`)}catch(e){error.value=e instanceof Error?e.message:t('common.error')}finally{submitting.value=false}}
onMounted(()=>void load());defineExpose({artifacts,selectedVersion,selectedArtifact,intentKey,submitting,error,load,submit})
</script>
