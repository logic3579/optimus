# P0 Plan 2a — Frontend skeleton + auth + layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the `optimus-fe/` SPA skeleton (Vite + Vue 3 + AntdV + Pinia + vue-router + vue-i18n + axios) so that an admin can log in, see the seeded menu tree with Coming-soon placeholders, edit their profile, change password, switch language/theme, and have access-token refresh + permission-based route guards working end-to-end.

**Architecture:** Pure SPA hosted via Vite dev server in development and nginx (Plan 3) in production. Talks to `optimus-be` at `/api/v1/*` via axios with a single-flight refresh interceptor. Pinia stores hold auth/menu/app state with `pinia-plugin-persistedstate` to survive page reloads. Routes split into static (login / 403 / 404 / 500 / profile) and dynamic (injected at first authenticated navigation from `/me/menus`, mapped to vue files via `import.meta.glob`). Per the §8 addendum: zero ProTable/ProForm wrappers, hand-written TS types, scoped SCSS only, TDD layered (utils/stores/hooks/api-client/directives/scripts/router-pure only). This plan also closes two backend gaps left over from Plan 1B: `PUT /me` and `PUT /me/password`.

**Tech Stack:** Vite 5 · Vue 3.4 · TypeScript 5 · ant-design-vue 4 · @ant-design/icons-vue · pinia + pinia-plugin-persistedstate · vue-router 4 · vue-i18n 9 · axios · SCSS (no preprocessor plugin needed; Vite native) · ESLint flat config (eslint-plugin-vue + @typescript-eslint + eslint-config-prettier) · vue-tsc · vitest + jsdom · bun ≥ 1.1 · backend Go 1.22+ (for Tasks 1-3).

**Spec references:**
- `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md` §8
- `docs/superpowers/specs/2026-06-08-p0-plan2a-fe-design-addendum.md` (binding precise decisions)

**Implementation notes for the agent:**
- All commands run from repo root unless noted. Frontend tasks use `cd optimus-fe` (or `git -C optimus-fe`).
- Use `bun` everywhere (never `npm`/`pnpm`/`yarn`). For docker compose, use the legacy `docker-compose` plugin only if `docker compose` is not installed — `project_colima_docker_socket` memory.
- The repository's `.git` lives at `/Users/logic/Projects/optimus/.git`. Backend root = `optimus-be/`.
- One task = one commit unless the task explicitly says otherwise.
- Each FE batch ends with the full sweep: `cd optimus-fe && bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build`. Whenever a task's verify step omits one of these, the agent should still run the whole sweep at the end of that subagent batch.

---

## File Structure Overview

**Backend (Tasks 1-3) — modify**
```
optimus-be/internal/modules/rbac/handler.go     (+ PUT /me, PUT /me/password)
optimus-be/internal/modules/rbac/handler_test.go (handler-level cases)
optimus-be/internal/modules/rbac/service.go     (+ UpdateMe, ChangeMyPassword adapters)
optimus-be/internal/modules/rbac/service_test.go
optimus-be/internal/modules/rbac/dto.go         (+ UpdateMeRequest, ChangePasswordRequest)
optimus-be/internal/modules/user/service.go     (+ ChangePassword method that verifies old)
optimus-be/internal/modules/user/service_test.go
optimus-be/internal/api/routes.go (or cmd/server/main.go) (wire two new routes)
optimus-be/api/docs/{docs.go,swagger.json,swagger.yaml} (swag init)
docs/api/swagger.json                            (mirror)
```

**Frontend (Tasks 4-21) — create from scratch**
```
optimus-fe/
├── .editorconfig
├── .env.development
├── .env.production
├── .eslintignore
├── .gitignore
├── .prettierrc                  (only to feed eslint-config-prettier; not actually run)
├── README.md
├── bun.lockb                    (auto-generated)
├── eslint.config.js
├── index.html
├── package.json
├── tsconfig.json
├── tsconfig.node.json
├── vite.config.ts
├── scripts/
│   ├── check-i18n-keys.ts
│   └── check-i18n-keys.test.ts
└── src/
    ├── App.vue
    ├── env.d.ts
    ├── main.ts
    ├── api/
    │   ├── auth.ts
    │   ├── client.ts
    │   ├── client.test.ts
    │   └── me.ts
    ├── assets/styles/
    │   └── utilities.scss
    ├── components/
    │   ├── AppHeader.vue
    │   ├── AppSidebar.vue
    │   ├── ConfirmButton.vue
    │   ├── LangSwitch.vue
    │   ├── PageHeader.vue
    │   └── ThemeToggle.vue
    ├── directives/
    │   ├── index.ts
    │   ├── permission.ts
    │   └── permission.test.ts
    ├── hooks/
    │   ├── useI18n.ts
    │   ├── usePermission.ts
    │   ├── usePermission.test.ts
    │   ├── useTable.ts
    │   └── useTable.test.ts
    ├── layouts/
    │   ├── BlankLayout.vue
    │   └── DefaultLayout.vue
    ├── locales/
    │   ├── en-US.json
    │   ├── index.ts
    │   └── zh-CN.json
    ├── router/
    │   ├── dynamic-routes.ts
    │   ├── dynamic-routes.test.ts
    │   ├── guards.ts
    │   ├── index.ts
    │   └── static-routes.ts
    ├── stores/
    │   ├── app.ts
    │   ├── app.test.ts
    │   ├── auth.ts
    │   ├── auth.test.ts
    │   ├── index.ts
    │   ├── menu.ts
    │   └── menu.test.ts
    ├── types/
    │   └── api.ts
    ├── utils/
    │   ├── http-error.ts
    │   ├── http-error.test.ts
    │   ├── permission.ts
    │   ├── permission.test.ts
    │   ├── token.ts
    │   └── token.test.ts
    └── views/
        ├── auth/Login.vue
        ├── dashboard/Index.vue
        ├── errors/{403,404,500}.vue
        ├── profile/Index.vue
        └── system/{users,roles,permissions,menus,audit-logs}/List.vue
```

**Root — modify**
```
.github/workflows/ci.yml        (+ frontend job)
README.md                       (+ frontend section)
```

---

## Subagent Batching

| Batch | Tasks | Focus |
|---|---|---|
| 1 | 1-3 | Backend `/me` write endpoints |
| 2 | 4-6 | FE scaffold + vite + locales |
| 3 | 7-9 | Types + TDD utils |
| 4 | 10-11 | Stores + axios single-flight |
| 5 | 12-14 | API modules + hooks + directives |
| 6 | 15-16 | i18n script + router |
| 7 | 17-19 | Layouts + components + auth/error views |
| 8 | 20-21 | Pages + main.ts wiring + CI + README |

---

## Task 1: Backend — user.Service.ChangePassword (TDD)

**Files:**
- Modify: `optimus-be/internal/modules/user/service.go`
- Modify: `optimus-be/internal/modules/user/service_test.go`

- [ ] **Step 1: Write failing test for ChangePassword**

Append to `optimus-be/internal/modules/user/service_test.go`:

```go
func TestService_ChangePassword_OK(t *testing.T) {
	deps := setupServiceTest(t)
	defer deps.cleanup()

	id, _ := deps.svc.Create(deps.ctx, 1, "1.1.1.1", "ua", CreateRequest{
		Username: "alice", Email: "alice@example.com", Password: "oldpass1234",
		DisplayName: "Alice",
	})
	require.NotZero(t, id)

	require.NoError(t, deps.svc.ChangePassword(deps.ctx, id, "1.1.1.1", "ua", "oldpass1234", "newpass5678"))

	// old password no longer works; new one does
	var u models.User
	require.NoError(t, deps.db.First(&u, id).Error)
	require.Error(t, crypto.ComparePassword(u.PasswordHash, "oldpass1234"))
	require.NoError(t, crypto.ComparePassword(u.PasswordHash, "newpass5678"))
}

func TestService_ChangePassword_WrongOld(t *testing.T) {
	deps := setupServiceTest(t)
	defer deps.cleanup()

	id, _ := deps.svc.Create(deps.ctx, 1, "1.1.1.1", "ua", CreateRequest{
		Username: "bob", Email: "bob@example.com", Password: "rightpass00",
	})
	err := deps.svc.ChangePassword(deps.ctx, id, "1.1.1.1", "ua", "wrongpass", "newpass5678")
	require.Error(t, err)
	var be *apperr.BizError
	require.ErrorAs(t, err, &be)
	require.Equal(t, apperr.CodeInvalidCredential, be.Code)
}
```

If a `crypto` import is missing in the test file, add `"optimus-be/internal/infra/crypto"`.

- [ ] **Step 2: Run test, verify it fails**

```bash
cd optimus-be && go test ./internal/modules/user/... -run TestService_ChangePassword -count=1
```

Expected: FAIL — method `ChangePassword` undefined on `*Service`.

- [ ] **Step 3: Implement ChangePassword**

In `optimus-be/internal/modules/user/service.go`, add below `SetPassword`:

```go
// ChangePassword verifies oldPassword then updates the hash.
// Distinct from SetPassword (admin reset) — it requires old credential and audits as user.change_password.
func (s *Service) ChangePassword(ctx context.Context, userID uint64, ip, ua, oldPassword, newPassword string) error {
	u, err := s.repo.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return err
	}
	if err := crypto.ComparePassword(u.PasswordHash, oldPassword); err != nil {
		return apperr.New(apperr.CodeInvalidCredential, "auth.invalid_credentials", "invalid old password")
	}
	hash, err := crypto.HashPassword(newPassword, s.opts.BcryptCost)
	if err != nil {
		return err
	}
	if err := s.repo.Update(ctx, userID, map[string]any{"password_hash": hash}); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &userID, Action: "user.change_password", TargetType: "user", TargetID: uintToStr(userID),
		IP: ip, UserAgent: ua,
	})
	return nil
}
```

Ensure imports in `service.go` include `"optimus-be/internal/infra/crypto"` (it may already; if not, add).

- [ ] **Step 4: Run test, verify it passes**

```bash
cd optimus-be && go test ./internal/modules/user/... -run TestService_ChangePassword -count=1
```

Expected: PASS.

- [ ] **Step 5: Run full module tests + lint**

```bash
cd optimus-be && go test ./internal/modules/user/... -count=1 && golangci-lint run ./internal/modules/user/...
```

Expected: PASS, no lint diagnostics.

- [ ] **Step 6: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-be/internal/modules/user/service.go optimus-be/internal/modules/user/service_test.go
git -C /Users/logic/Projects/optimus commit -m "feat(be/user): add ChangePassword with old-credential verification"
```

---

## Task 2: Backend — rbac.UpdateMe + ChangeMyPassword service adapters (TDD)

**Files:**
- Modify: `optimus-be/internal/modules/rbac/dto.go`
- Modify: `optimus-be/internal/modules/rbac/service.go`
- Modify: `optimus-be/internal/modules/rbac/service_test.go`

`MeService` in `rbac` becomes the orchestration layer for `/me` writes — it depends on `user.Service` for the actual mutations so the existing audit + bcrypt + validation rules are reused untouched.

- [ ] **Step 1: Extend MeService dependency on user.Service**

Edit `optimus-be/internal/modules/rbac/service.go`:

```go
package rbac

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/user"
)

type MeService struct {
	db    *gorm.DB
	cache *PermissionCache
	users *user.Service
}

func NewMeService(db *gorm.DB, cache *PermissionCache, users *user.Service) *MeService {
	return &MeService{db: db, cache: cache, users: users}
}
```

All existing call sites of `NewMeService` need to pass `*user.Service` — fix `cmd/server/main.go` accordingly (defer to Task 3's route-wiring step but the constructor signature change is part of this commit).

- [ ] **Step 2: Add DTOs for /me write requests**

Append to `optimus-be/internal/modules/rbac/dto.go`:

```go
// UpdateMeRequest is the body for PUT /me.
type UpdateMeRequest struct {
	Email       *string `json:"email"        binding:"omitempty,email,max=128"`
	DisplayName *string `json:"display_name" binding:"omitempty,max=128"`
	AvatarURL   *string `json:"avatar_url"   binding:"omitempty,max=512"`
}

// ChangePasswordRequest is the body for PUT /me/password.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=1,max=128"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}
```

- [ ] **Step 3: Write failing tests for the two new adapters**

Append to `optimus-be/internal/modules/rbac/service_test.go`:

```go
func TestMeService_UpdateMe_OK(t *testing.T) {
	d := setupMeServiceTest(t)
	defer d.cleanup()

	uid := d.seedUser(t, "alice", "alice@example.com")
	email := "alice2@example.com"
	display := "Alice Cooper"
	dto, err := d.svc.UpdateMe(d.ctx, uid, "1.1.1.1", "ua", UpdateMeRequest{Email: &email, DisplayName: &display})
	require.NoError(t, err)
	require.Equal(t, "alice2@example.com", dto.Email)
	require.Equal(t, "Alice Cooper", dto.DisplayName)
}

func TestMeService_ChangeMyPassword_OK(t *testing.T) {
	d := setupMeServiceTest(t)
	defer d.cleanup()

	uid := d.seedUserWithPassword(t, "alice", "alice@example.com", "oldpass1234")
	require.NoError(t, d.svc.ChangeMyPassword(d.ctx, uid, "1.1.1.1", "ua", "oldpass1234", "newpass5678"))
}

