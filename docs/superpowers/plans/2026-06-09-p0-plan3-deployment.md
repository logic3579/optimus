# P0 Plan 3 — Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land production-shape Docker images, an nginx reverse proxy, a single-machine `docker-compose.prod.yml`, and a CI guard so that `cd deploy && docker compose up -d --build` brings up a working stack with admin login.

**Architecture:** Five-service compose stack (postgres → migrate → seed → be → fe) using one multi-stage BE Dockerfile with three targets, a multi-stage FE Dockerfile that produces an nginx image, and a small new Go binary `cmd/migrate` that runs goose programmatically against embedded SQL.

**Tech Stack:** Go 1.25, docker compose v2, nginx 1.27-alpine, Vite 5 (`manualChunks` rollup option), goose v3 (programmatic API + `embed.FS`), pgx v5 stdlib driver, GitHub Actions docker/build-push-action v6.

**Reference spec:** `docs/superpowers/specs/2026-06-09-p0-plan3-deployment-design.md` (commit `e367765`).

---

## File map

| Task | New / Modified | Path | Responsibility |
|---|---|---|---|
| 1 | Modify | `optimus-be/internal/infra/config/config.go` | Add `ValidateForMigrate()` — DSN-only validation |
| 1 | Modify | `optimus-be/internal/infra/config/config_test.go` | Test new method |
| 2 | New | `optimus-be/migrations/embed.go` | Export `FS embed.FS` containing all `.sql` migrations |
| 3 | New | `optimus-be/cmd/migrate/main.go` | Programmatic goose: load config, open PG via pgx, run up/down/status |
| 3 | Modify | `optimus-be/go.mod` | Promote `github.com/jackc/pgx/v5` from indirect → direct |
| 4 | New | `optimus-be/cmd/migrate/main_test.go` | Build-tag `dbtest` smoke test using dockertest |
| 5 | New | `deploy/be.Dockerfile` | Multi-stage; targets `server` / `migrate` / `seed` |
| 6 | Modify | `optimus-fe/vite.config.ts` | Add `build.rollupOptions.output.manualChunks` (3 chunks) |
| 7 | New | `deploy/fe.Dockerfile` | bun build → nginx:1.27-alpine |
| 7 | New | `deploy/nginx.conf` | SPA fallback + `/api/v1/` proxy + gzip + cache + security headers |
| 8 | New | `.dockerignore` | At repo root |
| 8 | New | `deploy/docker-compose.prod.yml` | 5 services + named volume |
| 8 | New | `deploy/.env.example` | Required + optional env vars |
| 9 | Modify | `README.md` | Add "Production deploy" section |
| 9 | Modify | `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md` | Fix Go 1.22→1.25, bun.lockb→bun.lock, point at this plan |
| 10 | Modify | `.github/workflows/ci.yml` | Append `docker-build` job |

---

## Implementation notes (read before starting)

1. **`//go:embed` path constraint.** The spec §3.1 shows `//go:embed all:migrations` from inside `cmd/migrate/main.go`, but embed paths are relative to the source file and cannot use `..`. There's no `migrations/` subdir under `cmd/migrate/`. So we put the embed declaration **next to the SQL files** in `optimus-be/migrations/embed.go` and `import "optimus-be/migrations"` from `cmd/migrate`.

2. **`pgx/v5` is currently indirect** (in go.sum at `v5.9.2`, pulled via GORM). The new direct usage from `cmd/migrate` will need `go mod tidy` to promote it to a direct `require`. The plan includes a step for this.

3. **`bun.lock` is text, not `bun.lockb`** — confirmed in the working tree. The original platform spec §9.6 was wrong; the FE Dockerfile and Task 9 spec edit both fix this.

4. **Docker daemon on this workstation is Colima.** `DOCKER_HOST=unix:///Users/<you>/.colima/docker.sock` is set in the user shell (per memory). Verification steps that need docker assume the daemon is reachable; if a step's docker command errors with "Cannot connect", check `docker context ls` and switch contexts, or `colima start`.

5. **Bundle size acceptance is "best effort"** — actual antd-vue bundle size depends on tree-shaking and may differ. If a chunk overshoots its budget after step 1 of Task 6, adjust splits (e.g., move `@ant-design/icons-vue` into its own chunk) and re-verify. Don't tune indefinitely — main `index-*.js` < 250 KB is the only hard requirement.

6. **Commit message style** follows the existing repo convention: `<type>(<scope>): <summary>` (see recent commits like `feat(fe/views/audit): paginated list with filters + payload <pre> expand`).

---

## Task 1: Add `ValidateForMigrate` to config (TDD)

**Files:**
- Modify: `optimus-be/internal/infra/config/config.go` (append after `ValidateStrict`)
- Modify: `optimus-be/internal/infra/config/config_test.go` (append after existing tests)

- [ ] **Step 1: Write failing tests for `ValidateForMigrate`**

Append to `optimus-be/internal/infra/config/config_test.go`:

