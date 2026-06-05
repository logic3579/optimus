# P0: platform-skeleton 设计文档

- **项目**：optimus（DevOps 平台）
- **子项目**：P0 — platform-skeleton
- **日期**：2026-06-05
- **状态**：Draft → Approved

---

## 1. 背景与项目分解

optimus 是一个面向内部小团队（< 50 人，单租户）的 DevOps 平台，覆盖资产管理、K8s 管理、应用管理、可观测性、系统设置等多个领域。

因为整体范围跨越多个独立子系统，无法用单一 spec 涵盖。我们将其拆为 7 个子项目（DAG）：

```
[P0] platform-skeleton    # 后端骨架 + 前端骨架 + 登录 + 用户/角色/权限
       │
       ▼
[P1] credentials-vault    # SSH key / kubeconfig / 云凭证管理
       │
       ├──────────┬─────────────┬──────────────┐
       ▼          ▼             ▼              ▼
[P2] k8s-mgmt  [P4] assets  [P5] observability  [P6] cicd
       │
       ▼
[P3] applications
```

**P0 (本 spec)** 是其余所有子项目的硬前置依赖。它本身不解决任何 DevOps 域的业务问题，目标是为后续模块提供：
- 统一认证 / 授权 / 错误体系
- 统一的 API 包络、错误码、i18n
- 统一前端布局、路由、组件库、HTTP 封装
- 统一 audit / 日志 / 配置
- 统一本地开发 / 构建 / 部署流程

---

## 2. P0 范围

**P0 包含**：
- 后端骨架（Gin + GORM + Postgres）
- 前端骨架（Vite + Vue3 + Ant Design Vue + Pinia + vue-router + vue-i18n，从零搭）
- 登录 + JWT (access + refresh, refresh rotation, 重放检测)
- 用户管理（CRUD + 状态切换 + 角色绑定 + 密码重置）
- 角色管理（CRUD + 权限绑定）
- 菜单管理（树结构 + 动态权限绑定）
- 权限列表（代码注册，UI 只读）
- 操作审计（关键事件）
- 个人资料 / 改密
- 中英文 i18n

**P0 不包含**（防 scope creep）：
- 第三方登录 / SSO / OIDC / LDAP
- 用户分组 / 团队
- 密钥与凭证管理 → P1
- 通知系统（邮件/IM/webhook）
- 操作审计的归档 / 导出
- 国际化语言扩展（仅中英）
- 自定义主题（仅 antdv 默认 + dark）
- 移动端适配
- 多 tab 路由缓存
- WebSocket 服务端推送

---

## 3. 已锁定的技术决策

| 维度 | 决策 |
|---|---|
| 目标用户 | 内部小团队 < 50 人，单租户 |
| 仓库结构 | Monorepo：`optimus/{optimus-fe, optimus-be, docs}` |
| 前端栈 | Vite + Vue3 + Ant Design Vue + Pinia + vue-router + vue-i18n（从零搭） |
| 前端包管理 | **bun**（不使用 npm/pnpm/yarn） |
| 后端栈 | Go + Gin + GORM + PostgreSQL |
| 后端布局 | 按业务模块聚合：`internal/modules/{auth,user,rbac,...}/` |
| 认证 | 仅本地账号 + JWT (access + refresh)，refresh rotation + 重放检测 |
| 权限模型 | 中粒度 RBAC：User N:M Role N:M Permission，permission code 形如 `k8s:workload:read` |
| 权限注册 | **代码常量注册**，启动时 upsert，不靠 UI 增删 |
| i18n | 前端 vue-i18n；后端错误响应带 `message_key`，前端查 locale 文件翻译 |

## 4. 工程默认

| 项 | 选型 |
|---|---|
| 配置 | viper（YAML + env override） |
| 数据库迁移 | goose（SQL 迁移文件） |
| 日志 | slog（Go 1.21+ 标准库，结构化 JSON） |
| API 文档 | swag (swaggo)，注解生成 OpenAPI |
| 密码哈希 | bcrypt，cost 10 |
| 后端测试 | testify + dockertest（真实 Postgres） |
| 前端测试 | vitest（仅 utils / stores / 关键 hooks） |
| 本地开发 | docker-compose（Postgres + Adminer） |
| 生产部署 | 二进制 + Dockerfile，单机 docker-compose；K8s 部署延迟到 P2 |
| 热重载 | air（后端），vite（前端） |
| CI | GitHub Actions |
| Mock | 不引入 MSW |

