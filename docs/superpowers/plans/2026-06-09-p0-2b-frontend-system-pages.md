# P0 Plan 2b — Frontend system pages (CRUD) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 5 Coming-soon placeholders in `src/views/system/{users,roles,menus,permissions,audit-logs}/List.vue` with real CRUD wired to the stable `/api/v1` contracts, extending `useTable` with filters, abstracting a `form-diff` util shared with `profile/Index.vue`, and finishing the P0 release with a single `dev → main` merge.

**Architecture:** Pure additive on top of 2a-shipped skeleton. Every page follows the same shape: `PageHeader → filter row → a-table` (or `a-tree` for menus) `+ modals per CRUD action`. State stays per-component (no URL sync). API modules mirror BE DTOs 1:1, hand-written. TDD restricted to pure logic per 2a decision #8 — Vue components are not test-driven.

**Tech Stack:** Vue 3.4 SFC + setup-store · ant-design-vue 4 · TypeScript 5 strict · axios (single-flight refresh already in place) · Pinia + pinia-plugin-persistedstate · vue-i18n 9 · vitest + jsdom · bun ≥ 1.1.

**Spec references:**
- `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md` §6, §7, §12
- `docs/superpowers/specs/2026-06-08-p0-plan2a-fe-design-addendum.md` (binding decisions inherited)
- `docs/superpowers/specs/2026-06-09-p0-plan2b-fe-design-addendum.md` (binding 2b decisions)
- BE DTOs: `optimus-be/internal/modules/{user,role,menu,audit,permission}/dto.go`
- BE pagination contract: `optimus-be/internal/infra/pagination/pagination.go` — query `?page=&page_size=`, response `{ items, total, page, page_size }`

**Implementation notes for the agent:**
- All commands run from repo root unless noted. Frontend tasks use `cd optimus-fe` (or `git -C /Users/logic/Projects/optimus`).
- Use `bun` everywhere (never `npm`/`pnpm`/`yarn`). User memory: `feedback_node_package_manager`.
- Working tree root = `/Users/logic/Projects/optimus`. Frontend = `optimus-fe/`, backend = `optimus-be/`.
- Branch: `dev` — all commits land on `dev`. P0 release happens at Task 16 (single `dev → main` PR).
- One task = one commit unless explicitly noted (multi-modal tasks may produce 1-3 commits, declared in step list).
- Every batch ends with the FULL sweep: `cd optimus-fe && bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build`. If a task's explicit verify step omits one, still run the full sweep at batch end.
- BE is not modified in this plan. If a BE gap is discovered (endpoint not working, DTO drift), STOP and add BE tasks at the top per the 2a pattern.

---

## File Structure Overview

**Create (17 files)**
```
optimus-fe/src/api/user.ts
optimus-fe/src/api/role.ts
optimus-fe/src/api/menu.ts
optimus-fe/src/api/permission.ts
optimus-fe/src/api/audit.ts
optimus-fe/src/utils/form-diff.ts
optimus-fe/src/utils/form-diff.test.ts
optimus-fe/src/views/system/menus/computeDropTarget.ts
optimus-fe/src/views/system/menus/computeDropTarget.test.ts
optimus-fe/src/views/system/users/components/UserEditModal.vue
optimus-fe/src/views/system/users/components/UserRolesModal.vue
optimus-fe/src/views/system/users/components/UserResetPasswordModal.vue
optimus-fe/src/views/system/roles/components/RoleEditModal.vue
optimus-fe/src/views/system/roles/components/RolePermissionsModal.vue
optimus-fe/src/views/system/menus/components/MenuEditModal.vue
```

**Modify**
```
optimus-fe/src/types/api.ts                              (+ 5-module DTOs + PageResp<T>)
optimus-fe/src/hooks/useTable.ts                         (+ filters + TDD cases)
optimus-fe/src/hooks/useTable.test.ts
optimus-fe/src/locales/zh-CN.json                        (+ 5-module keys + form.* + perm.category.*)
optimus-fe/src/locales/en-US.json                        (same key set, en text)
optimus-fe/src/views/profile/Index.vue                   (refactor to use form-diff)
optimus-fe/src/views/system/users/List.vue               (full rewrite)
optimus-fe/src/views/system/roles/List.vue               (full rewrite)
optimus-fe/src/views/system/menus/List.vue               (full rewrite)
optimus-fe/src/views/system/permissions/List.vue         (full rewrite)
optimus-fe/src/views/system/audit-logs/List.vue          (full rewrite)
optimus-fe/src/main.ts                                   (provide() new 5 api modules)
```

---

## Subagent Batching

| Batch | Tasks | Focus |
|---|---|---|
| 1 | 1, 2, 3 | DTOs + 5 API modules |
| 2 | 4, 5, 6 | form-diff TDD · useTable filters TDD · i18n keys |
| 3 | 7, 8, 9 | profile refactor · 3 user modals · users page |
| 4 | 10, 11 | 2 role modals · roles page |
| 5 | 12, 13 | menu drop logic + modal · menus page |
| 6 | 14, 15, 16 | permissions · audit-logs · §12 sweep + P0 release prep |

---

## Task 1: Extend src/types/api.ts with 5-module DTOs

**Files:**
- Modify: `optimus-fe/src/types/api.ts`

- [ ] **Step 1: Append DTOs and PageResp**

Open `optimus-fe/src/types/api.ts`. Append after the existing `ChangePasswordRequest` interface (line 63) — keep all existing content above untouched:

```ts

// ─── Pagination envelope (used by all paginated list endpoints) ─────────────
export interface PageResp<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

// ─── Users ──────────────────────────────────────────────────────────────────
export interface UserSummary {
  id: number
  username: string
  email: string
  display_name: string
  status: 'enabled' | 'disabled'
  last_login_at?: string | null
  created_at: string
}
export interface UserRoleRef {
  id: number
  code: string
  name: string
}
export interface UserDetail extends UserSummary {
  avatar_url: string
  roles: UserRoleRef[]
}
export interface UserCreateRequest {
  username: string
  email: string
  password: string
  display_name?: string
  role_ids?: number[]
}
export interface UserUpdateRequest {
  email?: string
  display_name?: string
  avatar_url?: string
}
export interface UserSetRolesRequest {
  role_ids: number[]
}
export interface UserSetStatusRequest {
  status: 'enabled' | 'disabled'
}
export interface UserSetPasswordRequest {
  password: string
}
export interface UserListQuery {
  search?: string
  status?: 'enabled' | 'disabled' | ''
}

// ─── Roles ──────────────────────────────────────────────────────────────────
export interface RoleSummary {
  id: number
  code: string
  name: string
  description: string
  is_builtin: boolean
  created_at: string
}
export interface RoleDetail extends RoleSummary {
  permission_codes: string[]
}
export interface RoleCreateRequest {
  code: string
  name: string
  description?: string
}
export interface RoleUpdateRequest {
  name?: string
  description?: string
}
export interface RoleSetPermissionsRequest {
  permission_codes: string[]
}

// ─── Menus ──────────────────────────────────────────────────────────────────
export interface MenuNode {
  id: number
  parent_id?: number | null
  code: string
  name: string
  path: string
  component: string
  icon: string
  permission_code?: string | null
  sort_order: number
  hidden: boolean
  children?: MenuNode[]
}
export interface MenuCreateRequest {
  parent_id?: number | null
  code: string
  name: string
  path?: string
  component?: string
  icon?: string
  permission_code?: string | null
  sort_order?: number
  hidden?: boolean
}
export interface MenuUpdateRequest {
  parent_id?: number | null
  name?: string
  path?: string
  component?: string
  icon?: string
  permission_code?: string | null
  sort_order?: number
  hidden?: boolean
}

// ─── Permissions ────────────────────────────────────────────────────────────
export interface Permission {
  id: number
  code: string
  name: string
  category: string
  description: string
}

// ─── Audit logs ─────────────────────────────────────────────────────────────
export interface AuditLogEntry {
  id: number
  user_id?: number | null
  action: string
  target_type?: string
  target_id?: string
  payload: unknown
  ip?: string
  user_agent?: string
  created_at: string
}
export interface AuditListQuery {
  action?: string
  user_id?: number
  start?: string  // RFC3339
  end?: string    // RFC3339
}
```

- [ ] **Step 2: Typecheck**

```bash
cd optimus-fe && bun run typecheck
```

Expected: PASS, no errors.

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/types/api.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/types): add user/role/menu/permission/audit DTOs + PageResp<T>"
```

---

## Task 2: src/api/user.ts

**Files:**
- Create: `optimus-fe/src/api/user.ts`

User endpoints are the biggest API surface in 2b (8 methods). Pattern matches existing `api/me.ts` and `api/auth.ts` — a factory taking an `AxiosInstance` and returning a typed object.

- [ ] **Step 1: Write user.ts**

```ts
import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  UserSummary, UserDetail,
  UserCreateRequest, UserUpdateRequest,
  UserSetRolesRequest, UserSetStatusRequest, UserSetPasswordRequest,
  UserListQuery
} from '@/types/api'

export interface UserListParams extends UserListQuery {
  page: number
  page_size: number
}