```go
func TestValidateForMigrate_RequiresDSN(t *testing.T) {
	cfg := &config.Config{}
	require.Error(t, cfg.ValidateForMigrate())
}

func TestValidateForMigrate_AcceptsDSNOnly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.DSN = "host=localhost port=5432 user=u password=p dbname=d sslmode=disable"
	// Deliberately no JWT secret — migrations don't need it.
	require.NoError(t, cfg.ValidateForMigrate())
}
```

- [ ] **Step 2: Run tests, verify they fail**

```bash
cd optimus-be
go test ./internal/infra/config/ -run ValidateForMigrate -v
```
Expected: FAIL — `cfg.ValidateForMigrate undefined (type *config.Config has no field or method ValidateForMigrate)`.

- [ ] **Step 3: Add the `ValidateForMigrate` method**

Append to `optimus-be/internal/infra/config/config.go` after the closing `}` of `ValidateStrict`:

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

- [ ] **Step 4: Run tests, verify they pass**

```bash
cd optimus-be
go test ./internal/infra/config/ -run ValidateForMigrate -v
```
Expected: `--- PASS: TestValidateForMigrate_RequiresDSN` and `--- PASS: TestValidateForMigrate_AcceptsDSNOnly`.

- [ ] **Step 5: Run full config tests + lint, confirm no regression**

```bash
cd optimus-be
go test ./internal/infra/config/ -v
golangci-lint run ./internal/infra/config/...
```
Expected: all tests pass, lint clean.

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/infra/config/config.go optimus-be/internal/infra/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(be/config): add ValidateForMigrate for DSN-only validation

Migration cmd doesn't need a JWT secret. Adding a focused validator keeps
cmd/server's strict check (DSN + JWT) intact while letting cmd/migrate
boot with just a DSN.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `migrations` embed package

**Files:**
- New: `optimus-be/migrations/embed.go` (next to existing `*.sql` files)

- [ ] **Step 1: Create the embed file**

Write `optimus-be/migrations/embed.go`:

```go
// Package migrations exposes the SQL migration files as an embed.FS so they
// can be applied programmatically from cmd/migrate (and from tests) without
// needing the migrations/ directory to exist on disk at runtime.
//
// The existing Makefile target `migrate-up` still works against the on-disk
// directory; this package coexists with that path.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Verify the package builds and embeds the right files**

```bash
cd optimus-be
go build ./migrations/...
go vet ./migrations/...
```
Expected: both succeed silently.

- [ ] **Step 3: Verify embed contents at runtime via a one-shot script**

Create `optimus-be/migrations/embed_test.go`:

```go
package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbedContainsAllMigrations(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)
	var sqlCount int
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".sql" {
			sqlCount++
		}
	}
	require.Equal(t, 11, sqlCount, "expected 11 embedded migration files")
}
```

```bash
cd optimus-be
go test ./migrations/... -v
```
Expected: `PASS: TestEmbedContainsAllMigrations`.

- [ ] **Step 4: Run lint**

```bash
cd optimus-be
golangci-lint run ./migrations/...
```
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add optimus-be/migrations/embed.go optimus-be/migrations/embed_test.go
git commit -m "$(cat <<'EOF'
feat(be/migrations): expose embed.FS for programmatic goose use

Adds optimus-be/migrations/embed.go which embeds all *.sql files via
go:embed and exports them as migrations.FS. cmd/migrate (next task)
imports this package so the prod migrate binary doesn't need the
migrations dir on disk inside the container.

The existing Makefile target `migrate-up` (which uses the goose CLI
against the on-disk directory) is unaffected.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `cmd/migrate` binary

**Files:**
- New: `optimus-be/cmd/migrate/main.go`
- Modify: `optimus-be/go.mod` (auto-updated via `go mod tidy`)
- Modify: `optimus-be/go.sum` (auto-updated)

- [ ] **Step 1: Create `cmd/migrate/main.go`**

Write `optimus-be/cmd/migrate/main.go`:

```go
// Package main is the optimus-migrate binary: it applies pending Goose
// SQL migrations embedded into the binary against the Postgres instance
// configured by configs/config.yaml (overridable via OPTIMUS_* env vars).
//
// Exit codes:
//   0 — migrations applied successfully (including the no-op case where
//       the DB is already at head)
//   1 — any failure (config invalid, DB unreachable, migration error)
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/log"
	"optimus-be/migrations"
)

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

	if err := runGoose(db, *direction); err != nil {
		die("migrate "+*direction, err)
	}
	logger.Info("optimus-migrate done")
}

func runGoose(db *sql.DB, direction string) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	switch direction {
	case "up":
		return goose.Up(db, ".")
	case "down":
		return goose.Down(db, ".")
	case "status":
		return goose.Status(db, ".")
	default:
		return fmt.Errorf("unknown direction: %s", direction)
	}
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

- [ ] **Step 2: Promote `pgx` to a direct dependency via `go mod tidy`**