func TestMeService_ChangeMyPassword_WrongOld(t *testing.T) {
	d := setupMeServiceTest(t)
	defer d.cleanup()

	uid := d.seedUserWithPassword(t, "alice", "alice@example.com", "rightpass00")
	err := d.svc.ChangeMyPassword(d.ctx, uid, "1.1.1.1", "ua", "wrongpass", "newpass5678")
	require.Error(t, err)
}
```

If `setupMeServiceTest` does not exist yet, add it modeled on the existing `service_test.go` patterns: spin a dbtest schema, register permissions, build `user.Service` + `MeService`, expose helpers `seedUser` and `seedUserWithPassword`. (Use the same fixtures the file already employs; this is local plumbing, not new shared infra.)

- [ ] **Step 4: Run test, verify it fails**

```bash
cd optimus-be && go test ./internal/modules/rbac/... -run "TestMeService_(UpdateMe|ChangeMyPassword)" -tags=dbtest -count=1
```

Expected: FAIL — methods undefined.

- [ ] **Step 5: Implement UpdateMe and ChangeMyPassword on MeService**

Append to `optimus-be/internal/modules/rbac/service.go`:

```go
// UpdateMe applies partial profile edits and returns the refreshed Me DTO.
func (s *MeService) UpdateMe(ctx context.Context, userID uint64, ip, ua string, req UpdateMeRequest) (*MeUserDTO, error) {
	if err := s.users.Update(ctx, userID, ip, ua, userID, user.UpdateRequest{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		AvatarURL:   req.AvatarURL,
	}); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, userID)
}

// ChangeMyPassword verifies the user's old password and rotates the hash.
func (s *MeService) ChangeMyPassword(ctx context.Context, userID uint64, ip, ua, oldPassword, newPassword string) error {
	return s.users.ChangePassword(ctx, userID, ip, ua, oldPassword, newPassword)
}
```

If `user.Service.Update`'s signature uses positional actorID/targetID differently than `(ctx, actorID, ip, ua, targetID, req)`, adapt the call — the spec for user.Service.Update in this repo is exactly that signature (verified by reading `internal/modules/user/service.go` line ~107).

- [ ] **Step 6: Run test, verify it passes**

```bash
cd optimus-be && go test ./internal/modules/rbac/... -tags=dbtest -count=1
```

Expected: PASS for the new cases; all existing rbac dbtests still PASS.

- [ ] **Step 7: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-be/internal/modules/rbac/dto.go optimus-be/internal/modules/rbac/service.go optimus-be/internal/modules/rbac/service_test.go
git -C /Users/logic/Projects/optimus commit -m "feat(be/rbac): MeService.UpdateMe + ChangeMyPassword adapters"
```

---

## Task 3: Backend — register PUT /me and PUT /me/password + regen swagger

**Files:**
- Modify: `optimus-be/internal/modules/rbac/handler.go`
- Modify: `optimus-be/internal/modules/rbac/handler_test.go`
- Modify: `optimus-be/cmd/server/main.go` (NewMeService construction)
- Regen: `optimus-be/api/docs/{docs.go,swagger.json,swagger.yaml}` + mirror `docs/api/swagger.json`

- [ ] **Step 1: Add handler methods to rbac/handler.go**

Edit `optimus-be/internal/modules/rbac/handler.go`, register two new routes and the methods:

```go
func (h *Handler) RegisterMe(g *gin.RouterGroup) {
	g.GET("/me", h.getMe)
	g.PUT("/me", h.updateMe)
	g.PUT("/me/password", h.changeMyPassword)
	g.GET("/me/menus", h.getMyMenus)
	g.GET("/me/permissions", h.getMyPermissions)
}

// updateMe applies the authenticated user's profile edits.
// @Summary  Update current user profile
// @Tags     me
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body UpdateMeRequest true "profile patch"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Router   /me [put]
func (h *Handler) updateMe(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	var req UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.Wrap(err, apperr.CodeInvalidArgument, "common.invalid_argument", "invalid request body"))
		return
	}
	dto, err := h.svc.UpdateMe(c.Request.Context(), uid, c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, dto)
}

// changeMyPassword changes the authenticated user's password (requires old).
// @Summary  Change current user password
// @Tags     me
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body ChangePasswordRequest true "old/new password"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Router   /me/password [put]
func (h *Handler) changeMyPassword(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.Wrap(err, apperr.CodeInvalidArgument, "common.invalid_argument", "invalid request body"))
		return
	}
	if err := h.svc.ChangeMyPassword(c.Request.Context(), uid, c.ClientIP(), c.Request.UserAgent(), req.OldPassword, req.NewPassword); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, nil)
}
```

- [ ] **Step 2: Wire user.Service into NewMeService in main.go**

In `optimus-be/cmd/server/main.go`, locate the `rbac.NewMeService(db, cache)` call and add the `user.Service` arg. Adjust ordering: `user.NewService` must be built before `rbac.NewMeService`. Example skeleton (adapt to whatever the file actually looks like):

```go
userSvc := user.NewService(userRepo, permCache, auditRecorder, user.ServiceOptions{BcryptCost: cfg.Auth.BcryptCost})
// ... then:
meSvc := rbac.NewMeService(db, permCache, userSvc)
```

If the constructor needs are met by an existing `userSvc` (Plan 1C wired it), just append the arg.

- [ ] **Step 3: Add handler-level tests**

Append to `optimus-be/internal/modules/rbac/handler_test.go`:

```go
func TestHandler_UpdateMe_OK(t *testing.T) {
	h := buildMeHandlerHarness(t)
	defer h.cleanup()

	uid, token := h.loginUser(t, "alice", "alice@example.com", "oldpass1234")
	body := `{"display_name":"Alice C","email":"alice2@example.com"}`
	w := h.do(t, "PUT", "/api/v1/me", body, token)

	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"display_name":"Alice C"`)
	_ = uid
}

func TestHandler_ChangeMyPassword_WrongOld(t *testing.T) {
	h := buildMeHandlerHarness(t)
	defer h.cleanup()

	_, token := h.loginUser(t, "bob", "bob@example.com", "rightpass00")
	w := h.do(t, "PUT", "/api/v1/me/password", `{"old_password":"wrong","new_password":"newpass5678"}`, token)

	require.NotEqual(t, 200, w.Code)
}
```

`buildMeHandlerHarness` should follow the patterns already in `handler_test.go` (full Engine wired with JWTAuth + audit + cache). If you find scaffolding helpers already present (`buildHandlerHarness` etc.), reuse them.

- [ ] **Step 4: Run new handler tests**

```bash
cd optimus-be && go test ./internal/modules/rbac/... -tags=dbtest -count=1 -run "TestHandler_(UpdateMe|ChangeMyPassword)"
```

Expected: PASS.

- [ ] **Step 5: Regenerate swagger + mirror**

```bash
cd optimus-be && make swag && cp api/docs/swagger.json ../docs/api/swagger.json
```

`make swag` runs `swag init -g cmd/server/main.go -o api/docs`. Verify the swagger contains the two new operations:

```bash
grep -E '"(/me|/me/password)"' optimus-be/api/docs/swagger.json | head
```

Expected: both `/me` (now with put) and `/me/password` paths listed.

- [ ] **Step 6: Verify CI hooks still pass**

```bash
cd optimus-be && make swagger-diff && make perm-check
```

Expected: both silent (exit 0). `/me` writes do not need new permission codes so `perm-check` should be unchanged.

- [ ] **Step 7: Full backend regression**

```bash
cd optimus-be && go test ./... -count=1 && go test ./... -tags=dbtest -count=1 && golangci-lint run ./...
```

Expected: all green.

- [ ] **Step 8: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-be/internal/modules/rbac/handler.go optimus-be/internal/modules/rbac/handler_test.go optimus-be/cmd/server/main.go optimus-be/api/docs docs/api/swagger.json
git -C /Users/logic/Projects/optimus commit -m "feat(be/rbac): PUT /me and PUT /me/password + swagger regen"
```

---

## Task 4: FE — scaffold project (Vite + Vue 3 + TS + ESLint)

**Files:**
- Create: `optimus-fe/.editorconfig`
- Create: `optimus-fe/.gitignore`
- Create: `optimus-fe/.eslintignore`
- Create: `optimus-fe/eslint.config.js`
- Create: `optimus-fe/index.html`
- Create: `optimus-fe/package.json`
- Create: `optimus-fe/tsconfig.json`
- Create: `optimus-fe/tsconfig.node.json`
- Create: `optimus-fe/src/env.d.ts`
- Create: `optimus-fe/bun.lockb` (auto)
- Modify: `.gitignore` (root) — add `optimus-fe/node_modules` and `optimus-fe/dist`

- [ ] **Step 1: Make optimus-fe directory and seed files**

```bash
mkdir -p optimus-fe/src
```

`optimus-fe/package.json`:

```json
{
  "name": "optimus-fe",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc --noEmit && vite build",
    "preview": "vite preview --port 5173",
    "lint": "eslint . --max-warnings=0",
    "typecheck": "vue-tsc --noEmit",
    "test": "vitest run",
    "test:watch": "vitest",
    "i18n:check": "bun scripts/check-i18n-keys.ts"
  },
  "dependencies": {
    "ant-design-vue": "^4.2.3",
    "@ant-design/icons-vue": "^7.0.1",
    "axios": "^1.7.7",
    "dayjs": "^1.11.13",
    "pinia": "^2.2.4",
    "pinia-plugin-persistedstate": "^4.1.2",
    "vue": "^3.4.38",
    "vue-i18n": "^9.14.0",
    "vue-router": "^4.4.5"
  },
  "devDependencies": {
    "@types/node": "^22.7.4",
    "@typescript-eslint/eslint-plugin": "^8.8.0",
    "@typescript-eslint/parser": "^8.8.0",
    "@vitejs/plugin-vue": "^5.1.4",
    "@vue/test-utils": "^2.4.6",
    "eslint": "^9.11.1",
    "eslint-config-prettier": "^9.1.0",
    "eslint-plugin-vue": "^9.28.0",
    "happy-dom": "^15.7.4",
    "jsdom": "^25.0.1",
    "sass": "^1.79.4",
    "typescript": "^5.6.2",
    "vite": "^5.4.8",
    "vitest": "^2.1.2",
    "vue-eslint-parser": "^9.4.3",
    "vue-tsc": "^2.1.6"
  }
}
```

`optimus-fe/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "jsx": "preserve",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitOverride": true,
    "noFallthroughCasesInSwitch": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "resolveJsonModule": true,
    "allowImportingTsExtensions": false,
    "isolatedModules": true,
    "verbatimModuleSyntax": true,
    "noEmit": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    },
    "types": ["vite/client", "node"]
  },
  "include": ["src/**/*", "scripts/**/*", "*.config.ts", "*.config.js"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

`optimus-fe/tsconfig.node.json`:

```json
{
  "compilerOptions": {
    "composite": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "target": "ES2022",
    "strict": true,
    "skipLibCheck": true,
    "types": ["node"]
  },
  "include": ["vite.config.ts", "scripts/**/*"]
}
```

`optimus-fe/src/env.d.ts`:

```ts
/// <reference types="vite/client" />

declare module '*.vue' {
  import { DefineComponent } from 'vue'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface ImportMetaEnv {
  readonly VITE_APP_TITLE: string
  readonly VITE_API_BASE_URL: string
}
interface ImportMeta {
  readonly env: ImportMetaEnv
}
```

`optimus-fe/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Optimus</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

`optimus-fe/.gitignore`:

```
node_modules
dist
*.log
.DS_Store
```

`optimus-fe/.editorconfig`:

```
root = true

[*]
charset = utf-8
end_of_line = lf
indent_style = space
indent_size = 2
insert_final_newline = true
trim_trailing_whitespace = true
```

`optimus-fe/.eslintignore`:

```
dist
node_modules
```

`optimus-fe/eslint.config.js` (flat config):

```js
import vue from 'eslint-plugin-vue'
import vueParser from 'vue-eslint-parser'
import tsParser from '@typescript-eslint/parser'
import tsPlugin from '@typescript-eslint/eslint-plugin'
import prettier from 'eslint-config-prettier'

export default [
  ...vue.configs['flat/recommended'],
  prettier,
  {
    files: ['**/*.vue', '**/*.ts'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tsParser,
        sourceType: 'module',
        ecmaVersion: 'latest',
        extraFileExtensions: ['.vue']
      }
    },
    plugins: {
      '@typescript-eslint': tsPlugin
    },
    rules: {
      'vue/multi-word-component-names': 'off',
      'vue/no-v-html': 'off',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      '@typescript-eslint/consistent-type-imports': 'error',
      '@typescript-eslint/no-explicit-any': 'warn'
    }
  },
  {
    ignores: ['dist/**', 'node_modules/**']
  }
]
```

- [ ] **Step 2: Install dependencies**

```bash
cd optimus-fe && bun install
```

Expected: `bun.lockb` created; `node_modules` populated; no errors.

- [ ] **Step 3: Add /optimus-fe ignores to root .gitignore**

Read root `.gitignore`; if it does not already exclude `optimus-fe/node_modules` and `optimus-fe/dist`, append:

```
# Frontend
optimus-fe/node_modules/
optimus-fe/dist/
```

- [ ] **Step 4: Smoke-check toolchain**

```bash
cd optimus-fe && bun run lint || true
```

`lint` will currently fail with "no files match the pattern" because src is empty — that's fine. We're only confirming binaries resolve.

```bash
cd optimus-fe && bunx vue-tsc --version && bunx vite --version
```

Expected: both print versions.

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/.editorconfig optimus-fe/.gitignore optimus-fe/.eslintignore optimus-fe/eslint.config.js optimus-fe/index.html optimus-fe/package.json optimus-fe/tsconfig.json optimus-fe/tsconfig.node.json optimus-fe/src/env.d.ts optimus-fe/bun.lockb .gitignore
git -C /Users/logic/Projects/optimus commit -m "feat(fe): scaffold optimus-fe with Vite + Vue 3 + TS + ESLint flat config"
```

---

## Task 5: FE — vite config, env files, SCSS utilities

**Files:**
- Create: `optimus-fe/vite.config.ts`
- Create: `optimus-fe/.env.development`
- Create: `optimus-fe/.env.production`
- Create: `optimus-fe/src/assets/styles/utilities.scss`

- [ ] **Step 1: vite.config.ts**

```ts
import { defineConfig } from 'vite'
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
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: []
  }
})
```

- [ ] **Step 2: .env.development**

```
VITE_APP_TITLE=Optimus (dev)
VITE_API_BASE_URL=/api/v1
```

- [ ] **Step 3: .env.production**

```
VITE_APP_TITLE=Optimus
VITE_API_BASE_URL=/api/v1
```

- [ ] **Step 4: utilities.scss (small atomic helpers since we said no Tailwind)**

```scss
.u-flex { display: flex; }
.u-flex-1 { flex: 1; }
.u-gap-8 { gap: 8px; }
.u-gap-16 { gap: 16px; }
.u-mt-16 { margin-top: 16px; }
.u-mb-16 { margin-bottom: 16px; }
.u-w-full { width: 100%; }
.u-text-center { text-align: center; }
```

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/vite.config.ts optimus-fe/.env.development optimus-fe/.env.production optimus-fe/src/assets/styles/utilities.scss
git -C /Users/logic/Projects/optimus commit -m "feat(fe): vite config with /api/v1 proxy + scss utilities"
```

