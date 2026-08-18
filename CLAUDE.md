# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository shape

Monorepo with two deployable apps plus shared deployment assets:

- `optimus-be/` — Go 1.25 / Gin / GORM / Postgres backend. Modules under `internal/modules/`: `auth`, `user`, `role`, `rbac`, `menu`, `permission`, `audit`, `me` (P0); `credentials/{vault,sshkey,kubeconfig,cloudkey}` (P1); `k8s/{cluster,client,clusterscoped,workload,network,config,secret,log,yaml,apierr}` (P2, read-only).
- `optimus-fe/` — Vue 3 + Ant Design Vue + Pinia + vue-router SPA. Talks only to `/api/v1/*`. P2 adds `vue-codemirror` + `@codemirror/lang-yaml` + `@codemirror/theme-one-dark` for the YAML viewer.
- `deploy/` — the single Dev/UAT/Prod `docker-compose.yml`, one `.env.example`
  with local Dev defaults, and multi-stage Dockerfiles (`be.Dockerfile` builds
  one backend image containing the server/migrate/seed/vault-keygen binaries;
  `fe.Dockerfile` builds the nginx-served SPA).
  UAT and Production override the defaults in their untracked `.env`.
- `docs/superpowers/specs/` and `docs/superpowers/plans/` — the authoritative P0 design spec and execution plans. Permission/API contracts come from here.
- `docs/api/swagger.json` and `docs/permissions.md` — **generated artifacts**, checked in. CI (`make swagger-diff` / `make perm-check`) fails if they drift from source.

## Current project phase

As of 2026-08-18, P0-P6 are implemented and merged to `main`. P6 Application
Delivery completed its approved 2026-07-27 design and all 29 implementation
tasks. The final disposable Kubernetes/Helm smoke found and fixed two production
wiring gaps: system kubeconfig purposes now receive the required `system:`
prefix, and the production Helm adapter forwards `LoadVerifiedChart`.

P6 merged through PR #4 (`14be03d`), PR #5 (`f1857f5`) fixed CI reliability
and frontend build issues, and PR #6 (`db4606a`) added authentication feedback,
built-in RBAC/menu refinements, dynamic-route bootstrap fixes, and regression
tests. PR #7 merged the deployment preparation on 2026-08-18. `main` and
`origin/main` resolve to `f364af7`; `origin/dev` remains at PR #7's second
parent `f555d72`, while local `dev` adds only this progress-documentation
refresh. There was no source diff between the merged branch tip and `main`.
The repository has no release tag yet.

Local pre-release validation steps 1-11 now pass on `dev`. In addition to the
local UI/backend flow, Colima Kubernetes path, and backend quality gates, the
user completed the full P3 application lifecycle smoke and the full P4 AWS
assets smoke with a disposable read-only credential. The Kubernetes validation
exposed and fixed the missing authenticated-actor bridge into
`credentials.Consumer`; P4 synchronization was verified after removing an
ambient host `AWS_PROFILE` from the backend environment. Basic development
environment acceptance has passed.

Steps 12 and 13, `optimus-be/scripts/p5-smoke.md` and
`optimus-be/scripts/p6-smoke.md`, have not passed and are not waived. The next
milestone is preliminary local Dev acceptance: start the default full stack
through the unified Compose file on Colima, run the disposable P5 Prometheus
fixtures on Colima Docker, and run the disposable P6 PostgreSQL/chart fixtures
against an isolated namespace in Colima Kubernetes. Real UAT/Production
acceptance remains required afterward.

Release sign-off still requires local P5/P6 preliminary acceptance, UAT
deployment, a production-like persistent-data upgrade smoke from the P5
baseline `4e2d08b` to `f364af7` (including migration
`00023_p6_delivery.sql`, seed idempotency, restart, rollback, and recovery
checks), environment-specific P5/P6 checks, Production deployment and
acceptance, and only then tag/release.

The P4 release checklist has passed with a disposable read-only AWS credential.
Retain `optimus-be/scripts/p5-smoke.md` and
`optimus-be/scripts/p6-smoke.md` as the acceptance contracts for local and
future real-environment validation; do not mark either complete until it is
run.

