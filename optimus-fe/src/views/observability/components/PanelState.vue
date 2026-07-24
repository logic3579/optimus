<template><div class="panel-state" :data-state="state">{{ text }}</div></template>
<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ state: 'loading' | 'empty' | 'unsupported' | 'partial' | 'error'; message?: string }>()
const text = computed(() => props.message || ({ loading: 'Loading', empty: 'No data', unsupported: 'Unsupported', partial: 'Partial', error: 'Error' }[props.state]))
</script>