export function makeUserApi(client: AxiosInstance) {
  return {
    list: async (params: UserListParams) => {
      const r = await client.get<Envelope<PageResp<UserSummary>>>('/users', { params })
      return r.data.data
    },
    create: async (body: UserCreateRequest) => {
      const r = await client.post<Envelope<UserDetail>>('/users', body)
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<UserDetail>>(`/users/${id}`)
      return r.data.data
    },
    update: async (id: number, body: UserUpdateRequest) => {
      const r = await client.put<Envelope<UserDetail>>(`/users/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`/users/${id}`)
    },
    setRoles: async (id: number, body: UserSetRolesRequest) => {
      await client.put<Envelope<null>>(`/users/${id}/roles`, body)
    },
    setStatus: async (id: number, body: UserSetStatusRequest) => {
      await client.put<Envelope<null>>(`/users/${id}/status`, body)
    },
    setPassword: async (id: number, body: UserSetPasswordRequest) => {
      await client.put<Envelope<null>>(`/users/${id}/password`, body)
    }
  }
}

export type UserApi = ReturnType<typeof makeUserApi>
```

- [ ] **Step 2: Typecheck**

```bash
cd optimus-fe && bun run typecheck
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/api/user.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/api): user module — list/create/get/update/remove/setRoles/setStatus/setPassword"
```

---

## Task 3: src/api/{role,menu,permission,audit}.ts + main.ts provide() wiring

**Files:**
- Create: `optimus-fe/src/api/role.ts`
- Create: `optimus-fe/src/api/menu.ts`
- Create: `optimus-fe/src/api/permission.ts`
- Create: `optimus-fe/src/api/audit.ts`
- Modify: `optimus-fe/src/main.ts` (provide 5 new api modules)

Smaller modules grouped into one task. After this, all 5 new APIs are reachable via `inject()` in components.

- [ ] **Step 1: Write role.ts**

```ts
import type { AxiosInstance } from 'axios'
import type {
  Envelope,
  RoleSummary, RoleDetail,
  RoleCreateRequest, RoleUpdateRequest, RoleSetPermissionsRequest
} from '@/types/api'

export function makeRoleApi(client: AxiosInstance) {
  return {
    list: async () => {
      const r = await client.get<Envelope<RoleSummary[]>>('/roles')
      return r.data.data
    },
    create: async (body: RoleCreateRequest) => {
      const r = await client.post<Envelope<RoleDetail>>('/roles', body)
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<RoleDetail>>(`/roles/${id}`)
      return r.data.data
    },
    update: async (id: number, body: RoleUpdateRequest) => {
      const r = await client.put<Envelope<RoleDetail>>(`/roles/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`/roles/${id}`)
    },
    setPermissions: async (id: number, body: RoleSetPermissionsRequest) => {
      await client.put<Envelope<null>>(`/roles/${id}/permissions`, body)
    }
  }
}

export type RoleApi = ReturnType<typeof makeRoleApi>
```

- [ ] **Step 2: Write menu.ts**

```ts
import type { AxiosInstance } from 'axios'
import type {
  Envelope,
  MenuNode, MenuCreateRequest, MenuUpdateRequest
} from '@/types/api'

export function makeMenuApi(client: AxiosInstance) {
  return {
    list: async () => {
      const r = await client.get<Envelope<MenuNode[]>>('/menus')
      return r.data.data
    },
    create: async (body: MenuCreateRequest) => {
      const r = await client.post<Envelope<MenuNode>>('/menus', body)
      return r.data.data
    },
    update: async (id: number, body: MenuUpdateRequest) => {
      const r = await client.put<Envelope<MenuNode>>(`/menus/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`/menus/${id}`)
    }
  }
}

export type MenuApi = ReturnType<typeof makeMenuApi>
```

- [ ] **Step 3: Write permission.ts**

```ts
import type { AxiosInstance } from 'axios'
import type { Envelope, Permission } from '@/types/api'

export function makePermissionApi(client: AxiosInstance) {
  return {
    list: async () => {
      const r = await client.get<Envelope<Permission[]>>('/permissions')
      return r.data.data
    }
  }
}

export type PermissionApi = ReturnType<typeof makePermissionApi>
```

- [ ] **Step 4: Write audit.ts**

```ts
import type { AxiosInstance } from 'axios'
import type {
  Envelope, PageResp,
  AuditLogEntry, AuditListQuery
} from '@/types/api'

export interface AuditListParams extends AuditListQuery {
  page: number
  page_size: number
}

export function makeAuditApi(client: AxiosInstance) {
  return {
    list: async (params: AuditListParams) => {
      const r = await client.get<Envelope<PageResp<AuditLogEntry>>>('/audit-logs', { params })
      return r.data.data
    }
  }
}

export type AuditApi = ReturnType<typeof makeAuditApi>
```

- [ ] **Step 5: Wire into main.ts**

Read `optimus-fe/src/main.ts`. Locate the section that builds `meApi` / `authApi` and calls `app.provide('meApi', ...)` etc. Add the new 5 API factory calls and provides alongside them. The new imports go with the existing `@/api/*` imports.

Add imports:

```ts
import { makeUserApi } from '@/api/user'
import { makeRoleApi } from '@/api/role'
import { makeMenuApi } from '@/api/menu'
import { makePermissionApi } from '@/api/permission'
import { makeAuditApi } from '@/api/audit'
```

Where the existing `provide` calls are, append:

```ts
app.provide('userApi', makeUserApi(client))
app.provide('roleApi', makeRoleApi(client))
app.provide('menuApi', makeMenuApi(client))
app.provide('permissionApi', makePermissionApi(client))
app.provide('auditApi', makeAuditApi(client))
```

- [ ] **Step 6: Typecheck + lint**

```bash
cd optimus-fe && bun run typecheck && bun run lint
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/api/role.ts optimus-fe/src/api/menu.ts optimus-fe/src/api/permission.ts optimus-fe/src/api/audit.ts optimus-fe/src/main.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/api): role/menu/permission/audit modules + main.ts provide wiring"
```

---

## Task 4: src/utils/form-diff.ts (TDD)

**Files:**
- Create: `optimus-fe/src/utils/form-diff.ts`
- Create: `optimus-fe/src/utils/form-diff.test.ts`

Pure utility — shallow diff that produces a partial body for PATCH/PUT requests. Used by `UserEditModal` (Task 9) and refactored `profile/Index.vue` (Task 7).

- [ ] **Step 1: Write failing tests**

`optimus-fe/src/utils/form-diff.test.ts`:

```ts
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
```

- [ ] **Step 2: Run, verify fail**

```bash
cd optimus-fe && bun run test src/utils/form-diff.test.ts
```

Expected: FAIL — module './form-diff' not found.

- [ ] **Step 3: Implement form-diff.ts**

`optimus-fe/src/utils/form-diff.ts`:

```ts
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
```

- [ ] **Step 4: Run, verify pass**

```bash
cd optimus-fe && bun run test src/utils/form-diff.test.ts
```

Expected: 9 tests PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/utils/form-diff.ts optimus-fe/src/utils/form-diff.test.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/utils): form-diff shallow diff for PATCH-style request bodies (TDD)"
```

---

## Task 5: Extend src/hooks/useTable.ts with filters (TDD)

**Files:**
- Modify: `optimus-fe/src/hooks/useTable.ts`
- Modify: `optimus-fe/src/hooks/useTable.test.ts`

Add a `filters` ref + `setFilters(patch)` method. `setFilters` always resets page to 1 (UX consensus). Generic in two parameters `<T, F>` where `F` defaults to `Record<string, unknown>` so existing call sites (none in production code yet) compile unchanged.

- [ ] **Step 1: Add new failing tests**

Append to `optimus-fe/src/hooks/useTable.test.ts` BEFORE the closing `})`. Read the existing file first so the indentation matches.

```ts

  // ─── filters extension (Plan 2b) ───────────────────────────────────────────
  it('starts with empty filters by default', () => {
    const t = useTable<{ id: number }>({
      fetcher: vi.fn().mockResolvedValue({ items: [], total: 0 })
    })
    expect(t.filters.value).toEqual({})
  })

  it('honors defaultFilters', () => {
    const t = useTable<{ id: number }, { search: string }>({
      fetcher: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      defaultFilters: { search: 'foo' }
    })
    expect(t.filters.value).toEqual({ search: 'foo' })
  })

  it('passes filters through to fetcher on reload', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }, { search: string }>({
      fetcher,
      defaultFilters: { search: 'foo' }
    })
    await t.reload()
    expect(fetcher).toHaveBeenCalledWith({ page: 1, pageSize: 20, filters: { search: 'foo' } })
  })

  it('setFilters merges patch and resets page to 1', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }, { search: string; status: string }>({
      fetcher,
      defaultFilters: { search: 'a', status: 'enabled' }
    })
    await t.setPage(3)
    expect(t.page.value).toBe(3)
    await t.setFilters({ search: 'b' })
    expect(t.page.value).toBe(1)
    expect(t.filters.value).toEqual({ search: 'b', status: 'enabled' })
    expect(fetcher).toHaveBeenLastCalledWith({
      page: 1, pageSize: 20, filters: { search: 'b', status: 'enabled' }
    })
  })

  it('setFilters with empty patch still reloads (resetting page)', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.setPage(5)
    fetcher.mockClear()
    await t.setFilters({})
    expect(t.page.value).toBe(1)
    expect(fetcher).toHaveBeenCalledOnce()
  })
```

- [ ] **Step 2: Run, verify fail**

```bash
cd optimus-fe && bun run test src/hooks/useTable.test.ts
```

Expected: FAIL — `t.filters` undefined, `t.setFilters` not a function.

- [ ] **Step 3: Rewrite useTable.ts with filters dimension**

Replace `optimus-fe/src/hooks/useTable.ts` entirely:

```ts
import { ref, type Ref } from 'vue'

export interface PageRequest {
  page: number
  pageSize: number
}
export interface PageResult<T> {
  items: T[]
  total: number
}

export interface UseTableOptions<T, F = Record<string, unknown>> {
  fetcher: (req: PageRequest & { filters?: F }) => Promise<PageResult<T>>
  defaultPageSize?: number
  defaultFilters?: F
}

