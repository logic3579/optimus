# P0 Plan 2a — 前端骨架设计补遗（Spec §8 Addendum）

- **项目**：optimus
- **关联 spec**：`docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md` §8
- **日期**：2026-06-08
- **状态**：Approved（脑暴敲定）
- **范围**：本补遗只覆盖 P0 **Plan 2a**（前端骨架 + auth + layout + 通用基础设施）。Plan 2b（system 五大页面 + 收尾）另行设计。

本文件是 spec §8 的精确化与差异声明。当与 §8 不一致时，**以本文件为准**。

---

## 1. Plan 2 拆分

P0 前端按 spec §11 时间表的 W4-1 到 W5-5（约 9 工作日）整体不再单块交付，拆为两个 plan：

- **Plan 2a（本补遗）**：前端骨架、auth、layout、路由、通用基础设施、Coming-soon 占位、CI 接线
- **Plan 2b**：`system/{users,roles,permissions,menus,audit-logs}` 真实实现、主题/i18n 收尾、final polish

拆分动机：与 Plan 1A/1B/1C 节奏一致；单块体量 10-15 任务、子代理批次可控；Plan 2a 自身可单独 e2e 演示（admin 能登录、看占位、改密、切语言/主题、token refresh 工作）。

---

## 2. 已敲定的技术决策

| # | 维度 | 决策 |
|---|---|---|
| 1 | Plan 拆分 | 2a + 2b（见上） |
| 2 | TS 类型来源 | 手写 `src/types/api.ts`（不引入 openapi-typescript / swagger2openapi） |
| 3 | 样式 | 纯 antdv + scoped SCSS；不引入 Tailwind / UnoCSS |
| 4 | Pinia 持久化 | `pinia-plugin-persistedstate` |
| 5 | ProTable/ProForm | **零封装**；页面直接组装 antdv；`src/hooks/useTable.ts` 是唯一复用抓手 |
| 6 | i18n key 校验 | 手写 `scripts/check-i18n-keys.ts` + `bun run i18n:check` |
| 7 | 首次登录强制改密 | P0 着降；运维手工中转（README 写明） |
| 8 | TDD 边界 | 分层 TDD：`utils` / `stores` / `hooks` / `api/client` / `directives` / `scripts/check-i18n-keys` / `router.registerDynamicRoutes` 纯函数 → 强 TDD；Vue 组件 / 页面 / Layout → 不 TDD |
| 9 | 前后端联调 | vite proxy `/api/v1` → `http://localhost:8080`；前端代码 baseURL = `/api/v1`，与生产 nginx 同源策略对齐 |

---

## 3. 与 spec §8.1 目录结构的差异

**删**：

- `src/components/ProTable/` ← 决定 #5
- `src/components/ProForm/` ← 决定 #5

**加**：

- `src/components/AppSidebar.vue`、`src/components/AppHeader.vue`、`src/components/LangSwitch.vue`、`src/components/ThemeToggle.vue`（DefaultLayout 拆分用，避免单文件 > 300 行）
- `scripts/check-i18n-keys.ts`（决定 #6）
- `src/views/system/{users,roles,permissions,menus,audit-logs}/List.vue`（Plan 2a 是 Coming-soon 占位，见 §5；Plan 2b 完全重写）

**改注释**：

- `src/types/api.ts` 注释由"openapi-typescript 产物"改为"hand-written DTOs"（决定 #2）

保留：`PageHeader.vue` / `ConfirmButton.vue`、`hooks/{usePermission,useTable,useI18n}.ts`、`utils/{token,permission,http-error}.ts`、`directives/permission.ts`、三个 store、`router/{static-routes,guards,index}.ts`、两个 layout、`locales/{zh-CN,en-US}.json` + `index.ts`、`views/{auth/Login,profile/Index,dashboard/Index,errors/{403,404,500}}.vue`。

---

## 4. 关键技术细节（spec §8.2 – §8.7 的精确化）

### 4.1 axios single-flight refresh（§8.5）

```ts
// api/client.ts
let refreshing: Promise<TokenPair> | null = null

async function ensureFreshAccess(): Promise<TokenPair> {
  if (!refreshing) {
    refreshing = doRefresh().finally(() => { refreshing = null })
  }
  return refreshing
}
```

