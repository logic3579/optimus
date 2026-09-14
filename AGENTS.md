# Optimus Project Guide

This is the sole project convention and operating-contract file for the
Optimus repository. Keep durable architecture, workflow, environment, and
release guidance here; do not create a parallel assistant-specific guide.

## First Read

Start every substantial task by reading:

1. `AGENTS.md`
2. The active phase's design and implementation plan under
   `docs/superpowers/specs/` and `docs/superpowers/plans/`.
3. For P4 maintenance, `docs/superpowers/plans/2026-06-11-p4-assets.md` and
   `docs/superpowers/specs/2026-06-11-p4-assets-design.md`.

If mem0 is available, search with `user_id = "logic"` for the latest
`[CHECKPOINT YYYY-MM-DD]` memory for `optimus` and cross-check it against
`git status --short --branch` and recent
`git log --oneline --decorate --max-count=20 --all`.

Current expected project state (2026-09-14): P0-P6 are implemented and merged
to `main`. P6 Application Delivery completed the approved 2026-07-27 design
and all 29 plan tasks, including immutable Helm promotion runs, approvals, SSE
events, restart/reconciliation recovery, frontend delivery pages, generated
artifacts, and a real disposable Kubernetes/Helm smoke. The final smoke found
and fixed two production wiring gaps: system kubeconfig purposes require the
`system:` prefix, and the production Helm adapter must forward
`LoadVerifiedChart`. P6 merged through PR #4 (`14be03d`); PR #5 (`f1857f5`)
fixed CI reliability and frontend build issues; PR #6 (`db4606a`) refined
authentication feedback, built-in RBAC roles, menu metadata, dynamic-route
bootstrap, and related tests.

PR #7 merged the deployment preparation to `main` on 2026-08-18. `main` and
`origin/main` are at `f364af7`; `origin/dev` remains at PR #7's second parent
`f555d72`, and local `dev` advances it with progress and Codex configuration
commits. There was no source diff between the merged branch tip and `main`.
The main-push pipeline completed and published immutable
`main-f364af7` `optimus-be` and `optimus-fe` images to both GHCR and Docker Hub;
the corresponding cross-registry digests match. Deployment uses one
environment-parameterized Compose file, one environment example with local Dev
defaults, and one backend image containing all operational binaries. This
release uses that stack for Dev and Production only. No release tag exists yet.

Local pre-release validation steps 1-13 have passed. This includes the local
UI/backend path, Colima Kubernetes cluster connection, backend lint and
generated-artifact gates, the complete P3 application lifecycle smoke, and the
complete P4 AWS assets smoke with a disposable read-only credential. Local P5
acceptance passed with disposable Prometheus fixtures. Local P6 acceptance
passed on 2026-09-14 with disposable PostgreSQL/chart resources and an isolated
namespace in Colima Kubernetes, covering immutable promotion, approvals, SSE,
failure rollback, retry, interruption/reconciliation, and secret/audit
redaction; all disposable resources were removed without stopping Colima.
The Kubernetes validation also fixed the missing authenticated-actor bridge
into `credentials.Consumer`; P4 synchronization passed after removing an
ambient host `AWS_PROFILE` from the backend environment.

The selected deployment topology is Dev and Production only; UAT is explicitly
skipped for this release. The immediate next task is Production deployment.
Production rollout/sign-off still includes the production-like persistent-data
upgrade smoke from `4e2d08b` through `00023_p6_delivery.sql`,
Production-specific acceptance, and only then tag/release.

## Project Shape

- `optimus-be/`: Go 1.25, Gin, GORM, and PostgreSQL backend. Modules live under
  `internal/modules/`: P0 auth/user/role/RBAC/menu/permission/audit/me; P1
  credentials; P2 Kubernetes; P3 applications; P4 assets; P5 observability;
  and P6 delivery.
- `optimus-fe/`: Vue 3, Ant Design Vue, Pinia, vue-router, and vue-i18n SPA. It
  talks only to `/api/v1/*`; Kubernetes YAML uses CodeMirror.
