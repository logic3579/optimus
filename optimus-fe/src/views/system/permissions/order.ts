export const permissionCategoryOrder = [
  'assets',
  'k8s',
  'apps',
  'delivery',
  'observability',
  'credentials',
  'system'
] as const

const categoryRank = new Map<string, number>(
  permissionCategoryOrder.map((category, index) => [category, index])
)

export function comparePermissionCategories(a: string, b: string): number {
  const aRank = categoryRank.get(a) ?? permissionCategoryOrder.length
  const bRank = categoryRank.get(b) ?? permissionCategoryOrder.length
  return aRank - bRank || a.localeCompare(b)
}