```bash
cd optimus-be
go mod tidy
```
Expected: `go.mod` now has `github.com/jackc/pgx/v5 v5.9.2` in the **first** `require` block (not the indirect block). `go.sum` may add a few lines. No errors.

Verify the promotion:

```bash
grep -A 1 'jackc/pgx/v5' optimus-be/go.mod | head -5
```
Expected: `github.com/jackc/pgx/v5 v5.9.2` appears NOT under `// indirect`.

- [ ] **Step 3: Build the binary, verify it compiles**

```bash
cd optimus-be
go build -o /tmp/optimus-migrate ./cmd/migrate
/tmp/optimus-migrate -h 2>&1 | head -10
```
Expected: build succeeds with no output; `-h` shows the two flags `-config` and `-dir`. (Note: `flag` prints to stderr, hence `2>&1`.)

- [ ] **Step 4: Smoke test against the dev Postgres instance**

The repo-root `docker-compose.yml` already runs a dev Postgres. If it's not running:

```bash
cd /Users/logic/Projects/optimus
docker compose up -d postgres
```

Now point migrate at it. Use a temp config file so we don't pollute env, and bypass the `jwt.secret` check by using the existing `configs/config.yaml`:

```bash
cd optimus-be
OPTIMUS_DATABASE_DSN="host=localhost port=5432 user=optimus password=optimus dbname=optimus sslmode=disable" \
  /tmp/optimus-migrate -config configs/config.yaml -dir status
```
Expected: prints a list of migration files with `Applied At` timestamps (since migrations have been applied by prior work on dev). If the dev DB has NEVER been migrated, you'll see `Pending` for all 11 files instead — that's still OK as long as the command succeeds.

- [ ] **Step 5: Run lint on the new package**

```bash
cd optimus-be
golangci-lint run ./cmd/migrate/...
```
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add optimus-be/cmd/migrate/main.go optimus-be/go.mod optimus-be/go.sum
git commit -m "$(cat <<'EOF'
feat(be/cmd/migrate): add optimus-migrate binary for prod init

New cmd that calls goose programmatically against the embedded
migrations FS. Uses the same viper config + OPTIMUS_* env override
as cmd/server, but validates only the DSN (no JWT secret needed).
Promotes jackc/pgx/v5 to a direct dependency via the stdlib driver.

Will run as the init service in docker-compose.prod.yml.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add dockertest smoke test for `cmd/migrate`

**Files:**
- New: `optimus-be/cmd/migrate/main_test.go`

- [ ] **Step 1: Write the dockertest smoke test**

Write `optimus-be/cmd/migrate/main_test.go`:

```go
//go:build dbtest

package main

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

func TestRunGoose_UpAppliesAllMigrations(t *testing.T) {
	db, teardown := startRawPostgres(t)
	defer teardown()

	require.NoError(t, runGoose(db, "up"))

	var maxVersion int64
	require.NoError(t,
		db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&maxVersion))
	require.GreaterOrEqual(t, maxVersion, int64(11),
		"expected at least 11 migrations to be applied")

	var tableCount int
	require.NoError(t, db.QueryRow(`
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN (
			'users','roles','permissions','user_roles','role_permissions',
			'menus','refresh_tokens','audit_logs'
		)
	`).Scan(&tableCount))
	require.Equal(t, 8, tableCount, "all 8 business tables should exist")
}

func TestRunGoose_UpIsIdempotent(t *testing.T) {
	db, teardown := startRawPostgres(t)
	defer teardown()

	require.NoError(t, runGoose(db, "up"))
	// Second invocation must succeed without changing anything.
	require.NoError(t, runGoose(db, "up"))
}

func TestRunGoose_RejectsUnknownDirection(t *testing.T) {
	// No DB needed — runGoose returns the error before touching the DB.
	err := runGoose(nil, "sideways")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown direction")
}

// startRawPostgres boots a clean Postgres container with NO migrations
// pre-applied. Distinct from internal/infra/db.StartTestPostgres which
// auto-migrates — we explicitly want a virgin DB so runGoose has work to do.
func startRawPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err)

	dsn := fmt.Sprintf(
		"host=localhost port=%s user=test password=test dbname=test sslmode=disable",
		res.GetPort("5432/tcp"),
	)
	pool.MaxWait = 60 * time.Second

	var db *sql.DB
	require.NoError(t, pool.Retry(func() error {
		var openErr error
		db, openErr = sql.Open("pgx", dsn)
		if openErr != nil {
			return openErr
		}
		return db.Ping()
	}))

	return db, func() {
		_ = db.Close()
		_ = pool.Purge(res)
	}
}
```

- [ ] **Step 2: Run the smoke test with the `dbtest` tag**

Make sure colima/docker is up first (`docker ps` should respond).

```bash
cd optimus-be
go test ./cmd/migrate/... -tags=dbtest -race -count=1 -v
```
Expected: all 3 tests pass. The two PG-backed tests take ~20-30s each on cold image cache (PG container startup); cached runs ~5s.