- response 拦截器收到 401：
  - 若原请求 URL = `/auth/refresh` → 直接 `auth.reset()` + `router.push('/login?redirect=...')`
  - 否则 `await ensureFreshAccess()`：成功 → 用新 access token 重试一次原请求；失败 → 同上 reset + redirect
- 不做 N 级嵌套重试。
- 并发 N 个请求同时 401 → 全部 `await` 同一个 `refreshing` Promise，refresh 后并发重试。

### 4.2 路由守卫（§8.3）

```ts
beforeEach(to, from):
  if to.meta.public                              -> next()
  if !auth.accessToken                            -> next({path:'/login', query:{redirect: to.fullPath}})
  if !auth.userLoaded:
    try Promise.all([api.me(), api.meMenus(), api.mePermissions()])
        -> 写 store -> registerDynamicRoutes(menu.tree) -> next({...to, replace:true})
    catch -> auth.reset() -> next({path:'/login', query:{redirect: to.fullPath}})
  if to.meta.permission && !usePermission().has(to.meta.permission) -> next('/403')
  else next()
```

`registerDynamicRoutes`：

- 遍历 `menu.tree`，用 `import.meta.glob('/src/views/**/*.vue')` 字典查 `${component}.vue`
- 命中：`router.addRoute('Root', { path, component, name: code, meta:{permission} })`
- 未命中：`console.warn` 并跳过（白屏防御，spec §8.2 已声明）
- `component === ''` 的节点视为分组，不注册路由但 sidebar 仍渲染为折叠组

### 4.3 Pinia store schema（§8.4）

setup-store 风格。`pinia-plugin-persistedstate` 用 `paths` 选择性持久化。

```ts
// stores/auth.ts
const accessToken  = ref<string | null>(null)
const refreshToken = ref<string | null>(null)
const user         = ref<MeResp | null>(null)
const permissions  = ref<string[]>([])
const userLoaded   = computed(() => user.value !== null)
function reset(): void
// persist: { storage: localStorage, paths: ['accessToken','refreshToken','user','permissions'] }

// stores/menu.ts
const tree = ref<MeMenuNode[]>([])
// persist: { storage: sessionStorage, paths: ['tree'] }

// stores/app.ts
const locale            = ref<'zh-CN'|'en-US'>('zh-CN')
const theme             = ref<'light'|'dark'>('light')
const sidebarCollapsed  = ref(false)
// persist: { storage: localStorage, paths: ['locale','theme','sidebarCollapsed'] }
```

### 4.4 `v-permission`（§8.7）

```ts
// v-permission="'system:user:write'"             -> string，相当于 has(one)
// v-permission="['a','b']"                         -> all（交集）
// v-permission:any="['a','b']"                    -> any（并集）
// 不通过 -> el.parentNode?.removeChild(el)
```

`mounted` + `updated` 各检查一次；从 `useAuthStore().permissions` 取集合（已是 string[]，包成 `Set` 后查）。

### 4.5 i18n key 校验（§8.6）

`scripts/check-i18n-keys.ts`：

1. glob `src/**/*.{vue,ts}`，正则抓 `\$t\(['"\`]([^'"\`)]+)['"\`]` / `i18n\.t\(['"\`]([^'"\`)]+)['"\`]` / `\bt\(['"\`]([^'"\`)]+)['"\`]`
2. 与 `locales/zh-CN.json` flatten 后的 key 集合对比；缺失 key → `exit 1`
3. `zh-CN.json` 与 `en-US.json` 的 key 集合 diff（任一边多/少）→ `exit 1`
4. 反向"声明但未使用"只 warn 不 fail（菜单 `menu.*` / 权限 `perm.*` 由后端 seed/registry 间接消费，前端文件里搜不到）

`zh-CN.json` 是 schema 权威；新增 key 在两个文件里同步加。`package.json` 加 `"i18n:check": "bun scripts/check-i18n-keys.ts"`，CI 在 `build` 之前跑。

### 4.6 工具与版本默认值

