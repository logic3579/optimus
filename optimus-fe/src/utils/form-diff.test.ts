import { describe, it, expect } from 'vitest'
import { formDiff } from './form-diff'

describe('formDiff', () => {
  it('returns empty object when nothing changed', () => {
    const initial = { name: 'a', email: 'a@x', age: 30 }
    expect(formDiff(initial, { ...initial })).toEqual({})
  })

  it('returns only the changed keys', () => {
    const initial = { name: 'a', email: 'a@x' }
    const current = { name: 'a', email: 'b@x' }
    expect(formDiff(initial, current)).toEqual({ email: 'b@x' })
  })

  it('returns multiple changed keys', () => {
    const initial = { a: 1, b: 2, c: 3 }
    const current = { a: 1, b: 99, c: 100 }
    expect(formDiff(initial, current)).toEqual({ b: 99, c: 100 })
  })

  it('does not include keys present in initial but missing in current', () => {
    const initial = { a: 1, b: 2 }
    const current = { a: 1 } as { a: number; b?: number }
    expect(formDiff(initial, current)).toEqual({})
  })

  it('includes keys present in current but missing in initial', () => {
    const initial = {} as { extra?: string }
    const current = { extra: 'new' }
    expect(formDiff(initial, current)).toEqual({ extra: 'new' })
  })

  it('treats null and undefined as distinct', () => {
    const initial = { x: null as string | null }
    const current = { x: undefined as unknown as string | null }
    expect(formDiff(initial, current)).toEqual({ x: undefined })
  })

  it('uses Object.is — same value different reference is NOT a change', () => {
    const initial = { s: 'hello' }
    const current = { s: 'hello' }
    expect(formDiff(initial, current)).toEqual({})
  })

  it('treats nested objects by reference — pure shallow', () => {
    const a = { x: 1 }
    const initial = { obj: a }
    const current = { obj: { x: 1 } } // different reference, same content
    expect(formDiff(initial, current)).toEqual({ obj: { x: 1 } })
  })

  it('handles boolean and number primitives', () => {
    expect(formDiff({ on: true, n: 5 }, { on: false, n: 5 })).toEqual({ on: false })
    expect(formDiff({ on: true, n: 5 }, { on: true, n: 7 })).toEqual({ n: 7 })
  })
})
