import { describe, expect, it } from 'vitest'
import type { NormalizedResult } from '@/types/observability'
import { formatMetricValue, toChartOption } from './chart-adapter'

describe('chart adapter', () => {
  it('converts matrix series with stable label legends/order without mutation', () => {
    const input: NormalizedResult = { result_type: 'matrix', series: [
      { labels: { pod: 'z', ns: 'b' }, samples: [{ timestamp: 2, value: '2' }] },
      { labels: { ns: 'a', pod: 'a' }, samples: [{ timestamp: 1, value: '1' }] },
    ] }
    const before = structuredClone(input)
    const option = toChartOption(input)
    expect(option.series.map(x => x.name)).toEqual(['ns=a, pod=a', 'ns=b, pod=z'])
    expect(option.series[0]?.data).toEqual([[1000, 1]])
    expect(input).toEqual(before)
  })
  it('converts vector and scalar and treats special values as gaps', () => {
    expect(toChartOption({ result_type: 'vector', series: [{ labels: { job: 'api' }, samples: [{ timestamp: 1, value: 'NaN' }, { timestamp: 2, value: '+Inf' }] }] }).series[0]?.data).toEqual([[1000, null], [2000, null]])
    expect(toChartOption({ result_type: 'scalar', scalar: { timestamp: 3, value: '4.5' } }).series[0]?.data).toEqual([[3000, 4.5]])
    expect(toChartOption({ result_type: 'string', text: 'up' }).unsupported).toBe(true)
  })
  it.each([['percent', .42, '42%'], ['bytes', 1536, '1.5 KiB'], ['cores', .25, '250m'], ['seconds', 90, '1m 30s'], ['rate', 12.3, '12.3/s']])('formats %s units', (unit, value, output) => {
    expect(formatMetricValue(value as number, unit as string)).toBe(output)
  })
})