---

## 5. 顶层架构

```
┌────────────────────┐         ┌──────────────────────────────┐
│   optimus-fe       │  HTTPS  │     optimus-be (Gin)         │
│  Vue3 + AntdV      │ ──────► │ ┌──────────────────────────┐ │
│                    │  JWT    │ │ Middleware:              │ │
│  - LoginView       │         │ │  RequestID/Logger/Recover│ │
│  - Layout (菜单动 │         │ │  CORS / JWT / I18n /     │ │
│    态渲染)         │         │ │  RBAC / Audit            │ │
│  - User/Role/Perm  │         │ └──────────────────────────┘ │
│  - i18n (zh/en)    │         │ ┌──────────────────────────┐ │
└────────────────────┘         │ │ modules/                 │ │
                                │ │  auth/ user/ rbac/ menu/ │ │
                                │ │  audit/ health/          │ │
                                │ └──────────────────────────┘ │
                                │ ┌──────────────────────────┐ │
                                │ │ infra/                   │ │
                                │ │  db (GORM) / config /    │ │
                                │ │  log / errors / i18n /   │ │
                                │ │  middleware              │ │
                                │ └──────────────────────────┘ │
                                └──────────────┬───────────────┘
                                               │
                                          PostgreSQL
```

P0 不引入 Redis。JWT 无状态，refresh token 存 DB；权限缓存用进程内 `sync.Map`，TTL 60s。

---

## 6. 数据模型

P0 共 8 张表。所有需要时间审计的表带 `id BIGSERIAL`、`created_at`、`updated_at`。仅 `users` / `roles` / `menus` 带 `deleted_at`（软删）；其他硬删。

### 6.1 表结构

```
┌─────────────────────┐         ┌─────────────────────┐
│ users               │         │ roles               │
│ ─────────────────── │         │ ─────────────────── │
│ id                  │         │ id                  │
│ username            │         │ code                │  e.g. admin / operator / viewer
│ email               │         │ name                │  显示名（i18n key）
│ password_hash       │         │ description         │
│ display_name        │         │ is_builtin (bool)   │  内置角色不可删
│ avatar_url          │         │ created_at          │
│ status              │         │ updated_at          │
│ last_login_at       │         │ deleted_at          │
│ created_by          │         └─────────────────────┘
│ created_at          │
│ updated_at          │         ┌─────────────────────┐
│ deleted_at          │         │ permissions         │
└─────────────────────┘         │ ─────────────────── │
                                │ id                  │
┌─────────────────────┐         │ code (UNIQUE)       │  k8s:workload:read
│ user_roles          │         │ name (i18n key)     │
│ ─────────────────── │         │ category            │  k8s / assets / system
│ user_id ─┐          │         │ description         │
│ role_id ─┴ PK       │         │ created_at          │
│ created_at          │         │ updated_at          │
└─────────────────────┘         └─────────────────────┘

┌─────────────────────┐         ┌─────────────────────┐
│ role_permissions    │         │ menus               │
│ ─────────────────── │         │ ─────────────────── │
│ role_id ─┐          │         │ id                  │
│ perm_id ─┴ PK       │         │ parent_id (NULL→根) │
│ created_at          │         │ code                │
└─────────────────────┘         │ name (i18n key)     │
                                │ path                │  e.g. /system/users
┌─────────────────────┐         │ component           │  e.g. system/users/List
│ refresh_tokens      │         │ icon                │
│ ─────────────────── │         │ permission_code     │  NULL=公开
│ id                  │         │ sort_order          │
│ user_id             │         │ hidden (bool)       │
│ token_hash          │ sha256  │ created_at          │
│ expires_at          │         │ updated_at          │
│ revoked_at          │         │ deleted_at          │
│ user_agent          │         └─────────────────────┘
│ ip                  │
│ created_at          │
└─────────────────────┘         ┌─────────────────────┐
                                │ audit_logs          │
                                │ ─────────────────── │
                                │ id                  │
                                │ user_id (NULL=匿名) │  ON DELETE SET NULL
                                │ action              │  login / logout / user.create
                                │ target_type         │  user / role / permission / menu
                                │ target_id           │
                                │ payload (JSONB)     │  操作上下文（create→after, update→before/after diff, delete→before, 其它→任意 JSON）
                                │ ip / user_agent     │
                                │ created_at          │
                                └─────────────────────┘
```