---

## Task 6: FE — locales scaffolding (zh-CN + en-US + index)

**Files:**
- Create: `optimus-fe/src/locales/zh-CN.json`
- Create: `optimus-fe/src/locales/en-US.json`
- Create: `optimus-fe/src/locales/index.ts`

The initial key set is the union of what later tasks reference plus the menu/permission keys that backend already produces. Subsequent tasks add new keys to both files in the same commit.

- [ ] **Step 1: zh-CN.json**

```json
{
  "common": {
    "ok": "确定",
    "cancel": "取消",
    "save": "保存",
    "submit": "提交",
    "reset": "重置",
    "loading": "加载中…",
    "logout": "退出登录",
    "profile": "个人资料",
    "language": "语言",
    "theme": "主题",
    "theme_light": "浅色",
    "theme_dark": "深色"
  },
  "placeholder": {
    "coming_soon": "敬请期待"
  },
  "auth": {
    "login_title": "登录 Optimus",
    "username": "用户名",
    "password": "密码",
    "login": "登录",
    "invalid_credentials": "用户名或密码错误"
  },
  "profile": {
    "title": "个人资料",
    "display_name": "显示名",
    "email": "邮箱",
    "avatar_url": "头像 URL",
    "old_password": "原密码",
    "new_password": "新密码",
    "confirm_password": "确认新密码",
    "password_mismatch": "两次输入的新密码不一致",
    "update_ok": "资料已更新",
    "password_changed": "密码已修改",
    "change_password": "修改密码"
  },
  "errors": {
    "403_title": "无权访问",
    "404_title": "页面不存在",
    "500_title": "服务器错误",
    "back_home": "返回首页"
  },
  "network": {
    "error": "网络错误，请稍后再试"
  },
  "menu": {
    "dashboard": "仪表盘",
    "system": "系统管理",
    "system.users": "用户管理",
    "system.roles": "角色管理",
    "system.permissions": "权限列表",
    "system.menus": "菜单管理",
    "system.audit_logs": "操作审计"
  },
  "perm": {
    "system": {
      "user": { "read": "查看用户", "write": "新建/修改用户", "delete": "删除用户", "reset_pass": "重置用户密码" },
      "role": { "read": "查看角色", "write": "新建/修改角色及绑权", "delete": "删除角色" },
      "permission": { "read": "查看权限注册表" },
      "menu": { "read": "查看菜单", "write": "新建/修改菜单", "delete": "删除菜单" },
      "audit": { "read": "查看操作审计" }
    }
  }
}
```

- [ ] **Step 2: en-US.json**

```json
{
  "common": {
    "ok": "OK",
    "cancel": "Cancel",
    "save": "Save",
    "submit": "Submit",
    "reset": "Reset",
    "loading": "Loading…",
    "logout": "Log out",
    "profile": "Profile",
    "language": "Language",
    "theme": "Theme",
    "theme_light": "Light",
    "theme_dark": "Dark"
  },
  "placeholder": {
    "coming_soon": "Coming soon"
  },
  "auth": {
    "login_title": "Sign in to Optimus",
    "username": "Username",
    "password": "Password",
    "login": "Sign in",
    "invalid_credentials": "Invalid username or password"
  },
  "profile": {
    "title": "Profile",
    "display_name": "Display name",
    "email": "Email",
    "avatar_url": "Avatar URL",
    "old_password": "Current password",
    "new_password": "New password",
    "confirm_password": "Confirm new password",
    "password_mismatch": "Passwords do not match",
    "update_ok": "Profile updated",
    "password_changed": "Password changed",
    "change_password": "Change password"
  },
  "errors": {
    "403_title": "Forbidden",
    "404_title": "Not found",
    "500_title": "Server error",
    "back_home": "Back to home"
  },
  "network": {
    "error": "Network error, please try again later"
  },
  "menu": {
    "dashboard": "Dashboard",
    "system": "System",
    "system.users": "Users",
    "system.roles": "Roles",
    "system.permissions": "Permissions",
    "system.menus": "Menus",
    "system.audit_logs": "Audit logs"
  },
  "perm": {
    "system": {
      "user": { "read": "Read users", "write": "Create/update users", "delete": "Delete users", "reset_pass": "Reset user password" },
      "role": { "read": "Read roles", "write": "Create/update roles + bind perms", "delete": "Delete roles" },
      "permission": { "read": "Read permission registry" },
      "menu": { "read": "Read menus", "write": "Create/update menus", "delete": "Delete menus" },
      "audit": { "read": "Read audit logs" }
    }
  }
}
```

- [ ] **Step 3: locales/index.ts**

```ts
import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN.json'
import enUS from './en-US.json'
import zhCNAntd from 'ant-design-vue/es/locale/zh_CN'
import enUSAntd from 'ant-design-vue/es/locale/en_US'

export type SupportedLocale = 'zh-CN' | 'en-US'

export const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'en-US',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS
  }
})

export function antdLocale(loc: SupportedLocale) {
  return loc === 'zh-CN' ? zhCNAntd : enUSAntd
}
```

- [ ] **Step 4: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/locales
git -C /Users/logic/Projects/optimus commit -m "feat(fe/locales): zh-CN + en-US base + vue-i18n setup"
```

---

## Task 7: FE — hand-written DTOs in src/types/api.ts

**Files:**
- Create: `optimus-fe/src/types/api.ts`

- [ ] **Step 1: Write types/api.ts**

```ts
// Hand-written DTOs mirroring optimus-be /api/v1 contracts.
// Source of truth: docs/api/swagger.json + internal/modules/*/dto.go.
// When BE contracts change, update this file in the same PR.

export interface Envelope<T> {
  code: number
  data: T
  message: string
  message_key?: string
}

// Auth
export interface LoginRequest {
  username: string
  password: string
}
export interface TokenPair {
  access_token: string
  refresh_token: string
  expires_at: string // ISO timestamp
}
export interface RefreshRequest {
  refresh_token: string
}
export interface LogoutRequest {
  refresh_token?: string
}

// Me
export interface MeUser {
  id: number
  username: string
  email: string
  display_name: string
  avatar_url: string
  status: 'enabled' | 'disabled'
  last_login_at?: string | null
}

export interface MeMenuNode {
  id: number
  code: string
  name: string                   // i18n key, e.g. "menu.system.users"
  path: string                   // e.g. "/system/users"
  component: string              // e.g. "system/users/List"; "" = group node
  icon: string
  permission_code?: string | null
  sort_order: number
  hidden: boolean
  children?: MeMenuNode[]
}

export interface UpdateMeRequest {
  email?: string
  display_name?: string
  avatar_url?: string
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}
```

- [ ] **Step 2: Verify it type-checks (empty src is fine; tsc just needs the file to parse)**

```bash
cd optimus-fe && bun run typecheck
```

Expected: PASS (or warn about "noEmit no inputs" only on JS files — should still exit 0).

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/types/api.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/types): hand-written DTOs mirroring /api/v1 contracts"
```

---

## Task 8: FE — utils/token + utils/http-error (TDD)

**Files:**
- Create: `optimus-fe/src/utils/token.ts`
- Create: `optimus-fe/src/utils/token.test.ts`
- Create: `optimus-fe/src/utils/http-error.ts`
- Create: `optimus-fe/src/utils/http-error.test.ts`

- [ ] **Step 1: Write failing tests for token utils**

`optimus-fe/src/utils/token.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { decodeJwtPayload, isAccessTokenExpired } from './token'

describe('decodeJwtPayload', () => {
  it('returns null for empty / malformed tokens', () => {
    expect(decodeJwtPayload('')).toBeNull()
    expect(decodeJwtPayload('garbage')).toBeNull()
    expect(decodeJwtPayload('a.b')).toBeNull()
  })

  it('decodes a valid JWT payload (HS256, base64url)', () => {
    // header.payload.signature — payload = {"sub":"1","exp":2000000000}
    const token = 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIiwiZXhwIjoyMDAwMDAwMDAwfQ.sig'
    const payload = decodeJwtPayload(token)
    expect(payload).toEqual({ sub: '1', exp: 2_000_000_000 })
  })
})

describe('isAccessTokenExpired', () => {
  it('returns true for null/empty', () => {
    expect(isAccessTokenExpired(null)).toBe(true)
    expect(isAccessTokenExpired('')).toBe(true)
  })

  it('returns true when exp is in the past (with 5s skew)', () => {
    const past = Math.floor(Date.now() / 1000) - 60
    const token = makeToken({ exp: past })
    expect(isAccessTokenExpired(token)).toBe(true)
  })

  it('returns false when exp is comfortably in the future', () => {
    const future = Math.floor(Date.now() / 1000) + 3600
    const token = makeToken({ exp: future })
    expect(isAccessTokenExpired(token)).toBe(false)
  })

  it('returns true within the 5s grace skew window', () => {
    const nearFuture = Math.floor(Date.now() / 1000) + 3
    const token = makeToken({ exp: nearFuture })
    expect(isAccessTokenExpired(token)).toBe(true)
  })
})

function makeToken(payload: Record<string, unknown>): string {
  const b64url = (s: string) => btoa(s).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
  return `eyJhbGciOiJIUzI1NiJ9.${b64url(JSON.stringify(payload))}.sig`
}
```

- [ ] **Step 2: Run test, verify it fails**

```bash
cd optimus-fe && bun run test src/utils/token.test.ts
```

Expected: FAIL — module './token' not found.

- [ ] **Step 3: Implement token.ts**

`optimus-fe/src/utils/token.ts`:

```ts
const SKEW_SECONDS = 5

export interface JwtPayload {
  sub?: string
  exp?: number
  iat?: number
  jti?: string
  [k: string]: unknown
}

export function decodeJwtPayload(token: string | null | undefined): JwtPayload | null {
  if (!token) return null
  const parts = token.split('.')
  if (parts.length < 2 || !parts[1]) return null
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = b64 + '==='.slice((b64.length + 3) % 4)
    const raw = atob(padded)
    return JSON.parse(raw) as JwtPayload
  } catch {
    return null
  }
}

export function isAccessTokenExpired(token: string | null | undefined): boolean {
  if (!token) return true
  const p = decodeJwtPayload(token)
  if (!p || typeof p.exp !== 'number') return true
  const nowSec = Math.floor(Date.now() / 1000)
  return p.exp <= nowSec + SKEW_SECONDS
}
```

- [ ] **Step 4: Re-run test, verify it passes**

```bash
cd optimus-fe && bun run test src/utils/token.test.ts
```

Expected: PASS (4 tests).

- [ ] **Step 5: Write failing tests for http-error**

`optimus-fe/src/utils/http-error.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { BizError, parseEnvelopeError, isBizError } from './http-error'

describe('BizError', () => {
  it('carries code, message, message_key', () => {
    const e = new BizError(40101, 'invalid creds', 'auth.invalid_credentials')
    expect(e.code).toBe(40101)
    expect(e.message).toBe('invalid creds')
    expect(e.messageKey).toBe('auth.invalid_credentials')
    expect(isBizError(e)).toBe(true)
  })

  it('isBizError discriminates against plain Error', () => {
    expect(isBizError(new Error('x'))).toBe(false)
  })
})

describe('parseEnvelopeError', () => {
  it('returns BizError from a non-zero envelope', () => {
    const e = parseEnvelopeError({ code: 50001, data: null, message: 'oops', message_key: 'k' })
    expect(e).toBeInstanceOf(BizError)
    expect(e?.code).toBe(50001)
  })

  it('returns null for code=0 envelopes', () => {
    expect(parseEnvelopeError({ code: 0, data: {}, message: '' })).toBeNull()
  })

  it('returns null for non-object inputs', () => {
    expect(parseEnvelopeError(null)).toBeNull()
    expect(parseEnvelopeError(undefined)).toBeNull()
    expect(parseEnvelopeError('whatever')).toBeNull()
  })
})
```

