# P0 Plan 3 — Deployment Design

**Status**: Spec
**Date**: 2026-06-09
**Owner**: P0 sub-project
**Depends on**: P0 platform-skeleton (merged 2026-06-09, commit ~`048fedf` + merge to `main`)
**Supersedes**: §9.6 / §9.7 / §9.8(partial) of `2026-06-05-p0-platform-skeleton-design.md` (Dockerfile / prod compose / docker-build CI)

---

## 1. Goal and scope

P0 platform-skeleton has shipped: BE (Go 1.25 + Gin + GORM + Postgres) and FE (Vue3 + AntdV) are functional with 25 REST endpoints, 8 DB tables, 5 system admin pages, and a CI pipeline. What is missing for a deployable artifact:

1. Production-shape Docker images for BE and FE
2. An nginx reverse proxy that serves the SPA and proxies `/api/v1/` to BE
3. A single-machine `docker-compose.prod.yml` that brings up Postgres + migration + BE + FE in one command
4. Schema migrations run automatically as an init step before BE starts
5. FE main bundle split from 1.6 MB → ≤ 250 KB via Vite `manualChunks`
6. A lightweight CI job that builds the Dockerfiles on every PR so they cannot rot

**Out of scope** (deferred to later sub-projects):

- Image registry push (P2+)
- Kubernetes manifests (P2 dogfood)
- TLS termination (handled by external LB / cloudflare in real prod)
- Content-Security-Policy header (P1+)
- Multi-instance scaling, load balancing, blue/green (P5+)
- Sentry / OpenTelemetry / APM (P5)

This spec covers exactly the artifacts needed for a single-machine demo: `cd deploy && docker compose up -d --build` lands a working stack.

---

## 2. Architecture

```
                    ┌──────────────────┐
                    │   postgres:16     │  named volume: pgdata
                    │   (healthcheck)   │  not exposed externally
                    └────────┬──────────┘
                             │ service_healthy
                             ▼
                    ┌──────────────────┐
                    │  optimus-migrate  │  goose up, exit 0
                    │  (init, restart:no)│
                    └────────┬──────────┘
                             │ service_completed_successfully
                             ▼
                    ┌──────────────────┐
                    │   optimus-seed    │  perms.Register + seed.Run, exit 0
                    │  (init, restart:no)│  creates admin on first run; idempotent
                    └────────┬──────────┘
                             │ service_completed_successfully
                             ▼
                    ┌──────────────────┐
                    │   optimus-be      │  Go 1.25, /health probe
                    │   :8080 (internal)│  OPTIMUS_* env overrides
                    └────────┬──────────┘
                             │ service_healthy
                             ▼
                    ┌──────────────────┐
                    │   optimus-fe      │  nginx:1.27-alpine
                    │   :80 → host:80   │  SPA + /api/v1/ → optimus-be:8080
                    └──────────────────┘
```

Five compose services. Service dependency chain enforces order: postgres → migrate → seed → be → fe. Only fe binds a host port (`${HTTP_PORT:-80}`); everything else is internal-network.

**Why a separate `optimus-seed` service**: `cmd/seed` registers permission codes and creates the bootstrap admin user (printing the initial password once to logs). `cmd/server` calls `permissions.Register` itself, but does NOT create the admin user. Without a seed step in prod, no one can log in. `cmd/seed` is already idempotent (checks `existing user` before creating), so re-runs on subsequent `up` cycles are safe no-ops.

**Files added in this plan**:

```
deploy/
├── be.Dockerfile                # multi-stage; targets: server, migrate, seed
├── fe.Dockerfile                # multi-stage; bun build → nginx
├── nginx.conf                   # SPA fallback + reverse proxy + gzip + cache + security headers
├── docker-compose.prod.yml      # 5 services + named volume
└── .env.example                 # documents required + optional env vars
optimus-be/cmd/migrate/
└── main.go                      # programmatic goose with embed.FS
.dockerignore                    # at repo root
```

**Files modified**:

```
optimus-fe/vite.config.ts        # add build.rollupOptions.output.manualChunks
optimus-be/internal/infra/config/config.go  # add ValidateForMigrate()
.github/workflows/ci.yml         # append docker-build job
README.md                        # add "Production deploy" section
```

---

## 3. Backend: cmd/migrate + Dockerfile

### 3.1 `optimus-be/cmd/migrate/main.go` (new)

