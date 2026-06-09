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