- [ ] **Step 6: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/utils/http-error.test.ts
```

Expected: FAIL — module './http-error' not found.

- [ ] **Step 7: Implement http-error.ts**

`optimus-fe/src/utils/http-error.ts`:

```ts
export class BizError extends Error {
  readonly code: number
  readonly messageKey?: string
  constructor(code: number, message: string, messageKey?: string) {
    super(message)
    this.name = 'BizError'
    this.code = code
    this.messageKey = messageKey
  }
}

export function isBizError(e: unknown): e is BizError {
  return e instanceof BizError
}

export function parseEnvelopeError(body: unknown): BizError | null {
  if (!body || typeof body !== 'object') return null
  const env = body as { code?: unknown; message?: unknown; message_key?: unknown }
  if (typeof env.code !== 'number' || env.code === 0) return null
  const msg = typeof env.message === 'string' ? env.message : ''
  const key = typeof env.message_key === 'string' ? env.message_key : undefined
  return new BizError(env.code, msg, key)
}
```

- [ ] **Step 8: Re-run, verify it passes**

```bash
cd optimus-fe && bun run test src/utils/
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/utils/token.ts optimus-fe/src/utils/token.test.ts optimus-fe/src/utils/http-error.ts optimus-fe/src/utils/http-error.test.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/utils): token (jwt payload + expiry) and http-error (BizError + envelope parser) with tests"
```

---

## Task 9: FE — utils/permission (TDD)

**Files:**
- Create: `optimus-fe/src/utils/permission.ts`
- Create: `optimus-fe/src/utils/permission.test.ts`

- [ ] **Step 1: Write failing tests**

`optimus-fe/src/utils/permission.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { has, hasAll, hasAny } from './permission'

const perms = new Set(['system:user:read', 'system:user:write'])

describe('has', () => {
  it('returns true when the single permission is present', () => {
    expect(has(perms, 'system:user:read')).toBe(true)
  })
  it('returns false when absent', () => {
    expect(has(perms, 'system:role:delete')).toBe(false)
  })
})

describe('hasAll', () => {
  it('returns true only when every code is present', () => {
    expect(hasAll(perms, ['system:user:read', 'system:user:write'])).toBe(true)
    expect(hasAll(perms, ['system:user:read', 'system:role:read'])).toBe(false)
  })
  it('vacuously true for empty list', () => {
    expect(hasAll(perms, [])).toBe(true)
  })
})

describe('hasAny', () => {
  it('returns true if at least one code is present', () => {
    expect(hasAny(perms, ['system:role:read', 'system:user:read'])).toBe(true)
    expect(hasAny(perms, ['system:role:read'])).toBe(false)
  })
  it('vacuously false for empty list', () => {
    expect(hasAny(perms, [])).toBe(false)
  })
})
```

- [ ] **Step 2: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/utils/permission.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement**

`optimus-fe/src/utils/permission.ts`:

```ts
export function has(perms: ReadonlySet<string>, code: string): boolean {
  return perms.has(code)
}

export function hasAll(perms: ReadonlySet<string>, codes: readonly string[]): boolean {
  return codes.every(c => perms.has(c))
}

export function hasAny(perms: ReadonlySet<string>, codes: readonly string[]): boolean {
  return codes.some(c => perms.has(c))
}
```

- [ ] **Step 4: Re-run, verify pass**

```bash
cd optimus-fe && bun run test src/utils/permission.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/utils/permission.ts optimus-fe/src/utils/permission.test.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/utils): permission helpers (has/hasAll/hasAny) with tests"
```

---

## Task 10: FE — stores (auth, menu, app) + persistedstate (TDD)

**Files:**
- Create: `optimus-fe/src/stores/index.ts`
- Create: `optimus-fe/src/stores/auth.ts`
- Create: `optimus-fe/src/stores/auth.test.ts`
- Create: `optimus-fe/src/stores/menu.ts`
- Create: `optimus-fe/src/stores/menu.test.ts`
- Create: `optimus-fe/src/stores/app.ts`
- Create: `optimus-fe/src/stores/app.test.ts`

Persistedstate config goes in the store factory (setup-store style); we test the pure state-shape behavior — hydration tests cover the `setActiveTokens` / `setUser` / `reset` flows.

- [ ] **Step 1: stores/index.ts — pinia + plugin install**

`optimus-fe/src/stores/index.ts`:

```ts
import { createPinia } from 'pinia'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'

export function createAppPinia() {
  const pinia = createPinia()
  pinia.use(piniaPluginPersistedstate)
  return pinia
}
```

- [ ] **Step 2: Write failing tests for auth store**

`optimus-fe/src/stores/auth.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from './auth'
import type { MeUser } from '@/types/api'

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts empty', () => {
    const s = useAuthStore()
    expect(s.accessToken).toBeNull()
    expect(s.refreshToken).toBeNull()
    expect(s.user).toBeNull()
    expect(s.permissions).toEqual([])
    expect(s.userLoaded).toBe(false)
  })

  it('setActiveTokens populates both tokens', () => {
    const s = useAuthStore()
    s.setActiveTokens('access', 'refresh')
    expect(s.accessToken).toBe('access')
    expect(s.refreshToken).toBe('refresh')
  })

  it('setUser flips userLoaded computed', () => {
    const s = useAuthStore()
    const u: MeUser = { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }
    s.setUser(u)
    expect(s.userLoaded).toBe(true)
    expect(s.user?.username).toBe('a')
  })

  it('setPermissions stores codes', () => {
    const s = useAuthStore()
    s.setPermissions(['system:user:read', 'system:role:read'])
    expect(s.permissions).toEqual(['system:user:read', 'system:role:read'])
  })

  it('reset clears everything', () => {
    const s = useAuthStore()
    s.setActiveTokens('a', 'b')
    s.setUser({ id: 1, username: 'a', email: 'a', display_name: '', avatar_url: '', status: 'enabled' })
    s.setPermissions(['x'])
    s.reset()
    expect(s.accessToken).toBeNull()
    expect(s.refreshToken).toBeNull()
    expect(s.user).toBeNull()
    expect(s.permissions).toEqual([])
  })
})
```

- [ ] **Step 3: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/stores/auth.test.ts
```

Expected: FAIL — module './auth' not found.

- [ ] **Step 4: Implement auth store**

`optimus-fe/src/stores/auth.ts`:

```ts
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { MeUser } from '@/types/api'

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string | null>(null)
  const refreshToken = ref<string | null>(null)
  const user = ref<MeUser | null>(null)
  const permissions = ref<string[]>([])

  const userLoaded = computed(() => user.value !== null)

  function setActiveTokens(access: string | null, refresh: string | null) {
    accessToken.value = access
    refreshToken.value = refresh
  }
  function setUser(u: MeUser | null) {
    user.value = u
  }
  function setPermissions(codes: string[]) {
    permissions.value = codes
  }
  function reset() {
    accessToken.value = null
    refreshToken.value = null
    user.value = null
    permissions.value = []
  }

  return {
    accessToken,
    refreshToken,
    user,
    permissions,
    userLoaded,
    setActiveTokens,
    setUser,
    setPermissions,
    reset
  }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['accessToken', 'refreshToken', 'user', 'permissions']
  }
})
```

- [ ] **Step 5: Re-run auth test**

```bash
cd optimus-fe && bun run test src/stores/auth.test.ts
```

Expected: PASS.

- [ ] **Step 6: Write failing tests for menu store**

`optimus-fe/src/stores/menu.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useMenuStore } from './menu'
import type { MeMenuNode } from '@/types/api'

describe('useMenuStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts with empty tree', () => {
    expect(useMenuStore().tree).toEqual([])
  })

  it('setTree replaces the tree', () => {
    const tree: MeMenuNode[] = [{
      id: 1, code: 'dashboard', name: 'menu.dashboard',
      path: '/dashboard', component: 'dashboard/Index',
      icon: 'dashboard', sort_order: 0, hidden: false
    }]
    const s = useMenuStore()
    s.setTree(tree)
    expect(s.tree).toEqual(tree)
  })

  it('reset clears the tree', () => {
    const s = useMenuStore()
    s.setTree([{ id: 1, code: 'x', name: 'x', path: '/x', component: 'x', icon: '', sort_order: 0, hidden: false }])
    s.reset()
    expect(s.tree).toEqual([])
  })
})
```

- [ ] **Step 7: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/stores/menu.test.ts
```

Expected: FAIL.

- [ ] **Step 8: Implement menu store**

`optimus-fe/src/stores/menu.ts`:

```ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { MeMenuNode } from '@/types/api'

export const useMenuStore = defineStore('menu', () => {
  const tree = ref<MeMenuNode[]>([])

  function setTree(t: MeMenuNode[]) {
    tree.value = t
  }
  function reset() {
    tree.value = []
  }

  return { tree, setTree, reset }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.sessionStorage : undefined,
    pick: ['tree']
  }
})
```

- [ ] **Step 9: Re-run, verify pass**

```bash
cd optimus-fe && bun run test src/stores/menu.test.ts
```

Expected: PASS.

- [ ] **Step 10: Write failing tests for app store**

`optimus-fe/src/stores/app.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAppStore } from './app'

describe('useAppStore', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('defaults locale=zh-CN, theme=light, sidebarCollapsed=false', () => {
    const s = useAppStore()
    expect(s.locale).toBe('zh-CN')
    expect(s.theme).toBe('light')
    expect(s.sidebarCollapsed).toBe(false)
  })

  it('mutators flip values', () => {
    const s = useAppStore()
    s.setLocale('en-US')
    s.setTheme('dark')
    s.toggleSidebar()
    expect(s.locale).toBe('en-US')
    expect(s.theme).toBe('dark')
    expect(s.sidebarCollapsed).toBe(true)
  })
})
```

- [ ] **Step 11: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/stores/app.test.ts
```

Expected: FAIL.

- [ ] **Step 12: Implement app store**

`optimus-fe/src/stores/app.ts`:

```ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { SupportedLocale } from '@/locales'

export type Theme = 'light' | 'dark'

export const useAppStore = defineStore('app', () => {
  const locale = ref<SupportedLocale>('zh-CN')
  const theme = ref<Theme>('light')
  const sidebarCollapsed = ref(false)

  function setLocale(l: SupportedLocale) {
    locale.value = l
  }
  function setTheme(t: Theme) {
    theme.value = t
  }
  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  return { locale, theme, sidebarCollapsed, setLocale, setTheme, toggleSidebar }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['locale', 'theme', 'sidebarCollapsed']
  }
})
```

- [ ] **Step 13: Re-run all store tests**

```bash
cd optimus-fe && bun run test src/stores/
```

Expected: all PASS.

- [ ] **Step 14: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/stores
git -C /Users/logic/Projects/optimus commit -m "feat(fe/stores): auth/menu/app stores with persistedstate"
```

---

## Task 11: FE — axios client with single-flight refresh (TDD)

**Files:**
- Create: `optimus-fe/src/api/client.ts`
- Create: `optimus-fe/src/api/client.test.ts`

This is the trickiest TDD task — the test must prove that concurrent 401s share one refresh. We use vitest's `vi.fn` for a fake refresh and verify it is called exactly once even with two parallel failing requests.

- [ ] **Step 1: Write failing tests**

`optimus-fe/src/api/client.test.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import MockAdapter from 'axios-mock-adapter'   // dev dep added below
import { createApiClient, __resetRefreshState } from './client'
import { useAuthStore } from '@/stores/auth'