| 项 | 选型 |
|---|---|
| Vue | 3.4.x（latest stable minor） |
| ant-design-vue | 4.x（latest 4） |
| TypeScript | 5.x；`strict: true`、`noUncheckedIndexedAccess: true` |
| ESLint | flat config + `eslint-plugin-vue` + `@typescript-eslint` + `eslint-config-prettier` |
| 格式化 | 用 `eslint --fix` 兜底；不引 Prettier 独立工具 |
| Vitest 环境 | `jsdom` |
| Type-check | `vue-tsc --noEmit`，命令 `bun run typecheck` |
| 包管理 | bun ≥ 1.1；仓库提交 `bun.lockb`；安装用 `bun install --frozen-lockfile` |
| Pinia 写法 | setup-store |

### 4.7 vite dev proxy

```ts
// vite.config.ts
server: {
  port: 5173,
  proxy: { '/api/v1': { target: 'http://localhost:8080', changeOrigin: false } }
}
```

axios baseURL 直接写 `/api/v1`，开发/生产同路径；`changeOrigin: false` 因后端 cors 白名单已含 `http://localhost:5173`。

---

## 5. Coming-soon 占位（影响 Plan 2a 的额外结论）

后端 1A/1B/1C seed 已写入完整菜单树 `dashboard + system/{users,roles,permissions,menus,audit-logs}`，component 字段分别为：

- `dashboard/Index`
- `system/users/List`
- `system/roles/List`
- `system/permissions/List`
- `system/menus/List`
- `system/audit-logs/List`

为避免登录后 sidebar 五项 system 菜单全部被 §8.2 的"无对应文件 → 跳过 + warn"兜底吃掉（点击没反应），Plan 2a 需交付这 6 个文件的 **Coming-soon 占位**：

```vue
<!-- src/views/system/users/List.vue 等 -->
<template>
  <a-card>
    <a-empty :description="$t('placeholder.coming_soon')" />
  </a-card>
</template>
```

Plan 2b 完整重写这 5 张 system 页面；`dashboard/Index.vue` 也是 Plan 2a 的 Coming-soon 占位，其在 2b 之后是否换成真 dashboard 由 P0 后续决定（不在本补遗范围）。

---

## 6. Plan 2a 任务草稿（毛坯，正式拆分由 writing-plans 完成）

| # | 任务 | TDD? |
|---|---|---|
| 1 | 仓库脚手架：`bun create vite`、tsconfig、eslint flat、`bun.lockb` 入库、`.editorconfig`、`.gitignore` 增 `optimus-fe/{dist,node_modules}` | no |
| 2 | `vite.config.ts`（proxy `/api/v1`→8080、alias `@→src`、SCSS）、`.env.development` / `.env.production` | no |
| 3 | `src/types/api.ts` 手写 DTO（Envelope / Login / Refresh / Me / MeMenuNode / MePermissions / UpdateMe / ChangePassword） | no |
| 4 | `src/utils/token.ts` + `src/utils/http-error.ts`（BizError + parse Envelope） | **yes** |
| 5 | `src/utils/permission.ts`（has / hasAll / hasAny） | **yes** |
| 6 | `src/stores/{auth,menu,app}.ts` + persistedstate 接线 | **yes** |
| 7 | `src/api/client.ts`（axios + single-flight refresh + 401 重试 + Accept-Language 注入） | **yes**（含并发 401 用例） |
| 8 | `src/api/{auth,me}.ts` 端点封装 | no |
| 9 | `src/hooks/{usePermission,useTable,useI18n}.ts` | **yes** |
| 10 | `src/directives/permission.ts` + `directives/index.ts` 安装 | **yes** |
| 11 | `src/locales/{zh-CN,en-US}.json` 初始 key 集 + `src/locales/index.ts`（vue-i18n + antdv locale 联动） | no |
| 12 | `scripts/check-i18n-keys.ts` + `package.json` 加 `i18n:check` | **yes**（脚本可单元测） |
| 13 | `src/router/{static-routes,guards,index}.ts`（含 `registerDynamicRoutes`） | **yes**（`registerDynamicRoutes` 纯函数部分） |
| 14 | `src/layouts/BlankLayout.vue` + `src/layouts/DefaultLayout.vue` + 子组件 `AppSidebar/AppHeader/LangSwitch/ThemeToggle` | no |
| 15 | `src/components/{PageHeader,ConfirmButton}.vue` | no |
| 16 | `src/views/auth/Login.vue` + `src/views/errors/{403,404,500}.vue` | no |
| 17 | `src/views/dashboard/Index.vue` + 5 张 `system/*/List.vue` Coming-soon 占位 + `views/profile/Index.vue`（编辑 + 改密 form） | no |
| 18 | `src/main.ts`（pinia + persistedstate + router + i18n + antdv + directives 接线）+ `App.vue` + `index.html` + `README.md` + `.github/workflows/ci.yml` 增 frontend job | no |