A small Go binary that runs goose programmatically against the same Postgres instance the BE will use. Migrations are embedded into the binary via `embed.FS` so the image doesn't need a separate `migrations/` COPY.

```go
package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/log"
)

//go:embed all:migrations
var migrationsFS embed.FS

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	direction := flag.String("dir", "up", "up | down | status")
	flag.Parse()

	abs, err := filepath.Abs(*cfgPath)
	if err != nil {
		die("resolve config path", err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		die("load config", err)
	}
	if err := cfg.ValidateForMigrate(); err != nil {
		die("validate config", err)
	}

	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})
	logger.Info("optimus-migrate starting", "direction", *direction)

	db, err := sql.Open("pgx", cfg.Database.DSN)
	if err != nil {
		die("open db", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		die("set dialect", err)
	}

	switch *direction {
	case "up":
		err = goose.Up(db, "migrations")
	case "down":
		err = goose.Down(db, "migrations")
	case "status":
		err = goose.Status(db, "migrations")
	default:
		die("unknown direction: "+*direction, nil)
	}
	if err != nil {
		die("migrate "+*direction, err)
	}
	logger.Info("optimus-migrate done")
}

func die(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", msg)
	}
	os.Exit(1)
}
```

**Notes**:

- The directive is `//go:embed all:migrations`; the `all:` prefix is required for goose to discover SQL files in subpaths if the layout ever nests (current layout is flat, but `all:` is harmless and forward-safe).
- `pgx/v5/stdlib` is the SQL driver. `go.sum` already has `pgx/v5` via GORM Postgres dependency; verify during implementation and add `require github.com/jackc/pgx/v5` to `go.mod` explicitly if needed.
- Uses the same `config.Load()` as the server so `OPTIMUS_*` env override behaves identically.

### 3.2 `optimus-be/internal/infra/config/config.go` — add `ValidateForMigrate`

```go
// ValidateForMigrate enforces only DB connectivity. JWT secret is irrelevant
// to schema migrations and forcing it would require operators to set a dummy
// value in the migrate service env.
func (c *Config) ValidateForMigrate() error {
	if c.Database.DSN == "" {
		return errors.New("database.dsn is required")
	}
	return nil
}
```

`ValidateStrict` is unchanged; `cmd/server/main.go` is unchanged; only `cmd/migrate/main.go` calls the new method.

### 3.3 `deploy/be.Dockerfile` (new)

One Dockerfile, four build stages (`build`, `server`, `migrate`, `seed`), reusing the build cache for all three binaries.

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY optimus-be/go.mod optimus-be/go.sum ./
RUN go mod download
COPY optimus-be/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/optimus-be ./cmd/server
RUN CGO_ENABLED=0 go build -ldflags "-s -w" \
    -o /out/optimus-migrate ./cmd/migrate
RUN CGO_ENABLED=0 go build -ldflags "-s -w" \
    -o /out/optimus-seed ./cmd/seed

# ---- runtime: server ----
FROM alpine:3.20 AS server
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/optimus-be /usr/local/bin/optimus-be
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/optimus-be"]
CMD ["-config", "/etc/optimus/config.yaml"]

# ---- runtime: migrate ----
FROM alpine:3.20 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=build /out/optimus-migrate /usr/local/bin/optimus-migrate
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
ENTRYPOINT ["/usr/local/bin/optimus-migrate"]
CMD ["-config", "/etc/optimus/config.yaml", "-dir", "up"]

