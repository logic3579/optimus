export function has(perms: ReadonlySet<string>, code: string): boolean {
  return perms.has(code)
}

export function hasAll(perms: ReadonlySet<string>, codes: readonly string[]): boolean {
  return codes.every(c => perms.has(c))
}

export function hasAny(perms: ReadonlySet<string>, codes: readonly string[]): boolean {
  return codes.some(c => perms.has(c))
}