### 6.2 软删除与唯一键处理

GORM 默认软删除 + 普通 UNIQUE 约束的组合，会导致**同名用户无法再注册**等问题。处理方案：

**1) 重新审视哪些表需要软删**

| 表 | 是否软删 | 理由 |
|---|---|---|
| `users` | 软删 | 保 audit_logs 外键引用 |
| `roles` | 软删 | 同上 |
| `menus` | 软删 | 误删可恢复 |
| `permissions` | 硬删 | 代码注册常量，留陈旧行无害 |
| `user_roles` / `role_permissions` | 硬删 | 关联表 |
| `refresh_tokens` | 硬删 | 用 `revoked_at` 标记撤销 |
| `audit_logs` | 永不删 | 不可变 |

**2) Postgres 部分唯一索引（partial unique index）**

GORM model 字段**不写 `uniqueIndex` tag**，唯一性交给迁移：

```sql
CREATE UNIQUE INDEX users_username_uniq ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_email_uniq    ON users (email)    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX roles_code_uniq     ON roles (code)     WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX menus_code_uniq     ON menus (code)     WHERE deleted_at IS NULL;
```

效果：删除后旧 username 立即可被新账号占用，旧行仍留存供 audit 引用。

**3) 外键策略**

```sql
-- audit 必须在 user 硬删后仍可读
ALTER TABLE audit_logs
  ADD CONSTRAINT fk_user FOREIGN KEY (user_id)
  REFERENCES users(id) ON DELETE SET NULL;

-- 关联表跟随主表硬删
ALTER TABLE user_roles
  ADD CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_role FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
ALTER TABLE role_permissions
  ADD CONSTRAINT fk_role FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_perm FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE;
```

软删除 (`deleted_at IS NOT NULL`) 不触发 `ON DELETE`，所以软删 user 不会自动清 user_roles。**应用层在软删 user 的同一事务里同步删该用户的 user_roles 与 refresh_tokens**。

### 6.3 关键约定

- 菜单与权限通过 `menus.permission_code` 字符串关联（不强制 FK），允许菜单先于权限存在。
- 权限码由各业务模块在代码中以常量声明（`internal/infra/permissions/codes.go`），启动时 upsert 到 `permissions` 表。
- 初始 admin 用户：DB 为空时启动自动 seed，密码随机生成打印到 stdout（强制首次登录改）。
- 内置角色：`admin`（拥有全部权限）、`viewer`（仅 `*:*:read`）。

---

## 7. API 设计与认证流程

### 7.1 响应包络

```json
// 成功
{ "code": 0, "data": { ... }, "message": "" }

// 失败
{ "code": 40101, "data": null, "message": "invalid credentials", "message_key": "auth.invalid_credentials" }
```

- `code`：业务错误码（5 位），0 = 成功
- `message`：英文兜底（日志直接可读）
- `message_key`：i18n key，前端查 locale 文件翻译
- HTTP 状态码独立语义化（400/401/403/404/409/500）

**错误码分段**：
- `1xxxx` 系统级（panic / db / timeout）
- `4xxxx` 客户端错（与 HTTP 4xx 对应）
- `5xxxx` 服务端业务错

**分页**：`?page=1&page_size=20`，返回 `{ items, total, page, page_size }`。

**路由前缀**：`/api/v1/...`。

### 7.2 端点清单

