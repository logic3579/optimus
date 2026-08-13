# optimus

Internal DevOps platform — auth/RBAC, encrypted credentials, read-only K8s,
Helm applications, AWS asset discovery, Prometheus observability, and immutable
application delivery. Monorepo: `optimus-be` (Go/Gin/Postgres) and
`optimus-fe` (Vue 3/Ant Design Vue).

## Project status

P0-P6 are implemented and merged to `main` as of 2026-08-13. P6 Application
Delivery completed all 29 planned tasks and passed backend/frontend quality
gates plus a real disposable PostgreSQL + Kubernetes + Helm smoke. PR #5 then
fixed CI reliability and frontend build issues, and PR #6 refined authentication
feedback, built-in RBAC roles, menu metadata, and dynamic-route bootstrap.

Local pre-release validation steps 1-10 pass on `dev`, covering the local
UI/backend flow, Colima Kubernetes connectivity, and the backend lint,
Swagger-diff, and permission gates. The remaining release-sign-off work is
listed below.

The merged release-candidate baseline on `main` is `db4606a`. The current
`dev` branch adds the cross-platform Colima runtime policy, Colima Kubernetes
P6 smoke path, repository-local backend caches, and refreshed project status.
No release tag exists yet. The next milestone is release sign-off: confirm CI
on `dev`, merge it into `main`, run a production-like persistent-data upgrade
smoke from the P5 baseline `4e2d08b` through migration
`00023_p6_delivery.sql`, rerun the P4/P5/P6 manual release checks, and only then
tag/release.

## Repository layout

```
optimus/
├── optimus-be/      Go backend
├── optimus-fe/      Vue 3 frontend
├── docs/            Spec, plans, generated API/permission docs
├── deploy/          Production Docker Compose assets
├── .github/         CI workflows
└── docker-compose.yml   Local Postgres + Adminer
```

## Local development

Docker Compose commands use the Docker CLI plugin form exclusively. The
Homebrew plugin is available through `~/.docker/cli-plugins/docker-compose`;
verify setup with `docker compose version`.

```bash
# Infrastructure
docker compose up -d

# Backend
cd optimus-be
make tools           # one-off: install air, goose, swag, golangci-lint
export OPTIMUS_JWT_SECRET='dev-secret-must-be-32-bytes-min!!!'
export OPTIMUS_VAULT_MASTER_KEY="$(openssl rand -base64 32)"
make migrate-up
make seed            # prints initial admin password — copy it
make run             # air hot-reload on :8080
```

Adminer at http://localhost:8081 (system: PostgreSQL, server: postgres, user/pw/db: optimus).

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

## Production deploy (single-machine, Docker Compose)

1. `cd deploy`
2. `cp .env.example .env` and fill in the **REQUIRED** section.
   - JWT secret: `openssl rand -base64 48`
   - **P1 credentials-vault master key** (32-byte AES-256, base64): generate
     ONCE and back it up safely — losing it makes all encrypted credentials
     unrecoverable. Pick one of:
     - `cd ../optimus-be && KEY=$(go run ./cmd/vault-keygen) && echo "OPTIMUS_VAULT_MASTER_KEY=$KEY" >> ../deploy/.env`
     - `docker run --rm $(docker build -q -f deploy/be.Dockerfile --target vault-keygen ..) >> .env` (then prepend `OPTIMUS_VAULT_MASTER_KEY=`)
     - File mode: write the key to `/etc/optimus/vault.key`, `chmod 0400`, then in `.env` set `OPTIMUS_VAULT_MASTER_KEY_FILE=/etc/optimus/vault.key` and add a bind mount under `optimus-be.volumes` in `docker-compose.prod.yml`.
3. `docker compose -f docker-compose.prod.yml up -d --build`
4. Wait until the 3 long-running services are `healthy` and the 2 init containers have `Exited (0)` (~30s on warm cache):
   `docker compose -f docker-compose.prod.yml ps`
   Expected: `optimus-pg` healthy, `optimus-migrate` exited (0), `optimus-seed` exited (0), `optimus-be` healthy, `optimus-fe` healthy.
5. Verify: `curl -s http://localhost/api/v1/health` should return
   `{"code":0, "data":{"db":"ok","version":"<sha>"}, ...}`.
6. Retrieve the **initial admin password** from the seed logs:
   `docker logs optimus-seed | grep INITIAL`
   (Logged only on the first run; subsequent runs say
   `admin user already exists; no password generated`.)
7. Open http://localhost — log in as `admin` with the password from step 6.

**Useful commands** (run from the repo root):

- Logs:  `docker compose -f deploy/docker-compose.prod.yml logs -f optimus-be`
- Stop:  `docker compose -f deploy/docker-compose.prod.yml down`
- Reset DB (destructive): add `-v` to `down`.

**Local Docker note:** this workstation uses Colima. If `docker compose` cannot
find the daemon, run `colima status`, select the `colima` Docker context, and
use the socket reported by Colima through `DOCKER_HOST` only for tools that do
not honor Docker contexts.
