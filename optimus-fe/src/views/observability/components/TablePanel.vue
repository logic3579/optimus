<template><PanelState v-if="state" :state="state" :message="message" /><a-table v-else :columns="columns" :data-source="rows" :pagination="false" /></template>
<script setup lang="ts">
import{computed}from'vue';import type{NormalizedResult}from'@/types/observability';import PanelState from'./PanelState.vue';const props=defineProps<{result?:NormalizedResult;state?:'loading'|'empty'|'unsupported'|'partial'|'error';message?:string}>();const columns=[{title:'Labels',dataIndex:'labels'},{title:'Value',dataIndex:'value'}];const rows=computed(()=>props.result?.series?.map((x,i)=>({key:i,labels:Object.entries(x.labels).map(([k,v])=>`${k}=${v}`).join(', '),value:x.samples.at(-1)?.value??'—'}))??[])
</script>
