# optimus

Internal DevOps platform — auth/RBAC, encrypted credentials, read-only K8s,
Helm applications, AWS asset discovery, Prometheus observability, and immutable
application delivery. Monorepo: `optimus-be` (Go/Gin/Postgres) and
`optimus-fe` (Vue 3/Ant Design Vue).

## Project status

P0-P6 are implemented and merged to `main` as of 2026-08-18. P6 Application
Delivery completed all 29 planned tasks and passed backend/frontend quality
gates plus a real disposable PostgreSQL + Kubernetes + Helm smoke. PR #5 then
fixed CI reliability and frontend build issues, and PR #6 refined authentication
feedback, built-in RBAC roles, menu metadata, and dynamic-route bootstrap.

Local pre-release validation steps 1-11 pass, covering the local
UI/backend flow, Colima Kubernetes connectivity, backend quality gates, the P3
application lifecycle smoke, and the complete P4 AWS assets smoke. Basic
development environment acceptance has passed. Steps 12 and 13, the P5 and P6
smoke checks, remain pending—not passed or waived. The immediate next milestone
is to start the default full-stack Dev environment on Colima and complete P5
with disposable local Prometheus containers plus P6 with disposable
PostgreSQL/chart resources and an isolated Colima Kubernetes namespace.

PR #7 merged the deployment preparation on 2026-08-18. `main` and
`origin/main` are at `f364af7`; `origin/dev` remains at the merged branch tip
`f555d72`, while local `dev` adds only this progress-documentation refresh.
There was no source diff between that merged branch tip and `main`. CI
published immutable `main-f364af7` backend and frontend images to
both GHCR and Docker Hub with matching cross-registry digests. The unified
backend image also passed a cold Buildx build on a fresh 2C4G Colima VM. No
release tag exists yet. After local P5/P6 acceptance, the next deployment is
UAT before Production. Final release sign-off additionally requires a
production-like persistent-data upgrade smoke from the P5 baseline `4e2d08b`
through migration `00023_p6_delivery.sql`, the deferred P5/P6 checks, and only
then tag/release.

## Repository layout

```
optimus/
├── optimus-be/      Go backend
├── optimus-fe/      Vue 3 frontend
├── docs/            Spec, plans, generated API/permission docs
├── deploy/          Unified Dev/UAT/Prod Docker Compose assets
└── .github/         CI workflows
```

## Local development

Docker Compose commands use the Docker CLI plugin form exclusively. The
Homebrew plugin is available through `~/.docker/cli-plugins/docker-compose`;
verify setup with `docker compose version`.

```bash
# Infrastructure (from the repository root)
cd deploy
docker compose up -d postgres

# Backend
cd ../optimus-be
make tools           # one-off: install air, goose, swag, golangci-lint
export OPTIMUS_JWT_SECRET='dev-only-jwt-secret-at-least-32-bytes'
export OPTIMUS_VAULT_MASTER_KEY='MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY='
make migrate-up
make seed            # prints initial admin password — copy it
make run             # air hot-reload on :8080
```

PostgreSQL listens only on `127.0.0.1:5432`; inspect it with `psql`. The
repository does not run Adminer.

If the former root Compose stack is still running, stop it before starting the
unified stack because both bind port 5432. Removing the old Compose file does
not delete its container or volume. The new `optimus-dev` project uses a fresh
volume by default; set `COMPOSE_PROJECT_NAME=optimus` during migration only if
you deliberately want to reuse the former project's `pgdata` volume.

## Verifying everything

```bash
cd optimus-be && make test test-int lint swagger-diff perm-check
```

CI runs the same matrix; see `.github/workflows/ci.yml`.

## Frontend

The SPA lives in `optimus-fe/`. See [`optimus-fe/README.md`](optimus-fe/README.md) for setup and scripts.

Quick start (with backend already running):

```bash
cd optimus-fe
bun install
bun run dev   # http://localhost:5173, proxies /api/v1 to backend on :8080
```

## Documentation

