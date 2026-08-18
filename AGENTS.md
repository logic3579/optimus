# Optimus Codex Guide

This file is the Codex entry point for the Optimus repository. Treat
`CLAUDE.md` as the detailed historical guide and this file as the concise
operating contract for new Codex sessions.

## First Read

Start every substantial task by reading:

1. `CLAUDE.md`
2. The active phase's design and implementation plan under
   `docs/superpowers/specs/` and `docs/superpowers/plans/`.
3. For P4 maintenance, `docs/superpowers/plans/2026-06-11-p4-assets.md` and
   `docs/superpowers/specs/2026-06-11-p4-assets-design.md`.

If mem0 is available, search with `user_id = "logic"` for the latest
`[CHECKPOINT YYYY-MM-DD]` memory for `optimus` and cross-check it against
`git status --short --branch` and recent
`git log --oneline --decorate --max-count=20 --all`.

Current expected project state (2026-08-18): P0-P6 are implemented and merged
to `main`. P6 Application Delivery completed the approved 2026-07-27 design
and all 29 plan tasks, including immutable Helm promotion runs, approvals, SSE
events, restart/reconciliation recovery, frontend delivery pages, generated
artifacts, and a real disposable Kubernetes/Helm smoke. PR #5 fixed CI
reliability and frontend build issues; PR #6 refined authentication feedback,
built-in RBAC roles, menu metadata, dynamic-route bootstrap, and related tests.

PR #7 merged the deployment preparation to `main` on 2026-08-18. `main` and
`origin/main` are at `f364af7`; `origin/dev` remains at PR #7's second parent
`f555d72`, and local `dev` adds only this progress-documentation refresh. There
was no source diff between the merged branch tip and `main`. The main-push
pipeline completed and published immutable
`main-f364af7` `optimus-be` and `optimus-fe` images to both GHCR and Docker Hub;
the corresponding cross-registry digests match. Deployment uses one
Dev/UAT/Prod Compose file, one environment example with local Dev defaults,
and one backend image containing all operational binaries. No release tag
exists yet.

Local pre-release validation steps 1-11 have passed. This includes the local
UI/backend path, Colima Kubernetes cluster connection, backend lint and
generated-artifact gates, the complete P3 application lifecycle smoke, and the
complete P4 AWS assets smoke with a disposable read-only credential. P5 and P6
remain pending, not passed or waived. The immediate next task is to start the
default full-stack Dev environment through `deploy/docker-compose.yml` on
Colima and complete preliminary P5 and P6 acceptance locally: P5 uses
disposable Prometheus fixtures on Colima Docker, while P6 uses disposable
PostgreSQL/chart resources and an isolated namespace in Colima Kubernetes.
After local acceptance, deploy UAT, run the production-like persistent-data
upgrade smoke from `4e2d08b` through `00023_p6_delivery.sql`, complete
environment-specific acceptance, deploy Production, and only then tag/release.

## Project Shape

- Backend: `optimus-be/`, Go 1.25, Gin, GORM, Postgres.
- Frontend: `optimus-fe/`, Vue 3, Ant Design Vue, Pinia, vue-router, vue-i18n.
- Deployment: `deploy/docker-compose.yml` is the single Dev/UAT/Prod Compose
  entry point; `deploy/.env.example` contains local Dev defaults.
- Specs/plans: `docs/superpowers/specs/` and `docs/superpowers/plans/`.
- Generated artifacts: `docs/api/swagger.json` and `docs/permissions.md`.

## Commands

Backend commands run from `optimus-be/`:

- `make test`
- `make test-int`
- `make lint`
- `make swag`
- `make swagger-diff`
- `make dump-perms`
- `make perm-check`

Frontend commands run from `optimus-fe/`:

- `bun install`
- `bun run lint`
- `bun run typecheck`
- `bun run i18n:check`
- `bun run test`
- `bun run build`

Use `bun` only for frontend dependency work. Do not use npm, pnpm, or yarn.

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
  wired.
- Removing regions from a cloud account must explicitly soft-delete resources in
  those removed regions.
- P4 frontend paths must be lowercase/kebab-case to match Linux production.

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

Long-term project guidance also lives in the private Codex skill
`optimus-project` under `~/.codex/skills/optimus-project`.
