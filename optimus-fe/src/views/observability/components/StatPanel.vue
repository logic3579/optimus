<template><PanelState v-if="state" :state="state" :message="message" /><div v-else class="stat">{{ display }}</div></template>
<script setup lang="ts">
import { computed } from 'vue';import type{NormalizedResult}from'@/types/observability';import{formatMetricValue}from'./chart-adapter';import PanelState from'./PanelState.vue'
const props=defineProps<{result?:NormalizedResult;unit:string;state?:'loading'|'empty'|'unsupported'|'partial'|'error';message?:string}>();const display=computed(()=>{const sample=props.result?.scalar??props.result?.series?.[0]?.samples.at(-1);return sample?formatMetricValue(Number(sample.value),props.unit):'—'})
</script>