子代理批次（建议 8 批，每批 2-3 任务，与 1B/1C 经验一致）：

| 批 | 任务 |
|---|---|
| 1 | #1 + #2 + #11 |
| 2 | #3 + #4 + #5 |
| 3 | #6 + #8 |
| 4 | #7（独立成批降并发风险） |
| 5 | #9 + #10 |
| 6 | #12 + #13 |
| 7 | #14 + #15 + #16 |
| 8 | #17 + #18 |

每批结束跑：`bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build`。

---

## 7. 验收（Plan 2a）

**功能**：

- [ ] `bun install && bun run dev` 起 5173；访问 `/` 重定向到 `/login`
- [ ] 后端跑起后 admin 凭据登录 → 进入 `/dashboard` 占位
- [ ] 侧栏渲染后端菜单树（dashboard + system 五项 Coming-soon）
- [ ] 顶栏：语言切换（zh-CN/en-US，antdv ConfigProvider locale + vue-i18n 同步切，持久化）、主题切换（light/dark，antdv theme algorithm，持久化）、用户菜单（个人资料 / 退出登录）
- [ ] `/profile` 可编辑 display_name / email / avatar_url；可改密码（old + new + confirm）
- [ ] access token 到期触发 refresh 自动重试；并发 401 共享同一 refresh Promise
- [ ] refresh 失败（伪造 token）→ 自动 logout + `/login?redirect=/profile`
- [ ] 非 admin 用户访问 `/system/users` → `/403`
- [ ] `/404`、`/403`、`/500` 三个错误页直接访问可达
- [ ] 关闭浏览器再开：localStorage 持久化命中 → 自动三连 → 注入路由 → 落到原 URL

**工程**：

- [ ] `bun run lint` / `bun run typecheck` / `bun run i18n:check` / `bun run test` / `bun run build` 全绿
- [ ] CI `frontend` job 全绿

**显式不含（→ Plan 2b 或 Plan 3）**：

- system 五张页面真实 CRUD / 树编辑 / 权限穿梭 / 审计筛选 → 2b
- 强制首次登录改密 → P0 着降
- Dockerfile / nginx → Plan 3
- E2E / playwright → 不在 P0

---

## 8. 风险与降级

| 风险 | 降级 |
|---|---|
| `pinia-plugin-persistedstate` 与 setup-store 写法不亲（hydration 时序、ref 不识别等） | 改用 hand-rolled subscribe + `watch`；store schema 不变；TDD 用例覆盖 reset/hydrate |
| antdv 4.x 与 vue-i18n 9 在 ConfigProvider locale 切换上有时序问题 | locale 切换后 `window.location.reload()` 兜底；记 TODO 到 2b 修 |
| single-flight refresh 在 jsdom 下并发用例时序敏感 | 用 fake timers + 显式 await microtask 队列；任务 #7 独立成批 |
| backend `last_login_at` 未在 me/password 路径更新 → "首次登录"判定不可用 | 决定 #7 已着降；不依赖此字段 |
| 5 张 Coming-soon 占位与 2b 真实实现差异大 → 2b 不能简单 diff 覆盖 | 占位极简（`<a-empty>` + `$t('placeholder.coming_soon')`）；2b 完全重写而非 diff |
| 后端 `/me` 返回 user 但不返回 permissions，需第三次请求 `/me/permissions` | 接受 3 次并发请求；用 `Promise.all` 起；前端 retry 失败回退 logout |

---

## 9. 后续步骤

1. 用户 review 本补遗 → approve（已完成）
2. 调用 `superpowers:writing-plans` skill 把本补遗 + spec §8 转为 Plan 2a 实现计划
3. 用 `superpowers:subagent-driven-development` 执行，每子代理内部走 TDD（按 §2 决定 #8 的分层 TDD）
4. Plan 2a 合到 `dev` 后进入 Plan 2b 设计
