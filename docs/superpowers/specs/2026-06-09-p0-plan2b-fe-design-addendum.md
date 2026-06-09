# P0 Plan 2b — 前端 system 五大页面设计补遗（Spec §8 + 2a Addendum 续）

- **项目**：optimus
- **关联 spec**：`docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md` §6/§7/§8
- **关联 addendum**：`docs/superpowers/specs/2026-06-08-p0-plan2a-fe-design-addendum.md`
- **日期**：2026-06-09
- **状态**：Approved（脑暴敲定）
- **范围**：P0 **Plan 2b** —— 把 `system/{users,roles,permissions,menus,audit-logs}` 五张 Coming-soon 占位替换为真实 CRUD；扩展 `useTable` filters 维度；抽 `form-diff` 工具复用 2a 的 partial-update 语义。

本文件是 spec §8 的进一步精确化与 2a addendum 的延续。当与上游冲突时，**以本文件为准**。

---

## 1. 范围与产出

**Plan 2b 包含**：

- 五张 system 页面真实 CRUD（替换 2a 的 Coming-soon 占位）
- `src/types/api.ts` 扩展：user / role / menu / permission / audit DTO 全量
- `src/api/` 新增：`user.ts` / `role.ts` / `menu.ts` / `permission.ts` / `audit.ts`
- `src/hooks/useTable.ts` 扩展：filters 维度
- `src/utils/form-diff.ts` 新建（TDD）：把 2a profile 表单的 partial-update 模式抽出，供 user 编辑复用
- `src/locales/{zh-CN,en-US}.json` 补齐五张页面所需的 key（form label / 按钮 / 操作确认 / table 列头 / 操作 toast）
- §12 spec 验收清单全部勾完后，**单 PR `dev → main` 合 P0**

**Plan 2b 不包含**（防 scope creep）：

- URL query sync（page/pageSize/filters 不入地址栏，纯组件态）
- Dashboard 真实内容（仍占位，留 P0 后续或 P1 决定）
- 前端 E2E（playwright 等）
- 任何 BE 改动（如发现 BE gap，按 2a 模式先补 BE 任务到 plan 顶部）
- Dockerfile / nginx / 生产 compose（→ Plan 3）

---

## 2. 已敲定的技术决策

| # | 维度 | 决策 |
|---|---|---|
| 1 | users 列表筛选 UI | 单 `a-input-search`（驱动 BE `Search` 字段，LIKE username/email）+ `a-select` 控 status（all / enabled / disabled） |
| 2 | roles 权限绑定控件 | `<a-tree :checkable>` 按 permissions.category 分组；顶层=category，叶子=permission code |
| 3 | menus 编辑 | antdv 原生 `<a-tree :draggable>`；on-drop 计算 parent_id + sort_order → `PUT /menus/:id` |
| 4 | audit-logs payload 渲染 | `a-table` 的 `expandedRowRender` 内 `<pre>{ JSON.stringify(payload, null, 2) }</pre>`，零新依赖 |
| 5 | URL query sync | 不做。5 页全部纯组件态；刷新丢筛选可接受 |
| 6 | dev→main 合并节奏 | Plan 2b 通过 spec §12 验收后，**单 PR `dev → main`** 完成 P0 release（Plan 3 Docker/nginx 后续另起 PR 或同批） |
| 7 | 继承 2a | hand-written DTOs / 零 ProTable·ProForm 封装 / 纯 antdv + scoped SCSS / 分层 TDD / vite proxy / setup-store / pinia-plugin-persistedstate |

---

## 3. 与 spec §6/§7 的 BE 契约对齐

`internal/modules/*/dto.go` 已 stable，FE DTO 一一镜像（hand-written）：

| FE type | BE 包/类型 |
|---|---|
| `UserSummary` | `user.Summary` |
| `UserDetail` | `user.Detail`（含 `roles: RoleRef[]`） |
| `UserCreateRequest` | `user.CreateRequest`（含可选 `role_ids[]`） |
| `UserUpdateRequest` | `user.UpdateRequest`（pointer 字段 → FE 用 `?:`） |
| `UserSetRolesRequest` | `user.SetRolesRequest` |
| `UserSetStatusRequest` | `user.SetStatusRequest`（`'enabled'\|'disabled'`） |
| `UserSetPasswordRequest` | `user.SetPasswordRequest` |
| `UserListQuery` | `user.ListQuery`（`search`, `status`） |
| `RoleSummary` / `RoleDetail` | `role.Summary` / `role.Detail`（detail 含 `permission_codes: string[]`） |
| `RoleCreateRequest` / `RoleUpdateRequest` | `role.CreateRequest` / `role.UpdateRequest` |
| `RoleSetPermissionsRequest` | `role.SetPermissionsRequest`（`permission_codes: string[]`） |
| `MenuNode` | `menu.Node`（含 `parent_id?`, `children?`） |
| `MenuCreateRequest` / `MenuUpdateRequest` | `menu.CreateRequest` / `menu.UpdateRequest` |
| `Permission` | `permission.Permission`（id/code/name/category/description） |
| `AuditLogEntry` | `audit.LogEntry`（`payload: unknown`，rawJSON 保持） |
| `AuditListQuery` | `audit.ListQuery`（action / user_id / start / end） |