- Designs and plans: [`docs/superpowers/`](docs/superpowers/)
- P6 design: [`docs/superpowers/specs/2026-07-27-p6-application-delivery-design.md`](docs/superpowers/specs/2026-07-27-p6-application-delivery-design.md)
- P6 plan: [`docs/superpowers/plans/2026-07-27-p6-application-delivery.md`](docs/superpowers/plans/2026-07-27-p6-application-delivery.md)
- P6 disposable smoke: [`optimus-be/scripts/p6-smoke.md`](optimus-be/scripts/p6-smoke.md)
- Permissions: [`docs/permissions.md`](docs/permissions.md)
- API: [`docs/api/swagger.json`](docs/api/swagger.json) (also browsable at http://localhost:8080/swagger/ when running)

## Dev / UAT / Production with Docker Compose

All environments use [deploy/docker-compose.yml](deploy/docker-compose.yml).
Its built-in defaults and [deploy/.env.example](deploy/.env.example) describe
local Dev; UAT and Production provide environment-specific values through an
untracked `deploy/.env`.

### Local Dev

Run the production-shaped full stack using locally built images:

```bash
cd deploy
docker compose up -d --build
docker compose ps -a
```

Expected: `postgres`, `optimus-be`, and `optimus-fe` are healthy; `migrate`
and `seed` exit with status 0. Retrieve the first-run administrator password
with `docker compose logs seed | grep INITIAL`, then open
`http://127.0.0.1:8080`.

For backend/frontend hot reload, start only the database with
`cd deploy && docker compose up -d postgres`, then run `make run` under
`optimus-be/` and `bun run dev` under `optimus-fe/`. PostgreSQL is available to
the host through `127.0.0.1:5432`; use `psql` for database inspection. Adminer
is not included.

### UAT / Production

1. `cd deploy` and run `cp .env.example .env && chmod 600 .env`.
2. Replace every development credential and set:
   - `COMPOSE_PROJECT_NAME=optimus-uat` or `optimus-prod`.
   - `IMAGE_REPOSITORY=ghcr.io/<owner>` or `docker.io/<namespace>`.
   - The immutable `main-<short-sha>` `VERSION` published by CI.
   - The real HTTPS origin, database password, JWT secret, vault key, and
     environment capacity/retention values.
   - Generate the JWT secret with `openssl rand -base64 48`.
   - Generate the vault key once with
     `cd ../optimus-be && go run ./cmd/vault-keygen`, then back it up securely.
3. Log in to the registry and run
   `docker compose pull && docker compose up -d --no-build`.
4. Verify `docker compose ps -a`, then call
   `curl -s http://127.0.0.1:8080/api/v1/health`.

The health response is the raw unauthenticated probe
`{"db":"ok","version":"<sha>"}`; it intentionally does not use the normal API
envelope.

### Container image publishing

CI builds `optimus-be` and `optimus-fe`. The backend image contains the server,
migration, seed, and vault-keygen binaries; Compose runs the first three as
separate services with role-specific entrypoints. Image builds and publishing
run only for pushes to `main`; `dev` pushes and pull requests run the quality
jobs without building container images. A successful `main` run publishes both
images to `ghcr.io/<owner>/...` and Docker Hub. Configure these repository
settings before the first `main` run:

- Secret `DOCKERHUB_TOKEN` (use a Docker Hub access token, not a password).
- Optional secret `DOCKERHUB_USERNAME`; it defaults to the GitHub repository
  owner when the Docker Hub and GHCR usernames are the same.
- Optional variable `DOCKERHUB_NAMESPACE` when the target namespace differs
  from the effective Docker Hub username.

GHCR uses the workflow `GITHUB_TOKEN` with `packages: write`. Each image is
published with the immutable `main-<short-sha>` tag only.

**Useful commands** (run from the repo root):

- Logs: `cd deploy && docker compose logs -f optimus-be`.
- Stop: `cd deploy && docker compose down`.
- Reset DB (destructive): add `-v` to `down`.

**Local Docker note:** this workstation uses Colima. If `docker compose` cannot
find the daemon, run `colima status`, select the `colima` Docker context, and
use the socket reported by Colima through `DOCKER_HOST` only for tools that do
not honor Docker contexts.
