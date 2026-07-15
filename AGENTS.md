# Optimus Codex Guide

This file is the Codex entry point for the Optimus repository. Treat
`CLAUDE.md` as the detailed historical guide and this file as the concise
operating contract for new Codex sessions.

## First Read

Start every substantial task by reading:

1. `CLAUDE.md`
2. `docs/superpowers/plans/2026-06-11-p4-assets.md`
3. `docs/superpowers/specs/2026-06-11-p4-assets-design.md`

If mem0 is available, search with `user_id = "logic"` for the latest
`[CHECKPOINT YYYY-MM-DD]` memory for `optimus` and cross-check it against
`git status --short --branch` and recent
`git log --oneline --decorate --max-count=20 --all`.

Current expected project state: P0-P3 are implemented; `dev` contains the P4
assets design and 25-task implementation plan, but P4 code has not been landed.
The next implementation entry point is P4 Task 1.

## Project Shape

- Backend: `optimus-be/`, Go 1.25, Gin, GORM, Postgres.
- Frontend: `optimus-fe/`, Vue 3, Ant Design Vue, Pinia, vue-router, vue-i18n.
- Deployment: `deploy/` plus root `docker-compose.yml`.
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