- `deploy/`: the single environment-parameterized `docker-compose.yml`, one
  `.env.example` with local Dev defaults, and multi-stage backend/frontend
  Dockerfiles. The backend image contains server, migrate, seed, and
  vault-keygen binaries. The current release deploys Dev and Production only.
- `docs/superpowers/specs/` and `docs/superpowers/plans/`: authoritative phase
  designs and implementation plans.
- `docs/api/swagger.json` and `docs/permissions.md`: checked-in generated
  artifacts. CI fails when they drift from source.

## Commands

Backend commands run from `optimus-be/`:

- `make tools`
- `make run`
- `make build`
- `make test`
- `make test-int`
- `make lint`
- `make swag`
- `make swagger-diff`
- `make dump-perms`
- `make perm-check`
- `make migrate-up` / `make migrate-down`
- `make migrate-new name=<snake_case>`
- `make seed`

Run a focused backend test with
`go test ./internal/modules/user/... -run TestService_Create -race`.
Integration variants require Colima Docker and the `dbtest` build tag.
`OPTIMUS_JWT_SECRET` must be at least 32 bytes or the server refuses to start.

Frontend commands run from `optimus-fe/`:

- `bun install`
- `bun run dev`
- `bun run lint`
- `bun run typecheck`
- `bun run i18n:check`
- `bun run test`
- `bun run test:watch`
- `bun run build`

Use `bun` only for frontend dependency work. Do not use npm, pnpm, or yarn.
Run a focused frontend test with
`bun x vitest run path/to/file.test.ts -t "name pattern"`.

### Dev / Production Compose

Run the local production-shaped stack from `deploy/`:

```bash
docker compose up -d --build
docker compose ps -a
```

For host-side hot reload, start only PostgreSQL with
`docker compose up -d postgres`, then run `make run` in `optimus-be/` and `bun run dev` in
`optimus-fe/`. PostgreSQL is exposed only on `127.0.0.1:5432`; no Adminer
service is maintained.

For Production, copy `.env.example` to the untracked `.env`, replace every
development credential, and set the real Compose project name, image
repository, immutable `main-<short-sha>` version, HTTPS origin, secrets, and
capacity/retention values. Then run:

```bash
docker compose pull
docker compose up -d --no-build
docker compose logs seed | grep INITIAL
```

Expected steady state: PostgreSQL, backend, and frontend are healthy; migrate
and seed exit 0. The unauthenticated health endpoint returns the raw probe
`{"db":"ok","version":"<sha>"}`. Capture the first-run administrator password
from the seed log; it is printed only once.

Image builds and dual GHCR/Docker Hub publishing run only for `main` pushes;
`dev` pushes and pull requests run quality jobs without publishing. Published
release images use immutable `main-<short-sha>` tags.

## Local Runtime Policy

- Use Colima for project-local Docker and Kubernetes on both macOS and Linux.
- Start the default profile with
  `colima start --runtime docker --kubernetes`, then select the `colima` Docker
  and kube contexts.
- Verify readiness with `colima status`, `docker info`, and
  `kubectl cluster-info` before Docker-backed integration or smoke tests.
- Run Docker Compose, dockertest, `make test-int`, and P5 containers on Colima's
  Docker runtime. Do not silently use Docker Desktop or a host system Docker
  daemon instead.
- Run the full local stack from `deploy/` with `docker compose up -d --build`.
  Start only `postgres` when running the backend/frontend directly on the host.
  Connect with `psql` through the loopback-only PostgreSQL port; no Adminer
  service is maintained.
- Invoke Docker Compose exclusively through the Docker CLI as
  `docker compose ...`. The Homebrew Compose plugin is exposed through
  `~/.docker/cli-plugins/docker-compose`; verify it with
  `docker compose version` before local development or smoke tests.
- For tools that ignore Docker contexts, use the socket reported by
  `colima status` through `DOCKER_HOST`; never hardcode a macOS `/Users/...`
  path or assume `/var/run/docker.sock`.
- Use Colima's built-in Kubernetes for routine P2/P3 local development and P6
  release smoke. P6 must use the `colima` kube context and isolate resources in
  its disposable namespace; it must not stop or delete the shared Colima
  cluster during teardown.
