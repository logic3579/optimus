# P3 — applications Design

**Status**: Spec
**Date**: 2026-06-10
**Owner**: P3 sub-project
**Depends on**: P0 platform-skeleton (merged on `main`), P1 credentials-vault (merged on `main`), P2 k8s-mgmt (merged `2353378` on `main`, 2026-06-10)
**Downstream**: none in current DAG (P3 is a leaf of the DAG; P4/P5/P6 are siblings of P2 and independent of P3)

---

## 1. Goal and scope

P0 shipped auth / RBAC / users / roles / menus / audit / i18n / generic CRUD UI / deploy. P1 shipped the encrypted credentials vault with a Go-level `credentials.Consumer` seam returning decrypted kubeconfig YAML on demand. P2 shipped a read-only Kubernetes management console on top of P0 + P1 — clusters can be registered (via P1 kubeconfig) and 13 core resource kinds can be browsed plus pod log streaming.

P3 builds the **Helm-based application lifecycle manager** on top of P0 + P1 + P2. Operators register chart repositories (OCI or HTTP), register applications (a 1:1 row pointing at a Helm release in a specific cluster+namespace), and drive the full lifecycle through Helm — install / upgrade / rollback / uninstall — without leaving Optimus. Optimus's DB stores application metadata (name, owner, tags, chart pointer); Helm secrets in-cluster remain the source of truth for deployment state.

### 1.1 What P3 ships

1. Two new tables — `apps_chart_repos` and `apps_applications` — with a foreign-key chain `apps_applications → clusters` (P2) and `apps_applications → apps_chart_repos`. FK semantics in §3.
2. A new error code segment `42xxx` covering apps-domain generic / chart-repo upstream / helm release runtime failures. Numbering in §5.
3. Per-request `*action.Configuration` construction — every helm call obtains a fresh REST config from `credentials.Consumer.GetKubeconfig` and discards it after the response, matching the P2 client-factory pattern.
4. Three HTTP surfaces under `/api/v1/apps/*`:
   - `apps/repos` — chart repository CRUD + chart/version enumeration + default values fetch.
   - `apps/applications` — application metadata CRUD (separate from helm execution).
   - `apps/applications/:id/release/*` — install / upgrade / rollback / uninstall / status / history.
5. 10 new permission codes under category `apps` (§6), wired through P0's `RequirePermission` middleware.
6. A FE module under `src/views/apps/` with 6 page surfaces (Applications List/Detail/Install/Upgrade + ChartRepos List/Form), backed by `vue-codemirror` for the values editor (no new dependency — P2 already adopted it).
7. A small upstream feedback to P2: a new `apps/application/inuse.go` helper called by P2's cluster delete handler so cluster deletion is blocked with a user-facing error when applications still reference the cluster.
8. Updated `cmd/seed` so the built-in `k8s_operator` role gains `apps:release:install/upgrade/rollback/uninstall` and `apps:{application,repo}:read`. Other built-in roles unchanged. No new built-in role is introduced.

### 1.2 What P3 does NOT ship (deferred)

- Schema-driven values forms (P3 ships raw YAML editing only; chart-schema-to-form left to future P3.x).
- Multi-environment overlay (values-prod.yaml + values-staging.yaml + ad-hoc set). Each application is a single Helm release with a single values document.
- Multi-cluster fleet deployment (one application = one release in one cluster+namespace; cross-cluster deployment requires a second application row).
- Git-as-chart-source. v1 supports only OCI registry and HTTP repos.
- Asynchronous deployment operation tracking (no `apps_operations` table, no background worker, no SSE log streaming of helm output). All lifecycle calls are synchronous HTTP, helm runs without `--wait`, FE polls pod readiness via P2's resource read APIs.
- Helm hooks beyond chart defaults; we do not implement `--hooks`, `--no-hooks`, `--atomic`, `--force` flags as configurable knobs in v1. Helm SDK defaults apply.
- chart-schema validation against `values.schema.json`. FE shows the chart's default values for reference; validation is server-side at install time via helm itself.
- LRU cache for `index.yaml`. Each repo browse fetches `index.yaml` fresh. Acceptable because chart browsing is not a hot path.
- OCI fake-server integration tests. OCI behavior is verified via the manual smoke checklist in §10.4.
- localStorage draft caching of the values editor. The editor always loads either chart defaults or the current installed values — no stale-draft risk.

### 1.3 Anti-goals (will not be added later either)

- Optimus-side mutation of helm release secrets directly. All writes go through helm SDK action APIs so Helm's own state machine remains intact.
- Allowing operations on helm releases that Optimus did not register. The application row is a precondition for any release-level call.

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ optimus-be                                                            │
│                                                                       │
│   internal/modules/apps/                                              │
│     repo/         ←── apps_chart_repos CRUD + chart/version queries   │
│                       (OCI via helm registry.Client, HTTP via repo)   │
│     application/  ←── apps_applications CRUD + list merge w/ helm     │
│                       inuse helper for P2 cluster delete pre-check    │
│     release/      ←── install / upgrade / rollback / uninstall        │
│                       status / history via helm SDK action APIs       │
│     helmclient/   ←── per-request *action.Configuration factory       │
│                       restClientGetter shim for helm SDK              │
│     errs.go       ←── MapError: helm/registry → BizError 42xxx        │
│     module.go     ←── MountRoutes; assembles handlers + cache wiring  │
│                                                                       │
│   cmd/server/main.go ←── wire apps module after k8s module           │
│   cmd/seed/         ←── apps menu (1 parent + 2 children),           │
│                         k8s_operator gains apps:release:* perms       │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
              │ imports credentials.Consumer (P1 seam)
              │ imports k8s/cluster.Service (P2; for cluster_id lookup)
              ▼