# ---- runtime: seed ----
FROM alpine:3.20 AS seed
RUN apk add --no-cache ca-certificates
COPY --from=build /out/optimus-seed /usr/local/bin/optimus-seed
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
ENTRYPOINT ["/usr/local/bin/optimus-seed"]
CMD ["-config", "/etc/optimus/config.yaml"]
```

**Notes**:

- Build context is **repo root** (`..` from `deploy/`), so all `COPY` paths are prefixed with `optimus-be/`.
- Three compose services build from the same Dockerfile with different `target:` values — buildx will cache the shared `build` stage between them.
- `wget` in the server stage is for compose healthcheck (`wget -q --spider`); migrate and seed stages don't need it.
- `-s -w` strips debug info; `-X main.Version` injects the version string (`make build` previously didn't do this — now wired through `ARG VERSION`).
- The seed binary calls `config.ValidateStrict()` (existing behavior, unchanged), so the seed service MUST receive `OPTIMUS_JWT_SECRET` in env even though it doesn't use it for token signing. Validation symmetry with the server makes operational mistakes (typos in JWT secret) fail at seed time rather than at first login.

---

## 4. Frontend: vite.config + Dockerfile + nginx

### 4.1 `optimus-fe/vite.config.ts` — add `manualChunks`

```ts
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') }
  },
  server: {
    port: 5173,
    proxy: { '/api/v1': { target: 'http://localhost:8080', changeOrigin: false } }
  },
  build: {
    target: 'es2020',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia', 'pinia-plugin-persistedstate', 'axios'],
          antd:   ['ant-design-vue'],
          icons:  ['@ant-design/icons-vue'],
          utils:  ['dayjs', 'vue-i18n']
        }
      }
    }
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: []
  }
})
```

**Bundle acceptance (uncompressed, after `bun run build`)**:

| chunk | upper bound | rationale |
|---|---|---|
| `index-*.js` | < 250 KB | app code only; route splits already small |
| `vendor-*.js` | < 250 KB | vue + router + pinia + axios |
| `antd-*.js` | < 1.5 MB | ant-design-vue (without icons); gzip ~410 KB |
| `icons-*.js` | < 150 KB | @ant-design/icons-vue |
| `utils-*.js` | < 150 KB | dayjs + vue-i18n |

If implementation finds antd > 900 KB or vendor > 250 KB, revisit splits (likely move `@ant-design/icons-vue` into its own chunk) and update spec.

### 4.2 `deploy/fe.Dockerfile` (new)

```dockerfile
FROM oven/bun:1 AS build
WORKDIR /src
COPY optimus-fe/package.json optimus-fe/bun.lock ./
RUN bun install --frozen-lockfile
COPY optimus-fe/ ./
RUN bun run build

FROM nginx:1.27-alpine
RUN apk add --no-cache wget
COPY --from=build /src/dist /usr/share/nginx/html
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

**Notes**:

- `bun.lock` (text), not `bun.lockb` (which is what the old spec wrote).
- `bun run build` already runs `vue-tsc --noEmit && vite build` per `optimus-fe/package.json`.
- Build context is repo root; `COPY optimus-fe/` prefix.
- `wget` for healthcheck.

### 4.3 `deploy/nginx.conf` (new)

```nginx
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # ---- gzip ----
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_comp_level 6;
    gzip_types
        text/plain text/css text/javascript
        application/javascript application/json application/xml
        image/svg+xml;

    # ---- security headers (apply to everything) ----
    # NOTE: nginx add_header is replaced (not inherited) inside nested location
    # blocks that declare their own add_header. We repeat in /assets/ and
    # /index.html below.
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # ---- API reverse proxy ----
    location /api/v1/ {
        proxy_pass http://optimus-be:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }

    # ---- swagger passthrough (P0 internal tool; keep open) ----
    location /swagger/ {
        proxy_pass http://optimus-be:8080;
        proxy_set_header Host $host;
    }

    # ---- hashed static assets: 1-year immutable ----
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, max-age=31536000, immutable" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        try_files $uri =404;
    }

    # ---- SPA shell: never cache ----
    location = /index.html {
        add_header Cache-Control "no-cache, no-store, must-revalidate" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    }

    # ---- SPA fallback ----
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

---

## 5. Compose + env

### 5.1 `deploy/docker-compose.prod.yml` (new)

```yaml
name: optimus-prod