`PageResp<T> = { items: T[]; total: number; page: number; page_size: number }` 提取到 `types/api.ts`（2a 中 useTable 已实质使用，这里集中导出供 5 个 api 模块复用）。

**字段 nullability 规约**：BE pointer (`*string` / `*uint64` / `*bool` 等) → FE 用 `field?: T`（可选属性），不用 `field: T | null`。FE 调 PUT/POST 时省略不发即可。这与 2a profile 的 `UpdateMeRequest` 用法一致。

---

## 4. 与 2a 目录结构的增量

**新增文件（17 个）**：

```
src/api/user.ts
src/api/role.ts
src/api/menu.ts
src/api/permission.ts
src/api/audit.ts
src/utils/form-diff.ts
src/utils/form-diff.test.ts
src/views/system/users/components/UserEditModal.vue
src/views/system/users/components/UserRolesModal.vue
src/views/system/users/components/UserResetPasswordModal.vue
src/views/system/roles/components/RoleEditModal.vue
src/views/system/roles/components/RolePermissionsModal.vue
src/views/system/menus/components/MenuEditModal.vue
```

**改写**（2a 占位 → 真实页面）：

```
src/views/system/users/List.vue
src/views/system/roles/List.vue
src/views/system/menus/List.vue
src/views/system/permissions/List.vue
src/views/system/audit-logs/List.vue
```

**扩展**（既有文件）：

```
src/types/api.ts             ← 加 user/role/menu/permission/audit DTOs + PageResp<T>
src/hooks/useTable.ts        ← 加 filters 维度
src/hooks/useTable.test.ts   ← 加 filters 用例
src/locales/zh-CN.json       ← 加 user/role/menu/perm/audit i18n keys
src/locales/en-US.json       ← 同步加
src/views/profile/Index.vue  ← 改用 utils/form-diff.ts（不再内联 diff）
```

`profile/Index.vue` 重构是“收紧 2a 遗留 partial-update 语义”的体现：把内联 diff 抽公共 util，2b 的 `UserEditModal` 直接复用。

---

## 5. 关键技术细节

### 5.1 `useTable` filters 扩展

`src/hooks/useTable.ts` 当前签名仅有 `page` / `pageSize`。Plan 2b 扩为：

```ts
export interface UseTableOptions<T, F = Record<string, unknown>> {
  fetcher: (req: PageRequest & { filters?: F }) => Promise<PageResult<T>>
  defaultPageSize?: number
  defaultFilters?: F
}

export function useTable<T, F = Record<string, unknown>>(opts: UseTableOptions<T, F>) {
  // ...原有 page/pageSize/items/total/loading...
  const filters = ref<F>(opts.defaultFilters ?? ({} as F))

  async function setFilters(patch: Partial<F>) {
    filters.value = { ...filters.value, ...patch } as F
    page.value = 1
    await reload()
  }

  // reload 内部把 filters.value 透传给 fetcher
  return { ..., filters, setFilters }
}
```

**约束**：

- filters 是不透明 `Record<string, unknown>`（或泛型 `F`），具体形状由调用方负责；hook 不解释字段语义
- `setFilters` 始终重置 `page=1`（搜索/筛选变化时回到第一页是 UX 常识）
- 既有 `setPage` / `setPageSize` / `reload` 行为不变
- **TDD 用例**：`setFilters` 合并语义、page 重置、reload 调用次数

### 5.2 `utils/form-diff.ts`（TDD）

```ts
// 浅对比：对每个 current 键，若与 initial 不等则纳入 diff 结果
// 用于把 form 当前值压成 PUT/PATCH 请求的 partial body
export function formDiff<T extends Record<string, unknown>>(initial: T, current: T): Partial<T>
```

**规约**：

- 只做浅比较（`Object.is`），不递归对象/数组
- `undefined` / `''` / `null` 一视同仁地参与对比；调用方负责语义（如把空字符串视为「清空」还是「未填」）
- 不变的键不出现在结果里
- BE 所有 Update DTO 都是平铺指针字段（`*string` / `*int` / `*bool`），浅比较足够

