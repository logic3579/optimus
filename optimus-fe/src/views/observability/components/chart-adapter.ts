import type { NormalizedResult, Series } from '@/types/observability'

export interface ChartSeries { name: string; type: 'line'; showSymbol: false; data: [number, number | null][] }
export interface ChartOption { tooltip: { trigger: 'axis' }; legend: { data: string[] }; xAxis: { type: 'time' }; yAxis: { type: 'value' }; series: ChartSeries[]; unsupported?: boolean }

function legend(series: Series, index: number): string {
  const labels = Object.entries(series.labels).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `${key}=${value}`)
  return labels.join(', ') || `series ${index + 1}`
}
function value(raw: string): number | null {
  if (raw === 'NaN' || raw === '+Inf' || raw === '-Inf') return null
  const parsed = Number(raw)
  return Number.isFinite(parsed) ? parsed : null
}
export function toChartOption(result: NormalizedResult): ChartOption {
  const source = result.result_type === 'scalar' && result.scalar ? [{ labels: {}, samples: [result.scalar] }] : result.series ?? []
  const series = source.map((item, index) => ({
    name: legend(item, index), type: 'line' as const, showSymbol: false as const,
    data: item.samples.map(sample => [sample.timestamp * 1000, value(sample.value)] as [number, number | null]),
  })).sort((a, b) => a.name.localeCompare(b.name))
  return { tooltip: { trigger: 'axis' }, legend: { data: series.map(item => item.name) }, xAxis: { type: 'time' }, yAxis: { type: 'value' }, series, ...(result.result_type === 'string' ? { unsupported: true } : {}) }
}
export function formatMetricValue(value: number, unit: string): string {
  if (!Number.isFinite(value)) return '—'
  if (unit === 'percent') return `${Number((value * 100).toFixed(2))}%`
  if (unit === 'bytes') { const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; let n = value; let i = 0; while (Math.abs(n) >= 1024 && i < units.length - 1) { n /= 1024; i++ } return `${Number(n.toFixed(2))} ${units[i]}` }
  if (unit === 'cores') return value < 1 ? `${Number((value * 1000).toFixed(2))}m` : `${Number(value.toFixed(2))}`
  if (unit === 'seconds') { const minutes = Math.floor(value / 60); const seconds = Math.round(value % 60); return minutes ? `${minutes}m ${seconds}s` : `${seconds}s` }
  if (unit === 'rate') return `${Number(value.toFixed(2))}/s`
  return `${Number(value.toFixed(2))}`
}