```
[Public]
POST   /api/v1/auth/login              用户名+密码 → access + refresh
POST   /api/v1/auth/refresh            refresh → 新 access + 新 refresh（rotation）
POST   /api/v1/auth/logout             撤销当前 refresh（idempotent）
GET    /api/v1/health                  健康检查（含 db ping + version）

[Authenticated - 当前用户]
GET    /api/v1/me                      当前用户信息
PUT    /api/v1/me                      修改 display_name/avatar/email
PUT    /api/v1/me/password             改密码（需 old_password）
GET    /api/v1/me/menus                返回当前用户可见菜单树
GET    /api/v1/me/permissions          返回当前用户拥有的所有权限码

[Admin - system:user:*]
GET    /api/v1/users                   分页 + 搜索（username/email/status）
POST   /api/v1/users
GET    /api/v1/users/:id
PUT    /api/v1/users/:id
DELETE /api/v1/users/:id               软删（用户不能删自己；内置 admin 账号不可删）
PUT    /api/v1/users/:id/roles         批量设置角色
PUT    /api/v1/users/:id/status        enabled/disabled
PUT    /api/v1/users/:id/password      管理员重置密码

[Admin - system:role:*]
GET    /api/v1/roles
POST   /api/v1/roles
GET    /api/v1/roles/:id
PUT    /api/v1/roles/:id
DELETE /api/v1/roles/:id               软删（is_builtin 拒绝）
PUT    /api/v1/roles/:id/permissions   批量设置权限

[Admin - system:permission:read]
GET    /api/v1/permissions             权限码全列表

[Admin - system:menu:*]
GET    /api/v1/menus                   完整树
POST   /api/v1/menus
PUT    /api/v1/menus/:id
DELETE /api/v1/menus/:id               软删

[Admin - system:audit:read]
GET    /api/v1/audit-logs              分页 + 筛选（action/user_id/起止时间）
```

### 7.3 认证流程

**Access Token**：
- HS256，TTL 15 min
- 载荷：`{user_id, jti, iat, exp}`，**不包含 permissions**

**Refresh Token**：
- 随机 256-bit，sha256 后存 `refresh_tokens.token_hash`（不存原文）
- TTL 7 天
- 每次 refresh 撤销旧 token 并签发新对（rotation）

**Refresh 重放检测**：
- 收到 refresh 请求，若对应 hash 已 revoked，**视为 token 被泄露**
- 撤销该 user 全部未过期 refresh_tokens
- 记录 audit (`action=auth.refresh.replay`)
- 返回 401

**JWT 密钥**：从配置 / env 读，启动时强校验非空且长度 ≥ 32。

### 7.4 权限校验路径

```
JWT middleware → ctx.user_id
   ↓
RBAC middleware（route 声明所需权限码）
   ↓
查内存 cache (sync.Map)：user_id → []permission_codes，TTL 60s
   ↓ cache miss
   JOIN user_roles + role_permissions + permissions
   ↓
contains(required) ? next : 403
```

**Cache 失效**：
- 用户角色变更、角色权限变更 → 主动清除该 user_id 的缓存
- 角色权限变更影响多个用户 → 批量清除该角色的所有用户的缓存
- **作用域为单进程**：P0 是单实例部署，sync.Map 即可；如未来要多副本，需改为带 pub/sub 的分布式 cache（不在 P0）

### 7.5 中间件链顺序

```
[请求]
 → RequestID         （生成 X-Request-ID）
 → Logger            （结构化 JSON：method/path/status/latency/request_id/user_id）
 → Recover           （panic → 500 + audit）
 → CORS              （配置白名单）
 → JWT Auth          （白名单：/auth/login /auth/refresh /health；其他必须 Bearer）
 → I18n              （Accept-Language → ctx.lang）
 → RBAC              （per-route，由路由表声明 permission code）
 → Handler
 → AuditWriter       （由 handler 主动调用 ctx.Audit(...)）
```

### 7.6 安全要点

- bcrypt cost 10
- **登录限速**：同 IP / 同 username 5 次/分钟 → 限速 1 分钟，使用 `golang.org/x/time/rate`，不引 Redis
- JWT secret 启动校验长度 ≥ 32
- HTTPS：交给反代（nginx/caddy）终结
- CSRF：用 Bearer Token，不用 cookie，天然免疫

---

## 8. 前端架构

### 8.1 目录结构