describe('axios client single-flight refresh', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    __resetRefreshState()
  })

  it('refreshes once when two concurrent requests both 401', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('expired-access', 'good-refresh')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    const mock = new MockAdapter(client)
    let refreshCalls = 0
    mock.onPost('/auth/refresh').reply(() => {
      refreshCalls++
      return [200, { code: 0, data: { access_token: 'new-access', refresh_token: 'new-refresh', expires_at: '2099-01-01T00:00:00Z' }, message: '' }]
    })

    let firstHits = 0
    mock.onGet('/me').reply(config => {
      const auth = (config.headers?.Authorization as string) ?? ''
      if (auth.includes('expired-access')) {
        firstHits++
        return [401, { code: 40102, data: null, message: 'expired', message_key: 'auth.expired' }]
      }
      return [200, { code: 0, data: { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }, message: '' }]
    })

    const [r1, r2] = await Promise.all([client.get('/me'), client.get('/me')])
    expect(r1.data.data.username).toBe('a')
    expect(r2.data.data.username).toBe('a')
    expect(refreshCalls).toBe(1)
    expect(firstHits).toBeGreaterThanOrEqual(2)
    expect(auth.accessToken).toBe('new-access')
    expect(auth.refreshToken).toBe('new-refresh')
    expect(onLogout).not.toHaveBeenCalled()
  })

  it('logs out and rejects when refresh itself returns 401', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('expired-access', 'bad-refresh')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    const mock = new MockAdapter(client)
    mock.onPost('/auth/refresh').reply(401, { code: 40101, data: null, message: 'bad refresh' })
    mock.onGet('/me').reply(401, { code: 40102, data: null, message: 'expired' })

    await expect(client.get('/me')).rejects.toBeTruthy()
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('does not refresh for /auth/refresh itself (avoid loop)', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('any', 'any')
    const onLogout = vi.fn()
    const client = createApiClient({ baseURL: '/api/v1', onLogout })

    const mock = new MockAdapter(client)
    let calls = 0
    mock.onPost('/auth/refresh').reply(() => { calls++; return [401, { code: 40101, data: null, message: 'bad' }] })

    await expect(client.post('/auth/refresh', { refresh_token: 'x' })).rejects.toBeTruthy()
    expect(calls).toBe(1)
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('attaches Authorization + Accept-Language headers', async () => {
    const auth = useAuthStore()
    auth.setActiveTokens('access-x', null)
    const client = createApiClient({ baseURL: '/api/v1', onLogout: () => {}, getLocale: () => 'en-US' })

    const mock = new MockAdapter(client)
    let seen: Record<string, string> = {}
    mock.onGet('/me').reply(config => {
      seen = { ...config.headers } as Record<string, string>
      return [200, { code: 0, data: { id: 1, username: 'a', email: 'a@x', display_name: '', avatar_url: '', status: 'enabled' }, message: '' }]
    })

    await client.get('/me')
    expect(seen['Authorization']).toBe('Bearer access-x')
    expect(seen['Accept-Language']).toBe('en-US')
  })
})
```

Add `axios-mock-adapter` as a dev dependency:

```bash
cd optimus-fe && bun add -d axios-mock-adapter
```

- [ ] **Step 2: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/api/client.test.ts
```

Expected: FAIL — `./client` not found.

- [ ] **Step 3: Implement client.ts**

`optimus-fe/src/api/client.ts`:

```ts
import axios, { type AxiosInstance, type InternalAxiosRequestConfig, AxiosHeaders } from 'axios'
import { useAuthStore } from '@/stores/auth'
import type { Envelope, TokenPair } from '@/types/api'
import { parseEnvelopeError } from '@/utils/http-error'

export interface ClientOptions {
  baseURL: string
  onLogout: () => void
  getLocale?: () => string
}

let refreshing: Promise<TokenPair> | null = null

export function __resetRefreshState() {
  refreshing = null
}

interface RetriableConfig extends InternalAxiosRequestConfig {
  __retried?: boolean
}

export function createApiClient(opts: ClientOptions): AxiosInstance {
  const client = axios.create({ baseURL: opts.baseURL, timeout: 30_000 })

  client.interceptors.request.use(config => {
    const auth = useAuthStore()
    const headers = AxiosHeaders.from(config.headers)
    if (auth.accessToken) {
      headers.set('Authorization', `Bearer ${auth.accessToken}`)
    }
    if (opts.getLocale) {
      headers.set('Accept-Language', opts.getLocale())
    }
    config.headers = headers
    return config
  })

  client.interceptors.response.use(
    response => {
      // Successful HTTP but non-zero envelope code → throw BizError so callers .catch
      const body = response.data
      const bizErr = parseEnvelopeError(body)
      if (bizErr) throw bizErr
      return response
    },
    async error => {
      const status = error?.response?.status
      const original = error?.config as RetriableConfig | undefined
      const url = original?.url ?? ''
      const isRefreshCall = url.includes('/auth/refresh')

      if (status === 401 && !isRefreshCall && original && !original.__retried) {
        try {
          const pair = await ensureFreshAccess(client)
          original.__retried = true
          const headers = AxiosHeaders.from(original.headers)
          headers.set('Authorization', `Bearer ${pair.access_token}`)
          original.headers = headers
          return client.request(original)
        } catch (refreshErr) {
          opts.onLogout()
          throw refreshErr
        }
      }

      if (status === 401 && isRefreshCall) {
        opts.onLogout()
      }

      throw error
    }
  )

  async function ensureFreshAccess(c: AxiosInstance): Promise<TokenPair> {
    if (!refreshing) {
      refreshing = doRefresh(c).finally(() => { refreshing = null })
    }
    return refreshing
  }

  async function doRefresh(c: AxiosInstance): Promise<TokenPair> {
    const auth = useAuthStore()
    const rt = auth.refreshToken
    if (!rt) throw new Error('no refresh token')
    const resp = await c.post<Envelope<TokenPair>>('/auth/refresh', { refresh_token: rt })
    const pair = resp.data.data
    auth.setActiveTokens(pair.access_token, pair.refresh_token)
    return pair
  }

  return client
}
```

Note: the response interceptor's success path throws `BizError` for non-zero envelope codes; that throw is caught by callers (e.g. Login form shows the message). For 401-with-zero-code paths (server still returns HTTP 401 even on zero-code envelopes — but our backend never does this), the error branch handles it.

- [ ] **Step 4: Re-run client tests**

```bash
cd optimus-fe && bun run test src/api/client.test.ts
```

Expected: all 4 PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/api/client.ts optimus-fe/src/api/client.test.ts optimus-fe/package.json optimus-fe/bun.lockb
git -C /Users/logic/Projects/optimus commit -m "feat(fe/api): axios client with single-flight refresh + Bearer/Accept-Language headers"
```

---

## Task 12: FE — api/auth.ts + api/me.ts modules

**Files:**
- Create: `optimus-fe/src/api/auth.ts`
- Create: `optimus-fe/src/api/me.ts`

- [ ] **Step 1: api/auth.ts**

```ts
import type { AxiosInstance } from 'axios'
import type { Envelope, LoginRequest, LogoutRequest, RefreshRequest, TokenPair } from '@/types/api'

export function makeAuthApi(client: AxiosInstance) {
  return {
    login: async (body: LoginRequest) => {
      const r = await client.post<Envelope<TokenPair>>('/auth/login', body)
      return r.data.data
    },
    refresh: async (body: RefreshRequest) => {
      const r = await client.post<Envelope<TokenPair>>('/auth/refresh', body)
      return r.data.data
    },
    logout: async (body: LogoutRequest) => {
      await client.post<Envelope<null>>('/auth/logout', body)
    }
  }
}

export type AuthApi = ReturnType<typeof makeAuthApi>
```

- [ ] **Step 2: api/me.ts**

```ts
import type { AxiosInstance } from 'axios'
import type {
  ChangePasswordRequest, Envelope, MeMenuNode, MeUser, UpdateMeRequest
} from '@/types/api'

export function makeMeApi(client: AxiosInstance) {
  return {
    get: async () => (await client.get<Envelope<MeUser>>('/me')).data.data,
    update: async (body: UpdateMeRequest) =>
      (await client.put<Envelope<MeUser>>('/me', body)).data.data,
    changePassword: async (body: ChangePasswordRequest) => {
      await client.put<Envelope<null>>('/me/password', body)
    },
    menus: async () => (await client.get<Envelope<MeMenuNode[]>>('/me/menus')).data.data,
    permissions: async () => (await client.get<Envelope<string[]>>('/me/permissions')).data.data
  }
}

export type MeApi = ReturnType<typeof makeMeApi>
```

- [ ] **Step 3: Verify typecheck passes**

```bash
cd optimus-fe && bun run typecheck
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/api/auth.ts optimus-fe/src/api/me.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/api): auth + me endpoint modules"
```

---

## Task 13: FE — hooks/usePermission + useTable + useI18n (TDD for the first two)

**Files:**
- Create: `optimus-fe/src/hooks/usePermission.ts`
- Create: `optimus-fe/src/hooks/usePermission.test.ts`
- Create: `optimus-fe/src/hooks/useTable.ts`
- Create: `optimus-fe/src/hooks/useTable.test.ts`
- Create: `optimus-fe/src/hooks/useI18n.ts`

`useI18n` is a thin re-export wrapper, skip TDD.

- [ ] **Step 1: Write failing tests for usePermission**

`optimus-fe/src/hooks/usePermission.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { usePermission } from './usePermission'

describe('usePermission', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('has() returns true when permission is in the store', () => {
    useAuthStore().setPermissions(['system:user:read', 'system:user:write'])
    const p = usePermission()
    expect(p.has('system:user:read')).toBe(true)
    expect(p.has('system:role:delete')).toBe(false)
  })

  it('hasAll / hasAny work like their utils', () => {
    useAuthStore().setPermissions(['a', 'b'])
    const p = usePermission()
    expect(p.hasAll(['a', 'b'])).toBe(true)
    expect(p.hasAll(['a', 'c'])).toBe(false)
    expect(p.hasAny(['c', 'b'])).toBe(true)
    expect(p.hasAny(['c'])).toBe(false)
  })
})
```

- [ ] **Step 2: Run, verify fail**

```bash
cd optimus-fe && bun run test src/hooks/usePermission.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement usePermission**

`optimus-fe/src/hooks/usePermission.ts`:

```ts
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { has, hasAll, hasAny } from '@/utils/permission'

export function usePermission() {
  const auth = useAuthStore()
  const set = computed(() => new Set(auth.permissions))
  return {
    has: (code: string) => has(set.value, code),
    hasAll: (codes: readonly string[]) => hasAll(set.value, codes),
    hasAny: (codes: readonly string[]) => hasAny(set.value, codes),
    set
  }
}
```

- [ ] **Step 4: Re-run, verify pass**

```bash
cd optimus-fe && bun run test src/hooks/usePermission.test.ts
```

Expected: PASS.

- [ ] **Step 5: Write failing tests for useTable**

`optimus-fe/src/hooks/useTable.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { useTable } from './useTable'

describe('useTable', () => {
  it('starts with page 1, default pageSize, empty items, total 0', () => {
    const t = useTable<{ id: number }>({
      fetcher: vi.fn().mockResolvedValue({ items: [], total: 0 })
    })
    expect(t.page.value).toBe(1)
    expect(t.pageSize.value).toBe(20)
    expect(t.items.value).toEqual([])
    expect(t.total.value).toBe(0)
    expect(t.loading.value).toBe(false)
  })

  it('reload populates items and total', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [{ id: 1 }, { id: 2 }], total: 2 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.reload()
    expect(fetcher).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
    expect(t.items.value).toEqual([{ id: 1 }, { id: 2 }])
    expect(t.total.value).toBe(2)
  })

  it('setPage triggers reload with the new page', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.setPage(3)
    expect(fetcher).toHaveBeenCalledWith({ page: 3, pageSize: 20 })
    expect(t.page.value).toBe(3)
  })

  it('setPageSize resets to page 1', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.setPage(5)
    await t.setPageSize(50)
    expect(t.page.value).toBe(1)
    expect(t.pageSize.value).toBe(50)
    expect(fetcher).toHaveBeenLastCalledWith({ page: 1, pageSize: 50 })
  })

  it('fetcher error sets loading false and re-throws', async () => {
    const err = new Error('boom')
    const fetcher = vi.fn().mockRejectedValue(err)
    const t = useTable<{ id: number }>({ fetcher })
    await expect(t.reload()).rejects.toBe(err)
    expect(t.loading.value).toBe(false)
  })
})
```

- [ ] **Step 6: Run, verify fail**

```bash
cd optimus-fe && bun run test src/hooks/useTable.test.ts
```

Expected: FAIL.

- [ ] **Step 7: Implement useTable**

`optimus-fe/src/hooks/useTable.ts`:

```ts
import { ref } from 'vue'

export interface PageRequest {
  page: number
  pageSize: number
}
export interface PageResult<T> {
  items: T[]
  total: number
}

export interface UseTableOptions<T> {
  fetcher: (req: PageRequest) => Promise<PageResult<T>>
  defaultPageSize?: number
}

export function useTable<T>(opts: UseTableOptions<T>) {
  const page = ref(1)
  const pageSize = ref(opts.defaultPageSize ?? 20)
  const items = ref<T[]>([]) as { value: T[] }
  const total = ref(0)
  const loading = ref(false)

  async function reload() {
    loading.value = true
    try {
      const r = await opts.fetcher({ page: page.value, pageSize: pageSize.value })
      items.value = r.items
      total.value = r.total
    } finally {
      loading.value = false
    }
  }

  async function setPage(p: number) {
    page.value = p
    await reload()
  }

  async function setPageSize(s: number) {
    pageSize.value = s
    page.value = 1
    await reload()
  }

  return { page, pageSize, items, total, loading, reload, setPage, setPageSize }
}
```

- [ ] **Step 8: Re-run, verify pass**

```bash
cd optimus-fe && bun run test src/hooks/useTable.test.ts
```

Expected: PASS.

- [ ] **Step 9: Write useI18n**

`optimus-fe/src/hooks/useI18n.ts`:

```ts
import { useI18n as baseUseI18n } from 'vue-i18n'
import type { SupportedLocale } from '@/locales'

export function useI18n() {
  return baseUseI18n<{ message: Record<string, unknown> }, SupportedLocale>({ useScope: 'global' })
}
```

- [ ] **Step 10: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/hooks
git -C /Users/logic/Projects/optimus commit -m "feat(fe/hooks): usePermission + useTable + useI18n"
```

---

## Task 14: FE — directives/permission (TDD)

**Files:**
- Create: `optimus-fe/src/directives/permission.ts`
- Create: `optimus-fe/src/directives/permission.test.ts`
- Create: `optimus-fe/src/directives/index.ts`

- [ ] **Step 1: Write failing tests**

`optimus-fe/src/directives/permission.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { permissionDirective } from './permission'

