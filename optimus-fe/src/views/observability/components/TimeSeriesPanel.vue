<template><PanelState v-if="state" :state="state" :message="message" /><div v-else ref="element" class="chart" /></template>
<script setup lang="ts">
import { inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { NormalizedResult } from '@/types/observability'
import { toChartOption } from './chart-adapter'
import PanelState from './PanelState.vue'
echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])
type Chart = { setOption(option: unknown): void; resize(): void; dispose(): void }
const props = defineProps<{ result?: NormalizedResult; state?: 'loading' | 'empty' | 'unsupported' | 'partial' | 'error'; message?: string }>()
const element = ref<HTMLElement>(); const factory = inject<(el: HTMLElement) => Chart>('chartFactory', el => echarts.init(el)); const observerFactory = inject<(cb: () => void) => { observe(el: Element): void; disconnect(): void }>('resizeObserverFactory', cb => new ResizeObserver(cb)); let chart: Chart | undefined; let observer: { observe(el: Element): void; disconnect(): void } | undefined
function render() { if (chart && props.result) chart.setOption(toChartOption(props.result)) }
async function ensureChart() { await nextTick(); if (chart || !element.value || props.state) return; chart = factory(element.value); render(); observer = observerFactory(() => chart?.resize()); observer.observe(element.value) }
function teardownChart() { observer?.disconnect(); observer = undefined; chart?.dispose(); chart = undefined }
onMounted(ensureChart)
watch(() => props.result, render, { deep: true })
watch(() => props.state, state => { if (state) teardownChart(); else void ensureChart() })
onBeforeUnmount(teardownChart)
</script>
<style scoped>.chart{height:260px;min-width:0}</style>