```
optimus-fe/
├── public/
├── src/
│   ├── api/                   axios 封装 + 各模块接口
│   │   ├── client.ts
│   │   ├── auth.ts / user.ts / role.ts / permission.ts / menu.ts / audit.ts
│   ├── assets/
│   ├── components/            通用组件
│   │   ├── ProTable/          列表+分页+搜索封装
│   │   ├── ProForm/           表单封装
│   │   ├── PageHeader/
│   │   └── ConfirmButton/
│   ├── directives/
│   │   ├── permission.ts      v-permission
│   │   └── index.ts
│   ├── hooks/
│   │   ├── usePermission.ts
│   │   ├── useTable.ts
│   │   └── useI18n.ts
│   ├── layouts/
│   │   ├── DefaultLayout.vue  侧栏+顶栏+内容区
│   │   └── BlankLayout.vue    登录页
│   ├── locales/
│   │   ├── zh-CN.json
│   │   ├── en-US.json
│   │   └── index.ts
│   ├── router/
│   │   ├── index.ts
│   │   ├── guards.ts
│   │   └── static-routes.ts
│   ├── stores/                Pinia
│   │   ├── auth.ts            user / tokens / permissions
│   │   ├── menu.ts            menuTree
│   │   ├── app.ts             locale / theme / sidebarCollapsed
│   │   └── index.ts
│   ├── types/
│   │   ├── api.ts             openapi-typescript 产物
│   │   └── domain.ts
│   ├── utils/
│   │   ├── token.ts
│   │   ├── permission.ts
│   │   └── http-error.ts
│   ├── views/
│   │   ├── auth/Login.vue
│   │   ├── dashboard/Index.vue
│   │   ├── system/
│   │   │   ├── users/{List,Detail}.vue
│   │   │   ├── roles/
│   │   │   ├── menus/
│   │   │   ├── permissions/
│   │   │   └── audit-logs/
│   │   ├── profile/Index.vue
│   │   └── errors/{403,404,500}.vue
│   ├── App.vue
│   ├── main.ts
│   └── env.d.ts
├── .env.development
├── .env.production
├── index.html
├── tsconfig.json
├── vite.config.ts
├── package.json
├── bun.lockb
└── README.md
```

### 8.2 路由策略

**静态路由**（写死）：`/login`、`/403`、`/404`、`/500`、`/profile`

**动态路由**（登录后注入）：
1. 登录成功后 store 写入 user/tokens/permissions
2. 调 `GET /me/menus` 拿菜单树
3. 用 `import.meta.glob('/src/views/**/*.vue')` 把菜单 `component` 字段映射到组件
4. 通过 `router.addRoute()` 注入

**约定**：`menus.component = "system/users/List"` → `/src/views/system/users/List.vue`

**运行时校验**：动态路由注入时若菜单 `component` 无对应文件，跳过注入并 console.warn（不白屏）。已知 component 路径列表维护在 `docs/menus-components.md`，供后端 admin 配菜单时参考。

### 8.3 路由守卫

```
beforeEach:
  ├ 公开路由（login/404/...）? → next()
  ├ 无 access token? → /login（带 redirect）
  ├ 有 token 但 store 无 user 信息?
  │   并行调 /me + /me/menus + /me/permissions
  │   写 stores → 注入动态路由 → next({...to, replace: true})
  ├ to.meta.permission 存在且用户无此权限? → /403
  └ next()
```

### 8.4 Pinia stores

| Store | 持有 | 持久化 |
|---|---|---|
| `auth` | user / accessToken / refreshToken / permissions[] | localStorage |
| `menu` | menuTree | sessionStorage |
| `app` | locale / theme / sidebarCollapsed | localStorage |

### 8.5 axios 封装

- **request**：注入 `Authorization`、`Accept-Language`
- **response success**：检查 `body.code`，非 0 抛 `BizError`（携带 `message_key`）
- **response 401**：
  - 若是 `/auth/refresh` 本身 401 → 直接登出
  - 否则用 **single-flight** 模式（模块级 `let refreshing: Promise<...> | null`，所有遇到 401 的请求 `await` 同一个 Promise）串行化 refresh，避免并发 401 触发 N 次 refresh
  - refresh 成功 → 重试原请求
  - refresh 失败 → 登出 + 跳 `/login`
- **网络错误 / 5xx**：弹 `message.error`，`message_key='network.error'`
- **BizError**：由调用方决定弹不弹（表单校验错不应自动 toast）

### 8.6 i18n 策略