- Linux/WSL2 without `/dev/kvm` may use slower QEMU software virtualization.
  This local-runtime policy does not apply to CI runners.
- Keep backend `TMPDIR`, `GOCACHE`, and `GOLANGCI_LINT_CACHE` under the ignored
  `optimus-be/tmp/` tree. The backend Makefile owns these defaults. Do not place
  backend build caches under `/tmp`, which may be a size-limited tmpfs.

## Non-Negotiable Invariants

- Keep `optimus-be/go.mod` at `go 1.25`.
- Keep `k8s.io/client-go` and `k8s.io/apimachinery` pinned to `v0.30.14`.
- Keep `helm.sh/helm/v3` pinned to `v3.15.4` unless the compatibility story is
  reopened deliberately.
- Add permissions only in `optimus-be/internal/infra/permissions/codes.go`, then
  run `make dump-perms` and `make perm-check`.
- Regenerate Swagger with `make swag` after handler annotation or API contract
  changes, then run `make swagger-diff`.
- Keep client-facing backend errors inside the envelope and use `apperr.New` or
  `apperr.Wrap`; do not leak raw error text to clients.
- Preserve frontend i18n parity between `zh-CN` and `en-US`.
- Keep code comments in English.

## Backend Architecture

- Module layering is `dto.go` -> `repo.go` -> `service.go` -> `handler.go`.
  Handlers bind and validate input, services own business logic and audit/cache
  effects, repositories own GORM access, and handlers always return the fixed
  `{code,data,message,message_key?}` envelope.
- `cmd/server/main.go` is the only composition root. It loads configuration,
  registers all in-code permissions, creates the shared RBAC cache and audit
  recorder, wires credential consumers, and mounts routes.
- Every mutating service path records through that shared audit recorder; do
  not construct a second recorder.
- Mount protected routes through nested `Group("", middleware)` groups. Passing
  permission middleware as variadic `GET`/`POST` arguments is not equivalent
  when handlers are registered separately.
- Permission resolution joins permissions, roles, and users while excluding
  soft-deleted users and roles. Services that change roles, user roles, or role
  permissions must invalidate the appropriate shared permission cache.
- Access tokens expire after 15 minutes and refresh tokens after 168 hours.
  Refresh tokens are persisted and rotated; replay is rejected. Login is
  rate-limited per IP.
- Goose migrations live in `optimus-be/migrations/` and are embedded. Container
  and local migration commands use the same files. Models live in
  `internal/models/`; database integration tests use dockertest and `dbtest`.

## Frontend Architecture

- Bootstrap order is Pinia, Ant Design Vue, i18n, API client, provided module
  APIs, router guards, then mount.
- Static routes contain login/error/profile pages. On the first authenticated
  navigation, fetch `/me`, menus, and permissions in parallel, register dynamic
  routes, then replace-navigate to the original destination.
- Permission enforcement has two synchronized layers: route
  `meta.permission` and the `v-permission` directive. Both read the Pinia auth
  permission state; components must not re-fetch permissions.
- The API client validates the fixed envelope and converts nonzero codes to
  `BizError`. Concurrent HTTP and SSE 401 responses share the store's
  single-flight refresh promise; each original request is replayed at most once.
- Locale files are `src/locales/zh-CN.json` and `src/locales/en-US.json`.
  `bun run i18n:check` enforces parity. The Vite alias `@/*` maps to `src/*` and
  the local `/api/v1` proxy targets `http://localhost:8080`.

## Credentials and Kubernetes Architecture

- The P1 vault uses AES-256-GCM. Load `OPTIMUS_VAULT_MASTER_KEY` or
  `OPTIMUS_VAULT_MASTER_KEY_FILE` before opening the database so a missing key
  fails fast.
- `credentials.Consumer` is the sole downstream Go API for credentials. Never
  bypass it with HTTP or a second cipher. Use `credentials.WithActor` for
  non-HTTP actors. Kubeconfigs reject `exec` and `auth-provider` plugins.