**TDD 用例**：

- 全相等 → 返回 `{}`
- 单字段变更 → 返回单键对象
- 增删字段（current 多/少 key）→ 多余键纳入 diff；缺失键不纳入（current 缺即未编辑）
- `null` 与 `undefined` 不相等
- 同值不同引用对象不算变更（浅）

### 5.3 五张页面 UX 与组件拆分

#### 5.3.1 `system/users/List.vue`

**布局**：

```
PageHeader
┌─ filter row ────────────────────────────────────┐
│  a-input-search (search)   a-select (status)    │
│                                       [+ Create]│
└─────────────────────────────────────────────────┘
a-table
  columns: username · email · display_name · status badge ·
           last_login_at · created_at · actions
  pagination: a-pagination (绑 useTable page/pageSize)
expand row: 无

actions per row:
  - 编辑 (UserEditModal mode=edit, prefill detail)
  - 角色 (UserRolesModal, prefill detail.roles)
  - 重置密码 (UserResetPasswordModal)
  - 状态切换 (a-popconfirm → PUT /users/:id/status)
  - 删除 (a-popconfirm → DELETE; admin 账号灰)
```

**模式**：

- `UserEditModal` 同一组件覆盖 create / edit，`mode: 'create' | 'edit'` prop 区分
  - create：POST `/users` with `CreateRequest`
  - edit：先 `formDiff(initial, current)` 得到 partial，PUT `/users/:id`
  - 与 2a profile 同一套 partial-update 思维
- `UserRolesModal`：a-checkbox-group 列出 `GET /roles` 全表；onOk → PUT `/users/:id/roles`
- `UserResetPasswordModal`：单 password 输入 + 确认 → PUT `/users/:id/password`（仅管理员调用，无 old_password 校验）
- 删除/状态切换/重置密码全部 v-permission 守门（`system:user:delete` / `system:user:write` / `system:user:reset_pass`）

#### 5.3.2 `system/roles/List.vue`

**布局**：

```
PageHeader
[+ Create]
a-table
  columns: code · name · description · is_builtin badge · created_at · actions
  (列表通常 <20 条，pagination 接但常一页装下)

actions per row:
  - 编辑 (RoleEditModal; is_builtin=true 则只允许改 name/description)
  - 绑权限 (RolePermissionsModal)
  - 删除 (a-popconfirm; is_builtin=true 灰)
```

**`RolePermissionsModal` 关键流程**：

```
打开 modal:
  并行 fetch:
    - GET /permissions          → flat Permission[]
    - GET /roles/:id            → RoleDetail.permission_codes
  本地 groupBy(permissions, p => p.category) → tree data:
    [
      { title: $t('perm.category.system'), key: '__cat:system', children: [
          { title: p.name + '(' + p.code + ')', key: p.code }, ...
      ]},
      ...
    ]
  checkedKeys 初始化 = role.permission_codes
确认:
  const checked = treeRef.getCheckedKeys({ checkStrictly: false }).filter(k => !k.startsWith('__cat:'))
  PUT /roles/:id/permissions { permission_codes: checked }
```

**a-tree 配置**：`checkable` + `checkStrictly: false`（让父子勾选联动）+ `default-expand-all`。Category 节点 key 前缀 `__cat:` 是哨兵，提交前过滤掉。

#### 5.3.3 `system/menus/List.vue`

**布局**：

```
PageHeader
[+ Create root]
a-tree
  :tree-data="menus"     (整树一次拉，GET /menus 已返完整树)
  :draggable             (antdv 原生)
  show-line
  default-expand-all
  custom :title slot:
    <span>{{ $t(node.name) }}</span>
    <code>{{ node.path }}</code>
    inline actions: 编辑 / 新建子节点 / 删除
```

**on-drop 处理**（antdv 事件签名 `{ event, node, dragNode, dropPosition, dropToGap }`）：

```ts
async function onDrop({ dragNode, node: dropNode, dropPosition, dropToGap }) {
  // dropToGap=false → 拖入 dropNode 内部（作为子节点）
  // dropToGap=true  → 拖到 dropNode 同级（before/after by dropPosition）
  const { parent_id, sort_order } = computeDropTarget(menus.value, dragNode, dropNode, dropPosition, dropToGap)
  await menuApi.update(dragNode.dataRef.id, { parent_id, sort_order })
  await refresh()
}
```

`computeDropTarget` 是纯函数，**TDD 覆盖**：