- 所有 UI 字符串走 `$t('user.create.success')`
- 后端错误响应带 `message_key`，前端拿 key 查本地化文案
- 菜单显示名：`menus.name` 直接存 i18n key，展示时 `$t(menu.name)`
- AntdV 内置组件：`<a-config-provider :locale="...">` 切换 `zh_CN` / `en_US`
- **i18n key 治理**：
  - 前端 key 集合在 `src/locales/{zh-CN,en-US}.json`
  - 后端 key 常量在 `internal/infra/i18n/keys.go`
  - 共享文档 `docs/i18n-keys.md`
  - 前端 build 时扫 vue 文件，未声明的 key fail

### 8.7 v-permission

```vue
<a-button v-permission="'system:user:write'">Create</a-button>
<a-button v-permission="['system:user:read', 'system:user:write']">  <!-- 交集 -->
<a-button v-permission:any="['system:user:write', 'system:user:admin']">  <!-- 并集 -->
```

无权限时 DOM 直接移除（不是 hidden）。

### 8.8 主题与适配

- AntdV `theme.defaultAlgorithm` / `darkAlgorithm`，存 `app` store
- 不做自定义主题
- 仅适配 ≥ 1280 桌面端

---

## 9. 配置、本地开发、测试、部署

### 9.1 配置文件

`optimus-be/configs/config.yaml`（默认值，提交仓库）：

```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 15s
  write_timeout: 15s
  shutdown_timeout: 20s

database:
  driver: postgres
  dsn: "host=localhost port=5432 user=optimus password=optimus dbname=optimus sslmode=disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 1h

jwt:
  secret: ""                # env 注入，启动校验 ≥ 32
  access_ttl: 15m
  refresh_ttl: 168h

auth:
  bcrypt_cost: 10
  login_rate_limit:
    per_ip: 5
    per_username: 5
    window: 1m
    block: 1m

log:
  level: info
  format: json
  output: stdout

cors:
  allowed_origins: ["http://localhost:5173"]
  allowed_methods: [GET, POST, PUT, DELETE, OPTIONS]
  allow_credentials: false

i18n:
  default_lang: zh-CN
  supported: [zh-CN, en-US]

bootstrap:
  admin_username: admin
  admin_email: admin@example.com
```

**env override**：`OPTIMUS_DATABASE_DSN`、`OPTIMUS_JWT_SECRET` 等，下划线段对应 YAML 嵌套层级。

### 9.2 本地开发

`docker-compose.yml`（仓库根）：

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: optimus
      POSTGRES_PASSWORD: optimus
      POSTGRES_DB: optimus
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U optimus"]
      interval: 5s
  adminer:
    image: adminer:latest
    ports: ["8081:8080"]
    depends_on: [postgres]
volumes:
  pgdata:
```

开发命令：

```bash
# 基础设施
docker compose up -d

# 后端
cd optimus-be
make migrate-up
make run            # air 热重载

# 前端
cd optimus-fe
bun install
bun run dev         # http://localhost:5173
```

### 9.3 Makefile (optimus-be)

```makefile
run:           ; air
build:         ; go build -o bin/optimus-be ./cmd/server
test:          ; go test ./... -race -cover
test-int:      ; go test ./... -tags=integration
lint:          ; golangci-lint run
swag:          ; swag init -g cmd/server/main.go -o api/docs
migrate-up:    ; goose -dir migrations postgres "$$OPTIMUS_DATABASE_DSN" up
migrate-down:  ; goose -dir migrations postgres "$$OPTIMUS_DATABASE_DSN" down
migrate-new:   ; goose -dir migrations create $(name) sql
seed:          ; go run ./cmd/seed
perm-check:    ; go run ./cmd/perm-check
```

### 9.4 数据库迁移

```
optimus-be/migrations/
├── 00001_init_users.sql
├── 00002_init_roles_permissions.sql
├── 00003_init_user_roles.sql
├── 00004_init_role_permissions.sql
├── 00005_init_menus.sql
├── 00006_init_refresh_tokens.sql
├── 00007_init_audit_logs.sql
├── 00008_partial_unique_indexes.sql
└── 00009_foreign_keys.sql
```

每个文件含 `-- +goose Up` / `-- +goose Down`，迁移必须可回滚。

权限码 / 内置角色 / 初始菜单不在 SQL 里，由 `cmd/seed/main.go` 处理（代码是单一来源）。

### 9.5 测试策略

| 层 | 工具 | 范围 | P0 目标 |
|---|---|---|---|
| 后端单测 | testify | service / util / token / permission | 核心 ≥ 60% |
| 后端集成 | dockertest | repo + handler 端到端 happy path + 关键错误路径 | 覆盖 login/refresh/user CRUD/role 绑定/permission check |
| 前端单测 | vitest | utils / stores / usePermission | 逻辑性强的部分 |
| 前端 E2E | 不在 P0 | — | — |

**TDD 范围**：service 层 + 关键 handler 强制 TDD；vue 组件不强制。

集成测试用 dockertest 起真实 PG，每 case 独立 schema，并行跑，单 CI job ≤ 5 min。

### 9.6 构建

**optimus-be Dockerfile**：

```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/optimus-be ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/optimus-be /usr/local/bin/optimus-be
COPY configs/config.yaml /etc/optimus/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/optimus-be"]
CMD ["-config", "/etc/optimus/config.yaml"]
```

**optimus-fe Dockerfile**：

```dockerfile
FROM oven/bun:1 AS build
WORKDIR /src
COPY package.json bun.lockb ./
RUN bun install --frozen-lockfile
COPY . .
RUN bun run build