If `dockertest` errors with "Cannot connect to the Docker daemon", verify `DOCKER_HOST` env var is set or run `colima start`.

- [ ] **Step 3: Confirm tests are excluded from the default `make test`**

```bash
cd optimus-be
make test
```
Expected: passes; the new migrate tests are SKIPPED (build tag `dbtest` not set), so we see no migrate-related output here.

- [ ] **Step 4: Run the existing integration suite to make sure we didn't break it**

```bash
cd optimus-be
make test-int
```
Expected: all dbtest-tagged tests pass (including the new migrate ones and the existing seed/rbac/integration ones).

- [ ] **Step 5: Lint**

```bash
cd optimus-be
golangci-lint run --build-tags=dbtest ./cmd/migrate/...
```
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add optimus-be/cmd/migrate/main_test.go
git commit -m "$(cat <<'EOF'
test(be/cmd/migrate): dockertest smoke for embedded goose migrations

Three cases: applies all 11 migrations end-to-end against a virgin
Postgres, is idempotent on re-run, rejects unknown direction. Uses
its own startRawPostgres helper (not the shared StartTestPostgres
which auto-migrates) so the test exercises the real runGoose path.

Tagged dbtest, runs under `make test-int`.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add `deploy/be.Dockerfile`

**Files:**
- New: `deploy/be.Dockerfile`

- [ ] **Step 1: Create the deploy directory**

```bash
cd /Users/logic/Projects/optimus
mkdir -p deploy
```

- [ ] **Step 2: Write `deploy/be.Dockerfile`**

```dockerfile
# Multi-stage Dockerfile for the optimus-be Go services.
# Build context MUST be the repo root (not optimus-be/) so this file can
# also access deploy/nginx.conf via the fe Dockerfile build.
#
# Targets:
#   server  — the main HTTP API (cmd/server)
#   migrate — goose schema runner (cmd/migrate, init container)
#   seed    — permission registry + bootstrap admin (cmd/seed, init container)

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

# Cache go.sum / go.mod resolution as its own layer
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

- [ ] **Step 3: Build each target locally to verify**

(Requires colima/docker running.)

```bash
cd /Users/logic/Projects/optimus
docker build -f deploy/be.Dockerfile --target server   -t optimus-be:test     --build-arg VERSION=$(git rev-parse --short HEAD) .
docker build -f deploy/be.Dockerfile --target migrate  -t optimus-migrate:test .
docker build -f deploy/be.Dockerfile --target seed     -t optimus-seed:test    .
```
Expected: all three builds succeed. After the first one the `build` stage layer is fully cached for the next two.

- [ ] **Step 4: Verify image sizes are reasonable and binaries run**

```bash
docker images | grep -E '(optimus-be|optimus-migrate|optimus-seed):test'
docker run --rm optimus-be:test -h 2>&1 | head -5
docker run --rm optimus-migrate:test -h 2>&1 | head -5
docker run --rm optimus-seed:test -h 2>&1 | head -5
```
Expected:
- Each image < 50 MB (alpine + one static binary).
- `-h` output shows the flags from each binary's `flag.Parse()`.

- [ ] **Step 5: Clean up the test images**

```bash
docker rmi optimus-be:test optimus-migrate:test optimus-seed:test
```

- [ ] **Step 6: Commit**

```bash
git add deploy/be.Dockerfile
git commit -m "$(cat <<'EOF'
feat(deploy): multi-stage BE Dockerfile (server/migrate/seed targets)

Single Dockerfile, four stages: shared Go build + three minimal
alpine runtime images. Build context is the repo root so the same
file can be consumed by docker-compose.prod.yml alongside the FE
Dockerfile.

VERSION build-arg is wired into -X main.Version for /health output.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Vite manualChunks + bundle verification

**Files:**
- Modify: `optimus-fe/vite.config.ts`

- [ ] **Step 1: Establish baseline (so the "before" number is recorded)**

```bash
cd optimus-fe
bun run build 2>&1 | tail -20
ls -lh dist/assets/index-*.js
```
Expected: an `index-*.js` file ~1.6 MB. Note the size — we'll compare after the change.

- [ ] **Step 2: Update `optimus-fe/vite.config.ts`**

Replace the file contents with:

```ts
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src')
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: false
      }
    }
  },
  build: {
    target: 'es2020',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia', 'pinia-plugin-persistedstate', 'axios'],
          antd:   ['ant-design-vue', '@ant-design/icons-vue'],
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

- [ ] **Step 3: Re-run the build and inspect chunks**

```bash
cd optimus-fe
rm -rf dist
bun run build 2>&1 | tail -20
ls -lh dist/assets/index-*.js dist/assets/vendor-*.js dist/assets/antd-*.js dist/assets/utils-*.js
```
Expected:
- Four chunks named `index-<hash>.js`, `vendor-<hash>.js`, `antd-<hash>.js`, `utils-<hash>.js`.
- `index-*.js` < 250 KB (uncompressed).
- `vendor-*.js` < 250 KB.
- `antd-*.js` < 900 KB.
- `utils-*.js` < 150 KB.

If `antd-*.js` is over budget, split out icons:

```ts
antd:    ['ant-design-vue'],
icons:   ['@ant-design/icons-vue'],
```

Re-run step 3 after that change; update spec §4.1 (bundle table) in Task 9 to reflect the new chunk list.

- [ ] **Step 4: Verify gzip-encoded antd size is reasonable**

```bash
gzip -c optimus-fe/dist/assets/antd-*.js | wc -c
```
Expected: < 320 KB after gzip.

- [ ] **Step 5: Run typecheck + lint + unit tests + i18n check (full FE quality gate)**

```bash
cd optimus-fe
bun run typecheck && bun run lint && bun run test && bun run i18n:check
```
Expected: every command exits 0.

- [ ] **Step 6: Commit**

```bash
git add optimus-fe/vite.config.ts
git commit -m "$(cat <<'EOF'
perf(fe/build): split bundle into vendor/antd/utils chunks via manualChunks

Main index-*.js drops from ~1.6 MB to <250 KB by extracting vue/router/
pinia/axios into a vendor chunk, ant-design-vue + icons into an antd
chunk, and dayjs + vue-i18n into a utils chunk. Improves first-load
cacheability without changing app behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Add `deploy/fe.Dockerfile` + `deploy/nginx.conf`

**Files:**
- New: `deploy/fe.Dockerfile`
- New: `deploy/nginx.conf`

- [ ] **Step 1: Write `deploy/nginx.conf`**

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

    # ---- security headers (apply to everything by default) ----
    # NOTE: nginx add_header is REPLACED (not inherited) inside nested
    # location blocks that declare their own add_header. The /assets/
    # and /index.html blocks below re-declare them.
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

- [ ] **Step 2: Write `deploy/fe.Dockerfile`**

```dockerfile
# Multi-stage Dockerfile for the optimus-fe SPA.
# Build context MUST be the repo root so deploy/nginx.conf is reachable.

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

- [ ] **Step 3: Build the FE image locally to verify**

```bash
cd /Users/logic/Projects/optimus
docker build -f deploy/fe.Dockerfile -t optimus-fe:test .
docker images | grep optimus-fe:test
```
Expected: build succeeds; image ~50-70 MB.

- [ ] **Step 4: Spot-check the image contents and nginx config syntax**

```bash
docker run --rm optimus-fe:test ls /usr/share/nginx/html/assets/ | head -10
docker run --rm optimus-fe:test nginx -t
```
Expected:
- `ls` shows the four hashed chunks from Task 6 plus the per-page chunks.
- `nginx -t` prints `nginx: configuration file /etc/nginx/nginx.conf test is successful`.

- [ ] **Step 5: Optional sanity check — run the image and curl it**

```bash
docker run -d --rm --name optimus-fe-test -p 8090:80 optimus-fe:test
sleep 2
curl -sI http://localhost:8090/ | head -10
curl -sI -H "Accept-Encoding: gzip" http://localhost:8090/assets/$(docker run --rm optimus-fe:test sh -c 'ls /usr/share/nginx/html/assets/index-*.js | head -1 | xargs basename') | head -10
docker stop optimus-fe-test
```
Expected:
- `curl http://localhost:8090/` returns 200 with the three security headers.
- `curl /assets/index-*.js` returns 200 with `Content-Encoding: gzip` and `Cache-Control: public, max-age=31536000, immutable`.

Note: the `/api/v1/` reverse proxy will return 502 in this standalone run because there's no `optimus-be` container in the network — that's expected; full reverse-proxy verification happens in Task 9.

- [ ] **Step 6: Clean up**

```bash
docker rmi optimus-fe:test
```

- [ ] **Step 7: Commit**

```bash
git add deploy/fe.Dockerfile deploy/nginx.conf
git commit -m "$(cat <<'EOF'
feat(deploy): FE Dockerfile + nginx.conf for SPA + reverse proxy

bun build → nginx:1.27-alpine. nginx serves the SPA with history-mode
fallback, reverse-proxies /api/v1/ and /swagger/ to optimus-be:8080,
applies gzip + immutable cache for hashed /assets/, no-cache for
index.html, and three baseline security headers (re-declared in
nested locations to work around nginx add_header inheritance).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `.dockerignore` + `docker-compose.prod.yml` + `.env.example`

**Files:**
- New: `.dockerignore` (repo root)
- New: `deploy/docker-compose.prod.yml`
- New: `deploy/.env.example`

- [ ] **Step 1: Write `.dockerignore` at repo root**

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

# Local environment files. Vite reads .env.production at build time so it
# is intentionally NOT ignored.
**/.env
**/.env.local
**/.env.development

# Existing dev-only compose at the repo root
docker-compose.yml
```

- [ ] **Step 2: Write `deploy/docker-compose.prod.yml`**

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

