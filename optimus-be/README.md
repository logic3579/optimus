# optimus-be

Go backend for optimus. Gin + GORM + Postgres. P0 scope: auth/RBAC/user/role/permission/menu/audit + /me + /health.

## Layout

```
optimus-be/
├── cmd/
│   ├── server/              main HTTP server
│   ├── seed/                first-run bootstrap (admin + roles + menus)
│   └── dump-permissions/    regenerate docs/permissions.md
├── internal/
│   ├── infra/{config,db,log,errors,response,middleware,crypto,ratelimit,permissions,pagination}/
│   ├── modules/{auth,rbac,user,role,permission,menu,audit,health}/
│   ├── models/              GORM models
│   └── seed/                idempotent first-run logic
├── migrations/              goose SQL
├── api/docs/                generated swagger
└── tests/integration/       dockertest e2e
```

## Build / test / run

```bash
make tools           # one-off
make build           # → bin/optimus-be
make test            # unit + race
make test-int        # dbtest (dockertest brings up its own Postgres per test)
make lint
make swag            # regenerate api/docs/swagger.json + ../docs/api/swagger.json
make dump-perms      # regenerate ../docs/permissions.md
```

## Configuration

`configs/config.yaml` is the default; override via env (`OPTIMUS_*`). At minimum, `OPTIMUS_JWT_SECRET` must be set (≥ 32 bytes) — server refuses to start without it.

## Initial admin

`make seed` (or first `make run` when DB is empty) creates the `admin` user and prints a randomly generated password ONCE to stdout. Save it; you can change it via `PUT /api/v1/me/password` after login.
