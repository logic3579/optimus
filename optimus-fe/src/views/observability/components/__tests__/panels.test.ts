import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PanelState from '../PanelState.vue'
import TimeSeriesPanel from '../TimeSeriesPanel.vue'
vi.mock('@/hooks/useI18n',()=>({useI18n:()=>({t:(key:string)=>({loading:'Loading',empty:'No data',unsupported:'Unsupported',partial:'Partial',error:'Error'}[key.split('.').at(-1) as 'loading']??key)})}))

describe('observability panels', () => {
  it.each([['loading', 'Loading'], ['empty', 'No data'], ['unsupported', 'Unsupported'], ['partial', 'Partial'], ['error', 'failed']] as const)('renders %s state', (state, text) => {
    expect(mount(PanelState, { props: { state, message: state === 'error' ? 'failed' : undefined } }).text()).toContain(text)
  })
  it('initializes, updates, resizes and disposes chart lifecycle', async () => {
    const chart = { setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() }
    let resize: (() => void) | undefined; const disconnect = vi.fn()
    const wrapper = mount(TimeSeriesPanel, { props: { result: { result_type: 'scalar', scalar: { timestamp: 1, value: '2' } } }, global: { provide: { chartFactory: () => chart, resizeObserverFactory: (callback: () => void) => ({ observe: vi.fn(), disconnect, callback: resize = callback }) } } })
    await vi.waitFor(() => expect(chart.setOption).toHaveBeenCalledOnce())
    resize?.(); expect(chart.resize).toHaveBeenCalledOnce()
    await wrapper.setProps({ result: { result_type: 'scalar', scalar: { timestamp: 2, value: '3' } } })
    expect(chart.setOption).toHaveBeenCalledTimes(2)
    wrapper.unmount(); expect(chart.dispose).toHaveBeenCalledOnce(); expect(disconnect).toHaveBeenCalledOnce()
  })
  it('initializes only after loading becomes ready', async () => {
    const chart = { setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() }; const factory = vi.fn(() => chart)
    const wrapper = mount(TimeSeriesPanel, { props: { state: 'loading', result: { result_type: 'scalar', scalar: { timestamp: 1, value: '2' } } }, global: { provide: { chartFactory: factory, resizeObserverFactory: () => ({ observe: vi.fn(), disconnect: vi.fn() }) } } })
    expect(factory).not.toHaveBeenCalled(); await wrapper.setProps({ state: undefined }); await vi.waitFor(() => expect(factory).toHaveBeenCalledOnce())
    wrapper.unmount(); expect(chart.dispose).toHaveBeenCalledOnce()
  })
  it('tears down a detached chart and reinitializes after a state round trip', async () => {
    const charts = [0, 1].map(() => ({ setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() }))
    const disconnects = [vi.fn(), vi.fn()]
    const factory = vi.fn(() => charts[factory.mock.calls.length - 1]!)
    let observerIndex = 0
    const wrapper = mount(TimeSeriesPanel, { props: { result: { result_type: 'scalar', scalar: { timestamp: 1, value: '2' } } }, global: { provide: { chartFactory: factory, resizeObserverFactory: () => ({ observe: vi.fn(), disconnect: disconnects[observerIndex++]! }) } } })
    await vi.waitFor(() => expect(factory).toHaveBeenCalledOnce())
    await wrapper.setProps({ state: 'error' })
    expect(charts[0]!.dispose).toHaveBeenCalledOnce()
    expect(disconnects[0]).toHaveBeenCalledOnce()
    await wrapper.setProps({ state: undefined })
    await vi.waitFor(() => expect(factory).toHaveBeenCalledTimes(2))
    wrapper.unmount()
    expect(charts[1]!.dispose).toHaveBeenCalledOnce()
    expect(disconnects[1]).toHaveBeenCalledOnce()
  })
})