- [ ] **Step 3: Write `deploy/.env.example`**

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

- [ ] **Step 4: Validate compose config syntax**

```bash
cd /Users/logic/Projects/optimus/deploy
cp .env.example .env
# Fill in the two required values for the validation pass.
# Use sed for deterministic edits:
sed -i.bak "s|^OPTIMUS_JWT_SECRET=$|OPTIMUS_JWT_SECRET=$(openssl rand -base64 48 | tr -d '\n')|" .env
sed -i.bak "s|password=CHANGE_ME|password=optimus|g" .env
sed -i.bak "s|^POSTGRES_PASSWORD=CHANGE_ME|POSTGRES_PASSWORD=optimus|" .env
rm .env.bak
docker compose -f docker-compose.prod.yml config > /dev/null
echo "compose config OK"
```
Expected: prints `compose config OK`. No error from `docker compose config`.

- [ ] **Step 5: Confirm the missing-var case fails cleanly**

```bash
cd /Users/logic/Projects/optimus/deploy
# Move .env aside temporarily
mv .env .env.kept
docker compose -f docker-compose.prod.yml config 2>&1 | tail -5
# Should print an error referencing OPTIMUS_JWT_SECRET / OPTIMUS_DATABASE_DSN / POSTGRES_PASSWORD
mv .env.kept .env
```
Expected: `error: required variable POSTGRES_PASSWORD is missing` (or one of the others, depending on which compose finds first). Confirms `${VAR:?msg}` works.

