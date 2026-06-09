# optimus

Internal DevOps platform — auth, RBAC, K8s, applications, observability. Monorepo: `optimus-be` (Go/Gin/Postgres), `optimus-fe` (Vue3/AntdV).

## Repository layout

```
optimus/
├── optimus-be/      Go backend
├── optimus-fe/      Vue3 frontend (P0 Plan 2a)
├── docs/            Spec, plans, generated API/permission docs
├── deploy/          Production Docker assets (TBD — P0 Plan 3)
├── .github/         CI workflows
└── docker-compose.yml   Local Postgres + Adminer
```

## Local development

```bash
# Infrastructure
docker compose up -d

# Backend
cd optimus-be
make tools           # one-off: install air, goose, swag, golangci-lint
export OPTIMUS_JWT_SECRET='dev-secret-must-be-32-bytes-min!!!'
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

## Frontend (Plan 2a)

The SPA lives in `optimus-fe/`. See [`optimus-fe/README.md`](optimus-fe/README.md) for setup and scripts.

Quick start (with backend already running):

```bash
cd optimus-fe
bun install
bun run dev   # http://localhost:5173, proxies /api/v1 to backend on :8080
```

## Documentation

- Spec: [`docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`](docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md)
- Permissions: [`docs/permissions.md`](docs/permissions.md)
- API: [`docs/api/swagger.json`](docs/api/swagger.json) (also browsable at http://localhost:8080/swagger/ when running)

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

**Useful commands** (run from the repo root):

- Logs:  `docker compose -f deploy/docker-compose.prod.yml logs -f optimus-be`
- Stop:  `docker compose -f deploy/docker-compose.prod.yml down`
- Reset DB (destructive): add `-v` to `down`.

**Local docker note:** this workstation typically uses Colima. Set
`DOCKER_HOST=unix:///Users/<you>/.colima/docker.sock` if `docker compose`
can't find a daemon, or run `colima start`.