- 拖到 gap before/after 同级 → parent_id 不变，sort_order 取相邻两节点平均（或重排整层）
- 拖到节点内部 → parent_id = dropNode.id，sort_order = 子节点尾部 + 步进
- 自我拖到自身后代 → 拒绝（throw / antdv allowDrop 拦截）
- 排序号策略：使用「重排整层」更稳（避免浮点累积），即拖动后 PATCH 整层 sort_order 序列；但 BE 当前 UpdateRequest 只支持单条 PUT —— 取折中：**单条 PUT，sort_order 用整数大步距（10/20/30/…）+ 拖动时取相邻平均**。如未来步距碰撞，引一个 `POST /menus/:id/move` 端点（不在 2b 范围）

**MenuEditModal**：parent_id 用 `<a-tree-select>` 选父节点（root = null）；其余字段平铺 form。

**降级开关**：若 antdv 拖拽 onDrop 在多层嵌套出现位置错位（开发期 e2e 手测发现），fallback 到 Q3 Option B：行内 ↑/↓ 按钮 + edit modal 里改父级。这是风险预案，不是首选实现。

#### 5.3.4 `system/permissions/List.vue`

**纯只读**，不需要 modal。

```
PageHeader
a-input (filter, client-side; 模糊匹配 code/name/description)
a-collapse :active-key="所有 category"  (default 全展开)
  a-collapse-panel * len(categories):
    header: category 名 (i18n key: perm.category.<category>)
    a-list:
      a-list-item per permission:
        code (mono) | name (i18n: perm.<code>) | description
```

数据：`GET /permissions` 一次拉全，前端 groupBy + filter，不分页。

#### 5.3.5 `system/audit-logs/List.vue`

**布局**：

```
PageHeader
┌─ filter row ───────────────────────────────────────────────────┐
│  a-input (action LIKE)   a-input-number (user_id)              │
│  a-range-picker (created_at → start/end ISO)                   │
│                                              [Search] [Reset]  │
└────────────────────────────────────────────────────────────────┘
a-table
  columns: created_at · action · user_id · target_type · target_id · ip
  expandedRowRender:
    <pre style="margin:0; font-family:monospace; max-height:400px; overflow:auto;">
      {{ JSON.stringify(row.payload, null, 2) }}
    </pre>
  pagination: a-pagination
```

`AuditListQuery` 透传：`{ action, user_id, start, end }` 转 query string；range-picker 出 dayjs → toISOString。

---

## 6. i18n key 增量

`zh-CN.json` 与 `en-US.json` 同步追加（key 集合权威：zh-CN.json）。约 60-80 个新 key，按业务区组织：

```
system.users.*       (column headers / actions / modal titles / messages)
system.roles.*
system.menus.*
system.permissions.* (filter placeholder / empty)
system.audit_logs.*  (column headers / filter labels / payload empty)
form.*               (通用 form 校验提示，如 form.required / form.invalid_email)
perm.category.*      (category 标签：perm.category.system / perm.category.k8s / ...)
```

`scripts/check-i18n-keys.ts`（2a Task 12 引入）在 CI 跑，遗漏会 fail。每加一个 `$t('...')` 都必须两文件同步加 key。

---

## 7. Plan 2b 任务草稿（毛坯，正式拆分由 writing-plans 完成）

| # | 任务 | TDD? |
|---|---|---|
| 1 | `src/types/api.ts` 扩展：5 模块 DTO + `PageResp<T>` 导出 | no |
| 2 | `src/api/user.ts`（list / create / get / update / delete / setRoles / setStatus / setPassword） | no |
| 3 | `src/api/role.ts`（list / create / get / update / delete / setPermissions） | no |
| 4 | `src/api/menu.ts`（list / create / update / delete） | no |
| 5 | `src/api/permission.ts`（list） | no |
| 6 | `src/api/audit.ts`（list） | no |
| 7 | `src/utils/form-diff.ts` + tests | **yes** |
| 8 | `src/hooks/useTable.ts` 加 filters + 扩 tests | **yes** |
| 9 | i18n keys 增量（5 块业务 + form/perm.category） | no |
| 10 | `system/users/List.vue` + UserEditModal + UserRolesModal + UserResetPasswordModal | no |
| 11 | `views/profile/Index.vue` 重构改用 `form-diff` | no |
| 12 | `system/roles/List.vue` + RoleEditModal + RolePermissionsModal（含 a-tree groupBy 数据装配） | no |
| 13 | `system/menus/List.vue` + MenuEditModal + `computeDropTarget` 纯函数（**TDD**） | **yes**（仅 `computeDropTarget`） |
| 14 | `system/permissions/List.vue`（只读 + collapse + filter） | no |
| 15 | `system/audit-logs/List.vue`（filter + expandedRowRender） | no |
| 16 | spec §12 验收清单 sweep（功能 / 工程 / 安全） + 收尾 commit | no |

