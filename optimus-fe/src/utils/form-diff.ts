// Shallow diff between an initial form snapshot and the current form values.
// Returns a Partial<T> containing only the keys whose values changed.
//
// Used to compose PATCH/PUT bodies where the backend treats missing keys as
// "unchanged" (matches optimus-be Update DTOs which use *T pointer fields).
//
// Pure shallow: nested objects/arrays compared by reference via Object.is.
// Callers MUST keep their form models flat (matches all BE Update DTOs).
export function formDiff<T extends Record<string, unknown>>(
  initial: T,
  current: T
): Partial<T> {
  const out: Partial<T> = {}
  for (const key of Object.keys(current) as Array<keyof T>) {
    if (!Object.is(initial[key], current[key])) {
      out[key] = current[key]
    }
  }
  return out
}