export function useTable<T, F = Record<string, unknown>>(opts: UseTableOptions<T, F>) {
  const page = ref(1)
  const pageSize = ref(opts.defaultPageSize ?? 20)
  const items = ref<T[]>([]) as Ref<T[]>
  const total = ref(0)
  const loading = ref(false)
  const filters = ref<F>(opts.defaultFilters ?? ({} as F)) as Ref<F>

  async function reload() {
    loading.value = true
    try {
      const r = await opts.fetcher({
        page: page.value,
        pageSize: pageSize.value,
        filters: filters.value
      })
      items.value = r.items
      total.value = r.total
    } finally {
      loading.value = false
    }
  }

  async function setPage(p: number) {
    page.value = p
    await reload()
  }

  async function setPageSize(s: number) {
    pageSize.value = s
    page.value = 1
    await reload()
  }

  async function setFilters(patch: Partial<F>) {
    filters.value = { ...filters.value, ...patch } as F
    page.value = 1
    await reload()
  }

  return { page, pageSize, items, total, loading, filters, reload, setPage, setPageSize, setFilters }
}
```

**Heads-up:** the test at line ~20 of the existing `useTable.test.ts` asserts `expect(fetcher).toHaveBeenCalledWith({ page: 1, pageSize: 20 })`. After this change the fetcher is called with `{ page: 1, pageSize: 20, filters: {} }`. Update that assertion in the same edit:

```ts
// Old:
//   expect(fetcher).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
// New:
   expect(fetcher).toHaveBeenCalledWith({ page: 1, pageSize: 20, filters: {} })
```

Do the same for the `setPage` assertion (`{ page: 3, pageSize: 20, filters: {} }`) and the `setPageSize` final assertion (`{ page: 1, pageSize: 50, filters: {} }`). The existing 5 tests now expect the `filters: {}` key.

- [ ] **Step 4: Run, verify pass**

```bash
cd optimus-fe && bun run test src/hooks/useTable.test.ts
```

Expected: All 10 tests PASS.

- [ ] **Step 5: Typecheck**

```bash
cd optimus-fe && bun run typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/hooks/useTable.ts optimus-fe/src/hooks/useTable.test.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/hooks): useTable filters dimension with reset-to-page-1 semantics"
```

---

## Task 6: i18n keys — add 60+ keys across 5 modules + form.* + perm.category.*

**Files:**
- Modify: `optimus-fe/src/locales/zh-CN.json`
- Modify: `optimus-fe/src/locales/en-US.json`

`zh-CN.json` is the schema authority. Every new key MUST appear in both files. `bun run i18n:check` enforces this in CI.

- [ ] **Step 1: Read both locale files first**

```bash
cat optimus-fe/src/locales/zh-CN.json
cat optimus-fe/src/locales/en-US.json
```

Confirm the existing structure (see file overview in context). The new top-level groups to add: `form`, `confirm`, `perm.category`, and `system.{users,roles,menus,permissions,audit_logs}.*` (extending existing `menu.system.*` is the menu label set — that already exists; the new `system.*` namespace is for page content).

- [ ] **Step 2: Add keys to `zh-CN.json`**

Insert these top-level groups (alongside existing ones, before the closing `}`):

```json
  "form": {
    "required": "此项必填",
    "invalid_email": "邮箱格式不正确",
    "min_length": "至少 {n} 个字符",
    "max_length": "最多 {n} 个字符"
  },
  "confirm": {
    "delete_title": "确认删除？",
    "delete_desc": "此操作不可撤销",
    "disable_user": "确认禁用该用户？",
    "enable_user": "确认启用该用户？"
  },
  "perm": {
    "category": {
      "system": "系统管理",
      "k8s": "Kubernetes",
      "assets": "资产管理",
      "observability": "可观测性",
      "cicd": "CI/CD",
      "applications": "应用管理"
    }
  },
  "system": {
    "users": {
      "title": "用户管理",
      "create": "新建用户",
      "edit": "编辑用户",
      "search_placeholder": "搜索用户名或邮箱",
      "filter_status": "状态",
      "filter_status_all": "全部",
      "col_username": "用户名",
      "col_email": "邮箱",
      "col_display_name": "显示名",
      "col_status": "状态",
      "col_last_login": "上次登录",
      "col_created_at": "创建时间",
      "col_actions": "操作",
      "action_edit": "编辑",
      "action_roles": "角色",
      "action_reset_password": "重置密码",
      "action_disable": "禁用",
      "action_enable": "启用",
      "action_delete": "删除",
      "status_enabled": "启用",
      "status_disabled": "禁用",
      "form_username": "用户名",
      "form_email": "邮箱",
      "form_password": "密码",
      "form_display_name": "显示名",
      "form_avatar_url": "头像 URL",
      "form_roles": "角色",
      "roles_modal_title": "分配角色",
      "reset_password_title": "重置密码",
      "reset_password_new": "新密码",
      "create_ok": "已创建",
      "update_ok": "已保存",
      "delete_ok": "已删除",
      "status_ok": "状态已更新",
      "roles_ok": "角色已更新",
      "password_ok": "密码已重置"
    },
    "roles": {
      "title": "角色管理",
      "create": "新建角色",
      "edit": "编辑角色",
      "col_code": "代码",
      "col_name": "名称",
      "col_description": "描述",
      "col_is_builtin": "内置",
      "col_created_at": "创建时间",
      "col_actions": "操作",
      "action_edit": "编辑",
      "action_permissions": "权限",
      "action_delete": "删除",
      "builtin_yes": "是",
      "builtin_no": "否",
      "form_code": "代码",
      "form_name": "名称",
      "form_description": "描述",
      "permissions_modal_title": "绑定权限",
      "create_ok": "已创建",
      "update_ok": "已保存",
      "delete_ok": "已删除",
      "permissions_ok": "权限已更新"
    },
    "menus": {
      "title": "菜单管理",
      "create_root": "新建根节点",
      "create_child": "新建子节点",
      "edit": "编辑菜单",
      "action_edit": "编辑",
      "action_add_child": "添加子项",
      "action_delete": "删除",
      "form_parent": "父节点",
      "form_parent_root": "（根节点）",
      "form_code": "代码",
      "form_name": "显示名 (i18n key)",
      "form_path": "路由",
      "form_component": "组件",
      "form_icon": "图标",
      "form_permission_code": "权限码",
      "form_sort_order": "排序",
      "form_hidden": "隐藏",
      "create_ok": "已创建",
      "update_ok": "已保存",
      "delete_ok": "已删除",
      "drop_ok": "已移动",
      "drop_invalid": "不能拖到自身后代"
    },
    "permissions": {
      "title": "权限列表",
      "filter_placeholder": "搜索代码或名称"
    },
    "audit_logs": {
      "title": "操作审计",
      "filter_action": "操作类型",
      "filter_user_id": "用户 ID",
      "filter_range": "时间范围",
      "filter_search": "查询",
      "filter_reset": "重置",
      "col_created_at": "时间",
      "col_action": "操作",
      "col_user_id": "用户 ID",
      "col_target_type": "目标类型",
      "col_target_id": "目标 ID",
      "col_ip": "IP",
      "payload_title": "操作上下文"
    }
  }
```

- [ ] **Step 3: Add same keys to `en-US.json` with English text**

Same key structure, English values:

```json
  "form": {
    "required": "Required",
    "invalid_email": "Invalid email format",
    "min_length": "At least {n} characters",
    "max_length": "At most {n} characters"
  },
  "confirm": {
    "delete_title": "Delete?",
    "delete_desc": "This action cannot be undone.",
    "disable_user": "Disable this user?",
    "enable_user": "Enable this user?"
  },
  "perm": {
    "category": {
      "system": "System",
      "k8s": "Kubernetes",
      "assets": "Assets",
      "observability": "Observability",
      "cicd": "CI/CD",
      "applications": "Applications"
    }
  },
  "system": {
    "users": {
      "title": "Users",
      "create": "New user",
      "edit": "Edit user",
      "search_placeholder": "Search username or email",
      "filter_status": "Status",
      "filter_status_all": "All",
      "col_username": "Username",
      "col_email": "Email",
      "col_display_name": "Display name",
      "col_status": "Status",
      "col_last_login": "Last login",
      "col_created_at": "Created",
      "col_actions": "Actions",
      "action_edit": "Edit",
      "action_roles": "Roles",
      "action_reset_password": "Reset password",
      "action_disable": "Disable",
      "action_enable": "Enable",
      "action_delete": "Delete",
      "status_enabled": "Enabled",
      "status_disabled": "Disabled",
      "form_username": "Username",
      "form_email": "Email",
      "form_password": "Password",
      "form_display_name": "Display name",
      "form_avatar_url": "Avatar URL",
      "form_roles": "Roles",
      "roles_modal_title": "Assign roles",
      "reset_password_title": "Reset password",
      "reset_password_new": "New password",
      "create_ok": "Created",
      "update_ok": "Saved",
      "delete_ok": "Deleted",
      "status_ok": "Status updated",
      "roles_ok": "Roles updated",
      "password_ok": "Password reset"
    },
    "roles": {
      "title": "Roles",
      "create": "New role",
      "edit": "Edit role",
      "col_code": "Code",
      "col_name": "Name",
      "col_description": "Description",
      "col_is_builtin": "Built-in",
      "col_created_at": "Created",
      "col_actions": "Actions",
      "action_edit": "Edit",
      "action_permissions": "Permissions",
      "action_delete": "Delete",
      "builtin_yes": "Yes",
      "builtin_no": "No",
      "form_code": "Code",
      "form_name": "Name",
      "form_description": "Description",
      "permissions_modal_title": "Bind permissions",
      "create_ok": "Created",
      "update_ok": "Saved",
      "delete_ok": "Deleted",
      "permissions_ok": "Permissions updated"
    },
    "menus": {
      "title": "Menus",
      "create_root": "New root",
      "create_child": "New child",
      "edit": "Edit menu",
      "action_edit": "Edit",
      "action_add_child": "Add child",
      "action_delete": "Delete",
      "form_parent": "Parent",
      "form_parent_root": "(root)",
      "form_code": "Code",
      "form_name": "Name (i18n key)",
      "form_path": "Path",
      "form_component": "Component",
      "form_icon": "Icon",
      "form_permission_code": "Permission code",
      "form_sort_order": "Sort order",
      "form_hidden": "Hidden",
      "create_ok": "Created",
      "update_ok": "Saved",
      "delete_ok": "Deleted",
      "drop_ok": "Moved",
      "drop_invalid": "Cannot drop a node onto its own descendant"
    },
    "permissions": {
      "title": "Permissions",
      "filter_placeholder": "Search code or name"
    },
    "audit_logs": {
      "title": "Audit logs",
      "filter_action": "Action",
      "filter_user_id": "User ID",
      "filter_range": "Time range",
      "filter_search": "Search",
      "filter_reset": "Reset",
      "col_created_at": "Time",
      "col_action": "Action",
      "col_user_id": "User ID",
      "col_target_type": "Target type",
      "col_target_id": "Target ID",
      "col_ip": "IP",
      "payload_title": "Payload"
    }
  }