services:
  postgres:
    image: postgres:16-alpine
    container_name: optimus-pg
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-optimus}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}
      POSTGRES_DB: ${POSTGRES_DB:-optimus}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-optimus} -d ${POSTGRES_DB:-optimus}"]
      interval: 5s
      timeout: 5s
      retries: 10

  migrate:
    build:
      context: ..
      dockerfile: deploy/be.Dockerfile
      target: migrate
      args:
        VERSION: ${VERSION:-dev}
    image: optimus-migrate:${VERSION:-dev}
    container_name: optimus-migrate
    restart: "no"
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      OPTIMUS_DATABASE_DSN: ${OPTIMUS_DATABASE_DSN:?OPTIMUS_DATABASE_DSN is required}

  seed:
    build:
      context: ..
      dockerfile: deploy/be.Dockerfile
      target: seed
      args:
        VERSION: ${VERSION:-dev}
    image: optimus-seed:${VERSION:-dev}
    container_name: optimus-seed
    restart: "no"
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
    environment:
      OPTIMUS_DATABASE_DSN: ${OPTIMUS_DATABASE_DSN:?OPTIMUS_DATABASE_DSN is required}
      OPTIMUS_JWT_SECRET: ${OPTIMUS_JWT_SECRET:?OPTIMUS_JWT_SECRET is required (>= 32 bytes)}

  optimus-be:
    build:
      context: ..
      dockerfile: deploy/be.Dockerfile
      target: server
      args:
        VERSION: ${VERSION:-dev}
    image: optimus-be:${VERSION:-dev}
    container_name: optimus-be
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
      seed:
        condition: service_completed_successfully
    environment:
      OPTIMUS_DATABASE_DSN: ${OPTIMUS_DATABASE_DSN:?OPTIMUS_DATABASE_DSN is required}
      OPTIMUS_JWT_SECRET: ${OPTIMUS_JWT_SECRET:?OPTIMUS_JWT_SECRET is required (>= 32 bytes)}
      OPTIMUS_CORS_ALLOWED_ORIGINS: ${OPTIMUS_CORS_ALLOWED_ORIGINS:-["http://localhost"]}
      OPTIMUS_LOG_LEVEL: ${OPTIMUS_LOG_LEVEL:-info}
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 3s
      retries: 5
      start_period: 10s

  optimus-fe:
    build:
      context: ..
      dockerfile: deploy/fe.Dockerfile
    image: optimus-fe:${VERSION:-dev}
    container_name: optimus-fe
    restart: unless-stopped
    depends_on:
      optimus-be:
        condition: service_healthy
    ports:
      - "${HTTP_PORT:-80}:80"
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost/"]
      interval: 10s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
```

**Key decisions**:

- `${VAR:?msg}` form forces required vars — `docker compose up` aborts cleanly with a message if missing.
- migrate service has `restart: "no"` — it's an init container.
- migrate does NOT receive `OPTIMUS_JWT_SECRET` (uses `ValidateForMigrate` which only requires DSN).
- postgres port 5432 not exposed externally. To inspect locally, add `ports: ["127.0.0.1:5432:5432"]` ad-hoc.
- `HTTP_PORT` lets users avoid host port-80 collisions without editing compose.

### 5.2 `deploy/.env.example` (new)

```bash
# === REQUIRED ===

# 32+ byte JWT signing secret. Generate with: openssl rand -base64 48
OPTIMUS_JWT_SECRET=

# Postgres DSN. Hostname `postgres` matches the docker service name.
OPTIMUS_DATABASE_DSN=host=postgres port=5432 user=optimus password=CHANGE_ME dbname=optimus sslmode=disable

# Postgres credentials (MUST align with values in OPTIMUS_DATABASE_DSN above)
POSTGRES_USER=optimus
POSTGRES_PASSWORD=CHANGE_ME
POSTGRES_DB=optimus

# === OPTIONAL ===

# Image tag; set to git short SHA in CI, defaults to "dev"
VERSION=dev

# Host port to expose the FE on. Change if 80 is busy.
HTTP_PORT=80

# CORS allowlist (JSON array). Add your prod domain.
# OPTIMUS_CORS_ALLOWED_ORIGINS=["https://optimus.example.com"]

# Log level: debug | info | warn | error
OPTIMUS_LOG_LEVEL=info
```

### 5.3 `.dockerignore` (new, at repo root)

```
# Build artifacts
**/bin/
**/dist/
**/node_modules/

# Test / coverage
**/*.test
**/coverage.out

# IDE / OS
.git/
.idea/
.vscode/
.DS_Store

# Local environment files (but KEEP .env.production — vite reads it at build time)
**/.env
**/.env.local
**/.env.development

# Existing dev-only compose
docker-compose.yml
```

---

## 6. CI: docker-build job

Append to `.github/workflows/ci.yml`:

```yaml
  docker-build:
    runs-on: ubuntu-latest
    needs: [backend, frontend]
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3

      - name: build optimus-be (server)
        uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/be.Dockerfile
          target: server
          push: false
          load: false
          cache-from: type=gha,scope=be-server
          cache-to: type=gha,mode=max,scope=be-server
          build-args: |
            VERSION=${{ github.sha }}

      - name: build optimus-be (migrate)
        uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/be.Dockerfile
          target: migrate
          push: false
          load: false
          cache-from: type=gha,scope=be-migrate
          cache-to: type=gha,mode=max,scope=be-migrate

      - name: build optimus-be (seed)
        uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/be.Dockerfile
          target: seed
          push: false
          load: false
          cache-from: type=gha,scope=be-seed
          cache-to: type=gha,mode=max,scope=be-seed

      - name: build optimus-fe
        uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/fe.Dockerfile
          push: false
          load: false
          cache-from: type=gha,scope=fe
          cache-to: type=gha,mode=max,scope=fe
