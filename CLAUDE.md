# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository shape

Monorepo with two deployable apps plus shared deployment assets:

- `optimus-be/` — Go 1.25 / Gin / GORM / Postgres backend. P0 scope: auth, RBAC, users, roles, permissions, menus, audit, /me, /health.
- `optimus-fe/` — Vue 3 + Ant Design Vue + Pinia + vue-router SPA. Talks only to `/api/v1/*`.
- `deploy/` — production `docker-compose.prod.yml` + multi-stage Dockerfiles (`be.Dockerfile` builds `server` / `migrate` / `seed` targets; `fe.Dockerfile` builds the nginx-served SPA).
- `docs/superpowers/specs/` and `docs/superpowers/plans/` — the authoritative P0 design spec and execution plans. Permission/API contracts come from here.
- `docs/api/swagger.json` and `docs/permissions.md` — **generated artifacts**, checked in. CI (`make swagger-diff` / `make perm-check`) fails if they drift from source.
- `docker-compose.yml` (repo root) — local Postgres + Adminer only. Production stack lives in `deploy/`.

## Daily commands

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

`OPTIMUS_JWT_SECRET` (≥32 bytes) must be exported or the server refuses to start. Default DSN in `configs/config.yaml` matches the dev `docker-compose.yml` Postgres.

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

### Production deploy (run from repo root)

```bash
cp deploy/.env.example deploy/.env   # fill REQUIRED block, generate JWT with `openssl rand -base64 48`
docker compose -f deploy/docker-compose.prod.yml up -d --build
docker logs optimus-seed | grep INITIAL    # initial admin password (logged only on first run)
```

Expected steady state: `optimus-pg` healthy, `optimus-migrate` Exited(0), `optimus-seed` Exited(0), `optimus-be` healthy, `optimus-fe` healthy.

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

**API envelope handling** (`src/api/client.ts`): every response is checked against the envelope shape; non-zero `code` → throws a `BizError` so callers `.catch`. On HTTP 401 (not for `/auth/refresh` itself), a **single-flight** refresh kicks off (`refreshing: Promise<TokenPair> | null`) — concurrent 401s share one refresh call, then replay their original request once via `__retried`. If the refresh call itself 401s, `onLogout()` fires there and the retry path skips it to avoid double-logout.

**i18n**: keys in `src/locales/{zh-CN,en-US}.json`. `bun run i18n:check` enforces missing-key + cross-locale parity and is wired into CI; adding a key to one locale without the other breaks the build.

**Vite alias**: `@/*` → `src/*`. dev proxy: `/api/v1` → `http://localhost:8080` (set in `vite.config.ts`).

## Conventions worth knowing

- **bun everywhere on the frontend** — never npm/pnpm/yarn. CI uses `bun install --frozen-lockfile`.
- **Permission codes** live in one place (`internal/infra/permissions/codes.go`) and propagate to: the DB via `Register()` at startup, the `RequirePermission` middleware, the FE permission list via `/me/permissions`, and `docs/permissions.md`. Touch any of them and run `make dump-perms` + `make perm-check`.
- **Swagger annotations** are checked by `make swagger-diff` in CI. Add/modify a handler's `@Summary/@Param/@Success` and regenerate.
- **CORS env var is comma-separated, not JSON**: `OPTIMUS_CORS_ALLOWED_ORIGINS=https://a.example.com,https://b.example.com` (no brackets/quotes). The YAML config takes a list, but env override is comma-split. This bit Plan 3.
- **No raw error text to clients** — wrap in `apperr.BizError(code, ...)`. `response.Error` logs unhandled errors with `slog.Error` and returns generic `CodeInternal`.
- **Audit logging**: every mutating service path calls `audit.Recorder.Record(...)`. The recorder is shared so `/me` writes and admin `/users` writes hit the same sink — don't construct a second recorder.
- **CLAUDE Code skills/superpowers** are configured at `~/.claude/` and `.claude/`; the `.claude/settings.json` here only adjusts permissions/hooks for this repo.

## Gotchas (local-only)

- **Docker daemon is Colima on this workstation.** If `docker compose` / dockertest can't find a daemon, export `DOCKER_HOST=unix:///Users/<you>/.colima/docker.sock` or `colima start`. The `make test-int` and `tests/integration/` paths both depend on this.
- **HEAD vs GET on healthcheck**: the container healthchecks use `wget` which issues GET. Gin only registers GET handlers by default — keep `/api/v1/health` on GET, not HEAD-aliased.
- **Initial admin password is logged exactly once**, on the first run of `cmd/seed` (or first `make run` against an empty DB). Capture it from stdout / `docker logs optimus-seed | grep INITIAL`. Subsequent runs print "admin user already exists; no password generated." If you lose it, you must reset via DB.

## First-session checklist (new machine)

If this is the first time touching this repo on this Mac:

1. **Tools**: `brew install uv bun colima git` then `colima start`.
2. **mem0 API key**: `export MEM0_API_KEY="..."` (from password manager). Persist in `~/.zshrc`. The `.mcp.json` in this repo auto-loads the mem0 MCP server in Claude Code.
3. **First prompt** in Claude Code:
   > "Read CLAUDE.md, search mem0 for the optimus checkpoint, then `git log dev..main` to confirm what's actually merged. Summarize where the project is and recommend the next step."

   Claude should pull the latest `[CHECKPOINT YYYY-MM-DD]` memory from mem0 (single source of "current status"), cross-check against git, and propose the next plan task. All the durable conventions / gotchas / patterns (FE zero-wrapper, axios single-flight, v-permission, Pinia layering, TDD layering, nullability contract, vue-i18n drill trap, Colima socket path, etc.) live as separate mem0 atoms and surface on semantic search — no need to re-derive.
4. **Refreshing the checkpoint**: after finishing a plan or hitting a milestone, tell Claude "刷快照 / refresh the checkpoint" — it will delete the old `metadata.kind=checkpoint` atom and write a new one for today.