```

**Heads-up:** existing `menu.system.*` keys (`menu.system.users` etc., for sidebar labels) remain unchanged — they're a different namespace from the new `system.*.title` (for page H1). Do not collapse them.

- [ ] **Step 4: Run i18n:check**

```bash
cd optimus-fe && bun run i18n:check
```

Expected: PASS (zh/en key sets match; unused-key warnings for `system.*.*` and `perm.category.*` are acceptable — they'll be referenced in subsequent Tasks 9-15).

If `i18n:check` fails for "declared but unused" as a HARD error, edit `scripts/check-i18n-keys.ts` to only `console.warn` for that case (the addendum §4.5 spec already says warn-only). Inspect the script if needed:

```bash
grep -n "declared\|warn\|exit" optimus-fe/scripts/check-i18n-keys.ts
```

- [ ] **Step 5: Typecheck (JSON imports)**

```bash
cd optimus-fe && bun run typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/locales/zh-CN.json optimus-fe/src/locales/en-US.json
git -C /Users/logic/Projects/optimus commit -m "feat(fe/locales): add Plan 2b keys for system pages + form + perm.category"
```

---

## Task 7: Refactor profile/Index.vue to use form-diff

**Files:**
- Modify: `optimus-fe/src/views/profile/Index.vue`

The 2a profile sends `{display_name, email, avatar_url}` on every save regardless of what changed. Switch to `formDiff` so only changed fields go in the request body. Same observable behavior; tightens the partial-update semantics flagged in the 2b spec.

- [ ] **Step 1: Edit the script section**

Open `optimus-fe/src/views/profile/Index.vue`. The current `onSaveProfile` (lines 76-91 per Read above) sends a static body. Replace the script section's imports and the `onSaveProfile` function as follows.

Add import near the existing imports:

```ts
import { formDiff } from '@/utils/form-diff'
```

Add an initial-snapshot ref next to `profile`:

```ts
const initialProfile = ref({
  display_name: auth.user?.display_name ?? '',
  email: auth.user?.email ?? '',
  avatar_url: auth.user?.avatar_url ?? ''
})
```

Update the `onMounted` block to also seed `initialProfile`:

```ts
onMounted(async () => {
  if (!auth.user) {
    try {
      const me = await meApi.get()
      auth.setUser(me)
      profile.display_name = me.display_name
      profile.email = me.email
      profile.avatar_url = me.avatar_url
      initialProfile.value = {
        display_name: me.display_name,
        email: me.email,
        avatar_url: me.avatar_url
      }
    } catch { /* guard already redirects on 401 */ }
  }
})
```

Replace `onSaveProfile`:

```ts
async function onSaveProfile() {
  const patch = formDiff(initialProfile.value, {
    display_name: profile.display_name,
    email: profile.email,
    avatar_url: profile.avatar_url
  })
  if (Object.keys(patch).length === 0) {
    message.info(t('profile.update_ok'))
    return
  }
  profileSaving.value = true
  try {
    const updated = await meApi.update(patch)
    auth.setUser(updated)
    initialProfile.value = {
      display_name: updated.display_name,
      email: updated.email,
      avatar_url: updated.avatar_url
    }
    message.success(t('profile.update_ok'))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    profileSaving.value = false
  }
}
```

Add `ref` to the existing `import { ... } from 'vue'` line if it's not already in the imports (per the Read above, it already imports `inject, reactive, ref, onMounted`).

- [ ] **Step 2: Typecheck + lint**

```bash
cd optimus-fe && bun run typecheck && bun run lint
```

Expected: PASS.

- [ ] **Step 3: Manual smoke (optional but recommended)**

```bash
cd optimus-fe && bun run dev
```

Log in as admin, open `/profile`, change just `display_name`, save. Open DevTools Network → confirm the PUT `/api/v1/me` body is `{"display_name":"..."}` only (no `email` / `avatar_url`). Stop the dev server.

- [ ] **Step 4: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/profile/Index.vue
git -C /Users/logic/Projects/optimus commit -m "refactor(fe/profile): use form-diff for partial-update body"
```

---

## Task 8: User modals (UserEditModal + UserRolesModal + UserResetPasswordModal)

**Files:**
- Create: `optimus-fe/src/views/system/users/components/UserEditModal.vue`
- Create: `optimus-fe/src/views/system/users/components/UserRolesModal.vue`
- Create: `optimus-fe/src/views/system/users/components/UserResetPasswordModal.vue`

3 small modal components. Each is independently committable but bundled into one task for batch 3 efficiency.

- [ ] **Step 1: Write UserEditModal.vue**

`optimus-fe/src/views/system/users/components/UserEditModal.vue`:

```vue
<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('system.users.edit') : $t('system.users.create')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item v-if="!isEdit" :label="$t('system.users.form_username')" name="username">
        <a-input v-model:value="form.username" />
      </a-form-item>
      <a-form-item :label="$t('system.users.form_email')" name="email">
        <a-input v-model:value="form.email" />
      </a-form-item>
      <a-form-item v-if="!isEdit" :label="$t('system.users.form_password')" name="password">
        <a-input-password v-model:value="form.password" />
      </a-form-item>
      <a-form-item :label="$t('system.users.form_display_name')" name="display_name">
        <a-input v-model:value="form.display_name" />
      </a-form-item>
      <a-form-item :label="$t('system.users.form_avatar_url')" name="avatar_url">
        <a-input v-model:value="form.avatar_url" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { UserApi } from '@/api/user'
import type { UserDetail } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: UserDetail | null  // null/undefined → create mode
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  username: '',
  email: '',
  password: '',
  display_name: '',
  avatar_url: ''
})
let initialSnapshot = { email: '', display_name: '', avatar_url: '' }

const rules = computed(() => ({
  username: [{ required: true, min: 3, max: 64, message: t('form.required') }],
  email: [{ required: true, type: 'email', message: t('form.invalid_email') }],
  password: isEdit.value ? [] : [{ required: true, min: 8, message: t('form.min_length', { n: 8 }) }],
  display_name: [{ max: 128 }]
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.username = props.initial.username
      form.email = props.initial.email
      form.password = ''
      form.display_name = props.initial.display_name
      form.avatar_url = props.initial.avatar_url
      initialSnapshot = {
        email: props.initial.email,
        display_name: props.initial.display_name,
        avatar_url: props.initial.avatar_url
      }
    } else {
      form.username = ''
      form.email = ''
      form.password = ''
      form.display_name = ''
      form.avatar_url = ''
    }
  },
  { immediate: true }
)

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch = formDiff(initialSnapshot, {
        email: form.email,
        display_name: form.display_name,
        avatar_url: form.avatar_url
      })
      if (Object.keys(patch).length > 0) {
        await userApi.update(props.initial.id, patch)
      }
      message.success(t('system.users.update_ok'))
    } else {
      await userApi.create({
        username: form.username,
        email: form.email,
        password: form.password,
        display_name: form.display_name || undefined
      })
      message.success(t('system.users.create_ok'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 2: Write UserRolesModal.vue**

`optimus-fe/src/views/system/users/components/UserRolesModal.vue`:

```vue
<template>
  <a-modal
    :open="open"
    :title="$t('system.users.roles_modal_title')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-checkbox-group v-model:value="selected" style="display: flex; flex-direction: column; gap: 8px;">
        <a-checkbox v-for="r in roles" :key="r.id" :value="r.id">
          {{ r.name }} <span style="color:#999;">({{ r.code }})</span>
        </a-checkbox>
      </a-checkbox-group>
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch, inject } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { UserApi } from '@/api/user'
import type { RoleApi } from '@/api/role'
import type { UserDetail, RoleSummary } from '@/types/api'

