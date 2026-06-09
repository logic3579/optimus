import { describe, it, expect } from 'vitest'
import { computeDropTarget, isDescendant } from './computeDropTarget'
import type { MenuNode } from '@/types/api'

function makeTree(): MenuNode[] {
  // root1 (id=1, sort=10)
  //   child11 (id=11, sort=10)
  //   child12 (id=12, sort=20)
  // root2 (id=2, sort=20)
  return [
    {
      id: 1, code: 'r1', name: 'r1', path: '', component: '', icon: '',
      sort_order: 10, hidden: false, parent_id: null,
      children: [
        { id: 11, code: 'c11', name: 'c11', path: '', component: '', icon: '', sort_order: 10, hidden: false, parent_id: 1 },
        { id: 12, code: 'c12', name: 'c12', path: '', component: '', icon: '', sort_order: 20, hidden: false, parent_id: 1 }
      ]
    },
    { id: 2, code: 'r2', name: 'r2', path: '', component: '', icon: '', sort_order: 20, hidden: false, parent_id: null }
  ]
}

describe('isDescendant', () => {
  it('returns true if target is direct child', () => {
    const tree = makeTree()
    expect(isDescendant(tree[0]!, 11)).toBe(true)
  })
  it('returns true for self', () => {
    const tree = makeTree()
    expect(isDescendant(tree[0]!, 1)).toBe(true)
  })
  it('returns false for unrelated node', () => {
    const tree = makeTree()
    expect(isDescendant(tree[0]!, 2)).toBe(false)
  })
})

describe('computeDropTarget', () => {
  it('dropping INSIDE a node sets that node as parent (dropToGap=false)', () => {
    const tree = makeTree()
    const result = computeDropTarget(tree, /*dragId*/ 11, /*dropId*/ 2, /*dropPos*/ 0, /*dropToGap*/ false)
    expect(result).toEqual({ parent_id: 2, sort_order: 10 }) // first/only child of r2
  })

  it('dropping to gap BEFORE a root puts node at same parent with sort_order < first', () => {
    const tree = makeTree()
    // drop root2 before root1: pos -1 dropToGap=true
    const result = computeDropTarget(tree, 2, 1, -1, true)
    expect(result.parent_id).toBeNull()
    expect(result.sort_order).toBeLessThan(10)
  })

  it('dropping to gap AFTER a sibling puts node at same parent with sort_order between siblings', () => {
    const tree = makeTree()
    // drop c11 after c11 (same level): parent=1, between 10 and 20 → 15
    const result = computeDropTarget(tree, 12, 11, 1, true)
    expect(result).toEqual({ parent_id: 1, sort_order: 15 })
  })

  it('dropping INSIDE moves the node to be a child of the drop target', () => {
    const tree = makeTree()
    // drop r2 inside c11
    const result = computeDropTarget(tree, 2, 11, 0, false)
    expect(result.parent_id).toBe(11)
    expect(result.sort_order).toBeGreaterThanOrEqual(10)
  })

  it('throws when dropping onto own descendant', () => {
    const tree = makeTree()
    expect(() => computeDropTarget(tree, 1, 11, 0, false)).toThrow(/descendant/i)
  })

  it('throws when dropping onto self', () => {
    const tree = makeTree()
    expect(() => computeDropTarget(tree, 1, 1, 0, false)).toThrow(/descendant/i)
  })
})