┌──────────────────────────────────────────────────────────────────────┐
│ helm.sh/helm/v3 SDK                                                   │
│   action.{Install,Upgrade,Rollback,Uninstall,Status,History,List}     │
│   registry.Client (OCI) + repo.ChartRepository (HTTP)                 │
│   storage.driver "secrets" (default; in target namespace)             │
└──────────────────────────────────────────────────────────────────────┘
```

Each release-level HTTP request walks: `gin handler → JWTAuth → RequirePermission → release service → helmclient.Factory.NewForCluster(clusterID, namespace, purpose) → credentials.Consumer.GetKubeconfig → action.Configuration.Init(...) → helm action call → MapError → DTO`. No `*action.Configuration` survives the request.

Chart-repo browsing requests construct a fresh `registry.Client` (OCI) or `getter.HTTPGetter` (HTTP) per call, with credentials decrypted via `vault.Cipher` for that call only. Plaintext password never lives on the `Repo` struct beyond the function scope.

The FE module shape mirrors P0/P1/P2: per-page `List.vue` + optional `Detail.vue` + per-flow components. A new Pinia store `useAppsStore` holds only filter state (cluster/namespace). No application list cache.

---

## 3. Data model

### 3.1 New table: `apps_chart_repos`

```sql
CREATE TABLE apps_chart_repos (
  id                  BIGSERIAL    PRIMARY KEY,
  name                VARCHAR(64)  NOT NULL,
  type                VARCHAR(8)   NOT NULL CHECK (type IN ('oci','http')),
  url                 TEXT         NOT NULL,
  username            VARCHAR(255) NOT NULL DEFAULT '',
  encrypted_password  BYTEA        NOT NULL DEFAULT ''::BYTEA,
  description         TEXT         NOT NULL DEFAULT '',
  created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX apps_chart_repos_name_unique
  ON apps_chart_repos(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_chart_repos_deleted_at ON apps_chart_repos(deleted_at);
```

Notes:
- `encrypted_password` is produced by P1's `vault.Cipher.Encrypt`. The same master key (`OPTIMUS_VAULT_MASTER_KEY{,_FILE}`) is reused — no second cipher instance.
- `type` is immutable post-creation (changing OCI ↔ HTTP changes the URL semantics). Updating a repo row ignores any `type` field in the payload.
- `url` is not unique; the same upstream may be registered under multiple names with different credentials.
- An empty `username` AND empty `encrypted_password` means "anonymous access" (suitable for public charts).

### 3.2 New table: `apps_applications`

```sql
CREATE TABLE apps_applications (
  id              BIGSERIAL    PRIMARY KEY,
  name            VARCHAR(64)  NOT NULL,
  cluster_id      BIGINT       NOT NULL
                  REFERENCES clusters(id) ON DELETE RESTRICT,
  namespace       VARCHAR(63)  NOT NULL,
  release_name    VARCHAR(53)  NOT NULL,
  chart_repo_id   BIGINT       NOT NULL
                  REFERENCES apps_chart_repos(id) ON DELETE RESTRICT,
  chart_name      VARCHAR(128) NOT NULL,
  description     TEXT         NOT NULL DEFAULT '',
  tags            JSONB        NOT NULL DEFAULT '[]'::JSONB,
  owner_user_id   BIGINT       REFERENCES users(id) ON DELETE SET NULL,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at      TIMESTAMPTZ,
  CONSTRAINT apps_applications_tags_is_array
    CHECK (jsonb_typeof(tags) = 'array')
);
CREATE UNIQUE INDEX apps_applications_release_unique
  ON apps_applications(cluster_id, namespace, release_name)
  WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX apps_applications_name_unique
  ON apps_applications(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_applications_cluster_id     ON apps_applications(cluster_id);
CREATE INDEX apps_applications_owner_user_id  ON apps_applications(owner_user_id);
CREATE INDEX apps_applications_deleted_at     ON apps_applications(deleted_at);
```

Notes:
- `release_name` cap is 53 chars per Helm's release name limit.
- `namespace` cap is 63 chars per RFC 1123 DNS label.
- `(cluster_id, namespace, release_name)` is unique to prevent two Optimus rows pointing at one helm release.
- `chart_repo_id` is **mutable** via upgrade (you may switch chart source between releases) but `chart_name` is **immutable** (renaming the chart equals uninstall+install of a different application).
- `owner_user_id` uses `ON DELETE SET NULL` so deleting a user does not destroy application rows; orphaned applications remain searchable.
- No `current_revision` / `last_status` columns. These are fetched live from Helm on each detail-page render to avoid drift.

### 3.3 Migration file

A single new goose migration `00020_apps_tables.sql` creates both tables, all indexes, and inlines the FK declarations. The P0 chain's `00010_foreign_keys.sql` is not modified — it is the P0 fossil.

### 3.4 Helm release storage location

Helm 3's default storage driver `secrets` writes one Secret per revision into the target namespace named `sh.helm.release.v1.<release>.v<rev>`. P3 adopts this default unchanged so operators can cross-reference with `helm` CLI and `kubectl get secret`.

### 3.5 Cascade with P2 cluster delete

`clusters.kubeconfig_id` already declares `ON DELETE RESTRICT`. `apps_applications.cluster_id` likewise. The DB will block the delete with a constraint error if applications still reference a cluster, but the raw error is unhelpful.

P3 adds an explicit pre-check helper:

```go
// optimus-be/internal/modules/apps/application/inuse.go
package application

import "context"

// Counter returns the number of non-soft-deleted applications referencing the
// given cluster_id. It is the pre-check used by k8s cluster delete to surface a
// friendly BizError instead of a raw FK constraint violation.
type Counter interface {
    CountByClusterID(ctx context.Context, clusterID uint64) (int, error)
    CountByChartRepoID(ctx context.Context, repoID uint64) (int, error)
}
```

P2's `cluster.Service.Delete` is amended in P3 to call `apps/application.CountByClusterID` and return `CodeAppsApplicationInUse` (42001) when the count is non-zero. The two modules wire via an interface declared in `apps/application` and consumed in `k8s/cluster` — `k8s/cluster` does not import `apps/application` directly; `main.go` injects.

---

## 4. API surface

All paths prefixed `/api/v1`, JSON request/response, envelope `{code, data, message, message_key}`.

### 4.1 Chart repos

| Method | Path | Perm | Notes |
|---|---|---|---|
| GET | `/apps/repos` | `apps:repo:read` | Paginated list. Query: `name?`, `type?`, `page`, `page_size`. |
| GET | `/apps/repos/:id` | `apps:repo:read` | Detail. Returns `has_password: bool`, never the password itself. |
| POST | `/apps/repos` | `apps:repo:write` | Body: `{name, type, url, username, password, description}`. |
| PUT | `/apps/repos/:id` | `apps:repo:write` | Body shape same as POST. `type` ignored. `password == ""` → keep current. `password == null` → clear. |
| DELETE | `/apps/repos/:id` | `apps:repo:delete` | Pre-check via `CountByChartRepoID` — returns 42002 if any application references it. Soft delete. |
| GET | `/apps/repos/:id/charts` | `apps:repo:read` | Lists charts in the repo. OCI: `oras tags` style; HTTP: parses `index.yaml`. |
| GET | `/apps/repos/:id/charts/:chart/versions` | `apps:repo:read` | Lists semver versions for the given chart. Chart name in path is URL-escaped. |
| GET | `/apps/repos/:id/charts/:chart/versions/:version/values` | `apps:repo:read` | Returns the chart's bundled `values.yaml` as plain text for FE pre-fill. |

### 4.2 Applications

| Method | Path | Perm | Notes |
|---|---|---|---|
| GET | `/apps/applications` | `apps:application:read` | Paginated list. Query: `cluster_id?`, `namespace?`, `owner_user_id?`, `tag?`, `name?` (fuzzy), `page`, `page_size`. Returns DB rows only; live helm status is null. |
| GET | `/apps/applications/:id` | `apps:application:read` | Detail. Joins live helm status (`status: deployed|failed|pending|unknown`, `current_revision: int|null`) via helm SDK. |
| POST | `/apps/applications` | `apps:application:write` | Body: `{name, cluster_id, namespace, release_name, chart_repo_id, chart_name, description, tags, owner_user_id}`. Creates DB row only — no Helm call. |
| PUT | `/apps/applications/:id` | `apps:application:write` | Body: `{description, tags, owner_user_id}`. `cluster_id`, `namespace`, `release_name`, `chart_name` are immutable. `chart_repo_id` is mutated only by the upgrade endpoint (§4.3), not by this PUT. |
| DELETE | `/apps/applications/:id` | `apps:application:delete` | Soft delete. Pre-check: helm secret must not exist in the cluster (uninstall first), else returns 42204. |

Registration and deployment are intentionally split. The wizard FE composes them as a single user flow, but the API surfaces remain separate so "register without deploying" is a legal intermediate state.

### 4.3 Releases (per application)

| Method | Path | Perm | Notes |
|---|---|---|---|
| GET | `/apps/applications/:id/release` | `apps:application:read` | Returns live status `{status, revision, chart_version, app_version, updated_at, notes}`. 404 (40401) if release does not exist in cluster. |
| GET | `/apps/applications/:id/release/history` | `apps:application:read` | Returns `[{revision, status, chart_version, app_version, updated_at, description}]`. Unpaginated (helm caps at 256 revisions by default). |
| POST | `/apps/applications/:id/release/install` | `apps:release:install` | Body: `{chart_version, values_yaml}`. Helm install without `--wait`. Returns `{revision, status, chart_version, deployed_at}`. |
| POST | `/apps/applications/:id/release/upgrade` | `apps:release:upgrade` | Body: `{chart_repo_id?, chart_version, values_yaml}`. Helm upgrade. If `chart_repo_id` present, also patches the DB row. |
| POST | `/apps/applications/:id/release/rollback` | `apps:release:rollback` | Body: `{revision: int}`. Helm rollback. |
| POST | `/apps/applications/:id/release/uninstall` | `apps:release:uninstall` | Body: `{keep_history?: bool}`. Helm uninstall. |

All POSTs write to the P0 audit table via the shared `audit.Recorder`. Event names: `apps.repo.create/update/delete`, `apps.application.create/update/delete`, `apps.release.install/upgrade/rollback/uninstall`.

### 4.4 Application list performance

`GET /apps/applications` returns DB rows only (constant-time SQL with indexes); live status is left null. FE may opt-in to per-row status fetch on a "show live status" toggle that issues `GET /apps/applications/:id/release` for the current page (bounded ≤ 50 concurrent). This avoids fan-out helm calls on list-page open.

### 4.5 Cluster health reuse

P3 adds no new health endpoint. Application detail's "cluster reachable?" indicator reuses P2's `/api/v1/k8s/clusters/:id/health`.

---

## 5. Error codes

New segment under category `apps`, occupying `42xxx`. Avoid `42901` already taken by `CodeRateLimited`.

```go
// 42xxx P3 apps domain — chart repo upstream + helm release runtime.
// Distinct from 40xxx mirror because these encode upstream-helm/registry
// dependency state, not malformed client requests. See P3 spec §5.
//
// 42001-42099 apps generic
CodeAppsApplicationInUse            Code = 42001 // delete blocked: helm release still present
CodeAppsChartRepoInUse              Code = 42002 // delete blocked: still referenced by application(s)
CodeAppsReleaseNameDuplicate        Code = 42003 // (cluster_id,namespace,release_name) collision in DB
CodeAppsApplicationOnDeletedCluster Code = 42004 // referenced cluster is soft-deleted

// 42101-42199 chart repo upstream
CodeAppsRepoUnreachable    Code = 42101 // network/DNS/TLS failure
CodeAppsRepoUnauthorized   Code = 42102 // 401/403 from OCI or HTTP repo
CodeAppsRepoChartNotFound  Code = 42103 // chart name or version missing
CodeAppsRepoInvalidIndex   Code = 42104 // HTTP repo index.yaml parse failure
CodeAppsRepoOCIError       Code = 42105 // OCI manifest/blob fetch error
CodeAppsRepoOther          Code = 42199 // other upstream error

// 42201-42299 helm release runtime
CodeAppsReleaseAlreadyExists   Code = 42201 // install: release already exists
CodeAppsReleaseNotFound        Code = 42202 // upgrade/rollback/uninstall/status: helm secret missing
CodeAppsReleaseHistoryTooShort Code = 42203 // rollback target revision missing
CodeAppsReleaseStillPresent    Code = 42204 // application delete blocked: helm secret still exists
CodeAppsReleaseInvalidValues   Code = 42205 // values yaml parse error / not a map
CodeAppsReleaseOther           Code = 42299 // other helm SDK error
```

### 5.1 Error mapping `apps/errs.go`

```go
// MapError normalises helm SDK / chart repo / OCI errors into BizError 42xxx.
// Three-stage dispatch:
//   1. Helm storage driver sentinels (errors.Is)
//   2. Registry/repo errors (string match on err.Error())
//   3. Network errors (net.Error / *url.Error)
// Anything else falls through to CodeAppsReleaseOther.
//
// Returned BizError wraps the original via WithCause so structured logs keep
// the helm/registry message for operator triage; the client only sees
// message_key.
func MapError(err error) error
```

String matching is used for chart-repo errors because helm's registry/repo packages do not export sentinel errors. Each match path is unit-tested in `errs_test.go`.

---

## 6. Permissions & RBAC integration

### 6.1 New permission codes (10)

Appended to `internal/infra/permissions/codes.go`:

```go
{Code: "apps:application:read",     Category: "apps", NameKey: "perm.apps.application.read"},
{Code: "apps:application:write",    Category: "apps", NameKey: "perm.apps.application.write"},
{Code: "apps:application:delete",   Category: "apps", NameKey: "perm.apps.application.delete"},
{Code: "apps:release:install",      Category: "apps", NameKey: "perm.apps.release.install"},
{Code: "apps:release:upgrade",      Category: "apps", NameKey: "perm.apps.release.upgrade"},
{Code: "apps:release:rollback",     Category: "apps", NameKey: "perm.apps.release.rollback"},
{Code: "apps:release:uninstall",    Category: "apps", NameKey: "perm.apps.release.uninstall"},
{Code: "apps:repo:read",            Category: "apps", NameKey: "perm.apps.repo.read"},
{Code: "apps:repo:write",           Category: "apps", NameKey: "perm.apps.repo.write"},
{Code: "apps:repo:delete",          Category: "apps", NameKey: "perm.apps.repo.delete"},
```

CI runs `make perm-check` after `make dump-perms` to keep `docs/permissions.md` in sync.

### 6.2 Route mounting pattern

P0/P1/P2's nested-sub-group pattern is preserved — each sub-group declares `RequirePermission` middleware once via `Group("", mw)`, never as variadic args to `GET/POST`. Skeleton in `cmd/server/main.go`:

```go
func mountAppsRoutes(rg *gin.RouterGroup, h *apps.Handlers, cache *rbac.PermissionCache) {
    repos := rg.Group("/apps/repos")
    {
        repos.Group("", middleware.RequirePermission(cache, "apps:repo:read")).
            GET("", h.Repo.List).
            GET("/:id", h.Repo.Get).
            GET("/:id/charts", h.Repo.ListCharts).
            GET("/:id/charts/:chart/versions", h.Repo.ListVersions).
            GET("/:id/charts/:chart/versions/:version/values", h.Repo.GetDefaultValues)
        repos.Group("", middleware.RequirePermission(cache, "apps:repo:write")).
            POST("", h.Repo.Create).
            PUT("/:id", h.Repo.Update)
        repos.Group("", middleware.RequirePermission(cache, "apps:repo:delete")).
            DELETE("/:id", h.Repo.Delete)
    }

    apps := rg.Group("/apps/applications")
    {
        apps.Group("", middleware.RequirePermission(cache, "apps:application:read")).
            GET("", h.App.List).
            GET("/:id", h.App.Get).
            GET("/:id/release", h.Release.Status).
            GET("/:id/release/history", h.Release.History)
        apps.Group("", middleware.RequirePermission(cache, "apps:application:write")).
            POST("", h.App.Create).
            PUT("/:id", h.App.Update)
        apps.Group("", middleware.RequirePermission(cache, "apps:application:delete")).
            DELETE("/:id", h.App.Delete)
        apps.Group("", middleware.RequirePermission(cache, "apps:release:install")).
            POST("/:id/release/install", h.Release.Install)
        apps.Group("", middleware.RequirePermission(cache, "apps:release:upgrade")).
            POST("/:id/release/upgrade", h.Release.Upgrade)
        apps.Group("", middleware.RequirePermission(cache, "apps:release:rollback")).
            POST("/:id/release/rollback", h.Release.Rollback)
        apps.Group("", middleware.RequirePermission(cache, "apps:release:uninstall")).
            POST("/:id/release/uninstall", h.Release.Uninstall)
    }
}
```

### 6.3 Built-in role updates (seed)

Built-in roles updated; no new role introduced:

| Role | Added codes |
|---|---|
| `system_admin` | All 10 `apps:*` codes |
| `k8s_operator` | `apps:application:read`, `apps:repo:read`, `apps:release:{install,upgrade,rollback,uninstall}` |
| `k8s_viewer` | `apps:application:read`, `apps:repo:read` |

A `developer` business role is intentionally NOT seeded. Operators with the appropriate need create it manually post-deploy.

### 6.4 Menu seed

One parent + two children added:

```
apps             menu.apps             icon: AppstoreOutlined         (no perm; grouping only)
├── applications menu.apps.applications path: /apps/applications       perm: apps:application:read
└── chart-repos  menu.apps.chart-repos  path: /apps/chart-repos        perm: apps:repo:read
```

Menu `name` fields must exactly match the i18n keys, or the locale fallback shows raw keys. CI runs `bun run i18n:check`.

### 6.5 PermissionCache invalidation

No P3 mutation path touches RBAC (no role grants, no user-role changes). Therefore `cache.InvalidateUser` / `cache.InvalidateAll` is never called from P3 service code. Role seed at startup follows P0's path through `permissions.Register` + `seed.Roles`, which build the initial cache state.

---

## 7. Helm client factory

### 7.1 `helmclient.Factory`

```go
// Factory builds per-request *action.Configuration objects. Result is NOT cached
// across requests; helm action.Configuration's internal KubeClient is not safe
// to share across goroutines.
type Factory struct {
    cred     credentials.Consumer
    clusters cluster.Service // P2 cluster lookup (kubeconfig_id, context)
}

// NewForCluster builds a *action.Configuration whose REST config comes from
// credentials.Consumer. namespace is the target Helm namespace. purpose is the
// audit purpose string (e.g., "apps.release.install").
func (f *Factory) NewForCluster(
    ctx context.Context, clusterID uint64, namespace, purpose string,
) (*action.Configuration, error)
```

Implementation outline:

```go
func (f *Factory) NewForCluster(
    ctx context.Context, clusterID uint64, namespace, purpose string,
) (*action.Configuration, error) {
    cluster, err := f.clusters.Get(ctx, clusterID)
    if err != nil { return nil, err }

    kc, err := f.cred.GetKubeconfig(ctx, cluster.KubeconfigID, purpose)
    if err != nil { return nil, err }

    restCfg, err := buildRESTConfig(kc.YAML, cluster.Context)
    if err != nil { return nil, MapError(err) }

    rcGetter := &restClientGetter{restCfg: restCfg, namespace: namespace}

    actionCfg := new(action.Configuration)
    if err := actionCfg.Init(rcGetter, namespace, "secrets", slogDebugf); err != nil {
        return nil, MapError(err)
    }
    return actionCfg, nil
}
```

`restClientGetter` implements `genericclioptions.RESTClientGetter` (four methods). `slogDebugf` bridges helm's verbose logs to slog at DEBUG level — never to client.

### 7.2 OCI registry client lifecycle

`registry.Client` is constructed per call:

```go
func newRegistryClient(repo *Repo, password string) (*registry.Client, error) {
    c, err := registry.NewClient()
    if err != nil { return nil, MapError(err) }
    if repo.Username != "" || password != "" {
        if err := c.Login(repo.URL,
            registry.LoginOptBasicAuth(repo.Username, password)); err != nil {
            return nil, MapError(err)
        }
    }
    return c, nil
}
```

### 7.3 HTTP repo client lifecycle

Each call constructs a fresh `getter.HTTPGetter` (with basic-auth options when credentials present), downloads `index.yaml`, and parses it via helm's `repo.IndexFile`. No on-disk cache; no in-process LRU.

### 7.4 helm SDK version pinning

`helm.sh/helm/v3` is pinned to a minor compatible with `k8s.io/client-go v0.30.14` (P2's chosen pin). Per P3 plan task 1: run `go get helm.sh/helm/v3@v3.16.x && go mod tidy && go build ./... && go test ./...` end-to-end before any code is written. If incompatible, fall back to `v3.15.x`. The exact version is locked at the end of task 1 and recorded in CLAUDE.md.

### 7.5 vault password decryption

`apps_chart_repos.encrypted_password` decrypts on-demand at call time only:

```go
func (s *RepoService) decryptPassword(_ context.Context, repo *Repo) (string, error) {
    if len(repo.EncryptedPassword) == 0 { return "", nil }
    plain, err := s.cipher.Decrypt(repo.EncryptedPassword)
    if err != nil { return "", err }
    return string(plain), nil
}
```

Plaintext lives only on the function stack. Go string immutability means we cannot zeroise — accepted, consistent with P1.

---

## 8. Frontend module

### 8.1 Directory tree

```
optimus-fe/src/
├── api/apps/
│   ├── repo.ts         // RepoListResp / RepoDetail / RepoCharts / RepoVersions
│   ├── application.ts  // ApplicationSummary / ApplicationDetail / ListParams
│   └── release.ts      // ReleaseStatus / ReleaseHistory / Install/Upgrade/Rollback/Uninstall payloads
├── stores/apps.ts      // useAppsStore: filterClusterId, filterNamespace
├── types/apps.ts       // mirror of BE dto.* (Summary/Detail naming convention)
├── locales/{zh-CN,en-US}.json // apps.* / menu.apps.* / perm.apps.* / error.42*
└── views/apps/
    ├── Applications/
    │   ├── List.vue
    │   ├── Detail.vue
    │   ├── Install.vue                    // combined register + helm install wizard
    │   ├── Upgrade.vue
    │   └── components/
    │       ├── ApplicationFormBasic.vue
    │       ├── ChartPickerStep.vue
    │       ├── ValuesEditor.vue
    │       └── HistoryTable.vue
    └── ChartRepos/
        ├── List.vue
        └── Form.vue                        // modal form for create + edit
```

### 8.2 Routes (dynamic, via /me/menus)

```
/apps/applications              → Applications/List.vue
/apps/applications/:id          → Applications/Detail.vue
/apps/applications/new          → Applications/Install.vue
/apps/applications/:id/upgrade  → Applications/Upgrade.vue
/apps/chart-repos               → ChartRepos/List.vue
```

### 8.3 Install wizard

`Install.vue` is the combined register + install flow:

1. Step "Basics" — name / cluster / namespace / release_name / owner / tags / description. Submits `POST /apps/applications`, captures the new application_id.
2. Step "Choose chart" — repo → chart name → version, then `GET /repos/:id/charts/:chart/versions/:version/values` to seed the editor.
3. Step "Values" — `ValuesEditor` (CodeMirror YAML). Submit calls `POST /apps/applications/:id/release/install`.

Any step's failure preserves earlier steps' DB writes. The application list filter has a "registered, not deployed" pill so unfinished wizards remain discoverable.

### 8.4 ValuesEditor

Reuses P2's `vue-codemirror` + `@codemirror/lang-yaml` + `oneDark` setup (no new dependency). Two actions:

- "Load chart defaults" — confirms before overwriting current buffer.
- "Format" — `js-yaml` round-trip. `js-yaml` is added as a single new FE direct dependency (§8.8).

No localStorage draft cache. Each open initialises with either chart defaults or `GET /apps/applications/:id/release` returned values (upgrade flow). This trades one redundant re-edit on failed submit for zero risk of deploying stale data.

### 8.5 Pinia store

Single small store:

```ts
state: () => ({
  filterClusterId: null as number | null,
  filterNamespace: '' as string,
})
```

No application list cache. Each list-page mount fetches fresh.

### 8.6 i18n keys

49 new keys spanning `apps.*`, `menu.apps.*`, `perm.apps.*`, and `error.42*`. Full list in §A.1 (appendix). `bun run i18n:check` runs in CI.

### 8.7 v-permission usage

All action buttons declare permission via `v-permission`, never JS-side checks:

```html
<a-button v-permission="'apps:release:install'"   @click="goInstall">New install</a-button>
<a-button v-permission="'apps:release:upgrade'"   @click="goUpgrade">Upgrade</a-button>
<a-button v-permission="'apps:release:rollback'"  @click="rollback">Rollback</a-button>
<a-button v-permission="'apps:release:uninstall'" danger @click="uninstall">Uninstall</a-button>
```

### 8.8 New FE dependencies

**One new direct dependency: `js-yaml`** (≤ 20kB gzipped, no transitive surface). Used by `ValuesEditor` for the Format action (parse → dump round-trip) and by `Install.vue` to validate user input as a YAML map before submission. CodeMirror + `@codemirror/lang-yaml` come from P2. antdv, axios, pinia, vue-i18n from P0. No other additions.

---

## 9. P2 upstream feedback

P3 introduces one small change to P2's `k8s/cluster/handler.Delete`:

```go
// Before P3:
//   k8s/cluster.Service.Delete just calls repo.SoftDelete; FK ON DELETE RESTRICT
//   means deletion fails with a raw DB error if applications still reference it.
// After P3:
//   k8s/cluster.Service.Delete calls apps/application.Counter.CountByClusterID
//   first; if count > 0, returns CodeAppsApplicationInUse with a friendly
//   message_key.
```

`apps/application.Counter` is a 2-method interface declared in `apps/application` and consumed in `k8s/cluster`. `main.go` injects the implementation at wiring time. `k8s/cluster` does **not** import `apps/application` directly — the dependency goes through the interface to keep the P2 layer free of P3 imports.

---

## 10. Testing

### 10.1 BE unit tests

| Package | Coverage gate | Notes |
|---|---|---|
| `apps/repo` | ≥60% | repo service + service-level handler with `httptest.Server` mocking HTTP repos |
| `apps/application` | ≥60% | service + Counter + permission/audit paths |
| `apps/release` | ≥60% | helm SDK calls via `driver.NewMemory()` + `kubefake.PrintingKubeClient` |
| `apps/helmclient` | ≥60% | `restClientGetter` shim methods, error mapping in `buildRESTConfig` |
| `apps/errs` | ≥60% | every branch of `MapError` |

Helm SDK unit tests use an in-memory storage driver — see scaffold:

```go
// optimus-be/internal/modules/apps/release/release_test.go
func newFakeHelmCfg(t *testing.T) *action.Configuration {
    t.Helper()
    return &action.Configuration{
        Releases:     storage.Init(driver.NewMemory()),
        KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
        Capabilities: chartutil.DefaultCapabilities,
        Log:          func(_ string, _ ...interface{}) {},
    }
}
```

### 10.2 BE integration tests (`-tags=dbtest`)

| File | Verifies |
|---|---|
| `tests/integration/apps_repo_test.go` | repo CRUD, soft-delete, name unique conflict |
| `tests/integration/apps_application_test.go` | application CRUD, `(cluster,ns,release_name)` unique, FK RESTRICT, inuse counters |
| `tests/integration/apps_release_test.go` | release endpoint auth/audit (helm SDK calls stubbed) |

### 10.3 FE unit tests (vitest)

- `api/apps/{repo,application,release}.ts` — envelope shape, error propagation
- `ValuesEditor` — load defaults / format actions
- `HistoryTable` — rollback button gating by `v-permission`

### 10.4 OCI + HTTP manual smoke checklist (`optimus-be/scripts/p3-smoke.md`)

OCI flows are not automated. Manual checklist for release sign-off:

1. Start BE + log in as admin.
2. Add HTTP repo `https://charts.bitnami.com/bitnami`. List charts → select `nginx` → list versions → fetch default values.
3. Register application against a P2-registered dev cluster.
4. Install nginx; observe pod readiness via P2 list-pods. Upgrade (e.g., bump `replicaCount`); rollback; uninstall.
5. Add OCI repo `oci://ghcr.io/<account>/charts` with token; repeat install → upgrade → rollback → uninstall.
6. Attempt to delete the cluster while applications exist → expect 42001.

### 10.5 Swagger and permissions

- Every handler is swag-annotated (`@Summary @Description @Param @Success @Failure`).
- `make swag` regenerates both `optimus-be/api/docs/swagger.json` and `docs/api/swagger.json`.
- `make dump-perms` regenerates `docs/permissions.md`.
- CI runs `make swagger-diff` + `make perm-check`.

---

## 11. Acceptance criteria

### 11.1 Functional DoD

- [ ] admin CRUDs chart repos (OCI + HTTP); delete-with-references blocked with 42002.
- [ ] admin registers an application (DB row only); list shows it with `not deployed` indicator.
- [ ] admin completes install → upgrade → rollback → uninstall against one application; audit table has the four corresponding rows.
- [ ] Detail page shows helm history including status, chart_version, updated_at.
- [ ] Rollback to missing revision returns 42203.
- [ ] Install with duplicate `release_name` in same `(cluster, namespace)` returns 42201.
- [ ] Uninstall followed by delete-application succeeds; delete-application without prior uninstall returns 42204.
- [ ] Cluster delete while applications reference it: P2's cluster.Service.Delete returns 42001 via the inuse pre-check.
- [ ] All action buttons gated by `v-permission`; users without the perm see no button.
- [ ] `k8s_operator` role gains the four `apps:release:*` codes + read codes; can deploy but not register/edit/delete applications or repos.
- [ ] OCI + HTTP repo browsing both work (manual smoke).
- [ ] ValuesEditor refresh discards in-progress edits (no localStorage stash).
- [ ] All 49 new i18n keys present in both locales; `bun run i18n:check` passes.

### 11.2 Engineering DoD

- [ ] `make swag` output matches checked-in `docs/api/swagger.json` (CI `make swagger-diff` passes).
- [ ] `make dump-perms` output matches checked-in `docs/permissions.md` (CI `make perm-check` passes).
- [ ] Every `apps/*` package has ≥60% test coverage.
- [ ] `make lint` passes (golangci-lint clean).
- [ ] `bun run lint` passes (max warnings 0).
- [ ] `bun run typecheck` passes.
- [ ] `helmclient.Factory` documents per-request lifetime in its godoc; reviewers can confirm no caching.
- [ ] `apps_chart_repos.encrypted_password` uses P1's `vault.Cipher`; no second cipher instance introduced.
- [ ] helm SDK version pinned in go.mod and recorded in CLAUDE.md.
- [ ] `client-go v0.30.14` pin preserved (P2 invariant unaffected).

### 11.3 Security & audit DoD

- [ ] vault master key absent → BE fails fast at startup (existing P1 invariant; P3 reuses).
- [ ] All kubeconfig retrieval goes through `credentials.Consumer.GetKubeconfig` with purpose strings like `apps.release.install` / `apps.release.upgrade` / `apps.release.rollback` / `apps.release.uninstall` / `apps.release.status`.
- [ ] Repo list/detail endpoints never return `encrypted_password`; return `has_password: bool` instead.
- [ ] Every release write writes an audit row with `target_type=application`, `target_id=<id>`, `metadata={cluster_id, namespace, release_name, chart_version, revision}`.
- [ ] No write/exec/watch path is opened to k8s beyond what helm SDK uses internally. The P2 invariant "k8s endpoints are read-only by design" is unaffected.

---

## 12. Open questions / risks

| # | Risk | Mitigation |
|---|---|---|
| 1 | helm SDK ↔ client-go v0.30.14 version compatibility (helm latest pulls newer client-go transitively) | Plan task 1 is a hard gate: pin to v3.16.x; if breaks, fall back to v3.15.x. Record decision in CLAUDE.md. |
| 2 | OCI registry auth varies by registry (anonymous, basic, token, Docker Hub free-tier rate limits). Unit tests cannot cover all permutations. | Manual smoke checklist (§10.4) covers the two registries we actually use. |
| 3 | Helm storage driver "secrets" places secrets in the target namespace; if user has tight namespace RBAC on the kubeconfig, helm may fail on the second revision. | Document the requirement: the kubeconfig context's user must have `get/list/create/update/delete/patch` on `secrets` in the application's namespace. |
| 4 | helm rollback rewrites a new revision (revision N+1 with content of revision M); FE must show updated_at and revision numbers clearly to avoid user confusion. | UI revision table makes "this revision was a rollback of revision X" explicit when `description` contains `Rollback to`. |

---

## Appendix A — i18n keys

### A.1 New keys (49)

```
apps.title                              "应用"           "Applications"
apps.application.list.title             "应用列表"       "Applications"
apps.application.list.col.name          "名称"           "Name"
apps.application.list.col.cluster       "集群"           "Cluster"
apps.application.list.col.namespace     "命名空间"       "Namespace"
apps.application.list.col.release       "Release 名"     "Release"
apps.application.list.col.chart         "Chart"          "Chart"
apps.application.list.col.owner         "Owner"          "Owner"
apps.application.list.col.actions       "操作"           "Actions"
apps.application.list.action.detail     "详情"           "Detail"
apps.application.list.action.upgrade    "升级"           "Upgrade"
apps.application.list.action.uninstall  "卸载"           "Uninstall"
apps.application.list.action.installNew "新建应用"       "Install new"
apps.application.detail.title           "应用详情"       "Application detail"
apps.application.detail.section.basic   "基本信息"       "Basics"
apps.application.detail.section.history "Revision 历史" "Revision history"
apps.application.detail.btn.rollback    "回退到此版本"   "Rollback to this revision"
apps.repo.list.title                    "Chart 仓库"     "Chart repositories"
apps.repo.form.field.type               "类型"           "Type"
apps.repo.form.field.url                "URL"            "URL"
apps.repo.form.field.username           "用户名"         "Username"
apps.repo.form.field.password           "密码 / Token"   "Password / Token"
apps.repo.form.field.hasPassword        "已设置密码"     "Password set"
apps.install.step.basic                 "基本信息"       "Basics"
apps.install.step.chart                 "选择 Chart"     "Choose chart"
apps.install.step.values                "values 配置"    "Values"
apps.install.btn.submit                 "安装"           "Install"
apps.install.btn.loadDefaults           "加载默认 values" "Load defaults"
apps.upgrade.btn.submit                 "升级"           "Upgrade"
menu.apps                               "应用"           "Applications"
menu.apps.applications                  "应用列表"       "Applications"
menu.apps.chart-repos                   "Chart 仓库"     "Chart repos"
perm.apps.application.read              "查看应用"       "View applications"
perm.apps.application.write             "管理应用元数据" "Manage application metadata"
perm.apps.application.delete            "删除应用"       "Delete applications"
perm.apps.release.install               "安装 release"   "Install release"
perm.apps.release.upgrade               "升级 release"   "Upgrade release"
perm.apps.release.rollback              "回退 release"   "Rollback release"
perm.apps.release.uninstall             "卸载 release"   "Uninstall release"
perm.apps.repo.read                     "查看 Chart 仓库" "View chart repositories"
perm.apps.repo.write                    "管理 Chart 仓库" "Manage chart repositories"
perm.apps.repo.delete                   "删除 Chart 仓库" "Delete chart repositories"
error.42001                             "应用尚有 release 未卸载"    "Release still installed"
error.42002                             "仓库被应用引用，无法删除"   "Repository still referenced"
error.42003                             "release 名在该 namespace 已存在" "Release name already in use"
error.42004                             "应用关联的集群已删除"       "Cluster has been deleted"
error.42101                             "无法连接到 chart 仓库"      "Cannot reach chart repository"
error.42102                             "Chart 仓库认证失败"         "Chart repository auth failed"
error.42103                             "未找到 chart 或版本"        "Chart or version not found"
error.42104                             "Chart 仓库 index 解析失败"  "Failed to parse chart index"
error.42105                             "OCI 仓库拉取失败"           "OCI registry error"
error.42199                             "Chart 仓库错误"             "Chart repository error"
error.42201                             "Release 已存在"             "Release already exists"
error.42202                             "Release 不存在"             "Release not found"
error.42203                             "目标 revision 不存在"       "Target revision missing"
error.42204                             "Release 仍存在，请先卸载"   "Release still installed"
error.42205                             "values YAML 不合法"         "Invalid values YAML"
error.42299                             "Helm 错误"                  "Helm error"
```