const props = defineProps<{
  open: boolean
  user: UserDetail | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!
const roleApi = inject<RoleApi>('roleApi')!

const roles = ref<RoleSummary[]>([])
const selected = ref<number[]>([])
const loading = ref(false)
const saving = ref(false)

watch(
  () => props.open,
  async (open) => {
    if (!open || !props.user) return
    loading.value = true
    try {
      roles.value = await roleApi.list()
      selected.value = props.user.roles.map(r => r.id)
    } catch (e) {
      message.error(isBizError(e) ? e.message : t('network.error'))
    } finally {
      loading.value = false
    }
  }
)

async function onOk() {
  if (!props.user) return
  saving.value = true
  try {
    await userApi.setRoles(props.user.id, { role_ids: selected.value })
    message.success(t('system.users.roles_ok'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 3: Write UserResetPasswordModal.vue**

`optimus-fe/src/views/system/users/components/UserResetPasswordModal.vue`:

```vue
<template>
  <a-modal
    :open="open"
    :title="$t('system.users.reset_password_title')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('system.users.reset_password_new')" name="password">
        <a-input-password v-model:value="form.password" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { UserApi } from '@/api/user'
import type { UserDetail } from '@/types/api'

const props = defineProps<{
  open: boolean
  user: UserDetail | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!
const formRef = ref<FormInstance>()
const saving = ref(false)
const form = reactive({ password: '' })

const rules = {
  password: [{ required: true, min: 8, message: t('form.min_length', { n: 8 }) }]
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    form.password = ''
    formRef.value?.resetFields()
  }
)

async function onOk() {
  if (!props.user) return
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    await userApi.setPassword(props.user.id, { password: form.password })
    message.success(t('system.users.password_ok'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 4: Typecheck + lint + i18n:check**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run i18n:check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/users/components/
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/users): UserEditModal + UserRolesModal + UserResetPasswordModal"
```

---

## Task 9: system/users/List.vue (table + filters + actions wired)

**Files:**
- Modify: `optimus-fe/src/views/system/users/List.vue` (full rewrite over the 2a placeholder)

- [ ] **Step 1: Replace List.vue entirely**

```vue
<template>
  <a-card>
    <PageHeader :title="$t('system.users.title')" />

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        :placeholder="$t('system.users.search_placeholder')"
        style="width: 280px;"
        allow-clear
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="statusInput"
        style="width: 140px;"
        :options="statusOptions"
        @change="onStatusChange"
      />
      <a-button v-permission="'system:user:write'" type="primary" @click="openCreate">
        {{ $t('system.users.create') }}
      </a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'status'">
          <a-badge
            :status="record.status === 'enabled' ? 'success' : 'default'"
            :text="$t(`system.users.status_${record.status}`)"
          />
        </template>
        <template v-else-if="column.key === 'last_login_at'">
          {{ record.last_login_at ? formatTime(record.last_login_at) : '—' }}
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'system:user:write'" @click="openEdit(record)">{{ $t('system.users.action_edit') }}</a>
            <a v-permission="'system:user:write'" @click="openRoles(record)">{{ $t('system.users.action_roles') }}</a>
            <a v-permission="'system:user:reset_pass'" @click="openResetPassword(record)">{{ $t('system.users.action_reset_password') }}</a>
            <a-popconfirm
              :title="record.status === 'enabled' ? $t('confirm.disable_user') : $t('confirm.enable_user')"
              @confirm="toggleStatus(record)"
            >
              <a v-permission="'system:user:write'">
                {{ record.status === 'enabled' ? $t('system.users.action_disable') : $t('system.users.action_enable') }}
              </a>
            </a-popconfirm>
            <a-popconfirm
              :title="$t('confirm.delete_title')"
              :description="$t('confirm.delete_desc')"
              @confirm="remove(record)"
            >
              <a v-permission="'system:user:delete'" :class="{ 'a-disabled': record.username === 'admin' }">
                {{ $t('system.users.action_delete') }}
              </a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="table.setPage"
      @show-size-change="(_, size) => table.setPageSize(size)"
    />

    <UserEditModal v-model:open="editOpen" :initial="editingDetail" @saved="onSaved" />
    <UserRolesModal v-model:open="rolesOpen" :user="rolesUser" @saved="onSaved" />
    <UserResetPasswordModal v-model:open="resetOpen" :user="resetUser" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import UserEditModal from './components/UserEditModal.vue'
import UserRolesModal from './components/UserRolesModal.vue'
import UserResetPasswordModal from './components/UserResetPasswordModal.vue'
import type { UserApi } from '@/api/user'
import type { UserSummary, UserDetail, UserListQuery } from '@/types/api'

const { t } = useI18n()
const userApi = inject<UserApi>('userApi')!

interface UserFilters extends UserListQuery {}

const searchInput = ref('')
const statusInput = ref<'' | 'enabled' | 'disabled'>('')
const statusOptions = computed(() => [
  { value: '', label: t('system.users.filter_status_all') },
  { value: 'enabled', label: t('system.users.status_enabled') },
  { value: 'disabled', label: t('system.users.status_disabled') }
])

const table = useTable<UserSummary, UserFilters>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await userApi.list({
      page,
      page_size: pageSize,
      search: filters?.search || undefined,
      status: filters?.status || undefined
    })
    return { items: r.items, total: r.total }
  }
})

const columns = computed(() => [
  { key: 'username',     title: t('system.users.col_username'),     dataIndex: 'username' },
  { key: 'email',        title: t('system.users.col_email'),        dataIndex: 'email' },
  { key: 'display_name', title: t('system.users.col_display_name'), dataIndex: 'display_name' },
  { key: 'status',       title: t('system.users.col_status') },
  { key: 'last_login_at',title: t('system.users.col_last_login') },
  { key: 'created_at',   title: t('system.users.col_created_at') },
  { key: 'actions',      title: t('system.users.col_actions'), width: 360 }
])

const editOpen = ref(false)
const editingDetail = ref<UserDetail | null>(null)
const rolesOpen = ref(false)
const rolesUser = ref<UserDetail | null>(null)
const resetOpen = ref(false)
const resetUser = ref<UserDetail | null>(null)

function onSearch(v: string) { table.setFilters({ search: v || undefined }) }
function onSearchInputChange(e: Event) {
  const target = e.target as HTMLInputElement | null
  if (target && target.value === '') table.setFilters({ search: undefined })
}
function onStatusChange(v: '' | 'enabled' | 'disabled') {
  table.setFilters({ status: v || undefined })
}

async function openCreate() {
  editingDetail.value = null
  editOpen.value = true
}
async function openEdit(r: UserSummary) {
  try {
    editingDetail.value = await userApi.get(r.id)
    editOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}
async function openRoles(r: UserSummary) {
  try {
    rolesUser.value = await userApi.get(r.id)
    rolesOpen.value = true
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}
function openResetPassword(r: UserSummary) {
  resetUser.value = { ...(r as unknown as UserDetail) }
  resetOpen.value = true
}

async function toggleStatus(r: UserSummary) {
  const next = r.status === 'enabled' ? 'disabled' : 'enabled'
  try {
    await userApi.setStatus(r.id, { status: next })
    message.success(t('system.users.status_ok'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

async function remove(r: UserSummary) {
  if (r.username === 'admin') return
  try {
    await userApi.remove(r.id)
    message.success(t('system.users.delete_ok'))
    await table.reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onSaved() { table.reload() }

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

onMounted(() => { table.reload() })
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
}
.a-disabled {
  color: #ccc;
  pointer-events: none;
}
</style>
```

- [ ] **Step 2: Verify the v-permission directive accepts string**

The 2a `v-permission` directive (per addendum §4.4) supports `string | string[] | {arg:'any'}[]`. Confirm by reading:

```bash
grep -n "value" optimus-fe/src/directives/permission.ts
```

If the modifier branch is buggy, fall back to passing only strings (the current usage above is all strings — fine).

- [ ] **Step 3: Typecheck + lint + i18n:check + tests**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run i18n:check && bun run test
```

Expected: PASS.

- [ ] **Step 4: Build**

```bash
cd optimus-fe && bun run build
```

Expected: PASS, no Vue compiler / TS errors.

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/users/List.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/users): real CRUD list — search + status filter + 5 actions"
```

---

## Task 10: Role modals (RoleEditModal + RolePermissionsModal)

**Files:**
- Create: `optimus-fe/src/views/system/roles/components/RoleEditModal.vue`
- Create: `optimus-fe/src/views/system/roles/components/RolePermissionsModal.vue`

`RolePermissionsModal` is the most logic-heavy modal in 2b — it builds a tree from a flat permission list grouped by `category`.

- [ ] **Step 1: Write RoleEditModal.vue**

```vue
<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('system.roles.edit') : $t('system.roles.create')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item v-if="!isEdit" :label="$t('system.roles.form_code')" name="code">
        <a-input v-model:value="form.code" />
      </a-form-item>
      <a-form-item :label="$t('system.roles.form_name')" name="name">
        <a-input v-model:value="form.name" />
      </a-form-item>
      <a-form-item :label="$t('system.roles.form_description')" name="description">
        <a-textarea v-model:value="form.description" :rows="3" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { RoleApi } from '@/api/role'
import type { RoleSummary } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: RoleSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const roleApi = inject<RoleApi>('roleApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({ code: '', name: '', description: '' })
let initialSnapshot = { name: '', description: '' }

const rules = computed(() => ({
  code: [{ required: true, min: 2, max: 64, message: t('form.required') }],
  name: [{ required: true, max: 128, message: t('form.required') }]
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.code = props.initial.code
      form.name = props.initial.name
      form.description = props.initial.description
      initialSnapshot = { name: props.initial.name, description: props.initial.description }
    } else {
      form.code = ''
      form.name = ''
      form.description = ''
    }
  },
  { immediate: true }
)

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch = formDiff(initialSnapshot, { name: form.name, description: form.description })
      if (Object.keys(patch).length > 0) {
        await roleApi.update(props.initial.id, patch)
      }
      message.success(t('system.roles.update_ok'))
    } else {
      await roleApi.create({
        code: form.code,
        name: form.name,
        description: form.description || undefined
      })
      message.success(t('system.roles.create_ok'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 2: Write RolePermissionsModal.vue**

```vue
<template>
  <a-modal
    :open="open"
    :title="$t('system.roles.permissions_modal_title')"
    :confirm-loading="saving"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="loading">
      <a-tree
        v-if="treeData.length"
        v-model:checked-keys="checkedKeys"
        checkable
        :tree-data="treeData"
        :default-expand-all="true"
        :check-strictly="false"
      />
      <a-empty v-else />
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch, inject } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { RoleApi } from '@/api/role'
import type { PermissionApi } from '@/api/permission'
import type { RoleSummary, Permission } from '@/types/api'

const props = defineProps<{
  open: boolean
  role: RoleSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const roleApi = inject<RoleApi>('roleApi')!
const permissionApi = inject<PermissionApi>('permissionApi')!

const loading = ref(false)
const saving = ref(false)
const allPermissions = ref<Permission[]>([])
const checkedKeys = ref<string[]>([])

const CATEGORY_PREFIX = '__cat:'

interface PermTreeNode {
  title: string
  key: string
  children?: PermTreeNode[]
  selectable?: boolean
}

const treeData = computed<PermTreeNode[]>(() => {
  const byCategory = new Map<string, Permission[]>()
  for (const p of allPermissions.value) {
    const arr = byCategory.get(p.category) ?? []
    arr.push(p)
    byCategory.set(p.category, arr)
  }
  return Array.from(byCategory.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([category, perms]) => ({
      title: t(`perm.category.${category}`),
      key: `${CATEGORY_PREFIX}${category}`,
      selectable: false,
      children: perms
        .sort((a, b) => a.code.localeCompare(b.code))
        .map(p => ({
          title: `${t(`perm.${p.code.replace(/:/g, '.')}`)} (${p.code})`,
          key: p.code
        }))
    }))
})

watch(
  () => props.open,
  async (open) => {
    if (!open || !props.role) return
    loading.value = true
    try {
      const [perms, detail] = await Promise.all([
        permissionApi.list(),
        roleApi.get(props.role.id)
      ])
      allPermissions.value = perms
      checkedKeys.value = [...detail.permission_codes]
    } catch (e) {
      message.error(isBizError(e) ? e.message : t('network.error'))
    } finally {
      loading.value = false
    }
  }
)

async function onOk() {
  if (!props.role) return
  saving.value = true
  try {
    const codes = checkedKeys.value.filter(k => !k.startsWith(CATEGORY_PREFIX))
    await roleApi.setPermissions(props.role.id, { permission_codes: codes })
    message.success(t('system.roles.permissions_ok'))
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 3: Typecheck + lint**

```bash
cd optimus-fe && bun run typecheck && bun run lint
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/roles/components/
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/roles): RoleEditModal + RolePermissionsModal (checkable tree by category)"
```

---

## Task 11: system/roles/List.vue

**Files:**
- Modify: `optimus-fe/src/views/system/roles/List.vue` (full rewrite)

`GET /roles` is unpaginated (per spec §7.2 — returns flat array), so this list doesn't use `useTable`.

- [ ] **Step 1: Replace List.vue**

```vue
<template>
  <a-card>
    <PageHeader :title="$t('system.roles.title')" />

    <div class="filter-row u-mb-16">
      <a-button v-permission="'system:role:write'" type="primary" @click="openCreate">
        {{ $t('system.roles.create') }}
      </a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="roles"
      :loading="loading"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'is_builtin'">
          {{ record.is_builtin ? $t('system.roles.builtin_yes') : $t('system.roles.builtin_no') }}
        </template>
        <template v-else-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'system:role:write'" @click="openEdit(record)">{{ $t('system.roles.action_edit') }}</a>
            <a v-permission="'system:role:write'" @click="openPermissions(record)">{{ $t('system.roles.action_permissions') }}</a>
            <a-popconfirm
              :title="$t('confirm.delete_title')"
              :description="$t('confirm.delete_desc')"
              :disabled="record.is_builtin"
              @confirm="remove(record)"
            >
              <a v-permission="'system:role:delete'" :class="{ 'a-disabled': record.is_builtin }">
                {{ $t('system.roles.action_delete') }}
              </a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <RoleEditModal v-model:open="editOpen" :initial="editing" @saved="reload" />
    <RolePermissionsModal v-model:open="permOpen" :role="permRole" @saved="reload" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import RoleEditModal from './components/RoleEditModal.vue'
import RolePermissionsModal from './components/RolePermissionsModal.vue'
import type { RoleApi } from '@/api/role'
import type { RoleSummary } from '@/types/api'

const { t } = useI18n()
const roleApi = inject<RoleApi>('roleApi')!

const roles = ref<RoleSummary[]>([])
const loading = ref(false)

const editOpen = ref(false)
const editing = ref<RoleSummary | null>(null)
const permOpen = ref(false)
const permRole = ref<RoleSummary | null>(null)

const columns = computed(() => [
  { key: 'code',        title: t('system.roles.col_code'),        dataIndex: 'code' },
  { key: 'name',        title: t('system.roles.col_name'),        dataIndex: 'name' },
  { key: 'description', title: t('system.roles.col_description'),dataIndex: 'description' },
  { key: 'is_builtin',  title: t('system.roles.col_is_builtin') },
  { key: 'created_at',  title: t('system.roles.col_created_at') },
  { key: 'actions',     title: t('system.roles.col_actions'), width: 240 }
])

async function reload() {
  loading.value = true
  try {
    roles.value = await roleApi.list()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  editOpen.value = true
}
function openEdit(r: RoleSummary) {
  editing.value = r
  editOpen.value = true
}
function openPermissions(r: RoleSummary) {
  permRole.value = r
  permOpen.value = true
}

async function remove(r: RoleSummary) {
  if (r.is_builtin) return
  try {
    await roleApi.remove(r.id)
    message.success(t('system.roles.delete_ok'))
    await reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

onMounted(reload)
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
}
.a-disabled {
  color: #ccc;
  pointer-events: none;
}
</style>
```

- [ ] **Step 2: Typecheck + lint + i18n:check + tests + build**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run i18n:check && bun run test && bun run build
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/roles/List.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/roles): real CRUD list — edit + permissions tree + delete-builtin-guard"
```

---

## Task 12: computeDropTarget pure fn (TDD) + MenuEditModal.vue

**Files:**
- Create: `optimus-fe/src/views/system/menus/computeDropTarget.ts`
- Create: `optimus-fe/src/views/system/menus/computeDropTarget.test.ts`
- Create: `optimus-fe/src/views/system/menus/components/MenuEditModal.vue`

The drop-target math is the only TDD-able piece in the menu page. Everything else (a-tree, modal) is plumbing.

- [ ] **Step 1: Write failing tests for computeDropTarget**

`optimus-fe/src/views/system/menus/computeDropTarget.test.ts`:

```ts
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
```

- [ ] **Step 2: Run, verify fail**

```bash
cd optimus-fe && bun run test src/views/system/menus/computeDropTarget.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement computeDropTarget.ts**

`optimus-fe/src/views/system/menus/computeDropTarget.ts`:

```ts
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
```

- [ ] **Step 4: Run, verify pass**

```bash
cd optimus-fe && bun run test src/views/system/menus/computeDropTarget.test.ts
```

Expected: 9 tests PASS.

- [ ] **Step 5: Write MenuEditModal.vue**

`optimus-fe/src/views/system/menus/components/MenuEditModal.vue`:

```vue
<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('system.menus.edit') : (props.parentId ? $t('system.menus.create_child') : $t('system.menus.create_root'))"
    :confirm-loading="saving"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('system.menus.form_parent')" name="parent_id">
        <a-tree-select
          v-model:value="form.parent_id"
          :tree-data="parentOptions"
          allow-clear
          :placeholder="$t('system.menus.form_parent_root')"
          tree-default-expand-all
        />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_code')" name="code">
        <a-input v-model:value="form.code" :disabled="isEdit" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_name')" name="name">
        <a-input v-model:value="form.name" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_path')" name="path">
        <a-input v-model:value="form.path" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_component')" name="component">
        <a-input v-model:value="form.component" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_icon')" name="icon">
        <a-input v-model:value="form.icon" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_permission_code')" name="permission_code">
        <a-input v-model:value="form.permission_code" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_sort_order')" name="sort_order">
        <a-input-number v-model:value="form.sort_order" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_hidden')" name="hidden">
        <a-switch v-model:checked="form.hidden" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { MenuApi } from '@/api/menu'
import type { MenuNode } from '@/types/api'

interface TreeSelectOption {
  value: number | null
  label: string
  children?: TreeSelectOption[]
}

const props = defineProps<{
  open: boolean
  initial?: MenuNode | null      // null/undefined → create mode
  parentId?: number | null        // when creating a child of an existing node
  tree: MenuNode[]                // current full tree, used to build parent select
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const menuApi = inject<MenuApi>('menuApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  parent_id: null as number | null,
  code: '',
  name: '',
  path: '',
  component: '',
  icon: '',
  permission_code: '',
  sort_order: 10,
  hidden: false
})
let snapshot = { ...form }

const rules = computed(() => ({
  code: [{ required: true, min: 2, max: 64, message: t('form.required') }],
  name: [{ required: true, max: 128, message: t('form.required') }]
}))

function buildParentOptions(nodes: MenuNode[]): TreeSelectOption[] {
  return nodes.map(n => ({
    value: n.id,
    label: n.code,
    children: n.children ? buildParentOptions(n.children) : undefined
  }))
}
const parentOptions = computed(() => buildParentOptions(props.tree))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.parent_id = props.initial.parent_id ?? null
      form.code = props.initial.code
      form.name = props.initial.name
      form.path = props.initial.path
      form.component = props.initial.component
      form.icon = props.initial.icon
      form.permission_code = props.initial.permission_code ?? ''
      form.sort_order = props.initial.sort_order
      form.hidden = props.initial.hidden
    } else {
      form.parent_id = props.parentId ?? null
      form.code = ''
      form.name = ''
      form.path = ''
      form.component = ''
      form.icon = ''
      form.permission_code = ''
      form.sort_order = 10
      form.hidden = false
    }
    snapshot = { ...form }
  },
  { immediate: true }
)

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch = formDiff(snapshot, { ...form })
      if (Object.keys(patch).length > 0) {
        // Coerce '' → undefined for optional fields so backend doesn't see empty strings
        if (patch.permission_code === '') patch.permission_code = undefined
        await menuApi.update(props.initial.id, patch)
      }
      message.success(t('system.menus.update_ok'))
    } else {
      await menuApi.create({
        parent_id: form.parent_id,
        code: form.code,
        name: form.name,
        path: form.path || undefined,
        component: form.component || undefined,
        icon: form.icon || undefined,
        permission_code: form.permission_code || undefined,
        sort_order: form.sort_order,
        hidden: form.hidden
      })
      message.success(t('system.menus.create_ok'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
```

- [ ] **Step 6: Typecheck + lint**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run test
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/menus/computeDropTarget.ts optimus-fe/src/views/system/menus/computeDropTarget.test.ts optimus-fe/src/views/system/menus/components/MenuEditModal.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/menus): computeDropTarget TDD + MenuEditModal"
```

---

## Task 13: system/menus/List.vue (draggable tree)

**Files:**
- Modify: `optimus-fe/src/views/system/menus/List.vue` (full rewrite)

- [ ] **Step 1: Replace List.vue**

```vue
<template>
  <a-card>
    <PageHeader :title="$t('system.menus.title')" />

    <div class="filter-row u-mb-16">
      <a-button v-permission="'system:menu:write'" type="primary" @click="openCreateRoot">
        {{ $t('system.menus.create_root') }}
      </a-button>
    </div>

    <a-spin :spinning="loading">
      <a-tree
        v-if="tree.length"
        :tree-data="treeData"
        :default-expand-all="true"
        :draggable="true"
        :field-names="{ children: 'children', title: 'code', key: 'id' }"
        @drop="onDrop"
      >
        <template #title="node">
          <span class="menu-row">
            <span class="menu-code">{{ node.code }}</span>
            <code class="menu-path" v-if="node.path">{{ node.path }}</code>
            <span class="menu-name">{{ $t(node.name) }}</span>
            <span class="menu-actions">
              <a v-permission="'system:menu:write'" @click.stop="openEdit(node as MenuNode)">{{ $t('system.menus.action_edit') }}</a>
              <a v-permission="'system:menu:write'" @click.stop="openCreateChild(node as MenuNode)">{{ $t('system.menus.action_add_child') }}</a>
              <a-popconfirm
                :title="$t('confirm.delete_title')"
                :description="$t('confirm.delete_desc')"
                @confirm="remove(node as MenuNode)"
              >
                <a v-permission="'system:menu:delete'" @click.stop>{{ $t('system.menus.action_delete') }}</a>
              </a-popconfirm>
            </span>
          </span>
        </template>
      </a-tree>
      <a-empty v-else />
    </a-spin>

    <MenuEditModal
      v-model:open="editOpen"
      :initial="editing"
      :parent-id="parentForCreate"
      :tree="tree"
      @saved="reload"
    />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import MenuEditModal from './components/MenuEditModal.vue'
import { computeDropTarget } from './computeDropTarget'
import type { MenuApi } from '@/api/menu'
import type { MenuNode } from '@/types/api'

const { t } = useI18n()
const menuApi = inject<MenuApi>('menuApi')!

const tree = ref<MenuNode[]>([])
const loading = ref(false)
const editOpen = ref(false)
const editing = ref<MenuNode | null>(null)
const parentForCreate = ref<number | null>(null)

const treeData = computed(() => tree.value)

async function reload() {
  loading.value = true
  try {
    tree.value = await menuApi.list()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loading.value = false
  }
}

function openCreateRoot() {
  editing.value = null
  parentForCreate.value = null
  editOpen.value = true
}
function openCreateChild(parent: MenuNode) {
  editing.value = null
  parentForCreate.value = parent.id
  editOpen.value = true
}
function openEdit(node: MenuNode) {
  editing.value = node
  parentForCreate.value = null
  editOpen.value = true
}

async function remove(node: MenuNode) {
  try {
    await menuApi.remove(node.id)
    message.success(t('system.menus.delete_ok'))
    await reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

interface AntdvDropEvent {
  dragNode: { dataRef?: MenuNode; key: number }
  node: { dataRef?: MenuNode; key: number }
  dropPosition: number
  dropToGap: boolean
}

async function onDrop(info: AntdvDropEvent) {
  const dragId = info.dragNode.dataRef?.id ?? info.dragNode.key
  const dropId = info.node.dataRef?.id ?? info.node.key
  try {
    const target = computeDropTarget(tree.value, dragId, dropId, info.dropPosition, info.dropToGap)
    await menuApi.update(dragId, target)
    message.success(t('system.menus.drop_ok'))
    await reload()
  } catch (e) {
    if (e instanceof Error && e.message.toLowerCase().includes('descendant')) {
      message.warning(t('system.menus.drop_invalid'))
      return
    }
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

onMounted(reload)
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
}
.menu-row {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.menu-code { font-weight: 500; }
.menu-path { color: #888; }
.menu-name { color: #555; }
.menu-actions {
  margin-left: 16px;
  display: inline-flex;
  gap: 8px;
}
</style>
```

- [ ] **Step 2: Typecheck + lint + tests + build**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run test && bun run build
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/menus/List.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/menus): draggable tree CRUD + drop-to-PUT via computeDropTarget"
```

---

## Task 14: system/permissions/List.vue (read-only)

**Files:**
- Modify: `optimus-fe/src/views/system/permissions/List.vue` (full rewrite)

- [ ] **Step 1: Replace List.vue**

```vue
<template>
  <a-card>
    <PageHeader :title="$t('system.permissions.title')" />

    <a-input
      v-model:value="filter"
      :placeholder="$t('system.permissions.filter_placeholder')"
      style="max-width: 360px; margin-bottom: 16px;"
      allow-clear
    />

    <a-collapse v-model:active-key="activeKeys" :bordered="false">
      <a-collapse-panel v-for="group in filteredGroups" :key="group.category" :header="$t(`perm.category.${group.category}`)">
        <a-list :data-source="group.items" size="small">
          <template #renderItem="{ item }">
            <a-list-item>
              <a-list-item-meta>
                <template #title>
                  <code>{{ item.code }}</code>
                  <span style="margin-left: 12px;">{{ $t(`perm.${item.code.replace(/:/g, '.')}`) }}</span>
                </template>
                <template #description>{{ item.description }}</template>
              </a-list-item-meta>
            </a-list-item>
          </template>
        </a-list>
      </a-collapse-panel>
    </a-collapse>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import type { PermissionApi } from '@/api/permission'
import type { Permission } from '@/types/api'

const { t } = useI18n()
const permissionApi = inject<PermissionApi>('permissionApi')!

const all = ref<Permission[]>([])
const filter = ref('')
const activeKeys = ref<string[]>([])

interface PermGroup {
  category: string
  items: Permission[]
}

const filteredGroups = computed<PermGroup[]>(() => {
  const f = filter.value.trim().toLowerCase()
  const matched = f
    ? all.value.filter(p => p.code.toLowerCase().includes(f) || p.name.toLowerCase().includes(f) || p.description.toLowerCase().includes(f))
    : all.value
  const byCategory = new Map<string, Permission[]>()
  for (const p of matched) {
    const arr = byCategory.get(p.category) ?? []
    arr.push(p)
    byCategory.set(p.category, arr)
  }
  return Array.from(byCategory.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([category, items]) => ({ category, items: items.sort((a, b) => a.code.localeCompare(b.code)) }))
})

onMounted(async () => {
  try {
    all.value = await permissionApi.list()
    activeKeys.value = Array.from(new Set(all.value.map(p => p.category)))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
})
</script>
```

- [ ] **Step 2: Typecheck + lint + i18n:check**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run i18n:check
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/permissions/List.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/permissions): read-only grouped list with client-side filter"
```

---

## Task 15: system/audit-logs/List.vue

**Files:**
- Modify: `optimus-fe/src/views/system/audit-logs/List.vue` (full rewrite)

- [ ] **Step 1: Replace List.vue**

```vue
<template>
  <a-card>
    <PageHeader :title="$t('system.audit_logs.title')" />

    <div class="filter-row u-mb-16">
      <a-input
        v-model:value="actionInput"
        :placeholder="$t('system.audit_logs.filter_action')"
        style="width: 200px;"
        allow-clear
      />
      <a-input-number
        v-model:value="userIdInput"
        :placeholder="$t('system.audit_logs.filter_user_id')"
        style="width: 140px;"
      />
      <a-range-picker
        v-model:value="rangeInput"
        show-time
        :placeholder="[$t('system.audit_logs.filter_range'), '']"
      />
      <a-button type="primary" @click="applyFilters">{{ $t('system.audit_logs.filter_search') }}</a-button>
      <a-button @click="resetFilters">{{ $t('system.audit_logs.filter_reset') }}</a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      :expanded-row-render="expandedRowRender"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'created_at'">
          {{ formatTime(record.created_at) }}
        </template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="table.setPage"
      @show-size-change="(_, size) => table.setPageSize(size)"
    />
  </a-card>
</template>

<script setup lang="ts">
import { computed, h, inject, onMounted, ref } from 'vue'
import type { Dayjs } from 'dayjs'
import { useI18n } from '@/hooks/useI18n'
import { useTable } from '@/hooks/useTable'
import PageHeader from '@/components/PageHeader.vue'
import type { AuditApi } from '@/api/audit'
import type { AuditLogEntry, AuditListQuery } from '@/types/api'

const { t } = useI18n()
const auditApi = inject<AuditApi>('auditApi')!

const actionInput = ref('')
const userIdInput = ref<number | null>(null)
const rangeInput = ref<[Dayjs, Dayjs] | null>(null)

interface AuditFilters extends AuditListQuery {}

const table = useTable<AuditLogEntry, AuditFilters>({
  fetcher: async ({ page, pageSize, filters }) => {
    const r = await auditApi.list({
      page,
      page_size: pageSize,
      action: filters?.action || undefined,
      user_id: filters?.user_id,
      start: filters?.start,
      end: filters?.end
    })
    return { items: r.items, total: r.total }
  }
})

const columns = computed(() => [
  { key: 'created_at',  title: t('system.audit_logs.col_created_at'), width: 180 },
  { key: 'action',      title: t('system.audit_logs.col_action'),     dataIndex: 'action' },
  { key: 'user_id',     title: t('system.audit_logs.col_user_id'),    dataIndex: 'user_id', width: 100 },
  { key: 'target_type', title: t('system.audit_logs.col_target_type'),dataIndex: 'target_type', width: 140 },
  { key: 'target_id',   title: t('system.audit_logs.col_target_id'),  dataIndex: 'target_id', width: 140 },
  { key: 'ip',          title: t('system.audit_logs.col_ip'),         dataIndex: 'ip', width: 140 }
])

function expandedRowRender({ record }: { record: AuditLogEntry }) {
  const payload = record.payload === null || record.payload === undefined
    ? ''
    : JSON.stringify(record.payload, null, 2)
  return h('pre', {
    style: 'margin: 0; font-family: monospace; max-height: 400px; overflow: auto; background: #fafafa; padding: 12px; border-radius: 4px;'
  }, payload)
}

function applyFilters() {
  table.setFilters({
    action: actionInput.value || undefined,
    user_id: userIdInput.value ?? undefined,
    start: rangeInput.value?.[0] ? rangeInput.value[0].toISOString() : undefined,
    end: rangeInput.value?.[1] ? rangeInput.value[1].toISOString() : undefined
  })
}
function resetFilters() {
  actionInput.value = ''
  userIdInput.value = null
  rangeInput.value = null
  table.setFilters({ action: undefined, user_id: undefined, start: undefined, end: undefined })
}

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

onMounted(() => { table.reload() })
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}
</style>
```

- [ ] **Step 2: Typecheck + lint + i18n:check + tests + build**

```bash
cd optimus-fe && bun run typecheck && bun run lint && bun run i18n:check && bun run test && bun run build
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/system/audit-logs/List.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views/audit): paginated list with filters + payload <pre> expand"
```

---

## Task 16: §12 acceptance sweep + P0 release prep

**Files:**
- Verify (no edits expected unless something broken)
- Modify: `optimus-fe/README.md` (or root README.md) if any gaps in setup/run instructions
- Final commit + branch state for the `dev → main` PR

This task is verification-heavy. Every checkbox below MUST be confirmed (run the command, see output) before claiming complete. Stop and fix at the first failure.

- [ ] **Step 1: Full frontend sweep**

```bash
cd optimus-fe && bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build
```

Expected: ALL green. If any step fails, fix the underlying issue (do not skip), recommit, then re-run.

- [ ] **Step 2: Full backend regression (should still pass — 2b touched no BE)**

```bash
cd optimus-be && go test ./... -count=1 && go test ./... -tags=dbtest -count=1 && golangci-lint run ./...
```

Expected: ALL green. If any BE test fails, this represents a regression NOT caused by 2b — investigate via `git log -- optimus-be/` and address before proceeding.

If dbtest fails to start docker: confirm `DOCKER_HOST=unix:///Users/logic/.colima/docker.sock` is set in environment (per `project_colima_docker_socket` memory).

- [ ] **Step 3: Manual end-to-end smoke (the spec §12 functional acceptance list)**

```bash
# Terminal 1
docker compose up -d
cd optimus-be && make migrate-up && make run
# Terminal 2
cd optimus-fe && bun run dev
```

Then via browser at `http://localhost:5173`, log in as admin and verify EACH spec §12 functional bullet:

  - [ ] Empty DB self-bootstraps admin user (already verified in Plan 1; re-verify if DB was wiped)
  - [ ] Admin logs in, changes own password, creates a new user, creates a new role, binds permissions
  - [ ] A non-admin user only sees the menu items their role permits
  - [ ] Deleting a user frees the same username/email for re-registration
  - [ ] Refresh rotation works; replaying an old refresh token revokes all of that user's tokens (check with `audit-logs` page showing `auth.refresh.replay`)
  - [ ] zh-CN ↔ en-US toggle: full UI strings flip; backend error messages also translate via `message_key`
  - [ ] `/system/audit-logs` shows login/logout/user.*/role.*/menu.* events

Plus the 2b-added functional checks:

  - [ ] `/system/users`: search + status filter + paginate; create/edit/delete; toggle status; reset password; bind roles
  - [ ] `/system/roles`: create/edit/delete (built-in greyed); checkable-tree permission binding saves correctly
  - [ ] `/system/menus`: create root/child; drag a child to a different parent → confirm DB `parent_id` updated; drag within siblings → confirm `sort_order` changes; try to drop a parent onto its own descendant → see warning
  - [ ] `/system/permissions`: collapse panels per category; filter narrows results
  - [ ] `/profile`: edit just `display_name` → DevTools Network shows PUT body contains only `display_name`

- [ ] **Step 4: Engineering acceptance (spec §12)**

  - [ ] `make build` (backend) succeeds
  - [ ] `bun run build` (frontend) succeeds — confirmed in Step 1
  - [ ] CI on `dev` is green (or push and confirm). Quick check:

```bash
cd /Users/logic/Projects/optimus && gh pr checks $(git rev-parse HEAD) 2>/dev/null || gh run list --branch dev --limit 1
```

  - [ ] Backend core service coverage ≥ 60% (`make test` output)
  - [ ] dockertest integration covers login / refresh / user CRUD / role binding / permission check

- [ ] **Step 5: Security acceptance (spec §12)**

  - [ ] JWT secret startup validation in effect (env-based; verify `OPTIMUS_JWT_SECRET` ≥ 32 chars is required)
  - [ ] Login rate limit in effect (5/min per IP; quick curl loop should hit 429)
  - [ ] bcrypt cost ≥ 10 (check `optimus-be/configs/config.yaml`)
  - [ ] Soft-delete + partial unique index: delete a user, recreate with same username → succeeds
  - [ ] CORS whitelist effective
  - [ ] gosec runs in CI

- [ ] **Step 6: Push `dev` and open PR**

```bash
cd /Users/logic/Projects/optimus && git push origin dev
```

Open PR `dev → main` via `gh`:

```bash
gh pr create --base main --head dev --title "P0: platform skeleton (auth + RBAC + users/roles/menus/permissions/audit + FE skeleton + system pages)" --body "$(cat <<'EOF'
## Summary

P0 platform skeleton complete. Brings the entire scope of `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`:

- Backend: Gin + GORM + Postgres + JWT (access + refresh rotation + replay detection) + RBAC + audit + i18n + login rate limit
- Frontend: Vite + Vue 3 + ant-design-vue + Pinia (persisted) + vue-router + vue-i18n + axios single-flight refresh
- System pages: users / roles / menus / permissions / audit-logs full CRUD
- 8 DB tables + partial unique indexes + foreign keys

## Test plan

- [x] Backend unit + dbtest green (`make test` + dockertest)
- [x] Frontend lint / typecheck / i18n:check / test / build green
- [x] CI green
- [x] Spec §12 functional / engineering / security acceptance walked through manually (see plan §16)

## Related

- Spec: `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`
- 2a addendum: `docs/superpowers/specs/2026-06-08-p0-plan2a-fe-design-addendum.md`
- 2b addendum: `docs/superpowers/specs/2026-06-09-p0-plan2b-fe-design-addendum.md`
- Plan 2b: `docs/superpowers/plans/2026-06-09-p0-2b-frontend-system-pages.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Return the PR URL.

- [ ] **Step 7: Final commit — only if anything was modified in this task (often nothing)**

If you modified README or fixed an acceptance gap discovered during Step 3-5, commit it:

```bash
git -C /Users/logic/Projects/optimus add <paths>
git -C /Users/logic/Projects/optimus commit -m "chore(p0): final §12 acceptance sweep fixes"
```

Otherwise nothing to commit for Task 16 — the verification itself is the deliverable, and the PR is the release artifact.

**DO NOT MERGE THE PR.** The user reviews and merges manually. The plan is complete after the PR exists.

---

## Self-Review (writing-plans skill checklist)

**1. Spec coverage** — spec sections of the 2b addendum mapped to tasks:

| Spec §  | Coverage |
|---|---|
| Decisions #1 (users filter) | Task 9 (`a-input-search` + `a-select`) |
| Decisions #2 (roles tree) | Task 10 (RolePermissionsModal — checkable tree by category) |
| Decisions #3 (menu drag) | Task 12 (`computeDropTarget` TDD) + Task 13 (a-tree :draggable wiring) |
| Decisions #4 (audit `<pre>`) | Task 15 (`expandedRowRender` returns `<pre>`) |
| Decisions #5 (no URL sync) | Implicit — none of the page tasks sync to route.query |
| Decisions #6 (dev→main PR) | Task 16 Step 6 |
| Decision #7 (2a inheritance) | All tasks use hand-written DTOs, no ProTable, scoped SCSS |
| §3 BE contract alignment | Task 1 mirrors all DTOs |
| §4 directory increments | Tasks 2-15 create exactly the listed files |
| §5.1 useTable filters | Task 5 |
| §5.2 form-diff | Task 4 |
| §5.3.1 users page | Tasks 8 + 9 |
| §5.3.2 roles page | Tasks 10 + 11 |
| §5.3.3 menus page | Tasks 12 + 13 |
| §5.3.4 permissions | Task 14 |
| §5.3.5 audit-logs | Task 15 |
| §6 i18n increments | Task 6 |
| §7 task draft | Tasks 1-16 (consolidated from 16 to 16, batches match) |
| §8 acceptance | Task 16 |
| §9 risk: drag dropToGap edge cases | computeDropTarget tests cover before/after/inside + self-descendant |
| §9 risk: form-diff shallow misuse | form-diff doc string warns flat-only |

No gaps.

**2. Placeholder scan** — no "TBD" / "implement later" / "similar to Task N" present. All steps include either the actual code or the exact command + expected output.

**3. Type consistency** —
- `PageResp<T>` defined Task 1, consumed by `user.ts` Task 2 and `audit.ts` Task 3 — names match
- `formDiff` exported signature Task 4 ↔ imported Task 7, 8, 10, 12 — same
- `computeDropTarget(tree, dragId, dropId, dropPos, dropToGap)` Task 12 ↔ called Task 13 onDrop — same
- `useTable` extended Task 5 returns `{page, pageSize, items, total, loading, filters, reload, setPage, setPageSize, setFilters}` ↔ destructured in Tasks 9, 15 — match
- `UserApi.update(id, body)`, `setRoles(id, body)`, etc Task 2 ↔ used in Tasks 8, 9 — match
- All inject keys (`'userApi'`, `'roleApi'`, etc) provided in Task 3 main.ts changes ↔ injected in component tasks — match

No drift.