Deployment preparation merged through PR #7 and now has one Compose entry
point and one environment example with local Dev defaults and UAT/Production
overrides. The backend server, migrate, seed, and vault-keygen programs are
packaged in one image; Compose selects the role-specific entrypoint. A real
cold Buildx build
of that image passed on a freshly recreated 2C4G Colima VM. CI keeps quality
checks on `dev`, `main`, and pull requests, while image build and dual
GHCR/Docker Hub publishing run only on `main` pushes and emit the immutable
`main-<short-sha>` tag. The `main-f364af7` backend and frontend images are
published in both registries with matching cross-registry digests. The next
deployment action follows local P5/P6 Dev acceptance: deploy UAT before
Production acceptance.

## Daily commands

### Local Docker and Kubernetes runtime

Local development on both macOS and Linux uses **Colima** as the project
runtime boundary. Use the default Colima profile with the Docker runtime and
its built-in Kubernetes enabled:

```bash
colima start --runtime docker --kubernetes
docker context use colima
kubectl config use-context colima
colima status
docker info
kubectl cluster-info
```

All project-local `docker`, `docker compose`, dockertest, `make test-int`, and
P5 container fixtures must use the active Colima Docker context. Do not
silently fall back to Docker Desktop or a host system Docker daemon. If a tool
does not honor Docker contexts, obtain the socket path from `colima status` and
set `DOCKER_HOST` for that command or shell; do not hardcode a macOS
`/Users/...` socket path because Linux uses a different home layout.

Colima's built-in Kubernetes is the default cluster for routine P2/P3 local
development and P6 release smoke. The P6 checklist requires the `colima` kube
context and isolates all Kubernetes resources in a disposable namespace. Check
the active kube context before cluster operations:

```bash
kubectl config use-context colima
```

Colima supports both macOS and Linux. Linux/WSL2 hosts without `/dev/kvm` can
run through QEMU software virtualization, but startup and integration tests may
be slower. CI runners are not subject to this local-runtime rule.

All local backend build and analysis caches live under the ignored
`optimus-be/tmp/` tree. The backend Makefile prepares and exports repository-
local `TMPDIR`, `GOCACHE`, and `GOLANGCI_LINT_CACHE` paths automatically. Do not
redirect backend build caches to `/tmp`; this project can exceed a small tmpfs
while compiling the Helm, Kubernetes, and AWS dependency graph.

### Backend (run from `optimus-be/`)

| Goal | Command |
|---|---|
| One-off tool install (air, goose, swag, golangci-lint) | `make tools` |
| Hot-reload dev server on :8080 | `make run` (uses `air`) |
| Build static binary | `make build` → `bin/optimus-be` |
| Unit tests (race + cover) | `make test` |
| Integration tests (dockertest brings up real Postgres per package) | `make test-int` |
| Lint | `make lint` |
| Regenerate swagger + copy into `../docs/api/swagger.json` | `make swag` |
| Regenerate `../docs/permissions.md` from `internal/infra/permissions/codes.go` | `make dump-perms` |
| Apply / roll back migrations | `make migrate-up` / `make migrate-down` |
| New migration file | `make migrate-new name=<snake_case>` |
| Bootstrap admin (prints initial password ONCE on first run) | `make seed` |

Run a single test:
```bash
go test ./internal/modules/user/... -run TestService_Create -race
# integration variant (requires Colima/Docker — see Gotchas)
go test -tags=dbtest ./tests/integration/... -run TestUserCRUD -race -count=1
```

`OPTIMUS_JWT_SECRET` (≥32 bytes) must be exported or the server refuses to
start. The default DSN in `configs/config.yaml` matches the PostgreSQL service
exposed by `deploy/docker-compose.yml` on `127.0.0.1:5432`.

### Frontend (run from `optimus-fe/`)

Package manager is **bun** (never npm/pnpm/yarn).