**子代理批次（6 批，与 2a 节奏一致）**：

| 批 | 任务 |
|---|---|
| 1 | #1 + #2 + #3 + #4 + #5 + #6 + #7（types + 5 api + form-diff TDD） |
| 2 | #8 + #9（useTable filters TDD + i18n keys） |
| 3 | #10 + #11（users 页 + 三 modal + profile 重构） |
| 4 | #12（roles 页 + 两 modal） |
| 5 | #13（menus 页 + drag 逻辑 TDD） |
| 6 | #14 + #15 + #16（permissions + audit + 验收 sweep） |

每批结束跑：`cd optimus-fe && bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build`

---

## 8. 验收（Plan 2b）

**功能**：

- [ ] `/system/users`：搜索 + status 筛选 + 分页工作；Create 弹 modal、Edit 只发改动字段、角色绑定、重置密码、状态切换、软删；admin 账号删除按钮灰
- [ ] `/system/roles`：CRUD + 绑权限（a-tree by category）+ is_builtin 灰删除按钮
- [ ] `/system/menus`：CRUD + 拖拽改父级 + 拖拽同级排序；拖到自身后代被拦截
- [ ] `/system/permissions`：按 category 折叠展示 + 顶部 filter 输入搜索
- [ ] `/system/audit-logs`：action / user_id / 时间范围三筛选生效；展开行显示 payload 美化 JSON
- [ ] `/profile`：重构后表单仍只发改动字段（行为不变，实现走 `form-diff`）

**工程**：

- [ ] `bun run lint` / `typecheck` / `i18n:check` / `test` / `build` 全绿
- [ ] CI `frontend` job 全绿
- [ ] 新 i18n key 双语种同步

**P0 release（spec §12 合并条件）**：

- [ ] spec §12 三档验收（功能/工程/安全）全勾
- [ ] 单 PR `dev → main` 合并；merge message 含 P0 完成清单链接

**显式不含（→ Plan 3 或后续）**：

- URL query sync（→ 后续）
- Dashboard 真内容（→ P0 后续或 P1）
- Dockerfile / nginx / 生产 compose（→ Plan 3）
- E2E（不在 P0）

---

## 9. 风险与降级

| 风险 | 降级 |
|---|---|
| antdv `draggable` 在深层菜单的 dropPosition/dropToGap 计算错位 | `computeDropTarget` TDD 覆盖；e2e 手测；若多层失败，fallback 到 Q3 Option B：行内 ↑/↓ 按钮 + edit modal 改父级（task 13 内修） |
| sort_order 整数步距碰撞（频繁拖动后相邻间隔=1） | 单次拖动后重新整层均分（前端 batch PUT）；P0 数据量低不至触发；若触发，加 BE `POST /menus/:id/move`（不在 2b） |
| roles 权限树 category 出现新 category 但 i18n key 未声明 → tree 节点显示原始字符串 | `perm.category.*` key 在 `i18n:check` 里加白名单；新增 category 走 issue/PR 同步加 key |
| audit payload 含 K8s yaml 等大对象时行展开撑爆视口 | `<pre>` 加 `max-height: 400px; overflow:auto`；P0 数据少不会触发 |
| `form-diff` 浅比较被误用到嵌套对象（如 `roles[]` 数组）| util doc 明确「仅平铺标量」；roles 绑定不用 diff（始终全量提交 `role_ids[]`） |
| `useTable` 加泛型 `<T, F>` 破坏 2a 既有调用方（目前只有 useTable.test 与未来 5 页） | 2a 当前没有真实页面用 useTable；2b 是第一批使用方；hooks 升级与 5 页同 PR 同 batch 出 |
| Plan 2b 合 main 时累计 commits 多（28+本 plan ≈ 50+）| 不 squash；保留 commits（每任务一 commit，与 2a 一致），merge commit 标题概括 P0 |

---

## 10. 后续步骤

1. 用户 review 本补遗 → approve
2. 调用 `superpowers:writing-plans` skill 把本补遗 + spec §6/§7 + 2a addendum 转为 `docs/superpowers/plans/2026-06-09-p0-2b-frontend-system-pages.md`
3. 用 `superpowers:subagent-driven-development` 执行；每子代理内部走分层 TDD（utils/hooks/纯函数强 TDD；Vue 组件不强制）
4. Plan 2b 收尾 → spec §12 三档验收 → 单 PR `dev → main` → P0 release