```

**Behavior**:

- Runs after `backend` and `frontend` pass (so we don't waste runner time on a broken commit).
- `push: false` and `load: false` → image stays inside buildkit cache, not loaded into local daemon, never pushed.
- GHA cache scoped per build target. Cold ~5 min, warm ~1.5 min.
- No registry credentials needed; no secrets required.

---

## 7. Spec updates outside this file

Two existing files need light edits when this plan ships:

**`docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`**:

- §9.6: change Go `1.22` → `1.25`; `bun.lockb` → `bun.lock`; add a "See `2026-06-09-p0-plan3-deployment-design.md` for the production-grade Dockerfiles and compose" pointer.
- §9.7: keep the prose (it correctly says "single-machine docker-compose"); add the same pointer.

**`README.md`** at repo root — add a "Production deploy" section:

```markdown
## Production deploy (single-machine, docker-compose)

1. `cd deploy`
2. `cp .env.example .env` and fill in the REQUIRED section.
   Generate a JWT secret: `openssl rand -base64 48`
3. `docker compose -f docker-compose.prod.yml up -d --build`
4. Wait for all 4 services to be healthy (~30s on warm cache):
   `docker compose -f docker-compose.prod.yml ps`
5. Verify: `curl -s http://localhost/api/v1/health` → `{code:0, data:{db:"ok",version:"<sha>"}}`
6. Retrieve the **initial admin password**: `docker logs optimus-seed | grep INITIAL` — copy the printed password (logged only on the first run; subsequent runs say "admin user already exists").
7. Open http://localhost — log in as `admin` with the password from step 6.

Useful commands:
- Logs: `docker compose -f deploy/docker-compose.prod.yml logs -f optimus-be`
- Stop: `docker compose -f deploy/docker-compose.prod.yml down`
- Reset DB (destructive): `... down -v`

Local docker note: this repo's primary workstation uses Colima, so set
`DOCKER_HOST=unix:///Users/<you>/.colima/docker.sock` if `docker compose`
can't find a daemon.
```

---

## 8. Verification and acceptance

When all of the following pass, Plan 3 is done.

### 8.1 End-to-end stack

1. Fresh checkout of `main` post-merge.
2. `cd deploy && cp .env.example .env`; fill in `OPTIMUS_JWT_SECRET` (`openssl rand -base64 48`) and `POSTGRES_PASSWORD`.
3. `docker compose -f docker-compose.prod.yml up -d --build`.
4. `docker compose ps` shows: postgres `healthy`, migrate `exited (0)`, seed `exited (0)`, optimus-be `healthy`, optimus-fe `healthy`.
5. `curl -sf http://localhost/api/v1/health` returns `{"code":0, "data":{"db":"ok","version":"<sha>"}}`.
6. `docker logs optimus-seed | grep INITIAL` prints `INITIAL ADMIN CREDENTIALS ... username=admin password=<random>` on first run.
7. Browser at http://localhost shows the login page; assets load 200; no console errors.
8. Login with admin / <password from step 6> → all 5 admin pages (Users / Roles / Permissions / Menus / Audit) render and can be navigated.

### 8.2 Migration + seed

1. First `up`: `docker logs optimus-migrate` shows 11 goose `OK` lines, exits 0.
2. Second `up -d`: migrate re-runs, detects no pending, exits 0 immediately, BE starts as before.
3. `docker exec optimus-pg psql -U optimus -d optimus -c '\dt'` shows 8 business tables (users / roles / permissions / user_roles / role_permissions / menus / refresh_tokens / audit_logs) + `goose_db_version`.
4. First `up`: `docker logs optimus-seed` shows `permissions registered` + `INITIAL ADMIN CREDENTIALS ... password=<random>`, exits 0.
5. Second `up -d`: seed re-runs, logs `admin user already exists; no password generated`, exits 0.

### 8.3 Bundle split

1. `cd optimus-fe && bun run build`.
2. `ls -lh dist/assets/*.js` shows distinct `index-*.js`, `vendor-*.js`, `antd-*.js`, `icons-*.js`, `utils-*.js` chunks.
3. `index-*.js` < 250 KB, `vendor-*.js` < 250 KB, `antd-*.js` < 1.5 MB, `icons-*.js` < 150 KB, `utils-*.js` < 150 KB (uncompressed).

