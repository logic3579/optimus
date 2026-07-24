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
describe('P5 structural permission/casing audit', () => {
  it.each([
    ['../datasources/List.vue', 'a-button', 'if', 'canWrite'], ['../datasources/List.vue', 'a-popconfirm', 'if', 'canDelete'],
    ['../dashboards/List.vue', 'a-button', 'if', 'canWrite'], ['../dashboards/List.vue', 'a-popconfirm', 'if', 'canDelete'],
    ['../dashboards/List.vue', 'a', 'if', 'canMetricRead'], ['../kubernetes/Index.vue', 'a-card', 'if', 'canRead'],
    ['../dashboards/Detail.vue', 'div', 'permission', 'observability:dashboard:read'],
  ] as const)('%s <%s> binds v-%s=%s', (file, tag, directive, expression) => {
    expect(elements(views[file] ?? '').some(node => node.tag === tag && hasGate(node, directive, expression))).toBe(true)
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