function makeApp(template: string) {
  return defineComponent({
    setup() { return () => h('div', { innerHTML: template }) },
    directives: { permission: permissionDirective }
  })
}

describe('v-permission', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('removes element when single perm missing', () => {
    useAuthStore().setPermissions(['system:role:read'])
    const Cmp = defineComponent({
      template: '<div><a-button v-permission="\'system:user:write\'" class="target">x</a-button></div>',
      directives: { permission: permissionDirective }
    })
    const wrapper = mount(Cmp, { global: { stubs: { 'a-button': { template: '<button class="target"><slot/></button>' } } } })
    expect(wrapper.find('.target').exists()).toBe(false)
  })

  it('keeps element when single perm present', () => {
    useAuthStore().setPermissions(['system:user:write'])
    const Cmp = defineComponent({
      template: '<div><span v-permission="\'system:user:write\'" class="target">x</span></div>',
      directives: { permission: permissionDirective }
    })
    expect(mount(Cmp).find('.target').exists()).toBe(true)
  })

  it('array form requires ALL perms (intersection)', () => {
    useAuthStore().setPermissions(['a'])
    const All = defineComponent({
      template: '<div><span v-permission="[\'a\', \'b\']" class="target">x</span></div>',
      directives: { permission: permissionDirective }
    })
    expect(mount(All).find('.target').exists()).toBe(false)

    useAuthStore().setPermissions(['a', 'b'])
    expect(mount(All).find('.target').exists()).toBe(true)
  })

  it('v-permission:any requires AT LEAST ONE (union)', () => {
    useAuthStore().setPermissions(['c'])
    const Any = defineComponent({
      template: '<div><span v-permission:any="[\'a\', \'b\']" class="target">x</span></div>',
      directives: { permission: permissionDirective }
    })
    expect(mount(Any).find('.target').exists()).toBe(false)

    useAuthStore().setPermissions(['a'])
    expect(mount(Any).find('.target').exists()).toBe(true)
  })
})

// Suppress unused import warning
void makeApp
```

- [ ] **Step 2: Run, verify it fails**

```bash
cd optimus-fe && bun run test src/directives/permission.test.ts
```

Expected: FAIL — module './permission' not found.

- [ ] **Step 3: Implement**

`optimus-fe/src/directives/permission.ts`:

```ts
import type { Directive, DirectiveBinding } from 'vue'
import { useAuthStore } from '@/stores/auth'

type Arg = 'any' | undefined
type Value = string | readonly string[]

function check(value: Value, arg: Arg, perms: ReadonlySet<string>): boolean {
  if (typeof value === 'string') return perms.has(value)
  if (arg === 'any') return value.some(c => perms.has(c))
  return value.every(c => perms.has(c))
}

function apply(el: HTMLElement, binding: DirectiveBinding<Value>) {
  const auth = useAuthStore()
  const perms = new Set(auth.permissions)
  if (!check(binding.value, binding.arg as Arg, perms)) {
    el.parentNode?.removeChild(el)
  }
}

export const permissionDirective: Directive<HTMLElement, Value> = {
  mounted: apply,
  updated: apply
}
```

`optimus-fe/src/directives/index.ts`:

```ts
import type { App } from 'vue'
import { permissionDirective } from './permission'

export function installDirectives(app: App) {
  app.directive('permission', permissionDirective)
}
```

- [ ] **Step 4: Re-run, verify pass**

```bash
cd optimus-fe && bun run test src/directives/permission.test.ts
```

Expected: PASS (4 cases).

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/directives
git -C /Users/logic/Projects/optimus commit -m "feat(fe/directives): v-permission with string/all/any forms"
```

---

## Task 15: FE — scripts/check-i18n-keys.ts (TDD)

**Files:**
- Create: `optimus-fe/scripts/check-i18n-keys.ts`
- Create: `optimus-fe/scripts/check-i18n-keys.test.ts`

The script is invoked as a Bun CLI but the core is a pure function (`auditKeys`) that takes a file map + locale JSON, returns a result; we TDD that. The CLI wrapper just walks the FS and exits with the right code.

- [ ] **Step 1: Write failing tests**

`optimus-fe/scripts/check-i18n-keys.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { auditKeys, extractUsedKeys, flattenKeys } from './check-i18n-keys'

describe('extractUsedKeys', () => {
  it('finds $t / i18n.t / bare t(...) calls', () => {
    const src = `
      const x = $t('a.b')
      const y = i18n.t("c.d")
      const z = t('e.f')
      // $t('not.this') is in a comment but still matches by design (caught at runtime)
    `
    expect(extractUsedKeys('f.vue', src).sort()).toEqual(['a.b', 'c.d', 'e.f', 'not.this'].sort())
  })

  it('ignores non-string-literal call shapes', () => {
    const src = `$t(variable); t(x + 'y')`
    expect(extractUsedKeys('f.vue', src)).toEqual([])
  })
})

describe('flattenKeys', () => {
  it('walks nested objects into dot keys', () => {
    expect(flattenKeys({ a: { b: 'x', c: { d: 'y' } } }).sort()).toEqual(['a.b', 'a.c.d'])
  })
})

describe('auditKeys', () => {
  it('reports missing keys when used > declared', () => {
    const r = auditKeys({
      sources: [{ path: 'a.vue', usedKeys: ['x.y', 'x.z'] }],
      zhCN: { x: { y: 'present' } },
      enUS: { x: { y: 'present' } }
    })
    expect(r.missingFromZh).toContain('x.z')
  })

  it('reports zh/en symmetric diff', () => {
    const r = auditKeys({
      sources: [],
      zhCN: { a: '1' },
      enUS: { a: '1', b: '2' }
    })
    expect(r.zhEnMismatch).toContain('b')
  })

  it('no issues → empty arrays', () => {
    const r = auditKeys({
      sources: [{ path: 'a.vue', usedKeys: ['a'] }],
      zhCN: { a: '1' },
      enUS: { a: '1' }
    })
    expect(r.missingFromZh).toEqual([])
    expect(r.zhEnMismatch).toEqual([])
  })

  it('unused keys go to warnings, not errors', () => {
    const r = auditKeys({
      sources: [{ path: 'a.vue', usedKeys: [] }],
      zhCN: { a: '1' },
      enUS: { a: '1' }
    })
    expect(r.unused).toContain('a')
  })
})
```

- [ ] **Step 2: Run, verify fail**

