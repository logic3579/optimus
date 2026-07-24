import { describe, expect, it } from 'vitest'
import seed from '../../../../../optimus-be/internal/seed/seed.go?raw'
import en from '../../../locales/en-US.json?raw'
import zh from '../../../locales/zh-CN.json?raw'
import types from '../../../types/observability.ts?raw'
const views = import.meta.glob('../**/*.vue', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const credentials = import.meta.glob('../../credentials/http-credentials/**/*.vue', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const apis = import.meta.glob('../../../api/observability/*.ts', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const allViews = { ...views, ...credentials }
const contracts = [
  ['../datasources/List.vue', 'canWrite', 'observability:datasource:write'],
  ['../datasources/List.vue', 'canDelete', 'observability:datasource:delete'],
  ['../dashboards/List.vue', 'canWrite', 'observability:dashboard:write'],
  ['../dashboards/List.vue', 'canDelete', 'observability:dashboard:delete'],
  ['../dashboards/Detail.vue', 'v-permission', 'observability:dashboard:read'],
  ['../kubernetes/Index.vue', 'canRead', 'observability:metric:read'],
] as const
describe('P5 structural permission/casing audit', () => {
  it.each(contracts)('%s binds %s to %s', (file, gate, code) => {
    const source = views[file] ?? ''
    expect(source).toContain(gate)
    expect(source).toContain(code)
  })
  it.each([
    ['credentials/http-credentials/List', 'credentials:http:read', '../../credentials/http-credentials/List.vue'],
    ['observability/kubernetes/Index', 'observability:metric:read', '../kubernetes/Index.vue'],
    ['observability/dashboards/List', 'observability:dashboard:read', '../dashboards/List.vue'],
    ['observability/datasources/List', 'observability:datasource:read', '../datasources/List.vue'],
  ] as const)('seed row %s resolves exact Linux file', (component, permission, file) => {
    const path = `/${component.replace('/List', '').replace('/Index', '')}`
    const row = seed.split('\n').find(line => line.includes(`Component: "${component}"`)) ?? ''
    expect(row).toContain(`Path: "${path}"`)
    expect(row).toContain(`PermissionCode: sp("${permission}")`)
    expect(allViews[file]).toBeTypeOf('string')
  })
  it('has no structured alert identifiers', () => {
    const source = [seed, en, zh, types, ...Object.values(allViews), ...Object.values(apis)].join('\n')
    expect(source).not.toMatch(/(?:Code|Path|Component|PermissionCode|interface|type|const|function)[^\n]*(?:observability.*alert|alert.*observability)/i)
  })
})