- P2 Kubernetes endpoints are read-only: do not add exec, apply, write verbs,
  or watch without reopening the P2 design. Build and discard a fresh client
  per request. Normalize API-server failures through `k8s/apierr`.
- The secret `/data` reveal endpoint is the only path that returns plaintext
  Kubernetes secret values and remains gated by `k8s:secret:reveal`.
- Pod logs use SSE and `http.ResponseController(c.Writer).Flush()`. The frontend
  consumes the stream with `fetch` and `ReadableStream`, not `EventSource`, so
  the JWT remains in the Authorization header.
- The selected cluster ID belongs to the Kubernetes Pinia store and is chosen
  through `ClusterPicker`. Pages without a selection show a prompt instead of
  redirecting or choosing automatically.

## Generated Artifacts and Dependency Pins

- `make swag` updates both `optimus-be/api/docs/swagger.json` and
  `docs/api/swagger.json`; run `make swagger-diff` after regeneration.
- Permission codes originate only in
  `optimus-be/internal/infra/permissions/codes.go`, are registered into the DB
  at startup, gate backend routes/frontend controls, and generate
  `docs/permissions.md` through `make dump-perms`.
- The AWS SDK Go v2 modules and `github.com/robfig/cron/v3` must remain versions
  compatible with Go 1.25. Pin an offending transitive module instead of
  raising the Go directive.
- CORS environment values are comma-separated, not JSON arrays, for example
  `OPTIMUS_CORS_ALLOWED_ORIGINS=https://a.example.com,https://b.example.com`.

## P4 Assets Rules

For P4, follow `docs/superpowers/plans/2026-06-11-p4-assets.md` task by task.
The most important rules are:

- `credentials.Consumer` is the only path for cloud keys.
- Fetch cloud keys with a purpose like `assets.sync.<reason>` and wipe them
  after use.
- Build AWS clients per sweep/request; do not cache SDK clients.
- Only authoritative successful full sweeps may soft-delete missing resources.
- VPC and subnet sweeps are one transaction and one `network` sync-run unit.
- Manual sync is asynchronous: handler returns immediately and the worker owns
  the account lock.
- Cron writes `assets_sync_runs`; it does not write audit rows.
- Cloud-key delete must remain nil-safe when the P4 assets in-use counter is not
  wired. When wired, deleting a referenced cloud key fails with code `43001`.
- Removing regions from a cloud account must explicitly soft-delete resources in
  those removed regions.
- P4 frontend paths must be lowercase/kebab-case to match Linux production.
- `assets.Consumer` is the non-HTTP lookup seam for downstream phases and
  returns `ErrAssetsInstanceNotFound` for absent matches.
- P4 runtime configuration uses `OPTIMUS_ASSETS_SYNC_CRON`,
  `OPTIMUS_ASSETS_SYNC_STARTUP_DELAY`,
  `OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS`, and
  `OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT`.

P4's manual release checklist is `optimus-be/scripts/p4-smoke.md`. Use it
against a disposable read-only AWS credential before production sign-off; do
not add AWS write/manage APIs in P4.

## P5 Observability Rules

- P5 is metrics display only: no alerts, rules, notifications, CloudWatch,
  metric sample storage, logs, traces, or APM.
- Consume P1 HTTP credentials only through `credentials.Consumer`; never
  expose or audit secrets, authorization headers, custom CA PEM, or full
  PromQL.
- Private Prometheus targets require a narrow CIDR. Metadata and mixed DNS
  answers stay denied even under broad ranges. Never follow redirects or
  cache clients.
- Keep query count, concurrency, PromQL bytes, range, step, points, series,
  response, timeout, and enrichment limits enforced.
- Metric-only operators use the minimal query-source endpoint; built-ins must
  not call the administrative data-source list.
- Data-source, dashboard, and metric permissions remain independent; every
  API call and concrete UI control uses its exact gate.
- Preserve abort generations so stale definition/query responses cannot
  commit.
- Run `optimus-be/scripts/p5-smoke.md` with disposable local Prometheus; no
  production credential or Kubernetes cluster is needed.

## P6 Application Delivery Rules