- [ ] **Step 6: Commit (without the local .env — that's gitignored)**

First make sure `.env` is not staged:

```bash
git status deploy/
```
Expected to show: `deploy/docker-compose.prod.yml`, `deploy/.env.example`, `.dockerignore` as untracked. `.env` should NOT appear (we'll add `.env` to a deploy-scoped gitignore in step 7).

- [ ] **Step 7: Ensure `.env` is gitignored**

Check the repo root `.gitignore`:

```bash
grep -E '(^|/)\.env$' /Users/logic/Projects/optimus/.gitignore || echo "MISSING"
```

If it prints `MISSING`, append a `deploy/.env` rule:

```bash
cat >> /Users/logic/Projects/optimus/.gitignore <<'EOF'

# local prod env
deploy/.env
EOF
```

Then re-run `git status deploy/` and confirm `.env` is not listed.

- [ ] **Step 8: Commit**

```bash
cd /Users/logic/Projects/optimus
git add .dockerignore deploy/docker-compose.prod.yml deploy/.env.example .gitignore
git commit -m "$(cat <<'EOF'
feat(deploy): single-machine docker-compose.prod.yml + env template

Five services (postgres → migrate → seed → optimus-be → optimus-fe)
with the dependency chain enforced via service_healthy and
service_completed_successfully. Required env vars use \${VAR:?} so
docker compose up aborts cleanly when misconfigured.

Adds a repo-root .dockerignore that excludes build artifacts, env
files (except .env.production which vite reads at build), and the
dev-only docker-compose.yml. Adds deploy/.env to .gitignore.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: End-to-end verification + README + spec ref updates

**Files:**
- Modify: `README.md` (repo root)
- Modify: `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`

This task **runs the full stack** to confirm everything wired together correctly, then captures the lessons in docs.

- [ ] **Step 1: Start the full stack**

```bash
cd /Users/logic/Projects/optimus/deploy
docker compose -f docker-compose.prod.yml up -d --build
```
Expected: builds all images (cold ~3-5 min), starts containers, returns to prompt.

- [ ] **Step 2: Verify all five services are in their expected states**

```bash
sleep 15
docker compose -f docker-compose.prod.yml ps
```
Expected output (status column):
- `optimus-pg`        → `Up (healthy)`
- `optimus-migrate`   → `Exited (0)`
- `optimus-seed`      → `Exited (0)`
- `optimus-be`        → `Up (healthy)`
- `optimus-fe`        → `Up (healthy)`

If any service is `Restarting` or `Exited (1)`, capture its logs:

```bash
docker compose -f docker-compose.prod.yml logs --tail 50 <service-name>
```

- [ ] **Step 3: Verify health + reverse proxy**

```bash
curl -sf http://localhost/api/v1/health | head -1
```
Expected: a JSON envelope like `{"code":0,"data":{"db":"ok","version":"<sha>"},"message":"ok"}`.

- [ ] **Step 4: Retrieve the bootstrap admin password from seed logs**

```bash
docker logs optimus-seed | grep INITIAL
```
Expected: a line like `level=WARN msg="INITIAL ADMIN CREDENTIALS — RECORD THESE NOW (printed only once)" username=admin password=<random>`. Copy the password — you'll need it in step 6.

- [ ] **Step 5: Verify migration + seed idempotence**

Tear down (keeping the volume) and bring up again:

```bash
cd /Users/logic/Projects/optimus/deploy
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d --build
sleep 15
docker logs optimus-seed | tail -5
```
Expected: seed logs contain `admin user already exists; no password generated`. No new INITIAL line.

- [ ] **Step 6: Verify nginx behavior with curl**

```bash
# Asset cache header
ASSET=$(docker exec optimus-fe sh -c 'ls /usr/share/nginx/html/assets/index-*.js | head -1 | xargs basename')
echo "Testing asset: $ASSET"
curl -sI "http://localhost/assets/$ASSET" | grep -i cache-control
# Expected: Cache-Control: public, max-age=31536000, immutable

# index.html cache header
curl -sI http://localhost/index.html | grep -i cache-control
# Expected: Cache-Control: no-cache, no-store, must-revalidate

# Security headers
curl -sI http://localhost/ | grep -iE '(x-content-type|x-frame|referrer-policy)'
# Expected: three lines

# Gzip
curl -sI -H "Accept-Encoding: gzip" "http://localhost/assets/$ASSET" | grep -i content-encoding
# Expected: Content-Encoding: gzip

# SPA fallback
curl -s -o /dev/null -w "%{http_code}\n" http://localhost/some/deep/spa/route
# Expected: 200
```

- [ ] **Step 7: Verify browser login (manual)**

Open http://localhost in a browser. Log in as `admin` with the password from step 4. Click through Users / Roles / Permissions / Menus / Audit — each page should render without errors. Open DevTools → Network and confirm the chunk names (vendor / antd / utils) all load with `200`.

If any page errors, capture the BE log and the FE network panel screenshot, and stop here to debug.

- [ ] **Step 8: Verify env override**

```bash
# Add a debug log level
cd /Users/logic/Projects/optimus/deploy
sed -i.bak 's/^OPTIMUS_LOG_LEVEL=info/OPTIMUS_LOG_LEVEL=debug/' .env
docker compose -f docker-compose.prod.yml up -d optimus-be
sleep 5
docker logs optimus-be 2>&1 | grep -i '"level":"DEBUG"' | head -2
# Expected: at least one DEBUG line

# Reset
sed -i.bak 's/^OPTIMUS_LOG_LEVEL=debug/OPTIMUS_LOG_LEVEL=info/' .env
rm .env.bak
docker compose -f docker-compose.prod.yml up -d optimus-be
```

- [ ] **Step 9: Verify the regression-free criterion**

```bash
# Dev compose still works
cd /Users/logic/Projects/optimus
docker compose -f docker-compose.yml config > /dev/null && echo "dev compose OK"

# BE quality gate
cd optimus-be
make lint && make test
# Skip make test-int here if it would conflict with the running stack;
# alternatively run it after step 10 once the prod stack is torn down.

# FE quality gate
cd ../optimus-fe
bun run lint && bun run typecheck && bun run test && bun run build
```
Expected: every command exits 0.

- [ ] **Step 10: Tear down the prod stack**

```bash
cd /Users/logic/Projects/optimus/deploy
docker compose -f docker-compose.prod.yml down
# To also wipe the DB volume (useful before the final clean handoff):
# docker compose -f docker-compose.prod.yml down -v
```

- [ ] **Step 11: Update the README with the production deploy section**

Open `README.md` at the repo root. If a "Production deploy" section doesn't exist, append it; if it does (it shouldn't), replace it. The section content:

```markdown
## Production deploy (single-machine, docker-compose)

1. `cd deploy`
2. `cp .env.example .env` and fill in the **REQUIRED** section.
   Generate a JWT secret: `openssl rand -base64 48`
3. `docker compose -f docker-compose.prod.yml up -d --build`
4. Wait for all 5 services to be healthy (~30s on warm cache):
   `docker compose -f docker-compose.prod.yml ps`
5. Verify: `curl -s http://localhost/api/v1/health` should return
   `{"code":0, "data":{"db":"ok","version":"<sha>"}, ...}`.
6. Retrieve the **initial admin password** from the seed logs:
   `docker logs optimus-seed | grep INITIAL`
   (Logged only on the first run; subsequent runs say
   `admin user already exists; no password generated`.)
7. Open http://localhost — log in as `admin` with the password from step 6.

**Useful commands:**

- Logs:  `docker compose -f deploy/docker-compose.prod.yml logs -f optimus-be`
- Stop:  `docker compose -f deploy/docker-compose.prod.yml down`
- Reset DB (destructive): add `-v` to `down`.

**Local docker note:** this workstation typically uses Colima. Set
`DOCKER_HOST=unix:///Users/<you>/.colima/docker.sock` if `docker compose`
can't find a daemon, or run `colima start`.
```

Use this insertion point — append at the end of `README.md` (assuming there isn't already a Deploy section; check first with `grep -i 'production deploy' README.md`).

- [ ] **Step 12: Update the platform-skeleton spec stale references**

Open `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`.

Find §9.6 and change these:

- `FROM golang:1.22-alpine AS build` → `FROM golang:1.25-alpine AS build`
- `COPY package.json bun.lockb ./` → `COPY package.json bun.lock ./`

Find §9.7 and append after its last paragraph:

```markdown
> **Implemented by**: `docs/superpowers/specs/2026-06-09-p0-plan3-deployment-design.md` and `docs/superpowers/plans/2026-06-09-p0-plan3-deployment.md`. The simplified Dockerfile/compose sketches above were superseded by the production-grade multi-stage + 5-service compose stack defined there.
```

Find §9.6 and add the same pointer note at its end.

- [ ] **Step 13: Run a final regression sweep (lint + test, both sides)**

```bash
cd /Users/logic/Projects/optimus/optimus-be
make lint && make test && make test-int

cd /Users/logic/Projects/optimus/optimus-fe
bun run lint && bun run typecheck && bun run test && bun run build
```
Expected: all exit 0.

- [ ] **Step 14: Commit**

```bash
cd /Users/logic/Projects/optimus
git add README.md docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md
git commit -m "$(cat <<'EOF'
docs: production deploy README + supersede notes on P0 skeleton spec

Adds a step-by-step README section that walks an operator from
fresh checkout to logged-in admin in ~3 minutes. Updates the
platform-skeleton design's §9.6/§9.7 to reflect Go 1.25 and bun.lock,
and points readers at the deployment spec that supersedes those
sketches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: CI docker-build job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Append the docker-build job to `.github/workflows/ci.yml`**

Open the file. After the `frontend:` job (last line currently is `        run: bun run build`), add a blank line then the new job:

```yaml

  docker-build:
    name: Docker build
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

- [ ] **Step 2: YAML-validate locally**

```bash
cd /Users/logic/Projects/optimus
# yq isn't guaranteed installed; use python:
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML OK"
```
Expected: `YAML OK`.

- [ ] **Step 3: Verify the workflow shape (no unintended changes to existing jobs)**

```bash
git diff .github/workflows/ci.yml | head -80
```
Expected: only additions at the end of the file; no edits to the `backend` or `frontend` jobs.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
ci: add docker-build job guarding deploy/ Dockerfiles

Runs after backend+frontend pass. Builds all four targets (be-server,
be-migrate, be-seed, fe) using buildx with GHA cache. push=false /
load=false — images stay in build cache, never pushed. Catches
Dockerfile rot on every PR without needing registry credentials.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: (After push) Verify the job runs green on the PR**

After pushing the branch, open the PR's Actions tab. The `Docker build` job should appear under the `ci` workflow alongside `backend` and `frontend`. First run (cold cache) ~5-7 minutes; subsequent runs ~1.5-2 minutes.

If the job fails, examine the logs — common issues:
- `deploy/be.Dockerfile` not found → check `file:` paths are correct.
- `optimus-be/go.sum` not found → check `context: .` (must be repo root).
- Out-of-memory on the runner → buildx + Go can be heavy; consider adding `runs-on: ubuntu-latest-large` (paid feature) only if this becomes a chronic issue.

---

## Self-review notes (from writing-plans skill)

**Spec coverage check** — each spec section maps to at least one task:

| Spec section | Task(s) |
|---|---|
| §3.1 cmd/migrate main.go | Task 3 (with the embed-path fix using Task 2's package) |
| §3.2 ValidateForMigrate | Task 1 |
| §3.3 be.Dockerfile | Task 5 |
| §4.1 vite manualChunks | Task 6 |
| §4.2 fe.Dockerfile | Task 7 |
| §4.3 nginx.conf | Task 7 |
| §5.1 docker-compose.prod.yml | Task 8 |
| §5.2 .env.example | Task 8 |
| §5.3 .dockerignore | Task 8 |
| §6 CI docker-build job | Task 10 |
| §7 README + spec stale-ref edits | Task 9 (steps 11-12) |
| §8.1 end-to-end stack | Task 9 (steps 1-3, 7) |
| §8.2 migration + seed idempotence | Task 9 (steps 4-5) |
| §8.3 bundle split | Task 6 (steps 3-4) |
| §8.4 nginx behavior | Task 9 (step 6) |
| §8.5 env override | Task 9 (step 8) |
| §8.6 CI | Task 10 (step 5) |
| §8.7 regression-free | Task 9 (steps 9, 13) |
| §8.8 migrate smoke test | Task 4 |

**Type/identifier consistency check**:
- `runGoose(db, direction)` in Task 3 (impl) and Task 4 (test) — matches.
- `migrations.FS` in Task 2 (export) and Task 3 (import) — matches.
- `ValidateForMigrate()` in Task 1 (impl) and Task 3 (call site) — matches.
- chunk names `vendor` / `antd` / `utils` are consistent across spec, Task 6, Task 9.

**Plan-spec divergence (intentional)**:
- The plan introduces `optimus-be/migrations/embed.go` as a new tiny package because `//go:embed` in the spec's literal location wouldn't compile. The spec text was schematic. The plan implements the **intent** correctly.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-09-p0-plan3-deployment.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
