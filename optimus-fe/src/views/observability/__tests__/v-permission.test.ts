import { describe, expect, it } from 'vitest'
import { NodeTypes, parse, type ElementNode, type RootNode, type TemplateChildNode } from '@vue/compiler-dom'
import seed from '../../../../../optimus-be/internal/seed/seed.go?raw'
import en from '../../../locales/en-US.json?raw'
import zh from '../../../locales/zh-CN.json?raw'
import types from '../../../types/observability.ts?raw'
const views = import.meta.glob('../**/*.vue', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const credentials = import.meta.glob('../../credentials/http-credentials/**/*.vue', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const apis = import.meta.glob('../../../api/observability/*.ts', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const allViews = { ...views, ...credentials }
function elements(source: string) {
  const out: ElementNode[] = []
  const visit = (node: RootNode | TemplateChildNode) => {
    if (node.type === NodeTypes.ELEMENT) { out.push(node); node.children.forEach(visit) }
    else if (node.type === NodeTypes.ROOT) node.children.forEach(visit)
    else if (node.type === NodeTypes.IF) node.branches.forEach(branch => branch.children.forEach(visit))
    else if (node.type === NodeTypes.FOR) node.children.forEach(visit)
  }
  visit(parse(source))
  return out
}
function hasGate(node: ElementNode, name: string, expression: string) {
  return node.props.some(prop => prop.type === NodeTypes.DIRECTIVE && prop.name === name && prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION && prop.exp.content.includes(expression))
}
function hasEvent(node: ElementNode, event: string, handler: string) {
  return node.props.some(prop => prop.type === NodeTypes.DIRECTIVE && prop.name === 'on' && prop.arg?.type === NodeTypes.SIMPLE_EXPRESSION && prop.arg.content === event && prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION && prop.exp.content.includes(handler))
}
function exactPermissionBinding(source: string, variable: string, code: string) {
  const escaped = code.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`\\b${variable}\\s*=\\s*computed\\(\\(\\)\\s*=>\\s*(?:permission|p)\\.has\\(['"]${escaped}['"]\\)\\)`).test(source)
}
describe('P5 structural permission/casing audit', () => {
  it.each([
    ['../datasources/List.vue', 'a-button', 'click', 'openCreate', 'canWrite', 'observability:datasource:write'],
    ['../datasources/List.vue', 'a', 'click', 'openEdit', 'canWrite', 'observability:datasource:write'],
    ['../datasources/List.vue', 'a', 'click', 'testDatasource', 'canTest', 'observability:datasource:write'],
    ['../datasources/List.vue', 'a-popconfirm', 'confirm', 'remove', 'canDelete', 'observability:datasource:delete'],
    ['../datasources/List.vue', 'a-modal', 'ok', 'save', 'canWrite', 'observability:datasource:write'],
    ['../dashboards/List.vue', 'a-button', 'click', 'openCreate', 'canWrite', 'observability:dashboard:write'],
    ['../dashboards/List.vue', 'a', 'click', 'openEdit', 'canWrite', 'observability:dashboard:write'],
    ['../dashboards/List.vue', 'a', 'click', 'openDetail', 'canMetricRead', 'observability:metric:read'],
    ['../dashboards/List.vue', 'a-popconfirm', 'confirm', 'remove', 'canDelete', 'observability:dashboard:delete'],
    ['../kubernetes/Index.vue', 'a-button', 'click', 'run', 'canRead', 'observability:metric:read'],
    ['../../credentials/http-credentials/List.vue', 'a-button', 'click', 'openCreate', 'canWrite', 'credentials:http:write'],
    ['../../credentials/http-credentials/List.vue', 'a', 'click', 'openEdit', 'canWrite', 'credentials:http:write'],
    ['../../credentials/http-credentials/List.vue', 'a-popconfirm', 'confirm', 'remove', 'canDelete', 'credentials:http:delete'],
  ] as const)('%s <%s> %s=%s has gate %s bound to exact %s', (file, tag, event, handler, gate, permission) => {
    const source = allViews[file] ?? ''
    expect(elements(source).some(node => node.tag === tag && hasEvent(node, event, handler) && hasGate(node, 'if', gate))).toBe(true)
    expect(exactPermissionBinding(source, gate, permission)).toBe(true)
  })
  it.each([
    ['../datasources/List.vue', 'canRead', 'observability:datasource:read'],
    ['../dashboards/List.vue', 'canRead', 'observability:dashboard:read'],
    ['../kubernetes/Index.vue', 'canRead', 'observability:metric:read'],
    ['../../credentials/http-credentials/List.vue', 'canRead', 'credentials:http:read'],
  ] as const)('%s view container uses %s bound to exact %s', (file, gate, permission) => {
    const source = allViews[file] ?? ''
    expect(elements(source).some(node => node.tag === 'a-card' && hasGate(node, 'if', gate))).toBe(true)
    expect(exactPermissionBinding(source, gate, permission)).toBe(true)
  })
  it('dashboard detail has the exact declarative read gate', () => {
    expect(elements(views['../dashboards/Detail.vue'] ?? '').some(node => node.tag === 'div' && hasGate(node, 'permission', 'observability:dashboard:read'))).toBe(true)
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