| Goal | Command |
|---|---|
| Install | `bun install` |
| Dev server :5173 (proxies `/api/v1` → :8080) | `bun run dev` |
| Production build (typecheck + vite) | `bun run build` |
| Lint (`--max-warnings=0`) | `bun run lint` |
| Type check only | `bun run typecheck` |
| i18n key parity (zh-CN ↔ en-US) | `bun run i18n:check` |
| Vitest run/watch | `bun run test` / `bun run test:watch` |

Single test: `bun x vitest run path/to/file.test.ts -t "name pattern"`.

### Dev / UAT / Production Compose

```bash
cd deploy
docker compose up -d --build         # local full-stack Dev
docker compose ps -a
```

For host-side hot reload, start only PostgreSQL with
`cd deploy && docker compose up -d postgres`, then use `make run` and
`bun run dev`. Connect to PostgreSQL with `psql` on `127.0.0.1:5432`; there is
no Adminer service.

For UAT or Production:

```bash
cd deploy
cp .env.example .env                 # replace every development credential
docker compose pull
docker compose up -d --no-build
docker compose logs seed | grep INITIAL  # initial admin password (first run only)
```

Set the real environment's Compose project name, image repository, immutable
version, domains, secrets, and capacity/retention values in `.env`.

Expected steady state: `postgres` healthy, `migrate` Exited(0), `seed`
Exited(0), `optimus-be` healthy, and `optimus-fe` healthy.

## Architecture — backend

**Layering inside each `internal/modules/<name>/`**: `dto.go` → `repo.go` (GORM) → `service.go` (business + audit + cache invalidation) → `handler.go` (Gin binding/validation, calls `response.Success/Error`). The HTTP envelope is fixed: `{code, data, message, message_key?}` (see `internal/infra/response/envelope.go`). All errors flow through `apperr.BizError` with numeric codes from `internal/infra/errors/codes.go` — handlers never return raw error text to clients.

**Wiring**: `cmd/server/main.go` is the only composition root. It:
1. Loads config (Viper, `OPTIMUS_*` env overrides `configs/config.yaml`) and refuses to start on missing JWT secret.
2. Calls `permissions.Register(ctx, db, permissions.All)` to **upsert** every permission code from `internal/infra/permissions/codes.go` into the `permissions` table. This is the source of truth — new permissions are added by appending to `codes.go`, never by raw INSERT.
3. Builds a single `rbac.PermissionCache` with a 60s TTL (per spec §7.4). Every service that mutates roles/user-roles/role-permissions MUST call `cache.InvalidateUser(uid)` or `cache.InvalidateAll()` — there is no cross-process invalidation.
4. Mounts routes with **per-route `RequirePermission` middleware via nested sub-groups** (see the `mountUserRoutes`/`mountRoleRoutes`/... helpers in `main.go`). The comment there is load-bearing: passing middleware as variadic args to `GET/POST` is not equivalent — only `Group("", mw)` guarantees middleware runs before handlers registered separately.

**Auth flow**: `POST /api/v1/auth/login` → bcrypt verify → issue access (15m) + refresh (168h) JWTs signed by `crypto.NewJWTSigner(cfg.JWT.Secret)`. Refresh tokens are persisted (`refresh_tokens` table) and rotated on use; replay detection raises `CodeRefreshTokenReplay` (40104). Login is rate-limited per-IP via `ratelimit.NewLoginLimiter`.

**Permission resolution**: `PermissionCache.load` joins `permissions → role_permissions → user_roles → users` (filters `users.deleted_at IS NULL` and excludes soft-deleted roles). The middleware `RequirePermission(cache, "system:user:read")` is the only gate — no in-handler permission checks.

**Generated artifacts must stay in sync**:
- `make swag` writes both `optimus-be/api/docs/swagger.json` (consumed by the `_ "optimus-be/api/docs"` blank import that powers `/swagger/*`) **and** `docs/api/swagger.json`.
- `make dump-perms` writes `docs/permissions.md` from the in-code registry.
- CI runs `make swagger-diff` + `make perm-check` — both fail the build on drift. Always run these locally before committing handler annotation or permission code changes.

**Migrations**: goose SQL files in `optimus-be/migrations/`, embedded via `embed.go`. Both `cmd/migrate` (container) and `make migrate-up` (dev) use the same files. Foreign keys live in `00010_foreign_keys.sql` — schema-first work happens in earlier files, FKs and partial unique indexes added at the end of the chain.

