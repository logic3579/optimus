<template>
  <div>
    <a-alert v-if="error" type="error" :message="error" />
    <div v-for="(stage,index) in stages" :key="stage.environment_id" class="stage-row">
      <span>{{ environmentName(stage.environment_id) }}</span>
      <a-checkbox v-model:checked="stage.approval_required">{{ t('delivery.pipeline.approval_required') }}</a-checkbox>
      <a-input v-model:value="stage.timeout" :maxlength="16" />
      <a-button :disabled="index===0" @click="move(index,-1)">↑</a-button>
      <a-button :disabled="index===stages.length-1" @click="move(index,1)">↓</a-button>
    </div>
    <a-button v-permission="'delivery:pipeline:write'" data-testid="publish-pipeline" type="primary" @click="publish">
      {{ t('delivery.pipeline.publish_version', { version: nextVersion }) }}
    </a-button>
  </div>
</template>
<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import { Modal } from 'ant-design-vue'
import type { DeliveryPipelineApi } from '@/api/delivery/pipeline'
import { useI18n } from '@/hooks/useI18n'
import type { DeliveryEnvironment, DeliveryPipeline, DeliveryPipelineStageInput } from '@/types/delivery'
const props=defineProps<{projectId:number;environments:DeliveryEnvironment[];current:DeliveryPipeline|null}>(),emit=defineEmits<{published:[DeliveryPipeline]}>(),api=inject<DeliveryPipelineApi>('deliveryPipelineApi')!,{t}=useI18n(),stages=ref<DeliveryPipelineStageInput[]>([]),error=ref('')
const nextVersion=computed(()=>(props.current?.version??0)+1)
watch([()=>props.environments,()=>props.current],()=>{const configured=new Map(props.current?.stages.map(x=>[x.environment_id,x]));stages.value=props.environments.map(env=>{const old=configured.get(env.id);return{environment_id:env.id,approval_required:old?.approval_required??false,timeout:old?.timeout??'10m'}})},{immediate:true,deep:true})
function environmentName(id:number){return props.environments.find(x=>x.id===id)?.display_name??String(id)}
function move(index:number,delta:number){const target=index+delta;if(target<0||target>=stages.value.length)return;const copy=[...stages.value];[copy[index],copy[target]]=[copy[target]!,copy[index]!];stages.value=copy}
function seconds(value:string){const match=/^([1-9]\d*)(s|m|h)$/.exec(value);if(!match)return 0;return Number(match[1])*({s:1,m:60,h:3600}[match[2]!]??0)}
async function publish(){if(!stages.value.length){error.value=t('delivery.pipeline.empty');return}if(stages.value.some(x=>seconds(x.timeout)<60||seconds(x.timeout)>86400)){error.value=t('delivery.pipeline.timeout_invalid');return}error.value='';await new Promise<void>(resolve=>Modal.confirm({title:t('delivery.pipeline.publish_confirm',{version:nextVersion.value}),onOk:resolve}));emit('published',await api.publish(props.projectId,stages.value))}
defineExpose({stages,error,nextVersion,move,publish})
</script>
<style scoped>.stage-row{display:grid;grid-template-columns:1fr auto 120px auto auto;gap:8px;align-items:center;margin-bottom:8px}</style>