### 8.4 nginx behavior

1. `curl -I http://localhost/assets/index-<hash>.js` returns `Cache-Control: public, max-age=31536000, immutable`.
2. `curl -I http://localhost/index.html` returns `Cache-Control: no-cache, no-store, must-revalidate`.
3. `curl -I http://localhost/` returns `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin`.
4. `curl -I -H "Accept-Encoding: gzip" http://localhost/assets/index-<hash>.js` returns `Content-Encoding: gzip`.
5. `curl http://localhost/anything/deep/spa/route` returns the SPA `index.html` (HTTP 200).
6. `curl -sf http://localhost/api/v1/health` succeeds (reverse proxy works).

### 8.5 Env override

1. Add `OPTIMUS_LOG_LEVEL=debug` to `.env`; `docker compose up -d optimus-be` restarts the BE.
2. `docker logs optimus-be | head -10` shows debug-level lines.
3. Comment out `OPTIMUS_JWT_SECRET` in `.env`; `docker compose up -d optimus-be` fails fast with a "is required" message before container starts.

### 8.6 CI

1. New `docker-build` job appears on PR / push to dev / main and turns green.
2. Cold cache build < 7 min total wall-clock (be-server + be-migrate + fe combined).
3. Warm cache build < 2 min.
4. Manually breaking `be.Dockerfile` (e.g., `FROM nonexistent:1`) makes the job fail.

### 8.7 Regression-free

1. Existing `docker-compose.yml` (dev postgres + adminer) still works: `docker compose up -d` from repo root behaves as before.
2. `cd optimus-be && make lint && make test && make test-int` — all pass.
3. `cd optimus-fe && bun run lint && bun run typecheck && bun run test && bun run build` — all pass.

### 8.8 Migrate smoke test (added test)

Add `optimus-be/cmd/migrate/main_test.go` using the existing dockertest pattern:

- Spin up a PG container via dockertest (see `optimus-be/tests/` for the existing helper).
- Run `goose.Up` against it programmatically (calling the same code as `main`).
- Assert: 11 rows in `goose_db_version`, 8 business tables exist.
- Run again → `goose.Up` returns nil with no schema change.
- Gated by `//go:build dbtest` like the rest of the integration tests.

---

## 9. Risks and explicit non-goals

**Risks acknowledged**:

- **pgx not in go.sum**: if GORM does not transitively pin `jackc/pgx/v5/stdlib`, implementation must add it. Mitigated by a check during step 1 of the plan.
- **Image cache misses on go.mod changes**: by COPYing `go.mod`/`go.sum` first the layer is stable; only invalidates when deps actually change.
- **`OPTIMUS_CORS_ALLOWED_ORIGINS` env override semantics**: viper parses JSON arrays from env strings via `AutomaticEnv` — to verify during implementation. If it doesn't work cleanly, fallback is to require operators to mount a config.yaml overlay rather than env-only.

**Non-goals (explicit)**:

- No multi-instance support. Only one BE container; no leader election; no shared session store. Refresh tokens are stored in DB so a stateless restart works, but horizontal scale is P5.
- No TLS in this stack. Operators terminate TLS in front (cloudflare / nginx / ALB).
- No image push or registry credentials in CI. P2+ adds this when there's a release process.
- No K8s manifests. P2 dogfood owns that.
- No CSP. Punted to P1+ where the FE iframe / embed story is better understood.

---

## 10. Implementation order (writing-plans will refine)

Suggested task decomposition (each task ~ small PR-sized):

1. **BE: cmd/migrate + ValidateForMigrate + smoke test** — purely backend, no compose dep. (cmd/seed already exists; no code change needed for seed.)
2. **BE Dockerfile** with `server` / `migrate` / `seed` stages — depends on task 1
3. **FE: vite manualChunks + bundle acceptance check** — purely frontend, no compose dep
4. **FE Dockerfile + nginx.conf** — depends on task 3
5. **`.dockerignore` + `deploy/docker-compose.prod.yml` + `deploy/.env.example`** — depends on tasks 2 and 4
6. **Manual end-to-end run-through + spec/README edits** — depends on task 5
7. **CI docker-build job** — depends on tasks 2 and 4; can run in parallel with task 5/6

writing-plans will turn this into the detailed step-by-step plan with full code for each task.