FROM nginx:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

`deploy/nginx.conf` 要点：
- `/api/v1/` `proxy_pass` 到 optimus-be:8080
- 其他路径 `try_files $uri $uri/ /index.html`（SPA history mode）

### 9.7 生产部署

P0 假设单机 docker-compose：postgres + optimus-be + optimus-fe (nginx)。

`deploy/docker-compose.prod.yml` + `deploy/.env.example`。

K8s 部署留到 P2（dogfood）。

### 9.8 CI（GitHub Actions）

`.github/workflows/ci.yml`：

```
on: [push, pull_request]
jobs:
  backend:
    - setup-go
    - go mod download
    - golangci-lint
    - go test -race -cover
    - go test -tags=integration（dockertest 自动起 PG）
  frontend:
    - setup-bun
    - bun install --frozen-lockfile
    - bun run lint
    - bun run typecheck
    - bun run build
  swagger-diff:
    - 生成 swagger.json，与仓库 docs/api/swagger.json 比对
    - 不一致 → fail
  perm-check:
    - 启动一次，dump 已注册权限码集合，与 docs/permissions.md 比对
```

### 9.9 日志与可观测性（P0 最小）

- 结构化 JSON 日志，含 `request_id` / `user_id` / `latency_ms` / `status`
- 5xx 自动 log 完整 stack trace
- 不接入 Sentry / 链路追踪 → P5
- `GET /health` 返回 `{ db: ok, version: <sha> }`

---

## 10. 整体目录

```
optimus/
├── optimus-be/
│   ├── cmd/{server,seed,perm-check}/
│   ├── internal/
│   │   ├── modules/{auth,user,rbac,menu,audit,health}/
│   │   │   └── 每模块：handler.go / service.go / repo.go / model.go / dto.go
│   │   ├── infra/{db,config,log,errors,i18n,middleware,permissions}/
│   │   └── api/{routes.go,docs/}
│   ├── migrations/
│   ├── configs/
│   ├── Makefile
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── optimus-fe/                   # 见 §8.1
├── docs/
│   ├── superpowers/specs/
│   ├── api/swagger.json
│   ├── i18n-keys.md
│   └── permissions.md
├── deploy/
│   ├── docker-compose.prod.yml
│   ├── nginx.conf
│   └── .env.example
├── docker-compose.yml            # 本地开发
├── .github/workflows/ci.yml
├── .gitignore
└── README.md
```

---

## 11. 时间线（个人开发 + AI 辅助，全职估时）

