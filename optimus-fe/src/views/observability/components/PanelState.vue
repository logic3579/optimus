<template><div class="panel-state" :data-state="state">{{ text }}</div></template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from '@/hooks/useI18n'
const props = defineProps<{ state: 'loading' | 'empty' | 'unsupported' | 'partial' | 'error'; message?: string }>()
const { t } = useI18n()
const text = computed(() => props.message || t(`observability.panel_state.${props.state}`))
</script>