**Models** in `internal/models/` are the GORM struct definitions; tests in `tests/integration/` use `dockertest` (requires Docker — see Gotchas) and the `dbtest` build tag.

## Architecture — frontend

**Bootstrap order** (`src/main.ts`): Pinia → AntdV → i18n → API client (with `onLogout` callback that resets stores + redirects to `/login`) → per-module APIs are `provide`-injected (`authApi`, `meApi`, `userApi`, ...) → router guards installed last, then mount.

**Routing**: split into static (login / 403 / 404 / 500 / profile, in `router/static-routes.ts`) and dynamic (`router/dynamic-routes.ts`). On the first authenticated navigation, `router/guards.ts` calls `meApi.get/menus/permissions` in parallel and `registerDynamicRoutes(router, menus)` injects routes from `/me/menus`. Reroute the original target with `{ ...to, replace: true }` so it lands on the freshly-registered route.

**Permission enforcement** has two layers and they MUST stay aligned:
- Route gate: `to.meta.permission` checked in the guard (returns to `/forbidden` on miss).
- DOM gate: `v-permission` directive (`src/directives/`).
Both read the permission list from `useAuthStore().permissions` — never re-fetch in components.

**API envelope handling** (`src/api/client.ts`): every response is checked against the envelope shape; non-zero `code` → throws a `BizError` so callers `.catch`. On HTTP 401 (not for `/auth/refresh` itself), the axios interceptor calls `useAuthStore().refreshAccessToken()` which holds the **single-flight** promise (`refreshing: Promise<TokenPair> | null` as Pinia state) so concurrent axios 401s AND the P2 `useLogStream` SSE consumer share one `/auth/refresh` call. Original request replays once via `__retried`. If the refresh itself 401s, `onLogout()` fires inside the store action and the retry path skips it.

**i18n**: keys in `src/locales/{zh-CN,en-US}.json`. `bun run i18n:check` enforces missing-key + cross-locale parity and is wired into CI; adding a key to one locale without the other breaks the build.

**Vite alias**: `@/*` → `src/*`. dev proxy: `/api/v1` → `http://localhost:8080` (set in `vite.config.ts`).

## Architecture — credentials (P1) and k8s (P2)