```bash
cd optimus-fe && bun run test scripts/check-i18n-keys.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement**

`optimus-fe/scripts/check-i18n-keys.ts`:

```ts
/* eslint-disable @typescript-eslint/no-explicit-any */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const RE = /(?:\$t|i18n\.t|\bt)\(\s*['"`]([^'"`\s)]+)['"`]/g

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = path.join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (/\.(vue|ts)$/.test(p) && !p.endsWith('.test.ts')) out.push(p)
  }
  return out
}

export function extractUsedKeys(_path: string, src: string): string[] {
  const out = new Set<string>()
  for (const m of src.matchAll(RE)) {
    out.add(m[1])
  }
  return [...out]
}

export function flattenKeys(obj: Record<string, unknown>, prefix = ''): string[] {
  const out: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      out.push(...flattenKeys(v as Record<string, unknown>, key))
    } else {
      out.push(key)
    }
  }
  return out
}

export interface AuditInput {
  sources: Array<{ path: string; usedKeys: string[] }>
  zhCN: Record<string, unknown>
  enUS: Record<string, unknown>
}
export interface AuditResult {
  missingFromZh: string[]
  zhEnMismatch: string[]
  unused: string[]
}

export function auditKeys(input: AuditInput): AuditResult {
  const zhFlat = new Set(flattenKeys(input.zhCN))
  const enFlat = new Set(flattenKeys(input.enUS))
  const used = new Set<string>()
  for (const s of input.sources) for (const k of s.usedKeys) used.add(k)

  const missingFromZh: string[] = []
  for (const k of used) if (!zhFlat.has(k)) missingFromZh.push(k)

  const zhEnMismatch: string[] = []
  for (const k of zhFlat) if (!enFlat.has(k)) zhEnMismatch.push(k)
  for (const k of enFlat) if (!zhFlat.has(k)) zhEnMismatch.push(k)

  const unused: string[] = []
  for (const k of zhFlat) if (!used.has(k) && !k.startsWith('menu.') && !k.startsWith('perm.')) {
    unused.push(k)
  }

  return { missingFromZh, zhEnMismatch, unused }
}

// CLI entry — only runs when executed directly, not when imported by tests.
async function main() {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
  const files = walk(`${root}/src`)
  const sources = files.map(p => ({
    path: p,
    usedKeys: extractUsedKeys(p, readFileSync(p, 'utf8'))
  }))
  const zhCN = JSON.parse(readFileSync(`${root}/src/locales/zh-CN.json`, 'utf8'))
  const enUS = JSON.parse(readFileSync(`${root}/src/locales/en-US.json`, 'utf8'))
  const r = auditKeys({ sources, zhCN, enUS })

  let fatal = false
  if (r.missingFromZh.length) {
    console.error('Missing keys in zh-CN.json:')
    for (const k of r.missingFromZh) console.error('  -', k)
    fatal = true
  }
  if (r.zhEnMismatch.length) {
    console.error('zh-CN vs en-US key mismatch:')
    for (const k of r.zhEnMismatch) console.error('  -', k)
    fatal = true
  }
  if (r.unused.length) {
    console.warn(`Unused keys (warning only, ${r.unused.length}):`)
    for (const k of r.unused.slice(0, 10)) console.warn('  -', k)
    if (r.unused.length > 10) console.warn(`  … +${r.unused.length - 10} more`)
  }
  if (fatal) process.exit(1)
  console.log('i18n keys OK')
}

// Bun's import.meta.main exposes whether the file is the entrypoint.
// Tests import this file via vitest, so this branch only runs in the CLI invocation.
if ((import.meta as any).main) {
  await main()
}
```

The `walk` helper recursively collects `.vue` and non-test `.ts` files — `.test.ts` files are excluded so `$t('present')` strings inside test fixtures don't pollute the used-set.

- [ ] **Step 4: Re-run tests, verify pass**

```bash
cd optimus-fe && bun run test scripts/check-i18n-keys.test.ts
```

Expected: PASS.

- [ ] **Step 5: Run the CLI against the actual repo and ensure it succeeds**

```bash
cd optimus-fe && bun run i18n:check
```

Expected: `i18n keys OK` (no missing, no zh/en mismatch). If any `unused` warnings appear for non-menu/non-perm keys, that's fine — only missing/mismatch fail.

- [ ] **Step 6: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/scripts/check-i18n-keys.ts optimus-fe/scripts/check-i18n-keys.test.ts
git -C /Users/logic/Projects/optimus commit -m "feat(fe/tooling): scripts/check-i18n-keys.ts + bun run i18n:check"
```

---

## Task 16: FE — router (static + dynamic + guards + index) — partial TDD

**Files:**
- Create: `optimus-fe/src/router/static-routes.ts`
- Create: `optimus-fe/src/router/dynamic-routes.ts`
- Create: `optimus-fe/src/router/dynamic-routes.test.ts`
- Create: `optimus-fe/src/router/guards.ts`
- Create: `optimus-fe/src/router/index.ts`

The pure-function part of dynamic routes (`flattenMenusToRoutes`) is TDD'd; guards depend on the real Router/route objects and are smoke-tested manually.

- [ ] **Step 1: static-routes.ts**

```ts
import type { RouteRecordRaw } from 'vue-router'

export const staticRoutes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/auth/Login.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/403',
    name: 'forbidden',
    component: () => import('@/views/errors/403.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/404',
    name: 'notfound',
    component: () => import('@/views/errors/404.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/500',
    name: 'serverError',
    component: () => import('@/views/errors/500.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/',
    name: 'root',
    component: () => import('@/layouts/DefaultLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'profile',
        name: 'profile',
        component: () => import('@/views/profile/Index.vue')
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/404'
  }
]
```

- [ ] **Step 2: Write failing test for dynamic-routes**

`optimus-fe/src/router/dynamic-routes.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { flattenMenusToRoutes } from './dynamic-routes'
import type { MeMenuNode } from '@/types/api'

const tree: MeMenuNode[] = [
  {
    id: 1, code: 'dashboard', name: 'menu.dashboard',
    path: '/dashboard', component: 'dashboard/Index',
    icon: 'dashboard', sort_order: 0, hidden: false
  },
  {
    id: 2, code: 'system', name: 'menu.system',
    path: '/system', component: '', icon: 'setting', sort_order: 1, hidden: false,
    children: [
      {
        id: 3, code: 'system.users', name: 'menu.system.users',
        path: '/system/users', component: 'system/users/List',
        icon: '', permission_code: 'system:user:read',
        sort_order: 0, hidden: false
      }
    ]
  }
]

describe('flattenMenusToRoutes', () => {
  it('skips group nodes (empty component) and flattens leaves', () => {
    const components = new Map<string, () => Promise<unknown>>([
      ['dashboard/Index', async () => ({ default: {} })],
      ['system/users/List', async () => ({ default: {} })]
    ])
    const routes = flattenMenusToRoutes(tree, p => components.get(p))
    expect(routes.map(r => r.path).sort()).toEqual(['/dashboard', '/system/users'])
    const usersRoute = routes.find(r => r.path === '/system/users')!
    expect(usersRoute.name).toBe('system.users')
    expect(usersRoute.meta?.permission).toBe('system:user:read')
  })

  it('skips nodes whose component path has no loader (with warn)', () => {
    const components = new Map<string, () => Promise<unknown>>([
      ['dashboard/Index', async () => ({ default: {} })]
    ])
    const warns: string[] = []
    const routes = flattenMenusToRoutes(tree, p => components.get(p), msg => warns.push(msg))
    expect(routes.map(r => r.path)).toEqual(['/dashboard'])
    expect(warns.some(w => w.includes('system/users/List'))).toBe(true)
  })
})
```

- [ ] **Step 3: Run, verify fail**

```bash
cd optimus-fe && bun run test src/router/dynamic-routes.test.ts
```

Expected: FAIL.

- [ ] **Step 4: Implement dynamic-routes.ts**

`optimus-fe/src/router/dynamic-routes.ts`:

```ts
import type { Component } from 'vue'
import type { RouteRecordRaw, Router } from 'vue-router'
import type { MeMenuNode } from '@/types/api'

type Loader = () => Promise<{ default: Component } | Component>

export function flattenMenusToRoutes(
  tree: MeMenuNode[],
  resolve: (component: string) => Loader | undefined,
  warn: (msg: string) => void = m => console.warn(m)
): RouteRecordRaw[] {
  const out: RouteRecordRaw[] = []
  const walk = (nodes: MeMenuNode[]) => {
    for (const n of nodes) {
      if (n.component) {
        const loader = resolve(n.component)
        if (!loader) {
          warn(`[router] dropped menu '${n.code}': component '${n.component}' not found in glob`)
        } else {
          out.push({
            path: n.path,
            name: n.code,
            component: loader as unknown as Component,
            meta: {
              permission: n.permission_code ?? undefined,
              icon: n.icon,
              menuName: n.name
            }
          })
        }
      }
      if (n.children?.length) walk(n.children)
    }
  }
  walk(tree)
  return out
}

export function buildViewResolver(): (component: string) => Loader | undefined {
  const map = import.meta.glob('/src/views/**/*.vue')
  return (component: string) => {
    const key = `/src/views/${component}.vue`
    return map[key] as Loader | undefined
  }
}

export function registerDynamicRoutes(router: Router, tree: MeMenuNode[]) {
  const resolve = buildViewResolver()
  const routes = flattenMenusToRoutes(tree, resolve)
  for (const r of routes) {
    router.addRoute('root', r)
  }
}
```

- [ ] **Step 5: Re-run, verify pass**

```bash
cd optimus-fe && bun run test src/router/dynamic-routes.test.ts
```

Expected: PASS.

- [ ] **Step 6: guards.ts**

`optimus-fe/src/router/guards.ts`:

```ts
import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { usePermission } from '@/hooks/usePermission'
import { registerDynamicRoutes } from './dynamic-routes'
import type { MeApi } from '@/api/me'

export function installGuards(router: Router, meApi: MeApi) {
  router.beforeEach(async to => {
    if (to.meta?.public) return true

    const auth = useAuthStore()
    if (!auth.accessToken) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }

    if (!auth.userLoaded) {
      try {
        const [user, menus, perms] = await Promise.all([meApi.get(), meApi.menus(), meApi.permissions()])
        auth.setUser(user)
        auth.setPermissions(perms)
        useMenuStore().setTree(menus)
        registerDynamicRoutes(router, menus)
        return { ...to, replace: true }
      } catch {
        auth.reset()
        useMenuStore().reset()
        return { name: 'login', query: { redirect: to.fullPath } }
      }
    }

    const perm = to.meta?.permission as string | undefined
    if (perm && !usePermission().has(perm)) {
      return { name: 'forbidden' }
    }
    return true
  })
}
```

- [ ] **Step 7: index.ts**

`optimus-fe/src/router/index.ts`:

```ts
import { createRouter, createWebHistory, type Router } from 'vue-router'
import { staticRoutes } from './static-routes'

export function createAppRouter(): Router {
  return createRouter({
    history: createWebHistory(),
    routes: staticRoutes
  })
}
```

- [ ] **Step 8: Typecheck and commit**

```bash
cd optimus-fe && bun run typecheck && bun run test
```

Expected: PASS.

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/router
git -C /Users/logic/Projects/optimus commit -m "feat(fe/router): static + dynamic route injection + auth guard"
```

---

## Task 17: FE — layouts + sidebar/header/lang/theme subcomponents

**Files:**
- Create: `optimus-fe/src/layouts/BlankLayout.vue`
- Create: `optimus-fe/src/layouts/DefaultLayout.vue`
- Create: `optimus-fe/src/components/AppSidebar.vue`
- Create: `optimus-fe/src/components/AppHeader.vue`
- Create: `optimus-fe/src/components/LangSwitch.vue`
- Create: `optimus-fe/src/components/ThemeToggle.vue`

- [ ] **Step 1: BlankLayout.vue**

```vue
<template>
  <div class="blank-layout">
    <router-view />
  </div>
</template>

<style scoped lang="scss">
.blank-layout {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--ant-color-bg-layout, #f5f5f5);
}
</style>
```

- [ ] **Step 2: DefaultLayout.vue**

```vue
<template>
  <a-layout class="default-layout">
    <a-layout-sider v-model:collapsed="collapsed" :trigger="null" collapsible>
      <AppSidebar :collapsed="collapsed" />
    </a-layout-sider>
    <a-layout>
      <a-layout-header class="header">
        <AppHeader :collapsed="collapsed" @toggle="collapsed = !collapsed" />
      </a-layout-header>
      <a-layout-content class="content">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import AppSidebar from '@/components/AppSidebar.vue'
import AppHeader from '@/components/AppHeader.vue'

const app = useAppStore()
const collapsed = ref(app.sidebarCollapsed)
watch(collapsed, v => { if (v !== app.sidebarCollapsed) app.toggleSidebar() })
watch(() => app.sidebarCollapsed, v => { collapsed.value = v })
</script>

<style scoped lang="scss">
.default-layout {
  min-height: 100vh;
}
.header {
  background: #fff;
  padding: 0 16px;
  border-bottom: 1px solid var(--ant-color-border, #f0f0f0);
}
.content {
  padding: 16px;
  background: var(--ant-color-bg-layout, #f5f5f5);
}
</style>
```

- [ ] **Step 3: AppSidebar.vue**

```vue
<template>
  <div class="app-sidebar">
    <div class="logo">{{ collapsed ? 'O' : 'Optimus' }}</div>
    <a-menu
      :selected-keys="[currentKey]"
      :open-keys="openKeys"
      mode="inline"
      theme="dark"
      :items="items"
      @click="onClick"
      @open-change="keys => (openKeys = keys as string[])"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMenuStore } from '@/stores/menu'
import { useI18n } from '@/hooks/useI18n'
import type { MeMenuNode } from '@/types/api'
import type { ItemType } from 'ant-design-vue'

defineProps<{ collapsed: boolean }>()
const menu = useMenuStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

function buildItems(nodes: MeMenuNode[]): ItemType[] {
  return nodes.map(n => {
    const base = { key: n.code, label: t(n.name) }
    if (n.children?.length) return { ...base, children: buildItems(n.children) } as ItemType
    return base as ItemType
  })
}

const items = computed(() => buildItems(menu.tree))

const codeByPath = computed(() => {
  const map = new Map<string, string>()
  const walk = (ns: MeMenuNode[]) => {
    for (const n of ns) {
      if (n.path) map.set(n.path, n.code)
      if (n.children?.length) walk(n.children)
    }
  }
  walk(menu.tree)
  return map
})

const currentKey = computed(() => codeByPath.value.get(route.path) ?? '')
const openKeys = ref<string[]>([])

watch(() => menu.tree, ts => {
  // open every group by default
  openKeys.value = ts.filter(n => n.children?.length).map(n => n.code)
}, { immediate: true })

function onClick({ key }: { key: string }) {
  const node = findNode(menu.tree, key)
  if (node?.path) router.push(node.path)
}

function findNode(ns: MeMenuNode[], code: string): MeMenuNode | undefined {
  for (const n of ns) {
    if (n.code === code) return n
    const found = n.children ? findNode(n.children, code) : undefined
    if (found) return found
  }
}
</script>

<style scoped lang="scss">
.app-sidebar {
  height: 100%;
  background: #001529;
  display: flex;
  flex-direction: column;
}
.logo {
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  letter-spacing: 1px;
}
</style>
```

- [ ] **Step 4: AppHeader.vue**

```vue
<template>
  <div class="app-header">
    <a-button type="text" @click="$emit('toggle')">
      <MenuFoldOutlined v-if="!collapsed" />
      <MenuUnfoldOutlined v-else />
    </a-button>
    <div class="u-flex-1" />
    <LangSwitch />
    <ThemeToggle />
    <a-dropdown>
      <a-button type="text">
        <UserOutlined />
        {{ auth.user?.display_name || auth.user?.username || 'user' }}
      </a-button>
      <template #overlay>
        <a-menu @click="onMenuClick">
          <a-menu-item key="profile">{{ $t('common.profile') }}</a-menu-item>
          <a-menu-divider />
          <a-menu-item key="logout">{{ $t('common.logout') }}</a-menu-item>
        </a-menu>
      </template>
    </a-dropdown>
  </div>
</template>

<script setup lang="ts">
import { MenuFoldOutlined, MenuUnfoldOutlined, UserOutlined } from '@ant-design/icons-vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import LangSwitch from './LangSwitch.vue'
import ThemeToggle from './ThemeToggle.vue'

defineProps<{ collapsed: boolean }>()
defineEmits<{ toggle: [] }>()

const auth = useAuthStore()
const router = useRouter()

function onMenuClick({ key }: { key: string }) {
  if (key === 'profile') router.push('/profile')
  if (key === 'logout') {
    auth.reset()
    useMenuStore().reset()
    router.push('/login')
  }
}
</script>

<style scoped lang="scss">
.app-header {
  height: 48px;
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
```

- [ ] **Step 5: LangSwitch.vue**

```vue
<template>
  <a-dropdown>
    <a-button type="text">
      <GlobalOutlined /> {{ current }}
    </a-button>
    <template #overlay>
      <a-menu @click="onClick">
        <a-menu-item key="zh-CN">中文</a-menu-item>
        <a-menu-item key="en-US">English</a-menu-item>
      </a-menu>
    </template>
  </a-dropdown>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { GlobalOutlined } from '@ant-design/icons-vue'
import { useAppStore } from '@/stores/app'
import { useI18n } from '@/hooks/useI18n'
import type { SupportedLocale } from '@/locales'

const app = useAppStore()
const { locale } = useI18n()
const current = computed(() => (app.locale === 'zh-CN' ? '中' : 'EN'))

function onClick({ key }: { key: string }) {
  const l = key as SupportedLocale
  app.setLocale(l)
  locale.value = l
}
</script>
```

- [ ] **Step 6: ThemeToggle.vue**

```vue
<template>
  <a-button type="text" @click="toggle">
    <BulbOutlined v-if="app.theme === 'dark'" />
    <BulbFilled v-else />
  </a-button>
</template>

<script setup lang="ts">
import { BulbOutlined, BulbFilled } from '@ant-design/icons-vue'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
function toggle() {
  app.setTheme(app.theme === 'dark' ? 'light' : 'dark')
}
</script>
```

- [ ] **Step 7: Typecheck and commit**

```bash
cd optimus-fe && bun run typecheck && bun run lint
```

Expected: typecheck PASS. Lint may surface unused imports — fix inline.

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/layouts optimus-fe/src/components/AppSidebar.vue optimus-fe/src/components/AppHeader.vue optimus-fe/src/components/LangSwitch.vue optimus-fe/src/components/ThemeToggle.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/layout): DefaultLayout + BlankLayout + sidebar/header/lang/theme subcomponents"
```

---

## Task 18: FE — components PageHeader + ConfirmButton

**Files:**
- Create: `optimus-fe/src/components/PageHeader.vue`
- Create: `optimus-fe/src/components/ConfirmButton.vue`

- [ ] **Step 1: PageHeader.vue**

```vue
<template>
  <div class="page-header">
    <h2 class="title">{{ title }}</h2>
    <div class="actions"><slot /></div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<style scoped lang="scss">
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.title {
  margin: 0;
  font-size: 18px;
}
.actions {
  display: flex;
  gap: 8px;
}
</style>
```

- [ ] **Step 2: ConfirmButton.vue**

```vue
<template>
  <a-popconfirm
    :title="confirm ?? $t('common.ok')"
    :ok-text="$t('common.ok')"
    :cancel-text="$t('common.cancel')"
    @confirm="$emit('confirm')"
  >
    <slot />
  </a-popconfirm>
</template>

<script setup lang="ts">
defineProps<{ confirm?: string }>()
defineEmits<{ confirm: [] }>()
</script>
```

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/components/PageHeader.vue optimus-fe/src/components/ConfirmButton.vue
git -C /Users/logic/Projects/optimus commit -m "feat(fe/components): PageHeader + ConfirmButton"
```

---

## Task 19: FE — views/auth/Login.vue + views/errors/{403,404,500}.vue

**Files:**
- Create: `optimus-fe/src/views/auth/Login.vue`
- Create: `optimus-fe/src/views/errors/403.vue`
- Create: `optimus-fe/src/views/errors/404.vue`
- Create: `optimus-fe/src/views/errors/500.vue`

- [ ] **Step 1: Login.vue**

```vue
<template>
  <a-card class="login-card">
    <h1 class="title">{{ $t('auth.login_title') }}</h1>
    <a-form :model="form" layout="vertical" @finish="onSubmit">
      <a-form-item :label="$t('auth.username')" name="username" :rules="[{ required: true }]">
        <a-input v-model:value="form.username" autocomplete="username" />
      </a-form-item>
      <a-form-item :label="$t('auth.password')" name="password" :rules="[{ required: true }]">
        <a-input-password v-model:value="form.password" autocomplete="current-password" />
      </a-form-item>
      <a-form-item>
        <a-button type="primary" html-type="submit" :loading="loading" block>
          {{ $t('auth.login') }}
        </a-button>
      </a-form-item>
    </a-form>
  </a-card>
</template>

<script setup lang="ts">
import { reactive, ref, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import { isBizError } from '@/utils/http-error'
import type { AuthApi } from '@/api/auth'

const form = reactive({ username: '', password: '' })
const loading = ref(false)
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const authApi = inject<AuthApi>('authApi')!

async function onSubmit() {
  loading.value = true
  try {
    const pair = await authApi.login({ username: form.username, password: form.password })
    authStore.setActiveTokens(pair.access_token, pair.refresh_token)
    const redirect = (route.query.redirect as string) || '/dashboard'
    router.push(redirect)
  } catch (e) {
    if (isBizError(e)) {
      message.error(e.messageKey ? `auth.${e.messageKey}` : e.message)
    } else {
      message.error('network.error')
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.login-card {
  width: 360px;
  box-shadow: 0 2px 16px rgba(0, 0, 0, 0.06);
}
.title {
  font-size: 20px;
  margin: 0 0 16px;
  text-align: center;
}
</style>
```

- [ ] **Step 2: errors/403.vue**

```vue
<template>
  <a-result status="403" :title="$t('errors.403_title')">
    <template #extra>
      <a-button type="primary" @click="$router.push('/')">{{ $t('errors.back_home') }}</a-button>
    </template>
  </a-result>
</template>
```

- [ ] **Step 3: errors/404.vue**

```vue
<template>
  <a-result status="404" :title="$t('errors.404_title')">
    <template #extra>
      <a-button type="primary" @click="$router.push('/')">{{ $t('errors.back_home') }}</a-button>
    </template>
  </a-result>
</template>
```

- [ ] **Step 4: errors/500.vue**

```vue
<template>
  <a-result status="500" :title="$t('errors.500_title')">
    <template #extra>
      <a-button type="primary" @click="$router.push('/')">{{ $t('errors.back_home') }}</a-button>
    </template>
  </a-result>
</template>
```

- [ ] **Step 5: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/auth optimus-fe/src/views/errors
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views): Login + 403/404/500 error pages"
```

---

## Task 20: FE — dashboard + 5 system placeholders + profile page

**Files:**
- Create: `optimus-fe/src/views/dashboard/Index.vue`
- Create: `optimus-fe/src/views/system/users/List.vue`
- Create: `optimus-fe/src/views/system/roles/List.vue`
- Create: `optimus-fe/src/views/system/permissions/List.vue`
- Create: `optimus-fe/src/views/system/menus/List.vue`
- Create: `optimus-fe/src/views/system/audit-logs/List.vue`
- Create: `optimus-fe/src/views/profile/Index.vue`

- [ ] **Step 1: dashboard/Index.vue + 5 system placeholders (all share the same shape)**

`optimus-fe/src/views/dashboard/Index.vue`:

```vue
<template>
  <a-card>
    <PageHeader :title="$t('menu.dashboard')" />
    <a-empty :description="$t('placeholder.coming_soon')" />
  </a-card>
</template>

<script setup lang="ts">
import PageHeader from '@/components/PageHeader.vue'
</script>
```

Repeat for each of the five system placeholders with the appropriate `$t(...)` title:

- `system/users/List.vue` → `$t('menu.system.users')`
- `system/roles/List.vue` → `$t('menu.system.roles')`
- `system/permissions/List.vue` → `$t('menu.system.permissions')`
- `system/menus/List.vue` → `$t('menu.system.menus')`
- `system/audit-logs/List.vue` → `$t('menu.system.audit_logs')`

Each file is the same 7-line skeleton — copy and adjust only the title key.

- [ ] **Step 2: profile/Index.vue**

```vue
<template>
  <div class="profile-page">
    <PageHeader :title="$t('profile.title')" />

    <a-card :title="$t('profile.title')" class="u-mb-16">
      <a-form :model="profile" layout="vertical" @finish="onSaveProfile">
        <a-form-item :label="$t('profile.display_name')" name="display_name">
          <a-input v-model:value="profile.display_name" />
        </a-form-item>
        <a-form-item :label="$t('profile.email')" name="email" :rules="[{ type: 'email' }]">
          <a-input v-model:value="profile.email" />
        </a-form-item>
        <a-form-item :label="$t('profile.avatar_url')" name="avatar_url">
          <a-input v-model:value="profile.avatar_url" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" :loading="profileSaving">{{ $t('common.save') }}</a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card :title="$t('profile.change_password')">
      <a-form :model="pw" layout="vertical" @finish="onChangePassword">
        <a-form-item :label="$t('profile.old_password')" name="old_password" :rules="[{ required: true }]">
          <a-input-password v-model:value="pw.old_password" />
        </a-form-item>
        <a-form-item :label="$t('profile.new_password')" name="new_password" :rules="[{ required: true, min: 8 }]">
          <a-input-password v-model:value="pw.new_password" />
        </a-form-item>
        <a-form-item :label="$t('profile.confirm_password')" name="confirm" :rules="[{ validator: validateConfirm }]">
          <a-input-password v-model:value="pw.confirm" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" :loading="pwSaving">{{ $t('profile.change_password') }}</a-button>
        </a-form-item>
      </a-form>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { inject, reactive, ref, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useAuthStore } from '@/stores/auth'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import type { MeApi } from '@/api/me'

const { t } = useI18n()
const meApi = inject<MeApi>('meApi')!
const auth = useAuthStore()

const profile = reactive({
  display_name: auth.user?.display_name ?? '',
  email: auth.user?.email ?? '',
  avatar_url: auth.user?.avatar_url ?? ''
})

const pw = reactive({ old_password: '', new_password: '', confirm: '' })
const profileSaving = ref(false)
const pwSaving = ref(false)

onMounted(async () => {
  if (!auth.user) {
    try {
      auth.setUser(await meApi.get())
      profile.display_name = auth.user?.display_name ?? ''
      profile.email = auth.user?.email ?? ''
      profile.avatar_url = auth.user?.avatar_url ?? ''
    } catch { /* guard already redirects on 401 */ }
  }
})

async function onSaveProfile() {
  profileSaving.value = true
  try {
    const updated = await meApi.update({
      display_name: profile.display_name,
      email: profile.email,
      avatar_url: profile.avatar_url
    })
    auth.setUser(updated)
    message.success(t('profile.update_ok'))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    profileSaving.value = false
  }
}

async function validateConfirm(_: unknown, value: string) {
  if (value !== pw.new_password) {
    return Promise.reject(t('profile.password_mismatch'))
  }
  return Promise.resolve()
}

async function onChangePassword() {
  pwSaving.value = true
  try {
    await meApi.changePassword({ old_password: pw.old_password, new_password: pw.new_password })
    pw.old_password = ''
    pw.new_password = ''
    pw.confirm = ''
    message.success(t('profile.password_changed'))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    pwSaving.value = false
  }
}
</script>

<style scoped lang="scss">
.profile-page {
  max-width: 640px;
  margin: 0 auto;
}
</style>
```

- [ ] **Step 3: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/views/dashboard optimus-fe/src/views/system optimus-fe/src/views/profile
git -C /Users/logic/Projects/optimus commit -m "feat(fe/views): dashboard + 5 system Coming-soon placeholders + profile (edit + change password)"
```

---

## Task 21: FE — main.ts + App.vue + README + CI frontend job

**Files:**
- Create: `optimus-fe/src/App.vue`
- Create: `optimus-fe/src/main.ts`
- Create: `optimus-fe/README.md`
- Modify: `README.md` (root)
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: App.vue**

```vue
<template>
  <a-config-provider :locale="antd" :theme="{ algorithm }">
    <component :is="layout">
      <router-view />
    </component>
  </a-config-provider>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { theme as antdTheme } from 'ant-design-vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useI18n } from '@/hooks/useI18n'
import { antdLocale } from '@/locales'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import BlankLayout from '@/layouts/BlankLayout.vue'

const app = useAppStore()
const route = useRoute()
const { locale } = useI18n()

const antd = computed(() => antdLocale(app.locale))
const algorithm = computed(() => app.theme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm)

const layout = computed(() => (route.meta?.layout === 'blank' ? BlankLayout : DefaultLayout))

watch(() => app.locale, l => { locale.value = l }, { immediate: true })
</script>
```

- [ ] **Step 2: main.ts**

```ts
import { createApp } from 'vue'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import '@/assets/styles/utilities.scss'

import App from './App.vue'
import { createAppPinia } from '@/stores'
import { createAppRouter } from '@/router'
import { installGuards } from '@/router/guards'
import { i18n } from '@/locales'
import { installDirectives } from '@/directives'
import { createApiClient } from '@/api/client'
import { makeAuthApi } from '@/api/auth'
import { makeMeApi } from '@/api/me'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { useAppStore } from '@/stores/app'

const app = createApp(App)
const pinia = createAppPinia()
app.use(pinia)
app.use(Antd)
app.use(i18n)

const router = createAppRouter()

const client = createApiClient({
  baseURL: import.meta.env.VITE_API_BASE_URL,
  onLogout: () => {
    useAuthStore().reset()
    useMenuStore().reset()
    router.push('/login')
  },
  getLocale: () => useAppStore().locale
})

const authApi = makeAuthApi(client)
const meApi = makeMeApi(client)
app.provide('authApi', authApi)
app.provide('meApi', meApi)

installGuards(router, meApi)
app.use(router)
installDirectives(app)

app.mount('#app')
```

- [ ] **Step 3: optimus-fe/README.md**

```markdown
# optimus-fe

P0 frontend for Optimus (Vue 3 + AntdV + Pinia + vue-router + vue-i18n).

## Prerequisites

- bun ≥ 1.1 (`brew install oven-sh/bun/bun`)
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
```

- [ ] **Step 4: Root README.md — append frontend section**

Read current `README.md` first and append a "Frontend" section pointing to `optimus-fe/README.md`:

```markdown
## Frontend (Plan 2a)

The SPA lives in `optimus-fe/`. See `optimus-fe/README.md` for setup and scripts.

Quick start (with backend already running):

```bash
cd optimus-fe
bun install
bun run dev   # http://localhost:5173, proxies /api/v1 to backend on :8080
```
```

- [ ] **Step 5: CI workflow — add frontend job**

Edit `.github/workflows/ci.yml`. Append a new `frontend` job parallel to existing backend/swagger-diff/perm-check jobs:

```yaml
  frontend:
    name: Frontend
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: optimus-fe
    steps:
      - uses: actions/checkout@v4
      - name: Setup bun
        uses: oven-sh/setup-bun@v2
        with:
          bun-version: 1.1.x
      - name: Install
        run: bun install --frozen-lockfile
      - name: Lint
        run: bun run lint
      - name: Typecheck
        run: bun run typecheck
      - name: i18n keys
        run: bun run i18n:check
      - name: Unit tests
        run: bun run test
      - name: Build
        run: bun run build
```

- [ ] **Step 6: Local final sweep**

```bash
cd optimus-fe && bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build
```

Expected: all five PASS.

- [ ] **Step 7: End-to-end manual smoke (informational, not a gate)**

Start backend in another terminal:

```bash
cd optimus-be
docker compose up -d
make migrate-up
make seed     # only on a clean db; prints initial admin password
make run
```

Start frontend:

```bash
cd optimus-fe && bun run dev
```

Open http://localhost:5173/ and verify in order:

- Browser redirects to `/login`
- Sign in with the printed admin credentials
- Land on `/dashboard` (Coming-soon)
- Sidebar shows `Dashboard` + `System` (with 5 children)
- Top bar: language toggle (zh-CN ↔ en-US), theme toggle (light ↔ dark), user dropdown (Profile / Logout)
- Click any system child → its Coming-soon placeholder renders
- Open `/profile`, change `display_name`, save → success toast
- Change password (old correct, new ≥ 8 chars), save → success toast
- Log out → back to `/login`

- [ ] **Step 8: Commit**

```bash
git -C /Users/logic/Projects/optimus add optimus-fe/src/App.vue optimus-fe/src/main.ts optimus-fe/README.md README.md .github/workflows/ci.yml
git -C /Users/logic/Projects/optimus commit -m "feat(fe): wire main.ts + App.vue, add README + CI frontend job"
```

---

## Done

After Task 21 the branch state should be `dev` with ~22 commits on top of `985c0df` (3 BE + 18 FE + 1 final wire), all of:

```bash
cd optimus-be && go test ./... -count=1 && go test ./... -tags=dbtest -count=1 && golangci-lint run ./... && make swagger-diff && make perm-check
cd optimus-fe && bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build
```

green. Plan 2a complete; merge `dev` → `main` at the natural plan boundary (per 1A/1B/1C convention) before starting Plan 2b.
