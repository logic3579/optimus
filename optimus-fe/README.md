# optimus-fe

P0 frontend for Optimus (Vue 3 + AntdV + Pinia + vue-router + vue-i18n).

## Prerequisites

- bun >= 1.1 (`brew install oven-sh/bun/bun`)
- Backend running on `http://localhost:8080` (see `optimus-be/README.md`)

## Scripts

```bash
bun install              # install deps
bun run dev              # vite dev server at http://localhost:5173
bun run build            # vue-tsc + vite build into ./dist
bun run preview          # preview the production build
bun run lint             # eslint --max-warnings=0
bun run typecheck        # vue-tsc --noEmit
bun run i18n:check       # scripts/check-i18n-keys.ts (missing keys + zh/en symmetry)
bun run test             # vitest run
bun run test:watch       # vitest watch
```

## Architecture notes

- All API requests go through `src/api/client.ts` (axios + single-flight refresh).
- Routes split into static (login/403/404/500/profile) and dynamic (injected from `/me/menus` after first authenticated navigation).
- Permissions enforced two ways: `to.meta.permission` on routes, and `v-permission` on DOM elements.
- i18n keys live in `src/locales/{zh-CN,en-US}.json` and are validated by `bun run i18n:check`.
- Production deployment (nginx + Dockerfile) is Plan 3.

## First-run checklist

1. `cd ../optimus-be && docker compose up -d && make migrate-up && make run`
2. Note the admin password printed once on first boot.
3. `cd ../optimus-fe && bun install && bun run dev`
4. Open http://localhost:5173, log in as admin.
