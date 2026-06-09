import { describe, it, expect } from 'vitest'
import { auditKeys, extractUsedKeys, flattenKeys } from './check-i18n-keys'

describe('extractUsedKeys', () => {
  it('finds $t / i18n.t / bare t(...) calls', () => {
    const src = `
      const x = $t('a.b')
      const y = i18n.t("c.d")
      const z = t('e.f')
      // $t('not.this') is in a comment but still matches by design (caught at runtime)
    `
    expect(extractUsedKeys('f.vue', src).sort()).toEqual(['a.b', 'c.d', 'e.f', 'not.this'].sort())
  })

  it('ignores non-string-literal call shapes', () => {
    const src = `$t(variable); t(x + 'y')`
    expect(extractUsedKeys('f.vue', src)).toEqual([])
  })
})

describe('flattenKeys', () => {
  it('walks nested objects into dot keys', () => {
    expect(flattenKeys({ a: { b: 'x', c: { d: 'y' } } }).sort()).toEqual(['a.b', 'a.c.d'])
  })
})

describe('auditKeys', () => {
  it('reports missing keys when used > declared', () => {
    const r = auditKeys({
      sources: [{ path: 'a.vue', usedKeys: ['x.y', 'x.z'] }],
      zhCN: { x: { y: 'present' } },
      enUS: { x: { y: 'present' } }
    })
    expect(r.missingFromZh).toContain('x.z')
  })

  it('reports zh/en symmetric diff', () => {
    const r = auditKeys({
      sources: [],
      zhCN: { a: '1' },
      enUS: { a: '1', b: '2' }
    })
    expect(r.zhEnMismatch).toContain('b')
  })

  it('no issues → empty arrays', () => {
    const r = auditKeys({
      sources: [{ path: 'a.vue', usedKeys: ['a'] }],
      zhCN: { a: '1' },
      enUS: { a: '1' }
    })
    expect(r.missingFromZh).toEqual([])
    expect(r.zhEnMismatch).toEqual([])
  })

  it('unused keys go to warnings, not errors', () => {
    const r = auditKeys({
      sources: [{ path: 'a.vue', usedKeys: [] }],
      zhCN: { a: '1' },
      enUS: { a: '1' }
    })
    expect(r.unused).toContain('a')
  })
})
