import type { MenuNode } from '@/types/api'

export interface DropTarget {
  parent_id: number | null
  sort_order: number
}

const STEP = 10
const BEFORE_FIRST_DELTA = 5

// Walk tree to find a node by id.
function findNode(roots: MenuNode[], id: number): MenuNode | null {
  for (const r of roots) {
    if (r.id === id) return r
    if (r.children) {
      const found = findNode(r.children, id)
      if (found) return found
    }
  }
  return null
}

// Returns the parent node of `id`, or null if it's a root.
function findParent(roots: MenuNode[], id: number): MenuNode | null {
  for (const r of roots) {
    if (r.children?.some(c => c.id === id)) return r
    if (r.children) {
      const found = findParent(r.children, id)
      if (found) return found
    }
  }
  return null
}

// Returns the sibling list containing `id` and `id`'s index within it.
function findSiblings(roots: MenuNode[], id: number): { siblings: MenuNode[]; index: number } {
  const parent = findParent(roots, id)
  const siblings = parent ? (parent.children ?? []) : roots
  return { siblings, index: siblings.findIndex(n => n.id === id) }
}

// True if `targetId` equals `node` or appears anywhere in `node`'s subtree.
export function isDescendant(node: MenuNode, targetId: number): boolean {
  if (node.id === targetId) return true
  if (!node.children) return false
  return node.children.some(c => isDescendant(c, targetId))
}

// Compute the {parent_id, sort_order} the dragged node should have after the drop.
//
// Args mirror antdv tree onDrop event semantics:
//   - dragId:     id of the node being dragged
//   - dropId:     id of the node the user dropped onto
//   - dropPos:    antdv "dropPosition" (-1=before first sibling, 0=inside, +1=after target)
//                 NB: antdv's actual sign convention is "dropPosition === -1 means insert before
//                 the first child of the parent" — we treat any negative number as "before",
//                 any positive number as "after", 0 as "inside".
//   - dropToGap:  true = dropped onto a gap (sibling-level), false = dropped into the node body
export function computeDropTarget(
  tree: MenuNode[],
  dragId: number,
  dropId: number,
  dropPos: number,
  dropToGap: boolean
): DropTarget {
  const dragNode = findNode(tree, dragId)
  if (!dragNode) throw new Error('drag node not found')
  const dropNode = findNode(tree, dropId)
  if (!dropNode) throw new Error('drop node not found')

  if (isDescendant(dragNode, dropId)) {
    throw new Error('cannot drop onto own descendant')
  }

  if (!dropToGap) {
    // Drop INSIDE dropNode → becomes a child of dropNode at tail
    const children = dropNode.children ?? []
    const maxSort = children.length === 0 ? 0 : Math.max(...children.map(c => c.sort_order))
    return { parent_id: dropNode.id, sort_order: maxSort + STEP }
  }

  // Drop to gap → same parent as dropNode, position relative to dropNode
  const { siblings, index: dropIndex } = findSiblings(tree, dropId)
  const dropParent = findParent(tree, dropId)
  const parent_id = dropParent ? dropParent.id : null

  if (dropPos < 0) {
    // Before first sibling — sort_order = first.sort_order - BEFORE_FIRST_DELTA
    const first = siblings[0]
    const sort_order = first ? first.sort_order - BEFORE_FIRST_DELTA : STEP
    return { parent_id, sort_order }
  }

  // After dropNode: average with next sibling, or +STEP if it's the last
  const next = siblings[dropIndex + 1]
  if (next) {
    const sort_order = Math.floor((dropNode.sort_order + next.sort_order) / 2)
    // If they're adjacent integers (1 apart), we'd average to dropNode's value; fall back to +STEP.
    if (sort_order === dropNode.sort_order || sort_order === next.sort_order) {
      return { parent_id, sort_order: dropNode.sort_order + 1 }
    }
    return { parent_id, sort_order }
  }
  return { parent_id, sort_order: dropNode.sort_order + STEP }
}