**Credentials vault** (`internal/modules/credentials/`): AES-256-GCM application-layer encryption. Master key loaded from `OPTIMUS_VAULT_MASTER_KEY` or `_FILE` **before** `db.Open` so missing key fails fast. **`credentials.Consumer` (`consume.go`) is the SOLE public Go API** for downstream modules — never re-fetch credentials via HTTP, never new-up another cipher. Set non-HTTP actor with `credentials.WithActor(ctx, id)` (private `ctxKey` typed key — `context.WithValue` with raw string would trip staticcheck SA1029; gin's `c.Set/c.Get` with string is fine because it uses `map[string]any`). Kubeconfig validation rejects `exec` and `auth-provider` auth plugins (RCE attack surface). Kubeconfig `Delete` refuses while `k8s.clusters` references it via `k8s/cluster/inuse.CountByKubeconfigID`.

**k8s module** (`internal/modules/k8s/`): read-only console — NO `exec`, `apply`, write verbs, or `watch`. Per-request `kubernetes.Interface` built fresh in `client/factory.go` from `credentials.Consumer.GetKubeconfig(clusterID)` and **discarded after the handler returns** — no clientset caching. Apiserver errors normalized via `apierr/apierr.go.MapError` into 5 numeric codes (`41101` ClusterUnreachable, `41103` APIServerForbidden, `41104` APIServerUnauthorized, `41105` APIServerOther, `41202` LogUnavailable). Generic kind dispatcher in `workload/` handles 7 kinds (Deployment, StatefulSet, DaemonSet, Job, CronJob, Pod, ReplicaSet) via a `kind` path param. Pod logs stream over **SSE via Gin's `http.ResponseController` to bypass the response buffer** (`internal/modules/k8s/log/handler.go`). FE consumes SSE via `fetch()` + `ReadableStream` (NOT `EventSource`) so the JWT stays in `Authorization` (see `optimus-fe/src/api/k8s/log.ts` + `useLogStream` in `stores/k8s.ts`). YAML viewer/editor backed by `vue-codemirror`.

**Cluster picker** (`components/layout/ClusterPicker.vue` mounted in `AppHeader.vue`): selected cluster ID lives in the `k8s` Pinia store; every k8s page reads from it. Pages that need it but find it unset should show a "select a cluster" hint, not auto-redirect.

## Architecture — assets (P4)

**`assets` module** (`internal/modules/assets/`): AWS-only, read-only cloud
resource discovery. `cloud_accounts` binds a P1 cloud key to
`enabled_regions`; sweeps populate `aws_instances`, `aws_vpcs` together with
`aws_subnets`, and `aws_databases`. Missing resources are soft-deleted only
after an authoritative successful sweep; VPC and subnet persistence share one
transaction.

**Syncing** (`sync/`): a per-account in-memory lock gates cron and manual
work. `POST /api/v1/assets/cloud-accounts/{id}/sync` records its audit event,
starts a bounded detached worker, and returns immediately. The cron scheduler
uses `OPTIMUS_ASSETS_SYNC_CRON`, applies the configured startup delay, and
prunes `assets_sync_runs` after the configured retention period.

**Cloud credentials and consumers**: all sweeps obtain cloud keys only through
`credentials.Consumer` and discard fresh AWS clients after each sweep.
`assets.Consumer` in `consume.go` is the non-HTTP seam for later phases to
look up instances by private IP, instance ID, or VPC. It returns the
`ErrAssetsInstanceNotFound` sentinel for absent matches.

**P1 cloud-key protection**: `credentials/cloudkey.Service` accepts an
optional, nil-safe assets in-use counter. When it is wired at server startup,
deleting a referenced cloud key fails with `43001`.

**Configuration**: `OPTIMUS_ASSETS_SYNC_CRON`,
`OPTIMUS_ASSETS_SYNC_STARTUP_DELAY`,
`OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS`, and
`OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT`.

## Architecture — observability (P5)

**Scope**: P5 is a read-only metrics observability MVP. It stores Prometheus
data-source configuration and custom dashboard definitions, proxies bounded
instant/range/metadata queries, and renders built-in Kubernetes dashboards.
It does not persist metric samples. Alerts, alert rules, notifications,
Alertmanager, CloudWatch, logs, traces, and APM are explicitly out of scope.

**Data sources and credentials**: Basic and bearer secrets are P1 HTTP
credentials consumed only through `credentials.Consumer`. Custom CA PEM is
public trust material but is never returned or audited. Metric-only operators
use `GET /observability/query-sources`, limited to `id`, `name`, and nullable
`cluster_id`; built-ins must not call the administrative data-source list.

**SSRF and clients**: reject userinfo, non-HTTP(S) schemes, metadata,
loopback, link-local, reserved, mapped/translated private addresses, and mixed
DNS answers containing a denied address. Private destinations require a
narrow explicit CIDR. Redirects are never followed. Prometheus clients and
authorization headers are request-scoped and must not be cached or forwarded.

**Queries, audits, and RBAC**: enforce query count, concurrency, PromQL size,
range, step, points, series, response bytes, timeout, and enrichment limits.
Full PromQL, credentials, authorization headers, custom CA PEM, and raw
upstream errors must not enter logs, audits, Swagger, or client errors.
Dashboard audits use bounded SHA-256 PromQL fingerprints. Data-source,
dashboard, and metric permissions are independent; Kubernetes built-ins need
only metric read. Separate abort generations prevent stale definition/query
responses from updating newer views.

**Verification**: run backend `make test`, `make test-int`, `make lint`,
`make swagger-diff`, and `make perm-check`; frontend frozen install, lint,
typecheck, i18n check, tests, and build; then complete
`optimus-be/scripts/p5-smoke.md`.

## Architecture — application delivery (P6)

**Scope**: P6 promotes an immutable Helm chart artifact through an ordered
pipeline of environments bound to existing P3 applications. It supports
projects, immutable environment bindings, versioned pipelines, runs, approvals,
SSE timelines, cancellation, reconciliation, and linked retries. Arbitrary
shell commands, scripts, manifests, values, images, and credential inputs are
outside the execution contract.

**Execution and governance**: run creation resolves and freezes the chart
digest and all stage targets. The leased worker executes only the closed
`UpgradeExisting` operation using stable operation IDs. P3 direct
upgrade/uninstall is denied for managed applications; delivery receives a
narrow in-process capability. Initiators cannot approve their own runs.
Ambiguous executor outcomes become `outcome_unknown` and require persisted
inspection/reconciliation rather than an assumed retry.

**Credential and artifact boundaries**: Helm clients are fresh per operation.
System kubeconfig consumption is prefixed with `system:` before crossing
`credentials.Consumer`. `HelmChartLoader.LoadVerifiedChart` delegates digest
verification, parsing, and byte wiping to the P3 repository service. Values,
kubeconfigs, authorization headers, manifests, Helm notes, and raw errors must
not enter API, SSE, audit, or logs.

**Verification**: the approved spec and plan are
`docs/superpowers/specs/2026-07-27-p6-application-delivery-design.md` and
`docs/superpowers/plans/2026-07-27-p6-application-delivery.md`. Complete backend,
frontend, generated-artifact, architectural scan, and real disposable
`optimus-be/scripts/p6-smoke.md` gates before release.

## Conventions worth knowing

- **bun everywhere on the frontend** — never npm/pnpm/yarn. CI uses `bun install --frozen-lockfile`.
- **Permission codes** live in one place (`internal/infra/permissions/codes.go`) and propagate to: the DB via `Register()` at startup, the `RequirePermission` middleware, the FE permission list via `/me/permissions`, and `docs/permissions.md`. Touch any of them and run `make dump-perms` + `make perm-check`.
- **Swagger annotations** are checked by `make swagger-diff` in CI. Add/modify a handler's `@Summary/@Param/@Success` and regenerate.
- **CORS env var is comma-separated, not JSON**: `OPTIMUS_CORS_ALLOWED_ORIGINS=https://a.example.com,https://b.example.com` (no brackets/quotes). The YAML config takes a list, but env override is comma-split. This bit Plan 3.
- **No raw error text to clients** — wrap in `apperr.BizError(code, ...)`. `response.Error` logs unhandled errors with `slog.Error` and returns generic `CodeInternal`.
- **Audit logging**: every mutating service path calls `audit.Recorder.Record(...)`. The recorder is shared so `/me` writes and admin `/users` writes hit the same sink — don't construct a second recorder.
- **k8s.io/client-go is pinned to v0.30.14** (and apimachinery to v0.30.14) to keep `go.mod`'s go directive at 1.25. `go get k8s.io/client-go@latest` (v0.36+) transitively bumps go to 1.26+. If you ever do bump, also update `deploy/be.Dockerfile` (golang:1.26-alpine) and CI `go-version`.
- **`helm.sh/helm/v3` is pinned to v3.15.4** (fell back from v3.16.4 because the 3.16 line transitively upgrades `k8s.io/client-go` to v0.31.x, breaking the P2 v0.30.14 invariant). Bumping helm transitively bumps client-go, so any helm upgrade re-runs the P2 compatibility verification. Pin is checked at startup only by `go build`; no runtime assertion.
- **AWS SDK Go v2 modules** (`github.com/aws/aws-sdk-go-v2`, `.../config`, `.../credentials`, `.../service/ec2`, `.../service/rds`) and **`github.com/robfig/cron/v3`** are pinned for P4 assets and must not raise `go.mod` above Go 1.25; pin an offending submodule back instead of raising the project Go version.
- **k8s endpoints are read-only by design** — never add write/apply/exec/watch handlers without re-opening the P2 spec. The /data secret reveal endpoint is the only path that returns plaintext secret values; it is RBAC-gated by `k8s:secret:reveal`.
- **Claude Code uses the repository's local mem0 MCP configuration** — `.mcp.json` launches `uvx mem0-mcp-server`, and `.claude/settings.json` enables the `mem0` server. Keep `MEM0_API_KEY` exported before starting Claude Code. Unless the user explicitly requests another scope, use `user_id = "logic"` for mem0 reads and writes so project memories remain together.
- **CLAUDE Code skills/superpowers** are configured at `~/.claude/` and `.claude/`; the `.claude/settings.json` here only adjusts permissions/hooks for this repo.

## Gotchas (local-only)

- **Docker and local Kubernetes run through Colima on macOS and Linux.** If
  `docker compose`, dockertest, or `kubectl` cannot connect, run `colima status`,
  verify the `colima` Docker and kube contexts, and use the socket reported by
`colima status` for context-unaware tools. Do not assume `/var/run/docker.sock`
or a platform-specific Colima socket path.
- **Run Compose only as `docker compose ...`.** The Homebrew Compose plugin is
  exposed through `~/.docker/cli-plugins/docker-compose`; confirm the Docker CLI
  discovers it with `docker compose version` before running local or smoke
  workflows.
- **HEAD vs GET on healthcheck**: the container healthchecks use `wget` which issues GET. Gin only registers GET handlers by default — keep `/api/v1/health` on GET, not HEAD-aliased.
- **Initial admin password is logged exactly once**, on the first run of `cmd/seed` (or first `make run` against an empty DB). Capture it from stdout / `docker logs optimus-seed | grep INITIAL`. Subsequent runs print "admin user already exists; no password generated." If you lose it, you must reset via DB.
- **Vault master key must be set before BE starts.** `OPTIMUS_VAULT_MASTER_KEY` (base64'd 32 bytes) or `OPTIMUS_VAULT_MASTER_KEY_FILE`. Generate with `cd optimus-be && go run ./cmd/vault-keygen` (or the `vault-keygen` Dockerfile target). Loaded BEFORE `db.Open` so a missing/wrong key fails fast.
- **SSE responses bypass Gin's response buffer via `http.ResponseController(c.Writer).Flush()`** — using `c.Writer.Flush()` alone won't actually push bytes through gin's writer wrapper for streaming, and bypassing gin's writer breaks middleware. The pattern in `internal/modules/k8s/log/handler.go` is canonical.
- **`sigs.k8s.io/yaml` quirk**: a single-character namespace name like `"a"` round-trips through YAML decoding as the int `97` (ASCII byte). Use multi-character names (e.g., `"default"`, `"app"`) in tests to avoid this trap.

## First-session checklist (new machine)

If this is the first time touching this repo on macOS or Linux:

1. **Tools**: install `uv`, `bun`, `colima`, the Docker CLI with Docker Compose,
   `kubectl`, `helm`, and `git`. Homebrew may be used on both supported host
   platforms. Expose the Homebrew Compose plugin through
   `~/.docker/cli-plugins/docker-compose`, confirm it with
   `docker compose version`, then run
   `colima start --runtime docker --kubernetes` and verify the `colima` Docker
   and kube contexts.
2. **mem0 API key**: `export MEM0_API_KEY="..."` (from password manager). Persist in `~/.zshrc`. Claude Code auto-loads the repository's `.mcp.json`, which starts `uvx mem0-mcp-server`. Scope mem0 calls to `user_id = "logic"` unless another user is explicitly requested.
3. **First prompt** in Claude Code:
   > "Read CLAUDE.md, search mem0 for the optimus checkpoint, then `git log dev..main` to confirm what's actually merged. Summarize where the project is and recommend the next step."

   Claude should pull the latest `[CHECKPOINT YYYY-MM-DD]` memory from mem0 (single source of "current status"), cross-check against git, and propose the next plan task. All the durable conventions / gotchas / patterns (FE zero-wrapper, axios single-flight, v-permission, Pinia layering, TDD layering, nullability contract, vue-i18n drill trap, Colima socket path, etc.) live as separate mem0 atoms and surface on semantic search — no need to re-derive.
4. **Refreshing the checkpoint**: after finishing a plan or hitting a milestone, tell Claude "刷快照 / refresh the checkpoint" — it will delete the old `metadata.kind=checkpoint` atom and write a new one for today.