- P6 promotes immutable chart artifacts through ordered environments bound to
  existing P3 applications; it does not accept arbitrary commands, scripts,
  manifests, values, container images, or credentials.
- Direct P3 upgrade/uninstall of a delivery-managed application stays denied;
  only the closed in-process delivery capability may perform an upgrade.
- Resolve and persist the chart digest before run creation. Every stage must
  use the frozen repository, chart name, version, digest, application, cluster,
  namespace, release name, executor, approval policy, and timeout.
- Initiators cannot approve their own run. Project, pipeline, run, and approval
  permissions remain independent and every UI control uses its exact gate.
- Workers use database leases and stable operation IDs. Ambiguous outcomes go
  through reconciliation; never guess success or blindly replay Helm.
- System kubeconfig consumption must use a `system:` purpose. The production
  Helm loader must preserve `LoadVerifiedChart` digest verification.
- SSE and audit projections must never expose values, kubeconfigs, auth
  headers, manifests, Helm notes, or raw executor errors.
- Run `optimus-be/scripts/p6-smoke.md` only against disposable PostgreSQL, an
  isolated namespace in Colima Kubernetes, and disposable chart-repository
  resources.

## Codex Environment Notes

Project-local Codex configuration lives in `.codex/config.toml` and contains
the hosted mem0 and Context7 Streamable HTTP MCP servers plus the local Serena
STDIO MCP server:

```toml
[mcp_servers.mem0]
url = "https://mcp.mem0.ai/mcp"
bearer_token_env_var = "MEM0_API_KEY"
startup_timeout_sec = 15
tool_timeout_sec = 60
default_tools_approval_mode = "auto"

[mcp_servers.context7]
url = "https://mcp.context7.com/mcp"
env_http_headers = { "CONTEXT7_API_KEY" = "CONTEXT7_API_KEY" }

[mcp_servers.serena]
startup_timeout_sec = 15
command = "serena"
args = ["start-mcp-server", "--project-from-cwd", "--context=codex"]
```

Export `MEM0_API_KEY` and `CONTEXT7_API_KEY` before starting Codex. Context7's
`env_http_headers` mapping keeps its key out of the tracked TOML file. The
hosted mem0 connection does not have a Codex configuration field for a default
memory user. Unless the user explicitly requests another scope, pass
`user_id = "logic"` to mem0 tools; for tools that accept structured filters,
include `{"user_id": "logic"}` in the filter. This convention applies to reads
and writes so project memories remain in the same scope.

Serena must be available as `serena` on `PATH`. Its `--context=codex` mode
provides symbol-aware project navigation while avoiding unnecessary overlap
with Codex built-ins. Serena-generated project metadata and caches live under
`.serena/` and are ignored by Git.

## Local Gotchas

- Container health checks use GET; keep `/api/v1/health` registered for GET.
- The initial administrator password is printed exactly once by seed. If it is
  lost, reset it through the database.
- Generate the vault key with `go run ./cmd/vault-keygen`, store it securely,
  and never rotate it casually; an absent or incorrect key prevents startup or
  credential decryption.
- A PostgreSQL named volume retains its original database password even if
  `.env` changes. Never use `docker compose down -v` as a password-rotation
  technique on persistent environments because it erases all data.
- Use multi-character namespaces in YAML round-trip tests; a one-character
  namespace can be decoded unexpectedly by `sigs.k8s.io/yaml`.

## First-Session Checklist

On a new macOS or Linux machine:

1. Install `uv`, `bun`, `colima`, Docker CLI with Compose, `kubectl`, `helm`,
   `git`, and `serena`.
2. Export `MEM0_API_KEY` and `CONTEXT7_API_KEY` before starting Codex.
3. Start Colima with Docker and Kubernetes, select both `colima` contexts, and
   verify Docker, Compose, and Kubernetes connectivity.
4. Read this `AGENTS.md`, retrieve the latest Optimus mem0 checkpoint under
   `user_id = "logic"`, and cross-check it with local and remote Git state.
5. After a milestone, update the single current mem0 checkpoint and this file
   together so durable guidance and current status do not drift.