| 阶段 | 任务 | 估时 |
|---|---|---|
| W1-1 | 仓库骨架、Makefile、docker-compose、配置、日志、错误体系 | 1.5d |
| W1-2 | DB 迁移（8 张表 + 部分唯一索引 + 外键）、GORM models | 1d |
| W1-3 | seed（admin + 内置角色 + 菜单常量 + 权限码注册） | 0.5d |
| W2-1 | auth：login / refresh / logout / JWT / bcrypt / 登录限速 | 2d |
| W2-2 | rbac：permission cache / RBAC middleware / `/me` 系列 | 1d |
| W2-3 | user：CRUD / 状态切换 / 密码重置 / 角色绑定 | 1d |
| W3-1 | role + permission 列表 + menu CRUD | 1.5d |
| W3-2 | audit + audit middleware + 关键操作埋点 | 1d |
| W3-3 | swagger 注解 + CI 校验 | 0.5d |
| W4-1 | 前端骨架：vite / router / pinia / i18n / axios / utils / types | 1.5d |
| W4-2 | 通用组件：DefaultLayout / ProTable / ProForm / PageHeader / v-permission | 2d |
| W4-3 | Login + 路由守卫 + 动态路由 + 错误页 | 1d |
| W5-1 | system/users 页 | 1.5d |
| W5-2 | system/roles 页（权限穿梭框） | 1d |
| W5-3 | system/menus 页（树编辑） | 1d |
| W5-4 | system/permissions 只读 + system/audit-logs | 1d |
| W5-5 | profile + i18n 切换 + 主题切换 + 收尾 | 0.5d |
| W6 | Dockerfile + nginx + 生产 compose + 联调 + README | 1.5d |

**总计：约 21-22 工作日 / 约 5 周。** 正式拆分由 writing-plans skill 完成。

---

## 12. 验收标准

### 功能验收
- [ ] 启动后从空 DB 自举出 admin 用户（密码打印到日志）
- [ ] admin 能登录、改密、创建用户、创建角色、绑权
- [ ] 普通用户登录后只能看到其权限码允许的菜单
- [ ] 删除用户后，同名 username/email 可再注册
- [ ] refresh rotation 生效；旧 refresh 重放触发用户全 token 撤销 + audit
- [ ] zh-CN/en-US 切换：前端全字符串切换 + 后端错误按 message_key 翻译
- [ ] /audit-logs 能查看 login/logout/user.*/role.*/menu.* 事件

### 工程验收
- [ ] `bun run build` 与 `make build` 全绿
- [ ] CI 全绿（含 swagger-diff、perm-check）
- [ ] 后端核心 service 单测覆盖 ≥ 60%
- [ ] 集成测试覆盖：login / refresh / user CRUD / role 绑定 / permission check
- [ ] `docker compose -f deploy/docker-compose.prod.yml up` 在干净机器起服务
- [ ] README 含：本地开发步骤、env 列表、迁移命令、首次登录说明

### 安全验收
- [ ] JWT secret 启动校验
- [ ] 登录限速生效
- [ ] bcrypt cost ≥ 10
- [ ] 软删 + 部分唯一索引：复用 username 测试通过
- [ ] CORS 白名单生效
- [ ] gosec 在 CI 跑

---

## 13. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 从零搭前端比基于模板慢 1-2 周 | 提前接受，不中途切模板；通用组件做扎实，后续模块复用 |
| 权限码常量散布在各模块，注册遗漏 | 启动时 diff 已注册集合与 DB 现状，多/少都 log warn；CI `make perm-check` 强校验 |
| dockertest 慢，CI 时间膨胀 | 集成测试只覆盖关键路径；并行跑；单 job ≤ 5 min |
| 菜单 component 字段与实际文件不匹配 → 白屏 | 前端构建时 lint 校验所有 `menus.component` 对应的 vue 文件存在 |
| 401 拦截器 + refresh rotation 并发竞态 | axios 拦截器用 single-flight 模式，一个 pending refresh 复用 Promise |
| i18n key 前后端漂移 | key 集合放在 `docs/i18n-keys.md`；前端 build 扫 vue 文件，未声明的 key fail；后端常量集中 |
| AI 辅助生成代码可能绕过 RBAC | 路由集中在 `internal/api/routes.go`，每条路由必须显式声明 `permission:""` 或具体码；CI grep 强制非 auth 路由必须有 permission 配置 |

---

## 14. 后续步骤

1. 用户 review 本 spec 文件 → approve
2. 调用 `superpowers:writing-plans` skill，把本 spec 转成实现计划
3. 用 `superpowers:executing-plans` 或 `superpowers:subagent-driven-development` 落地实现
4. 每个模块走 TDD（superpowers:test-driven-development）

P0 完成后再以同样流程进入 P1 (credentials-vault)。
