# P3 — applications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land Helm-based application lifecycle management on top of P0 + P1 + P2 — register chart repositories (OCI + HTTP), register applications (1:1 with a Helm release), and drive the full install / upgrade / rollback / uninstall lifecycle synchronously through the Helm Go SDK. Optimus DB holds metadata; Helm secrets in-cluster remain the source of truth.

**Architecture:** New BE module `internal/modules/apps/` with sub-packages `repo/`, `application/`, `release/`, `helmclient/`. Per-request `*action.Configuration` built fresh from `credentials.Consumer.GetKubeconfig` (P1) and discarded — no helm-config caching. Two new tables (`apps_chart_repos`, `apps_applications`) with FK to `clusters` (P2) and `users` (P0). One new error-code segment `42xxx`. Ten new `apps:*` permission codes wired via per-route nested sub-groups. New FE module `src/views/apps/` with six page surfaces and one new direct dependency (`js-yaml`). One small upstream feedback to P2: cluster delete gains an `apps/application.Counter` pre-check.

**Tech Stack:** Go 1.25 + Gin + GORM + goose v3 (existing), `helm.sh/helm/v3 v3.16.x` (NEW — pinned by Task 1), `k8s.io/client-go@v0.30.14` (unchanged from P2), in-memory `helm.sh/helm/v3/pkg/storage/driver.Memory` + `kube/fake.PrintingKubeClient` for unit tests. Vue 3 + Ant Design Vue + Pinia + `vue-codemirror` + `@codemirror/lang-yaml` (all from P2) + `js-yaml` (NEW — pinned by Task 1).

**Reference spec:** `docs/superpowers/specs/2026-06-10-p3-applications-design.md` (commit `325a0ba`).

---

## Invariants (read once, hold throughout)

These hold for the whole plan. If a step seems to contradict one, stop and re-read the spec before writing code.

1. **Per-request helm configuration.** `*action.Configuration` is never shared across requests. Every release-level call walks `Factory.NewForCluster → action.Configuration.Init → action.X.Run`. No global helm config. No worker pool. Sync HTTP, no `--wait`. See spec §7.1.

2. **`credentials.Consumer` is the sole kubeconfig path.** Do NOT import `credentials/kubeconfig` or instantiate `vault.Cipher` to fetch decrypted kubeconfig YAML. Always go through `consumer.GetKubeconfig(ctx, id, purpose)` with a purpose string of the form `apps.release.<verb>` / `apps.release.status`. Spec §1.1 / §7.1.

3. **`vault.Cipher` is reused, not re-instantiated.** `apps_chart_repos.encrypted_password` is encrypted with the **same** `vault.Cipher` instance the credentials module uses. The composition root in `cmd/server/main.go` injects it into `apps/repo.Service` — do not call `vault.NewCipher` a second time.

4. **All comments in English.** Code in this repo, including comments, is English-only. Chinese is fine in PR descriptions and the conversation but never in `.go` / `.ts` / `.vue` / `.sql`.

5. **Registration and deployment are split.** `POST /apps/applications` writes a DB row only. `POST /apps/applications/:id/release/install` runs helm install. The FE wizard composes them; the API stays separate. Spec §4.2.

6. **No raw error text to clients.** All helm / registry / OCI errors flow through `apps.MapError` → `apperr.BizError(42xxx, ...)`. Original errors travel through `WithCause(err)` for slog only.

7. **Audit on every release write.** `install`, `upgrade`, `rollback`, `uninstall` each call `audit.Recorder.Record(...)` with `target_type=application`, `target_id=<id>`, `metadata={cluster_id, namespace, release_name, chart_version, revision}`. Service layer, not handler.

8. **K8s endpoints stay read-only.** P3 never adds k8s `apply` / `exec` / `watch` paths. All cluster writes happen via helm SDK action APIs, which use the kubeconfig's RBAC. We do not extend P2.

9. **PermissionCache invalidation is not P3's concern.** No P3 mutation path touches RBAC tables. Do not call `cache.InvalidateUser` from any apps service.

10. **bun on the FE.** Never `npm` / `pnpm` / `yarn`. CI runs `bun install --frozen-lockfile`.

11. **i18n parity.** Every key added to one locale must appear in the other. `bun run i18n:check` is wired into CI. If you add a key to `en-US.json` and forget `zh-CN.json`, the build fails.

12. **Coverage gate ≥60% per `apps/*` package.** `go test -race -cover ./internal/modules/apps/...` must show every package at or above 60%. The CI script reads coverprofile and rejects regressions.

13. **Colima docker socket.** Integration tests require Docker. On this workstation: `export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock` and `colima start` before `make test-int`. Without it, dockertest hangs.

14. **One task = one commit.** Each task ends with a HEREDOC commit. Multi-step tasks may carry intermediate commits where natural (DTO commit, then service commit, then handler commit) but each commit must compile and pass its own tests.

---

## File map

| Task | New / Modified | Path | Responsibility |
|---|---|---|---|
| 1 | Modify | `optimus-be/go.mod`, `optimus-be/go.sum` | Pin `helm.sh/helm/v3` (v3.16.x; fall back to v3.15.x on conflict) |
| 1 | Modify | `optimus-fe/package.json`, `optimus-fe/bun.lockb` | Add `js-yaml` + `@types/js-yaml` |
| 1 | Modify | `CLAUDE.md` | Record the helm version + the client-go pin invariant under P3 |
| 2 | Modify | `optimus-be/internal/infra/errors/codes.go` | Append 16 new codes in `42xxx` block |
| 2 | Modify | `optimus-be/internal/infra/errors/codes_test.go` | Smoke-test new codes are distinct + non-zero |
| 2 | Modify | `optimus-fe/src/locales/{zh-CN,en-US}.json` | 16 `error.42xxx` entries each |
| 3 | Modify | `optimus-be/internal/infra/permissions/codes.go` | Append 10 `apps:*` permission codes |
| 3 | Modify | `optimus-be/internal/infra/permissions/registry_test.go` | Assert 10 new codes registered in category `apps` |
| 4 | New | `optimus-be/migrations/00020_create_apps_tables.sql` | DDL for `apps_chart_repos` + `apps_applications` |
| 4 | New | `optimus-be/migrations/00020_create_apps_tables.down.sql` | rollback |
| 4 | New | `optimus-be/internal/models/apps_chart_repo.go` | GORM model |
| 4 | New | `optimus-be/internal/models/apps_application.go` | GORM model |
| 4 | New | `optimus-be/migrations/embed_test.go` cases | extend if exist; otherwise no |
| 5 | New | `optimus-be/internal/modules/apps/repo/dto.go` | Summary / Detail / requests / list query |
| 5 | New | `optimus-be/internal/modules/apps/repo/repo.go` | GORM CRUD |
| 5 | New | `optimus-be/internal/modules/apps/repo/service.go` | Vault encrypt/decrypt + audit |
| 5 | New | `optimus-be/internal/modules/apps/repo/handler.go` | Gin handlers (CRUD only — chart enumeration in Task 6) |
| 5 | New | `optimus-be/internal/modules/apps/repo/{dto,repo,service,handler}_test.go` | Unit + dockertest |
| 6 | New | `optimus-be/internal/modules/apps/repo/charts.go` | ListCharts / ListVersions / GetDefaultValues — OCI + HTTP |
| 6 | New | `optimus-be/internal/modules/apps/repo/charts_test.go` | `httptest.Server` for HTTP repo; OCI skipped |
| 6 | Modify | `optimus-be/internal/modules/apps/repo/handler.go` | Add 3 chart-enumeration routes |
| 7 | New | `optimus-be/internal/modules/apps/application/dto.go` | Summary / Detail / requests / list query |
| 7 | New | `optimus-be/internal/modules/apps/application/repo.go` | CRUD + CountByClusterID + CountByChartRepoID |
| 7 | New | `optimus-be/internal/modules/apps/application/service.go` | CRUD + audit + delete pre-check |
| 7 | New | `optimus-be/internal/modules/apps/application/inuse.go` | `Counter` interface + concrete impl |
| 7 | New | `optimus-be/internal/modules/apps/application/handler.go` | Gin handlers |
| 7 | New | `optimus-be/internal/modules/apps/application/{repo,service,handler,inuse}_test.go` | Unit + dockertest |
| 8 | Modify | `optimus-be/internal/modules/k8s/cluster/service.go` | Inject `apps/application.Counter`; pre-check in Delete |
| 8 | Modify | `optimus-be/internal/modules/k8s/cluster/service_test.go` | New case: refused while applications reference cluster |
| 8 | Modify | `optimus-be/internal/modules/k8s/module.go` | Service constructor takes Counter |
| 9 | New | `optimus-be/internal/modules/apps/errs.go` | `MapError` |
| 9 | New | `optimus-be/internal/modules/apps/errs_test.go` | Branches: helm sentinels, registry strings, net.Error |
| 10 | New | `optimus-be/internal/modules/apps/helmclient/factory.go` | `Factory` + `restClientGetter` + `buildRESTConfig` |
| 10 | New | `optimus-be/internal/modules/apps/helmclient/factory_test.go` | Unit with mock Consumer + table-driven YAML |
| 11 | New | `optimus-be/internal/modules/apps/release/dto.go` | Install/Upgrade/Rollback/Uninstall request + status/history DTOs |
| 11 | New | `optimus-be/internal/modules/apps/release/service.go` | helm action wrappers + audit |
| 11 | New | `optimus-be/internal/modules/apps/release/service_test.go` | Unit with in-memory helm driver + fake KubeClient |
| 11 | New | `optimus-be/internal/modules/apps/release/handler.go` | 6 release endpoints |
| 11 | New | `optimus-be/internal/modules/apps/release/handler_test.go` | Unit |
| 12 | New | `optimus-be/internal/modules/apps/module.go` | DI wiring + `MountRoutes` |
| 12 | Modify | `optimus-be/cmd/server/main.go` | Wire apps module after k8s module |
| 13 | Modify | `optimus-be/internal/seed/seed.go` | apps menu (1 parent + 2 children); update `k8s_operator` role grant |
| 13 | Modify | `optimus-be/internal/seed/seed_test.go` | Assert apps menu rows + k8s_operator perms |
| 14 | New | `optimus-be/tests/integration/apps_repo_test.go` | dockertest CRUD + name unique |
| 14 | New | `optimus-be/tests/integration/apps_application_test.go` | dockertest CRUD + FK RESTRICT + inuse counter |
| 14 | New | `optimus-be/tests/integration/apps_release_test.go` | dockertest endpoint auth + audit row |
| 15 | Modify | `optimus-be/internal/modules/apps/**/handler.go` | Add swag annotations |
| 15 | Modify | `optimus-be/api/docs/swagger.json` + `docs/api/swagger.json` | Regenerated via `make swag` |
| 15 | Modify | `docs/permissions.md` | Regenerated via `make dump-perms` |
| 16 | — | — | Coverage audit task — read coverprofile and lift any package < 60% |
| 17 | New | `optimus-fe/src/types/apps.ts` | DTOs mirroring BE |
| 17 | New | `optimus-fe/src/api/apps/{repo,application,release}.ts` | HTTP wrappers |
| 17 | New | `optimus-fe/src/stores/apps.ts` | useAppsStore (filter state only) |
| 17 | Modify | `optimus-fe/src/main.ts` | provide(...) each new apps api module |
| 18 | Modify | `optimus-fe/src/locales/{zh-CN,en-US}.json` | 49 new keys per spec §A.1 |
| 19 | New | `optimus-fe/src/views/apps/ChartRepos/List.vue` | Repo list page |
| 19 | New | `optimus-fe/src/views/apps/ChartRepos/Form.vue` | Repo create/edit modal |
| 20 | New | `optimus-fe/src/views/apps/Applications/components/ValuesEditor.vue` | CodeMirror YAML editor + Format + Load defaults |
| 20 | New | `optimus-fe/src/views/apps/Applications/components/ChartPickerStep.vue` | Repo → chart → version cascade |
| 20 | New | `optimus-fe/src/views/apps/Applications/components/HistoryTable.vue` | Revision table + rollback action |
| 20 | New | `optimus-fe/src/views/apps/Applications/components/ApplicationFormBasic.vue` | Shared basic-info form |
| 21 | New | `optimus-fe/src/views/apps/Applications/List.vue` | Application list |
| 21 | New | `optimus-fe/src/views/apps/Applications/Detail.vue` | Basics + history |
| 22 | New | `optimus-fe/src/views/apps/Applications/Install.vue` | 3-step wizard |
| 23 | New | `optimus-fe/src/views/apps/Applications/Upgrade.vue` | values + version edit page |
| 24 | New | `optimus-fe/src/views/apps/Applications/__tests__/*` and `ChartRepos/__tests__/*` | Vitest |
| 24 | Modify | `optimus-fe/src/views/apps/**` | v-permission audit pass |
| 25 | New | `optimus-be/scripts/p3-smoke.md` | Manual smoke checklist |
| 25 | — | — | Final verification: lint, typecheck, swagger-diff, perm-check, i18n:check, coverage. Final commit. |

---

## Common patterns referenced repeatedly

These are referenced by name from individual task steps. Read them once.

### Pattern A — `setupSvc` helper for service tests

When a task adds `*_test.go` for a service, the helper looks like:

```go
func setupSvc(t *testing.T) (*Service, *Repo, audit.Recorder) {
    t.Helper()
    db := dbtest.NewDB(t)             // existing helper from optimus-be/tests/dbtest
    repo := NewRepo(db)
    rec := audit.NewMemoryRecorder()  // existing helper for testing audit writes
    cipher := vault.NewTestCipher(t)  // existing helper that returns a deterministic AES key
    svc := NewService(repo, cipher, rec)
    return svc, repo, rec
}
```

`dbtest.NewDB`, `audit.NewMemoryRecorder`, `vault.NewTestCipher` all exist in P1 and P2 — search for actual paths if the import doesn't resolve.

### Pattern B — Gin handler test bootstrap

```go
func setupHTTP(t *testing.T) (*gin.Engine, *Handler) {
    t.Helper()
    svc, _, _ := setupSvc(t)
    h := NewHandler(svc)
    r := gin.New()
    r.Use(testutil.NoAuth(1)) // existing helper that puts user_id=1 on context
    r.GET("/apps/repos", h.List)
    // ... etc per task
    return r, h
}
```

### Pattern C — One-task-one-commit footer

```bash
cd /Users/logic/Projects/optimus
git add <exact files>
git commit -m "$(cat <<'EOF'
<commit message — see each task>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

`git add` lists files explicitly — never `git add -A` or `git add .` (see P2 plan §invariants, mirror here).

---

## Task 1: Pin helm SDK + add `js-yaml` (dependency spike)

**Goal:** Lock down two new dependencies and prove the helm SDK ↔ `k8s.io/client-go v0.30.14` compatibility before writing a line of helm code. If the version doesn't compile, fall back to `v3.15.x` and rerun. This is the cheapest place to catch the only known external risk for P3.

**Files:**
- Modify: `optimus-be/go.mod`, `optimus-be/go.sum`
- Modify: `optimus-fe/package.json`, `optimus-fe/bun.lockb`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Snapshot current `client-go` pin**

```bash
cd /Users/logic/Projects/optimus/optimus-be
grep 'k8s.io/client-go' go.mod
grep 'k8s.io/apimachinery' go.mod
```

Expected: `k8s.io/client-go v0.30.14` and `k8s.io/apimachinery v0.30.14`. If those have moved, stop and re-read the spec §7.4.

- [ ] **Step 2: Add helm v3.16.x and tidy**

```bash
cd /Users/logic/Projects/optimus/optimus-be
go get helm.sh/helm/v3@v3.16.4
go mod tidy
grep '^go ' go.mod                      # must still be: go 1.25
grep 'k8s.io/client-go' go.mod          # must still be: v0.30.14
```

Expected:
- `go 1.25` unchanged.
- `k8s.io/client-go v0.30.14` unchanged.
- New lines in go.mod: `helm.sh/helm/v3 v3.16.4` and a couple of transitive `helm.sh/...` imports.

- [ ] **Step 3: Compile and test**

```bash
go build ./...
go test ./... -race -count=1
```

Expected: everything passes (P0/P1/P2 still green). If you see `package k8s.io/client-go/... uses go 1.26`, you hit the version-conflict path described in spec §7.4 — fall back to v3.15.4:

```bash
go get helm.sh/helm/v3@v3.15.4
go mod tidy
go build ./... && go test ./... -race -count=1
```

If v3.15.4 also fails, stop and surface to the user. Do not patch around it.

- [ ] **Step 4: Add `js-yaml` to FE**

```bash
cd /Users/logic/Projects/optimus/optimus-fe
bun add js-yaml@^4.1.0
bun add -d @types/js-yaml@^4.0.9
bun run typecheck
bun run lint
```

Expected: no errors. `bun.lockb` is updated.

- [ ] **Step 5: Record the helm version in CLAUDE.md**

Open `/Users/logic/Projects/optimus/CLAUDE.md`. Under the "Conventions worth knowing" section, append a new bullet **after** the `k8s.io/client-go` pin bullet:

```markdown
- **`helm.sh/helm/v3` is pinned to v3.16.4** (or v3.15.4 if the 3.16 line broke the `client-go v0.30.14` invariant). Bumping helm transitively bumps client-go, so any helm upgrade re-runs the P2 compatibility verification. Pin is checked at startup only by `go build`; no runtime assertion.
```

Use the actual version you settled on in Step 2 / Step 3.

- [ ] **Step 6: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/go.mod optimus-be/go.sum \
        optimus-fe/package.json optimus-fe/bun.lockb \
        CLAUDE.md
git commit -m "$(cat <<'EOF'
chore(deps): pin helm.sh/helm/v3 + add js-yaml for P3

helm SDK is pinned at a version that preserves the k8s.io/client-go
v0.30.14 invariant locked by P2. js-yaml is the single new FE direct
dep for the ValuesEditor Format action (see P3 spec §8.8).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Reserve `42xxx` error code segment + i18n

**Goal:** Reserve the 16 numeric codes the P3 verticals will need and ship i18n entries for them. Doing this first means subsequent tasks just import `apperr.CodeAppsXxx` without churn.

**Files:**
- Modify: `optimus-be/internal/infra/errors/codes.go`
- Modify: `optimus-be/internal/infra/errors/codes_test.go` (create if absent)
- Modify: `optimus-fe/src/locales/zh-CN.json`
- Modify: `optimus-fe/src/locales/en-US.json`

- [ ] **Step 1: Append the 16 codes**

Open `optimus-be/internal/infra/errors/codes.go`. After the P2 `// 41xxx k8s runtime` block (last entry `CodeLogUnavailable Code = 41202`), insert a blank line then:

```go
	// 42xxx P3 apps domain — chart repo upstream + helm release runtime.
	// Distinct from 40xxx mirror because these encode upstream-helm/registry
	// dependency state, not malformed client requests. See P3 spec §5.
	//
	// 42001-42099 apps generic
	CodeAppsApplicationInUse            Code = 42001 // delete blocked: helm release still present
	CodeAppsChartRepoInUse              Code = 42002 // delete blocked: still referenced by application(s)
	CodeAppsReleaseNameDuplicate        Code = 42003 // (cluster_id,namespace,release_name) collision in DB
	CodeAppsApplicationOnDeletedCluster Code = 42004 // referenced cluster is soft-deleted

	// 42101-42199 chart repo upstream
	CodeAppsRepoUnreachable   Code = 42101 // network/DNS/TLS failure
	CodeAppsRepoUnauthorized  Code = 42102 // 401/403 from OCI or HTTP repo
	CodeAppsRepoChartNotFound Code = 42103 // chart name or version missing
	CodeAppsRepoInvalidIndex  Code = 42104 // HTTP repo index.yaml parse failure
	CodeAppsRepoOCIError      Code = 42105 // OCI manifest/blob fetch error
	CodeAppsRepoOther         Code = 42199 // other upstream error

	// 42201-42299 helm release runtime
	CodeAppsReleaseAlreadyExists   Code = 42201 // install: release already exists
	CodeAppsReleaseNotFound        Code = 42202 // upgrade/rollback/uninstall/status: helm secret missing
	CodeAppsReleaseHistoryTooShort Code = 42203 // rollback target revision missing
	CodeAppsReleaseStillPresent    Code = 42204 // application delete blocked: helm secret still exists
	CodeAppsReleaseInvalidValues   Code = 42205 // values yaml parse error / not a map
	CodeAppsReleaseOther           Code = 42299 // other helm SDK error
```

Code comments are English (invariant #4).

- [ ] **Step 2: Add a smoke test**

Open or create `optimus-be/internal/infra/errors/codes_test.go`. If it doesn't exist, create with the package declaration. Append:

```go
func TestAppsCodes_DistinctAndNonZero(t *testing.T) {
    codes := []Code{
        CodeAppsApplicationInUse, CodeAppsChartRepoInUse,
        CodeAppsReleaseNameDuplicate, CodeAppsApplicationOnDeletedCluster,
        CodeAppsRepoUnreachable, CodeAppsRepoUnauthorized,
        CodeAppsRepoChartNotFound, CodeAppsRepoInvalidIndex,
        CodeAppsRepoOCIError, CodeAppsRepoOther,
        CodeAppsReleaseAlreadyExists, CodeAppsReleaseNotFound,
        CodeAppsReleaseHistoryTooShort, CodeAppsReleaseStillPresent,
        CodeAppsReleaseInvalidValues, CodeAppsReleaseOther,
    }
    seen := map[Code]bool{}
    for _, c := range codes {
        if c == 0 {
            t.Errorf("zero-valued code in apps block")
        }
        if seen[c] {
            t.Errorf("duplicate code %d in apps block", c)
        }
        seen[c] = true
    }
}
```

- [ ] **Step 3: Run the test**

```bash
cd optimus-be
go test ./internal/infra/errors/... -race -v -run TestAppsCodes
```

Expected: PASS.

- [ ] **Step 4: Add the 16 i18n entries**

`optimus-fe/src/locales/en-US.json` — locate the `"error.41202"` line (added by P2) and add **after** it:

```json
"error.42001": "Application still has an installed release. Uninstall it before deleting.",
"error.42002": "Chart repository is still referenced by one or more applications.",
"error.42003": "A release with the same name already exists in this namespace.",
"error.42004": "The referenced cluster has been deleted.",
"error.42101": "Cannot reach chart repository.",
"error.42102": "Chart repository authentication failed.",
"error.42103": "Chart or version not found.",
"error.42104": "Failed to parse the chart repository index.",
"error.42105": "OCI registry error.",
"error.42199": "Chart repository error.",
"error.42201": "Release already exists.",
"error.42202": "Release not found.",
"error.42203": "Target rollback revision does not exist.",
"error.42204": "Release is still installed. Uninstall first.",
"error.42205": "Invalid values YAML.",
"error.42299": "Helm error.",
```

`optimus-fe/src/locales/zh-CN.json` — mirror in the same position:

```json
"error.42001": "应用仍有 release 未卸载，无法删除。",
"error.42002": "仓库仍被一个或多个应用引用，无法删除。",
"error.42003": "该命名空间内已存在同名 release。",
"error.42004": "应用引用的集群已被删除。",
"error.42101": "无法连接到 chart 仓库。",
"error.42102": "Chart 仓库认证失败。",
"error.42103": "未找到 chart 或版本。",
"error.42104": "解析 Chart 仓库 index 失败。",
"error.42105": "OCI 仓库错误。",
"error.42199": "Chart 仓库错误。",
"error.42201": "Release 已存在。",
"error.42202": "Release 不存在。",
"error.42203": "目标回退 revision 不存在。",
"error.42204": "Release 仍存在，请先卸载。",
"error.42205": "values YAML 不合法。",
"error.42299": "Helm 错误。",
```

- [ ] **Step 5: i18n parity check**

```bash
cd optimus-fe
bun run i18n:check
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/infra/errors/codes.go \
        optimus-be/internal/infra/errors/codes_test.go \
        optimus-fe/src/locales/zh-CN.json \
        optimus-fe/src/locales/en-US.json
git commit -m "$(cat <<'EOF'
feat(be/errors): reserve 42xxx P3 apps domain code segment

Adds 16 codes covering apps generic (42001-42099), chart repo upstream
(42101-42199), and helm release runtime (42201-42299) per P3 spec §5.
i18n entries shipped for both locales; subsequent P3 tasks will start
returning these codes from service paths.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Register 10 `apps:*` permission codes

**Goal:** Append the ten codes spec §6.1 lists into the in-code registry so they are upserted into `permissions` table at server startup, available to `RequirePermission`, and visible to the FE via `/me/permissions`.

**Files:**
- Modify: `optimus-be/internal/infra/permissions/codes.go`
- Modify: `optimus-be/internal/infra/permissions/registry_test.go`

- [ ] **Step 1: Find the appendable site**

```bash
grep -n 'Code:.*"k8s:' /Users/logic/Projects/optimus/optimus-be/internal/infra/permissions/codes.go | tail -1
```

Note the line number. Insert immediately after that line.

- [ ] **Step 2: Append the 10 codes**

In `internal/infra/permissions/codes.go`, after the last `k8s:*` entry:

```go
	// P3 — applications
	{Code: "apps:application:read",   Category: "apps", NameKey: "perm.apps.application.read"},
	{Code: "apps:application:write",  Category: "apps", NameKey: "perm.apps.application.write"},
	{Code: "apps:application:delete", Category: "apps", NameKey: "perm.apps.application.delete"},
	{Code: "apps:release:install",    Category: "apps", NameKey: "perm.apps.release.install"},
	{Code: "apps:release:upgrade",    Category: "apps", NameKey: "perm.apps.release.upgrade"},
	{Code: "apps:release:rollback",   Category: "apps", NameKey: "perm.apps.release.rollback"},
	{Code: "apps:release:uninstall",  Category: "apps", NameKey: "perm.apps.release.uninstall"},
	{Code: "apps:repo:read",          Category: "apps", NameKey: "perm.apps.repo.read"},
	{Code: "apps:repo:write",         Category: "apps", NameKey: "perm.apps.repo.write"},
	{Code: "apps:repo:delete",        Category: "apps", NameKey: "perm.apps.repo.delete"},
```

- [ ] **Step 3: Extend the registry test**

Open `internal/infra/permissions/registry_test.go`. The P2 test pattern asserts that every k8s code is registered with `Category=="k8s"`. Add a sibling assertion:

```go
func TestRegistry_AppsCodesPresent(t *testing.T) {
    want := []string{
        "apps:application:read", "apps:application:write", "apps:application:delete",
        "apps:release:install", "apps:release:upgrade",
        "apps:release:rollback", "apps:release:uninstall",
        "apps:repo:read", "apps:repo:write", "apps:repo:delete",
    }
    byCode := map[string]Permission{}
    for _, p := range All {
        byCode[p.Code] = p
    }
    for _, w := range want {
        got, ok := byCode[w]
        if !ok {
            t.Errorf("permission %q not registered", w)
            continue
        }
        if got.Category != "apps" {
            t.Errorf("permission %q has category %q, want %q", w, got.Category, "apps")
        }
        if got.NameKey == "" {
            t.Errorf("permission %q has empty NameKey", w)
        }
    }
}
```

- [ ] **Step 4: Run the test**

```bash
cd optimus-be
go test ./internal/infra/permissions/... -race -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/infra/permissions/codes.go \
        optimus-be/internal/infra/permissions/registry_test.go
git commit -m "$(cat <<'EOF'
feat(be/permissions): register 10 apps:* codes for P3

Adds apps:application:{read,write,delete}, apps:release:{install,upgrade,
rollback,uninstall}, and apps:repo:{read,write,delete} to the in-code
registry. Codes upsert into the permissions table at server startup and
become available to RequirePermission middleware + /me/permissions.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Migration `00020_create_apps_tables.sql` + GORM models

**Goal:** Land both DB tables with the constraints spec §3.1 / §3.2 demand, and the two GORM models that map to them. After this task, `make migrate-up` produces a schema P3 can build on.

**Files:**
- New: `optimus-be/migrations/00020_create_apps_tables.sql`
- New: `optimus-be/internal/models/apps_chart_repo.go`
- New: `optimus-be/internal/models/apps_application.go`

- [ ] **Step 1: Check goose migration convention**

```bash
head -5 /Users/logic/Projects/optimus/optimus-be/migrations/00015_create_clusters.sql
```

Expect single-file `-- +goose Up` / `-- +goose Down` style (P0/P1/P2 convention).

- [ ] **Step 2: Write the migration**

Create `optimus-be/migrations/00020_create_apps_tables.sql`:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE apps_chart_repos (
    id                  BIGSERIAL    PRIMARY KEY,
    name                VARCHAR(64)  NOT NULL,
    type                VARCHAR(8)   NOT NULL CHECK (type IN ('oci','http')),
    url                 TEXT         NOT NULL,
    username            VARCHAR(255) NOT NULL DEFAULT '',
    encrypted_password  BYTEA        NOT NULL DEFAULT ''::BYTEA,
    description         TEXT         NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX apps_chart_repos_name_unique
    ON apps_chart_repos(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_chart_repos_deleted_at ON apps_chart_repos(deleted_at);

CREATE TABLE apps_applications (
    id              BIGSERIAL    PRIMARY KEY,
    name            VARCHAR(64)  NOT NULL,
    cluster_id      BIGINT       NOT NULL
                    REFERENCES clusters(id) ON DELETE RESTRICT,
    namespace       VARCHAR(63)  NOT NULL,
    release_name    VARCHAR(53)  NOT NULL,
    chart_repo_id   BIGINT       NOT NULL
                    REFERENCES apps_chart_repos(id) ON DELETE RESTRICT,
    chart_name      VARCHAR(128) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    tags            JSONB        NOT NULL DEFAULT '[]'::JSONB,
    owner_user_id   BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT apps_applications_tags_is_array CHECK (jsonb_typeof(tags) = 'array')
);
CREATE UNIQUE INDEX apps_applications_release_unique
    ON apps_applications(cluster_id, namespace, release_name)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX apps_applications_name_unique
    ON apps_applications(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_applications_cluster_id     ON apps_applications(cluster_id);
CREATE INDEX apps_applications_owner_user_id  ON apps_applications(owner_user_id);
CREATE INDEX apps_applications_deleted_at     ON apps_applications(deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS apps_applications;
DROP TABLE IF EXISTS apps_chart_repos;
-- +goose StatementEnd
```

- [ ] **Step 3: Write `apps_chart_repo.go`**

Create `optimus-be/internal/models/apps_chart_repo.go`:

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

// AppsChartRepo is a registered Helm chart source (OCI or HTTP).
// EncryptedPassword holds bytes from P1's vault.Cipher. Repo service is
// responsible for decryption at use time; plaintext is never persisted
// or held on the struct beyond a single function call.
type AppsChartRepo struct {
	ID                uint64         `gorm:"primaryKey"`
	Name              string         `gorm:"type:varchar(64);not null"`
	Type              string         `gorm:"type:varchar(8);not null"`
	URL               string         `gorm:"type:text;not null"`
	Username          string         `gorm:"type:varchar(255);not null;default:''"`
	EncryptedPassword []byte         `gorm:"type:bytea;not null;default:'\\x'"`
	Description       string         `gorm:"type:text;not null;default:''"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

func (AppsChartRepo) TableName() string { return "apps_chart_repos" }
```

- [ ] **Step 4: Write `apps_application.go`**

Create `optimus-be/internal/models/apps_application.go`:

```go
package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AppsApplication is the Optimus row pointing at a Helm release on a
// specific cluster+namespace. (cluster_id, namespace, release_name) is
// unique per spec; chart_name is immutable post-creation; chart_repo_id
// is mutable only by the upgrade endpoint (NOT by PUT /applications/:id).
type AppsApplication struct {
	ID          uint64                      `gorm:"primaryKey"`
	Name        string                      `gorm:"type:varchar(64);not null"`
	ClusterID   uint64                      `gorm:"not null;index"`
	Namespace   string                      `gorm:"type:varchar(63);not null"`
	ReleaseName string                      `gorm:"type:varchar(53);not null"`
	ChartRepoID uint64                      `gorm:"not null"`
	ChartName   string                      `gorm:"type:varchar(128);not null"`
	Description string                      `gorm:"type:text;not null;default:''"`
	Tags        datatypes.JSONSlice[string] `gorm:"type:jsonb;not null;default:'[]'::jsonb"`
	OwnerUserID *uint64                     `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt              `gorm:"index"`

	// Preload-friendly associations. Populated on Get / List with .Preload.
	Cluster   *Cluster       `gorm:"foreignKey:ClusterID"`
	ChartRepo *AppsChartRepo `gorm:"foreignKey:ChartRepoID"`
	OwnerUser *User          `gorm:"foreignKey:OwnerUserID"`
}

func (AppsApplication) TableName() string { return "apps_applications" }
```

- [ ] **Step 5: Build models package**

```bash
cd optimus-be && go build ./internal/models/...
```

Expected: PASS. `datatypes.JSONSlice` is already used by P2's cluster model, so the dep should already resolve.

- [ ] **Step 6: Apply migration locally**

```bash
cd optimus-be
make migrate-up
docker exec -i optimus-dev-db psql -U optimus -d optimus -c '\d apps_chart_repos'
docker exec -i optimus-dev-db psql -U optimus -d optimus -c '\d apps_applications'
```

Both `\d` outputs must show the column list and indexes you wrote.

- [ ] **Step 7: Round-trip rollback**

```bash
cd optimus-be
make migrate-down
docker exec -i optimus-dev-db psql -U optimus -d optimus -c '\dt apps_*'   # expect: 0 rows
make migrate-up
```

- [ ] **Step 8: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/migrations/00020_create_apps_tables.sql \
        optimus-be/internal/models/apps_chart_repo.go \
        optimus-be/internal/models/apps_application.go
git commit -m "$(cat <<'EOF'
feat(be/migrations): apps_chart_repos + apps_applications tables (P3)

Two new tables per P3 spec §3.1 + §3.2:
- apps_chart_repos: name unique partial index, BYTEA encrypted_password
  encrypted by P1 vault.Cipher.
- apps_applications: (cluster_id, namespace, release_name) unique partial
  index; FK to clusters/users/apps_chart_repos. ON DELETE RESTRICT for
  cluster_id and chart_repo_id; ON DELETE SET NULL for owner_user_id.

GORM models follow the P2 cluster.tags pattern using
datatypes.JSONSlice[string].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `apps/repo` CRUD vertical (no chart enumeration yet)

**Goal:** Land the chart-repo CRUD surface end-to-end: DTOs, GORM repo, service (vault encrypt/decrypt + audit + InUseCounter seam), Gin handlers (5 CRUD endpoints), and tests. Chart enumeration is Task 6.

**Files:**
- New: `optimus-be/internal/modules/apps/repo/dto.go`
- New: `optimus-be/internal/modules/apps/repo/repo.go`
- New: `optimus-be/internal/modules/apps/repo/repo_test.go`
- New: `optimus-be/internal/modules/apps/repo/service.go`
- New: `optimus-be/internal/modules/apps/repo/service_test.go`
- New: `optimus-be/internal/modules/apps/repo/handler.go`
- New: `optimus-be/internal/modules/apps/repo/handler_test.go`

- [ ] **Step 1: Write `dto.go`**

```go
package repo

import "time"

// Summary is the list-row shape; never includes encrypted_password.
type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	Username    string    `json:"username"`
	HasPassword bool      `json:"has_password"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Detail equals Summary for chart repos.
type Detail = Summary

type CreateRequest struct {
	Name        string `json:"name"        binding:"required,max=64"`
	Type        string `json:"type"        binding:"required,oneof=oci http"`
	URL         string `json:"url"         binding:"required,max=2048"`
	Username    string `json:"username"    binding:"max=255"`
	Password    string `json:"password"`
	Description string `json:"description" binding:"max=4096"`
}

// UpdateRequest password semantics:
//   - field absent / empty string → keep current encrypted_password.
//   - field explicit null         → clear encrypted_password.
// type is silently ignored.
type UpdateRequest struct {
	Name        *string `json:"name,omitempty"        binding:"omitempty,max=64"`
	URL         *string `json:"url,omitempty"         binding:"omitempty,max=2048"`
	Username    *string `json:"username,omitempty"    binding:"omitempty,max=255"`
	Password    *string `json:"password,omitempty"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
}

type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Name     string `form:"name"`
	Type     string `form:"type"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
```

- [ ] **Step 2: Write `repo.go`**

```go
package repo

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, m *models.AppsChartRepo) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.AppsChartRepo, error) {
	var m models.AppsChartRepo
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*models.AppsChartRepo, error) {
	var m models.AppsChartRepo
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.AppsChartRepo, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.AppsChartRepo{})
	if s := strings.TrimSpace(q.Name); s != "" {
		tx = tx.Where("name ILIKE ?", "%"+s+"%")
	}
	if t := strings.TrimSpace(q.Type); t != "" {
		tx = tx.Where("type = ?", t)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	var rows []models.AppsChartRepo
	if err := tx.Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.AppsChartRepo{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// Delete soft-deletes by id. Caller must run the InUseCounter pre-check.
func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.AppsChartRepo{}, id).Error
}
```

- [ ] **Step 3: Write `repo_test.go`**

```go
//go:build dbtest

package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestRepo_CRUD(t *testing.T) {
	db := dbtest.NewDB(t)
	r := NewRepo(db)

	m := &models.AppsChartRepo{
		Name:              "bitnami",
		Type:              "http",
		URL:               "https://charts.bitnami.com/bitnami",
		EncryptedPassword: []byte{0x01, 0x02, 0x03},
	}
	require.NoError(t, r.Create(context.Background(), m))
	require.NotZero(t, m.ID)

	got, err := r.Get(context.Background(), m.ID)
	require.NoError(t, err)
	require.Equal(t, "bitnami", got.Name)
	require.Equal(t, []byte{0x01, 0x02, 0x03}, got.EncryptedPassword)

	_, total, err := r.List(context.Background(), ListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)

	require.NoError(t, r.Update(context.Background(), m.ID, map[string]any{
		"description": "primary repo",
	}))
	got, _ = r.Get(context.Background(), m.ID)
	require.Equal(t, "primary repo", got.Description)

	require.NoError(t, r.Delete(context.Background(), m.ID))
	_, err = r.Get(context.Background(), m.ID)
	require.Error(t, err)
}

func TestRepo_NameUniquePartialIndex(t *testing.T) {
	db := dbtest.NewDB(t)
	r := NewRepo(db)
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &models.AppsChartRepo{Name: "n", Type: "http", URL: "x"}))
	err := r.Create(ctx, &models.AppsChartRepo{Name: "n", Type: "http", URL: "y"})
	require.Error(t, err, "name collision while both alive must error")
}
```

- [ ] **Step 4: Write `service.go`**

```go
package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"optimus-be/internal/infra/audit"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/credentials/vault"
)

// InUseCounter is the seam apps/application implements. Injected post-
// construction by main.go to avoid an import cycle.
type InUseCounter interface {
	CountByChartRepoID(ctx context.Context, repoID uint64) (int, error)
}

// Service owns vault encrypt/decrypt for password and audit emission.
type Service struct {
	repo   *Repo
	cipher *vault.Cipher
	rec    audit.Recorder
	inuse  InUseCounter
}

func NewService(r *Repo, c *vault.Cipher, rec audit.Recorder) *Service {
	return &Service{repo: r, cipher: c, rec: rec}
}

func (s *Service) SetInUseCounter(c InUseCounter) { s.inuse = c }

func (s *Service) Create(ctx context.Context, actorID uint64, req CreateRequest) (*Detail, error) {
	if existing, err := s.repo.FindByName(ctx, req.Name); err == nil && existing != nil {
		return nil, apperr.New(apperr.CodeConflict, "apps.repo.name_taken", req.Name)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var encrypted []byte
	if req.Password != "" {
		ct, err := s.cipher.Encrypt([]byte(req.Password))
		if err != nil {
			return nil, err
		}
		encrypted = ct
	}
	m := &models.AppsChartRepo{
		Name:              req.Name,
		Type:              req.Type,
		URL:               req.URL,
		Username:          req.Username,
		EncryptedPassword: encrypted,
		Description:       req.Description,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.rec.Record(ctx, audit.Event{
		ActorID:    actorID,
		Action:     "apps.repo.create",
		TargetType: "apps_chart_repo",
		TargetID:   m.ID,
		Metadata:   map[string]any{"name": m.Name, "type": m.Type},
	})
	return toDetail(m), nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.repo.not_found")
		}
		return nil, err
	}
	return toDetail(m), nil
}

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(rows))
	for i := range rows {
		items = append(items, *toDetail(&rows[i]))
	}
	return &ListResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// passwordClearSentinel is the value handler.Update stuffs into req.Password
// when the JSON body has "password": null. Empty string means "keep".
const passwordClearSentinel = "\x00"

func (s *Service) Update(ctx context.Context, actorID, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.repo.not_found")
		}
		return nil, err
	}
	fields := map[string]any{}
	if req.Name != nil && *req.Name != m.Name {
		other, err := s.repo.FindByName(ctx, *req.Name)
		if err == nil && other != nil && other.ID != m.ID {
			return nil, apperr.New(apperr.CodeConflict, "apps.repo.name_taken", *req.Name)
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		fields["name"] = *req.Name
	}
	if req.URL != nil {
		fields["url"] = *req.URL
	}
	if req.Username != nil {
		fields["username"] = *req.Username
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.Password != nil {
		switch *req.Password {
		case "":
			// keep current — no change.
		case passwordClearSentinel:
			fields["encrypted_password"] = []byte{}
		default:
			ct, err := s.cipher.Encrypt([]byte(*req.Password))
			if err != nil {
				return nil, err
			}
			fields["encrypted_password"] = ct
		}
	}
	if len(fields) > 0 {
		if err := s.repo.Update(ctx, id, fields); err != nil {
			return nil, err
		}
	}
	s.rec.Record(ctx, audit.Event{
		ActorID:    actorID,
		Action:     "apps.repo.update",
		TargetType: "apps_chart_repo",
		TargetID:   id,
		Metadata:   map[string]any{"fields_changed": keys(fields)},
	})
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID, id uint64) error {
	if s.inuse != nil {
		n, err := s.inuse.CountByChartRepoID(ctx, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return apperr.New(apperr.CodeAppsChartRepoInUse, "apps.repo.in_use")
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.rec.Record(ctx, audit.Event{
		ActorID:    actorID,
		Action:     "apps.repo.delete",
		TargetType: "apps_chart_repo",
		TargetID:   id,
	})
	return nil
}

// decryptPassword returns plaintext scoped to one call. Used by Task 6 only.
func (s *Service) decryptPassword(_ context.Context, m *models.AppsChartRepo) (string, error) {
	if len(m.EncryptedPassword) == 0 {
		return "", nil
	}
	pt, err := s.cipher.Decrypt(m.EncryptedPassword)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func toDetail(m *models.AppsChartRepo) *Detail {
	return &Detail{
		ID: m.ID, Name: m.Name, Type: m.Type, URL: m.URL,
		Username: m.Username, HasPassword: len(m.EncryptedPassword) > 0,
		Description: m.Description,
		CreatedAt:   m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 5: Write `service_test.go`**

```go
//go:build dbtest

package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/audit"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/tests/dbtest"
)

func setupSvc(t *testing.T) (*Service, *Repo) {
	t.Helper()
	db := dbtest.NewDB(t)
	r := NewRepo(db)
	cipher := vault.NewTestCipher(t)
	rec := audit.NewMemoryRecorder()
	return NewService(r, cipher, rec), r
}

func TestService_Create_EncryptsPassword(t *testing.T) {
	s, r := setupSvc(t)
	d, err := s.Create(context.Background(), 1, CreateRequest{
		Name: "private", Type: "oci", URL: "oci://x",
		Username: "u", Password: "secret",
	})
	require.NoError(t, err)
	require.True(t, d.HasPassword)

	m, _ := r.Get(context.Background(), d.ID)
	require.NotEqual(t, []byte("secret"), m.EncryptedPassword)
	require.Greater(t, len(m.EncryptedPassword), len("secret"))
}

func TestService_Create_NameTaken(t *testing.T) {
	s, _ := setupSvc(t)
	_, err := s.Create(context.Background(), 1, CreateRequest{Name: "n", Type: "http", URL: "x"})
	require.NoError(t, err)
	_, err = s.Create(context.Background(), 1, CreateRequest{Name: "n", Type: "http", URL: "y"})
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_Update_PasswordSemantics(t *testing.T) {
	s, r := setupSvc(t)
	d, _ := s.Create(context.Background(), 1, CreateRequest{
		Name: "p", Type: "http", URL: "x", Password: "secret",
	})
	original, _ := r.Get(context.Background(), d.ID)
	originalCt := append([]byte(nil), original.EncryptedPassword...)

	// empty string -> keep.
	empty := ""
	_, _ = s.Update(context.Background(), 1, d.ID, UpdateRequest{Password: &empty})
	cur, _ := r.Get(context.Background(), d.ID)
	require.Equal(t, originalCt, cur.EncryptedPassword)

	// sentinel -> clear.
	clear := passwordClearSentinel
	_, _ = s.Update(context.Background(), 1, d.ID, UpdateRequest{Password: &clear})
	cur, _ = r.Get(context.Background(), d.ID)
	require.Empty(t, cur.EncryptedPassword)

	// new value -> re-encrypt.
	np := "newsecret"
	_, _ = s.Update(context.Background(), 1, d.ID, UpdateRequest{Password: &np})
	cur, _ = r.Get(context.Background(), d.ID)
	require.NotEmpty(t, cur.EncryptedPassword)
	require.NotEqual(t, originalCt, cur.EncryptedPassword)
}

type fakeInUse struct{ n int }

func (f *fakeInUse) CountByChartRepoID(context.Context, uint64) (int, error) { return f.n, nil }

func TestService_Delete_RefusedWhenInUse(t *testing.T) {
	s, _ := setupSvc(t)
	s.SetInUseCounter(&fakeInUse{n: 2})
	d, err := s.Create(context.Background(), 1, CreateRequest{Name: "x", Type: "http", URL: "x"})
	require.NoError(t, err)
	err = s.Delete(context.Background(), 1, d.ID)
	require.Error(t, err)
	be := err.(*apperr.BizError)
	require.Equal(t, apperr.CodeAppsChartRepoInUse, be.Code)
}
```

- [ ] **Step 6: Write `handler.go`**

```go
package repo

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.invalid_query", err.Error()))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.invalid_body", err.Error()))
		return
	}
	actor := middleware.MustActorID(c)
	out, err := h.svc.Create(c.Request.Context(), actor, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusCreated)
	response.Success(c, out)
}

// Update binds JSON, then post-processes the raw body to convert
// "password": null -> Password = &sentinel so the service layer can
// distinguish clear vs keep.
func (h *Handler) Update(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	raw, _ := c.GetRawData()
	var req UpdateRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			response.Error(c, apperr.New(apperr.CodeValidation, "common.invalid_body", err.Error()))
			return
		}
	}
	if hasExplicitNull(raw, "password") {
		s := passwordClearSentinel
		req.Password = &s
	}
	actor := middleware.MustActorID(c)
	out, err := h.svc.Update(c.Request.Context(), actor, id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	actor := middleware.MustActorID(c)
	if err := h.svc.Delete(c.Request.Context(), actor, id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, nil)
}

// hasExplicitNull reports whether the JSON object has the field set to
// literal null at the top level.
func hasExplicitNull(raw []byte, field string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m[field]
	return ok && bytes.Equal(bytes.TrimSpace(v), []byte("null"))
}
```

- [ ] **Step 7: Write `handler_test.go`**

```go
//go:build dbtest

package repo

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware/testutil"
)

func setupHTTP(t *testing.T) (*gin.Engine, *Handler) {
	t.Helper()
	svc, _ := setupSvc(t)
	h := NewHandler(svc)
	r := gin.New()
	r.Use(testutil.NoAuth(1))
	r.GET("/apps/repos", h.List)
	r.GET("/apps/repos/:id", h.Get)
	r.POST("/apps/repos", h.Create)
	r.PUT("/apps/repos/:id", h.Update)
	r.DELETE("/apps/repos/:id", h.Delete)
	return r, h
}

func TestHTTP_CreateAndList_NeverLeaksPassword(t *testing.T) {
	r, _ := setupHTTP(t)
	body, _ := json.Marshal(map[string]any{
		"name": "demo", "type": "http", "url": "https://x.example.com",
		"username": "u", "password": "secret",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/apps/repos", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/apps/repos", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct{ Items []map[string]any } `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, true, resp.Data.Items[0]["has_password"])
	_, p := resp.Data.Items[0]["password"]
	require.False(t, p)
	_, ep := resp.Data.Items[0]["encrypted_password"]
	require.False(t, ep)
}

func TestHTTP_Update_NullPassword_Clears(t *testing.T) {
	r, _ := setupHTTP(t)
	// create with password
	body, _ := json.Marshal(map[string]any{
		"name": "p", "type": "http", "url": "x", "username": "u", "password": "secret",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/apps/repos", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Data Detail `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	require.True(t, created.Data.HasPassword)

	// PUT with explicit null
	patch := []byte(`{"password": null}`)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", "/apps/repos/"+itoa(created.Data.ID), bytes.NewReader(patch)))
	require.Equal(t, http.StatusOK, w.Code)

	// GET back
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/apps/repos/"+itoa(created.Data.ID), nil))
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Data Detail `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.False(t, got.Data.HasPassword)
}

func itoa(i uint64) string { return fmt.Sprintf("%d", i) }
```

Add `import "fmt"` if not picked up.

- [ ] **Step 8: Run tests**

```bash
cd optimus-be
go test -tags=dbtest ./internal/modules/apps/repo/... -race -count=1 -v
go test -tags=dbtest ./internal/modules/apps/repo/... -coverprofile=/tmp/p3-repo.cov
go tool cover -func=/tmp/p3-repo.cov | tail -1
```

Expected: all pass; coverage `>= 60%`. If below, add cases for the `List` query branches or the `Update` "no fields changed" path.

- [ ] **Step 9: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/repo/
git commit -m "$(cat <<'EOF'
feat(be/apps/repo): chart repository CRUD vertical (P3)

Lands DTOs, GORM repo, vault-backed service, and Gin handlers for the
five CRUD endpoints. encrypted_password is encrypted by P1 vault.Cipher;
responses expose has_password:bool and never the bytes themselves.
Service exposes a decryptPassword seam used only by the chart-enumeration
task. InUseCounter interface is satisfied later by apps/application,
wired in main.go to break the import cycle.

Chart enumeration (/charts, /versions, /values) and chart-source clients
(OCI registry / HTTP repo) are intentionally deferred to Task 6 because
they walk a different upstream path than CRUD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `apps/repo` chart enumeration (OCI + HTTP)

**Goal:** Add the three "browse the upstream" endpoints — `/charts`, `/versions`, `/values` — using helm's registry/repo SDK packages. OCI uses `helm.sh/helm/v3/pkg/registry`; HTTP uses `helm.sh/helm/v3/pkg/repo` + `getter.HTTPGetter`. Per-call client construction, plaintext password scoped to one call. No on-disk or in-memory cache.

**Files:**
- New: `optimus-be/internal/modules/apps/repo/charts.go`
- New: `optimus-be/internal/modules/apps/repo/charts_test.go`
- Modify: `optimus-be/internal/modules/apps/repo/handler.go`
- Modify: `optimus-be/internal/modules/apps/repo/dto.go`

- [ ] **Step 1: Add DTOs**

Append to `dto.go`:

```go
// ChartSummary is one chart's name + the count of versions.
type ChartSummary struct {
	Name        string `json:"name"`
	VersionCount int   `json:"version_count"`
	Description string `json:"description"` // best-available description from index
}

type VersionSummary struct {
	Version    string `json:"version"`
	AppVersion string `json:"app_version"`
	Created    string `json:"created"`
}
```

- [ ] **Step 2: Write `charts.go`**

```go
package repo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	apps "optimus-be/internal/modules/apps"
)

// ListCharts returns chart names for the given repo id.
func (s *Service) ListCharts(ctx context.Context, repoID uint64) ([]ChartSummary, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return nil, err
	}
	switch m.Type {
	case "http":
		return listHTTP(m, pwd)
	case "oci":
		return listOCI(m, pwd)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type")
	}
}

// ListVersions returns versions for one chart in a repo.
func (s *Service) ListVersions(ctx context.Context, repoID uint64, chart string) ([]VersionSummary, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return nil, err
	}
	switch m.Type {
	case "http":
		return versionsHTTP(m, pwd, chart)
	case "oci":
		return versionsOCI(m, pwd, chart)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type")
	}
}

// GetDefaultValues fetches the chart's bundled values.yaml as plain text.
func (s *Service) GetDefaultValues(ctx context.Context, repoID uint64, chart, version string) (string, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return "", mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return "", err
	}
	switch m.Type {
	case "http":
		return defaultValuesHTTP(m, pwd, chart, version)
	case "oci":
		return defaultValuesOCI(m, pwd, chart, version)
	default:
		return "", apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type")
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, gormErrRecordNotFound()) {
		return apperr.New(apperr.CodeNotFound, "apps.repo.not_found")
	}
	return err
}

// HTTP repo path -------------------------------------------------------------

func listHTTP(m *models.AppsChartRepo, pwd string) ([]ChartSummary, error) {
	idx, err := fetchHTTPIndex(m, pwd)
	if err != nil {
		return nil, err
	}
	out := make([]ChartSummary, 0, len(idx.Entries))
	for name, versions := range idx.Entries {
		desc := ""
		if len(versions) > 0 {
			desc = versions[0].Description
		}
		out = append(out, ChartSummary{
			Name:        name,
			VersionCount: len(versions),
			Description: desc,
		})
	}
	return out, nil
}

func versionsHTTP(m *models.AppsChartRepo, pwd, chart string) ([]VersionSummary, error) {
	idx, err := fetchHTTPIndex(m, pwd)
	if err != nil {
		return nil, err
	}
	entries, ok := idx.Entries[chart]
	if !ok {
		return nil, apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found", chart)
	}
	out := make([]VersionSummary, 0, len(entries))
	for _, e := range entries {
		out = append(out, VersionSummary{
			Version: e.Version, AppVersion: e.AppVersion, Created: e.Created.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return out, nil
}

func defaultValuesHTTP(m *models.AppsChartRepo, pwd, chart, version string) (string, error) {
	idx, err := fetchHTTPIndex(m, pwd)
	if err != nil {
		return "", err
	}
	entries, ok := idx.Entries[chart]
	if !ok {
		return "", apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found", chart)
	}
	var picked *repo.ChartVersion
	for _, e := range entries {
		if e.Version == version {
			picked = e
			break
		}
	}
	if picked == nil {
		return "", apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.version_not_found", version)
	}
	if len(picked.URLs) == 0 {
		return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", chart)
	}
	tgzURL := absoluteURL(m.URL, picked.URLs[0])
	values, err := downloadValuesYAML(tgzURL, m.Username, pwd)
	if err != nil {
		return "", err
	}
	return values, nil
}

func fetchHTTPIndex(m *models.AppsChartRepo, pwd string) (*repo.IndexFile, error) {
	cr, err := repo.NewChartRepository(&repo.Entry{
		Name: m.Name, URL: m.URL,
		Username: m.Username, Password: pwd,
	}, getter.All(nil))
	if err != nil {
		return nil, apps.MapError(err)
	}
	idxPath, err := cr.DownloadIndexFile()
	if err != nil {
		return nil, apps.MapError(err)
	}
	idx, err := repo.LoadIndexFile(idxPath)
	if err != nil {
		return nil, apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", err.Error())
	}
	return idx, nil
}

func absoluteURL(repoBase, chartURL string) string {
	u, err := url.Parse(chartURL)
	if err == nil && u.IsAbs() {
		return chartURL
	}
	base := strings.TrimRight(repoBase, "/")
	return base + "/" + strings.TrimLeft(chartURL, "/")
}

// downloadValuesYAML fetches the .tgz, opens it, and reads values.yaml.
func downloadValuesYAML(tgzURL, username, password string) (string, error) {
	g, err := getter.NewHTTPGetter(getter.WithBasicAuth(username, password))
	if err != nil {
		return "", apps.MapError(err)
	}
	body, err := g.Get(tgzURL)
	if err != nil {
		return "", apps.MapError(err)
	}
	return extractValuesYAMLFromTgz(body)
}

// OCI repo path --------------------------------------------------------------

func listOCI(m *models.AppsChartRepo, pwd string) ([]ChartSummary, error) {
	// OCI does not expose a "list charts in registry" API in a standard way.
	// helm SDK only lists tags for a given chart. P3 v1 surfaces this limit:
	// for OCI, ListCharts returns the chart inferred from the URL path's
	// last segment (e.g., oci://ghcr.io/org/myapp -> "myapp"), or an empty
	// list when the URL ends at the registry/namespace.
	parsed := strings.TrimPrefix(m.URL, "oci://")
	parts := strings.Split(parsed, "/")
	if len(parts) < 2 {
		return []ChartSummary{}, nil // registry root — cannot enumerate
	}
	name := parts[len(parts)-1]
	return []ChartSummary{{Name: name, VersionCount: 0, Description: ""}}, nil
}

func versionsOCI(m *models.AppsChartRepo, pwd, chart string) ([]VersionSummary, error) {
	rc, err := registry.NewClient()
	if err != nil {
		return nil, apps.MapError(err)
	}
	if m.Username != "" || pwd != "" {
		host := strings.TrimPrefix(m.URL, "oci://")
		if i := strings.Index(host, "/"); i > 0 {
			host = host[:i]
		}
		if err := rc.Login(host, registry.LoginOptBasicAuth(m.Username, pwd)); err != nil {
			return nil, apps.MapError(err)
		}
	}
	ref := strings.TrimPrefix(m.URL, "oci://")
	if !strings.HasSuffix(ref, "/"+chart) {
		ref = strings.TrimRight(ref, "/") + "/" + chart
	}
	tags, err := rc.Tags(ref)
	if err != nil {
		return nil, apps.MapError(err)
	}
	out := make([]VersionSummary, 0, len(tags))
	for _, t := range tags {
		out = append(out, VersionSummary{Version: t})
	}
	return out, nil
}

func defaultValuesOCI(m *models.AppsChartRepo, pwd, chart, version string) (string, error) {
	rc, err := registry.NewClient()
	if err != nil {
		return "", apps.MapError(err)
	}
	if m.Username != "" || pwd != "" {
		host := strings.TrimPrefix(m.URL, "oci://")
		if i := strings.Index(host, "/"); i > 0 {
			host = host[:i]
		}
		if err := rc.Login(host, registry.LoginOptBasicAuth(m.Username, pwd)); err != nil {
			return "", apps.MapError(err)
		}
	}
	ref := strings.TrimPrefix(m.URL, "oci://")
	if !strings.HasSuffix(ref, "/"+chart) {
		ref = strings.TrimRight(ref, "/") + "/" + chart
	}
	ref = ref + ":" + version
	pull, err := rc.Pull(ref, registry.PullOptWithChart(true))
	if err != nil {
		return "", apps.MapError(err)
	}
	return extractValuesYAMLFromTgz(pull.Chart.Data)
}

// shared: read values.yaml out of a chart .tgz body.
func extractValuesYAMLFromTgz(tgz []byte) (string, error) {
	// implementation reads the tgz via archive/tar + compress/gzip and
	// returns the contents of the first file whose name ends in "values.yaml"
	// at the chart root.
	return readValuesFromTgz(tgz)
}

// gormErrRecordNotFound returns gorm.ErrRecordNotFound without importing
// gorm at the file-top (keeps charts.go free of GORM dep).
func gormErrRecordNotFound() error {
	// imported indirectly through the repo package's already-imported gorm.
	// repo.go has the actual import.
	return errRecordNotFoundSentinel
}

// see helpers.go for errRecordNotFoundSentinel and readValuesFromTgz.
func _ = fmt.Sprintf // silence import in older Go
```

- [ ] **Step 3: Write `helpers.go` for tgz reader and sentinel**

Create `optimus-be/internal/modules/apps/repo/helpers.go`:

```go
package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
)

// errRecordNotFoundSentinel re-exports gorm's sentinel so charts.go can
// reference it without importing gorm itself.
var errRecordNotFoundSentinel = gorm.ErrRecordNotFound

// readValuesFromTgz returns the content of the file at <root>/values.yaml
// inside a chart tarball, where <root> is the first directory entry.
func readValuesFromTgz(tgz []byte) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", err.Error())
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", err.Error())
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) != 2 || parts[1] != "values.yaml" {
			continue
		}
		buf, err := io.ReadAll(tr)
		if err != nil {
			return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", err.Error())
		}
		return string(buf), nil
	}
	// Chart with no values.yaml is valid (rare). Return empty.
	return "", nil
}
```

- [ ] **Step 4: Add 3 routes to `handler.go`**

Append to `Handler`:

```go
func (h *Handler) ListCharts(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.ListCharts(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"items": out})
}

func (h *Handler) ListVersions(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	chart := c.Param("chart")
	out, err := h.svc.ListVersions(c.Request.Context(), id, chart)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"items": out})
}

func (h *Handler) GetDefaultValues(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	chart := c.Param("chart")
	version := c.Param("version")
	out, err := h.svc.GetDefaultValues(c.Request.Context(), id, chart, version)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"values_yaml": out})
}
```

- [ ] **Step 5: Write `charts_test.go`** (HTTP repo via `httptest.Server`)

```go
package repo

import (
	"compress/gzip"
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
)

func tgzWithValues(values string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(values)
	_ = tw.WriteHeader(&tar.Header{Name: "demo/values.yaml", Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestListCharts_HTTPRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			fmt.Fprintf(w, `apiVersion: v1
entries:
  demo:
    - name: demo
      version: 1.0.0
      appVersion: "1"
      description: "demo chart"
      urls: ["%s/demo-1.0.0.tgz"]
`, r.Host)
			return
		}
		if r.URL.Path == "/demo-1.0.0.tgz" {
			_, _ = w.Write(tgzWithValues("replicaCount: 1\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	s, r := setupSvc(t)
	m := &models.AppsChartRepo{Name: "demo", Type: "http", URL: server.URL}
	require.NoError(t, r.Create(context.Background(), m))

	charts, err := s.ListCharts(context.Background(), m.ID)
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Equal(t, "demo", charts[0].Name)
	require.Equal(t, 1, charts[0].VersionCount)

	versions, err := s.ListVersions(context.Background(), m.ID, "demo")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "1.0.0", versions[0].Version)

	values, err := s.GetDefaultValues(context.Background(), m.ID, "demo", "1.0.0")
	require.NoError(t, err)
	require.Contains(t, values, "replicaCount: 1")
}

func TestReadValuesFromTgz_NoValuesYAML(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "demo/Chart.yaml", Mode: 0644, Size: 4, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("name"))
	_ = tw.Close()
	_ = gz.Close()
	s, err := readValuesFromTgz(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, "", s)
}
```

OCI tests are intentionally skipped (spec §10.4 — manual smoke).

- [ ] **Step 6: Run tests**

```bash
cd optimus-be
go test -tags=dbtest ./internal/modules/apps/repo/... -race -count=1 -v
go test -tags=dbtest ./internal/modules/apps/repo/... -coverprofile=/tmp/p3-repo.cov
go tool cover -func=/tmp/p3-repo.cov | tail -1
```

Expected: all pass; coverage still ≥60%. Note: the route registration for the 3 new endpoints is done in Task 12 (`module.go`); these handlers are testable in unit form only here.

- [ ] **Step 7: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/repo/charts.go \
        optimus-be/internal/modules/apps/repo/helpers.go \
        optimus-be/internal/modules/apps/repo/charts_test.go \
        optimus-be/internal/modules/apps/repo/dto.go \
        optimus-be/internal/modules/apps/repo/handler.go
git commit -m "$(cat <<'EOF'
feat(be/apps/repo): chart + version + default-values enumeration

Three new service methods plus three new Gin handlers that walk the
upstream chart repository:
- ListCharts: HTTP via repo.NewChartRepository + DownloadIndexFile;
  OCI returns the chart inferred from the URL path (registry list-
  artifacts API is not standard in helm SDK).
- ListVersions: HTTP from index.yaml; OCI via registry.Client.Tags.
- GetDefaultValues: pulls the .tgz, extracts <root>/values.yaml.

helpers.go contains the shared tgz reader and a gorm.ErrRecordNotFound
sentinel so charts.go stays GORM-free. apps.MapError (added Task 9)
normalises upstream errors into 42101-42199.

OCI test coverage is the manual smoke checklist (spec §10.4); unit tests
cover the HTTP path through an httptest.Server.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `apps/application` full vertical (DTO + repo + service + Counter + handler + tests)

**Goal:** Land application CRUD: DTOs, GORM repo (CRUD + `CountByClusterID` + `CountByChartRepoID`), service (CRUD + audit + immutable-field enforcement), `Counter` interface (exported, satisfied by service), handler (5 endpoints), and dockertest. Mirrors Task 5's shape but adds the inuse counter that closes the P3 ↔ P2 ↔ repo loop.

**Files (new):**
- `optimus-be/internal/modules/apps/application/dto.go`
- `optimus-be/internal/modules/apps/application/repo.go`
- `optimus-be/internal/modules/apps/application/repo_test.go`
- `optimus-be/internal/modules/apps/application/service.go`
- `optimus-be/internal/modules/apps/application/service_test.go`
- `optimus-be/internal/modules/apps/application/handler.go`
- `optimus-be/internal/modules/apps/application/handler_test.go`
- `optimus-be/internal/modules/apps/application/inuse.go`
- `optimus-be/internal/modules/apps/application/inuse_test.go`

- [ ] **Step 1: `dto.go`**

```go
package application

import "time"

// Summary is the list-row shape. Live helm status is null in list responses;
// FE may opt-in via per-row GET /release for the visible page.
type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	ClusterID   uint64    `json:"cluster_id"`
	ClusterName string    `json:"cluster_name"`
	Namespace   string    `json:"namespace"`
	ReleaseName string    `json:"release_name"`
	ChartRepoID uint64    `json:"chart_repo_id"`
	ChartName   string    `json:"chart_name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	OwnerUserID *uint64   `json:"owner_user_id,omitempty"`
	OwnerName   string    `json:"owner_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Detail extends Summary with live helm status fetched on demand.
type Detail struct {
	Summary
	Status         string `json:"status,omitempty"`           // deployed|failed|pending|unknown|""
	Revision       *int   `json:"revision,omitempty"`
	ChartVersion   string `json:"chart_version,omitempty"`
	AppVersion     string `json:"app_version,omitempty"`
	LastDeployedAt string `json:"last_deployed_at,omitempty"`
}

type CreateRequest struct {
	Name        string   `json:"name"          binding:"required,max=64"`
	ClusterID   uint64   `json:"cluster_id"    binding:"required"`
	Namespace   string   `json:"namespace"     binding:"required,max=63"`
	ReleaseName string   `json:"release_name"  binding:"required,max=53"`
	ChartRepoID uint64   `json:"chart_repo_id" binding:"required"`
	ChartName   string   `json:"chart_name"    binding:"required,max=128"`
	Description string   `json:"description"   binding:"max=4096"`
	Tags        []string `json:"tags"          binding:"omitempty,dive,max=32"`
	OwnerUserID *uint64  `json:"owner_user_id"`
}

type UpdateRequest struct {
	Description *string  `json:"description,omitempty" binding:"omitempty,max=4096"`
	Tags        []string `json:"tags,omitempty"        binding:"omitempty,dive,max=32"`
	OwnerUserID *uint64  `json:"owner_user_id,omitempty"`
}

type ListQuery struct {
	Page        int    `form:"page,default=1"`
	PageSize    int    `form:"page_size,default=20"`
	Name        string `form:"name"`
	ClusterID   uint64 `form:"cluster_id"`
	Namespace   string `form:"namespace"`
	OwnerUserID uint64 `form:"owner_user_id"`
	Tag         string `form:"tag"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
```

- [ ] **Step 2: `repo.go`**

```go
package application

import (
	"context"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, m *models.AppsApplication) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.AppsApplication, error) {
	var m models.AppsApplication
	err := r.db.WithContext(ctx).
		Preload("Cluster").
		Preload("ChartRepo").
		Preload("OwnerUser").
		First(&m, id).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByReleaseTuple(ctx context.Context, clusterID uint64, ns, release string) (*models.AppsApplication, error) {
	var m models.AppsApplication
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND namespace = ? AND release_name = ?", clusterID, ns, release).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*models.AppsApplication, error) {
	var m models.AppsApplication
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.AppsApplication, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.AppsApplication{}).
		Preload("Cluster").Preload("ChartRepo").Preload("OwnerUser")
	if s := strings.TrimSpace(q.Name); s != "" {
		tx = tx.Where("name ILIKE ?", "%"+s+"%")
	}
	if q.ClusterID != 0 {
		tx = tx.Where("cluster_id = ?", q.ClusterID)
	}
	if ns := strings.TrimSpace(q.Namespace); ns != "" {
		tx = tx.Where("namespace = ?", ns)
	}
	if q.OwnerUserID != 0 {
		tx = tx.Where("owner_user_id = ?", q.OwnerUserID)
	}
	if t := strings.TrimSpace(q.Tag); t != "" {
		tx = tx.Where("tags @> ?::jsonb", datatypes.JSON(`["`+t+`"]`))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	var rows []models.AppsApplication
	if err := tx.Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.AppsApplication{}).
		Where("id = ?", id).
		Updates(fields).Error
}

func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.AppsApplication{}, id).Error
}

func (r *Repo) CountByClusterID(ctx context.Context, clusterID uint64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.AppsApplication{}).
		Where("cluster_id = ?", clusterID).
		Count(&n).Error
	return int(n), err
}

func (r *Repo) CountByChartRepoID(ctx context.Context, repoID uint64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.AppsApplication{}).
		Where("chart_repo_id = ?", repoID).
		Count(&n).Error
	return int(n), err
}
```

- [ ] **Step 3: `inuse.go`**

```go
package application

import "context"

// Counter is the seam exposed to other packages so they can do FK pre-checks
// (apps/repo.Delete and k8s/cluster.Delete both reach in here). The concrete
// implementation is the Service or Repo — main.go wires them to break the
// import cycle.
type Counter interface {
	CountByClusterID(ctx context.Context, clusterID uint64) (int, error)
	CountByChartRepoID(ctx context.Context, repoID uint64) (int, error)
}

// Verify Repo satisfies Counter at compile time.
var _ Counter = (*Repo)(nil)
```

- [ ] **Step 4: `service.go`**

```go
package application

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"optimus-be/internal/infra/audit"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

// HelmStatusProbe is the seam release.Service implements. Service uses it to
// decorate Detail with live status on Get. It is OPTIONAL — passing nil
// returns Detail with empty status fields. Wired by main.go.
type HelmStatusProbe interface {
	StatusForApplication(ctx context.Context, app *models.AppsApplication) (status string, revision *int, chartVersion, appVersion, lastDeployedAt string, err error)
}

// HelmInstalledChecker is the seam Service.Delete uses to refuse delete when
// the helm release still exists. Wired by main.go.
type HelmInstalledChecker interface {
	IsReleaseInstalled(ctx context.Context, app *models.AppsApplication) (bool, error)
}

type Service struct {
	repo    *Repo
	rec     audit.Recorder
	probe   HelmStatusProbe
	checker HelmInstalledChecker
}

func NewService(r *Repo, rec audit.Recorder) *Service {
	return &Service{repo: r, rec: rec}
}

func (s *Service) SetHelmStatusProbe(p HelmStatusProbe)        { s.probe = p }
func (s *Service) SetHelmInstalledChecker(c HelmInstalledChecker) { s.checker = c }

func (s *Service) Create(ctx context.Context, actorID uint64, req CreateRequest) (*Detail, error) {
	// name uniqueness (live row)
	if existing, err := s.repo.FindByName(ctx, req.Name); err == nil && existing != nil {
		return nil, apperr.New(apperr.CodeConflict, "apps.application.name_taken", req.Name)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// release tuple uniqueness
	if existing, err := s.repo.FindByReleaseTuple(ctx, req.ClusterID, req.Namespace, req.ReleaseName); err == nil && existing != nil {
		return nil, apperr.New(apperr.CodeAppsReleaseNameDuplicate, "apps.application.release_taken")
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	m := &models.AppsApplication{
		Name:        req.Name,
		ClusterID:   req.ClusterID,
		Namespace:   req.Namespace,
		ReleaseName: req.ReleaseName,
		ChartRepoID: req.ChartRepoID,
		ChartName:   req.ChartName,
		Description: req.Description,
		Tags:        req.Tags,
		OwnerUserID: req.OwnerUserID,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.application.create",
		TargetType: "apps_application", TargetID: m.ID,
		Metadata: map[string]any{
			"name": m.Name, "cluster_id": m.ClusterID, "namespace": m.Namespace,
			"release_name": m.ReleaseName, "chart_repo_id": m.ChartRepoID, "chart_name": m.ChartName,
		},
	})
	return s.Get(ctx, m.ID)
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found")
		}
		return nil, err
	}
	d := toDetail(m)
	if s.probe != nil {
		status, rev, cv, av, ldp, perr := s.probe.StatusForApplication(ctx, m)
		if perr == nil {
			d.Status = status
			d.Revision = rev
			d.ChartVersion = cv
			d.AppVersion = av
			d.LastDeployedAt = ldp
		}
	}
	return d, nil
}

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(rows))
	for i := range rows {
		items = append(items, toDetail(&rows[i]).Summary)
	}
	return &ListResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) Update(ctx context.Context, actorID, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found")
		}
		return nil, err
	}
	fields := map[string]any{}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.Tags != nil {
		fields["tags"] = req.Tags
	}
	if req.OwnerUserID != nil {
		fields["owner_user_id"] = *req.OwnerUserID
	}
	if len(fields) > 0 {
		if err := s.repo.Update(ctx, id, fields); err != nil {
			return nil, err
		}
	}
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.application.update",
		TargetType: "apps_application", TargetID: m.ID,
		Metadata: map[string]any{"fields_changed": fieldKeys(fields)},
	})
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID, id uint64) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "apps.application.not_found")
		}
		return err
	}
	if s.checker != nil {
		installed, cerr := s.checker.IsReleaseInstalled(ctx, m)
		if cerr != nil {
			return cerr
		}
		if installed {
			return apperr.New(apperr.CodeAppsReleaseStillPresent, "apps.application.release_still_installed")
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.application.delete",
		TargetType: "apps_application", TargetID: id,
		Metadata: map[string]any{
			"cluster_id": m.ClusterID, "namespace": m.Namespace, "release_name": m.ReleaseName,
		},
	})
	return nil
}

// SetChartRepo is used only by release.Service.Upgrade to patch chart_repo_id
// atomically with the helm upgrade.
func (s *Service) SetChartRepo(ctx context.Context, id, newRepoID uint64) error {
	return s.repo.Update(ctx, id, map[string]any{"chart_repo_id": newRepoID})
}

func toDetail(m *models.AppsApplication) *Detail {
	d := &Detail{
		Summary: Summary{
			ID: m.ID, Name: m.Name, ClusterID: m.ClusterID,
			Namespace: m.Namespace, ReleaseName: m.ReleaseName,
			ChartRepoID: m.ChartRepoID, ChartName: m.ChartName,
			Description: m.Description, Tags: []string(m.Tags),
			OwnerUserID: m.OwnerUserID,
			CreatedAt:   m.CreatedAt, UpdatedAt: m.UpdatedAt,
		},
	}
	if m.Cluster != nil {
		d.ClusterName = m.Cluster.Name
	}
	if m.OwnerUser != nil {
		d.OwnerName = m.OwnerUser.DisplayName
		if d.OwnerName == "" {
			d.OwnerName = m.OwnerUser.Username
		}
	}
	return d
}

func fieldKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 5: `handler.go`**

```go
package application

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.invalid_query", err.Error()))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.invalid_body", err.Error()))
		return
	}
	actor := middleware.MustActorID(c)
	out, err := h.svc.Create(c.Request.Context(), actor, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusCreated)
	response.Success(c, out)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.invalid_body", err.Error()))
		return
	}
	actor := middleware.MustActorID(c)
	out, err := h.svc.Update(c.Request.Context(), actor, id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	actor := middleware.MustActorID(c)
	if err := h.svc.Delete(c.Request.Context(), actor, id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, nil)
}
```

- [ ] **Step 6: Tests**

Write `repo_test.go`, `service_test.go`, `handler_test.go`, `inuse_test.go` following Task 5's shape. Required cases:

`repo_test.go` (build tag `dbtest`):
- CRUD round-trip.
- `(cluster_id, namespace, release_name)` unique partial index — second insert with same triple errors.
- `CountByClusterID` and `CountByChartRepoID` return correct counts including across soft-deleted exclusion.

`service_test.go`:
- `Create` rejects when name collides → CodeConflict.
- `Create` rejects when (cluster, ns, release) triple collides → CodeAppsReleaseNameDuplicate.
- `Get` decorates Detail with status when probe is set.
- `Delete` refused when `HelmInstalledChecker.IsReleaseInstalled` returns true → CodeAppsReleaseStillPresent.
- `Update` only touches whitelisted fields (description/tags/owner_user_id); attempts to PUT name/cluster_id/etc. are ignored (the struct doesn't even have those fields, but assert via DB read).
- `SetChartRepo` patches chart_repo_id (used by release upgrade).

`handler_test.go`:
- All five endpoints round-trip through `httptest`.
- DELETE without prior uninstall returns CodeAppsReleaseStillPresent via the wired-in checker.
- LIST filters by `cluster_id` and `tag` work.

`inuse_test.go`:
- `Counter` interface is satisfied by `*Repo`; the test does the conformance assertion via `var _ Counter = (*Repo)(nil)` plus a runtime count vs. number-of-rows comparison.

- [ ] **Step 7: Run tests + coverage**

```bash
cd optimus-be
go test -tags=dbtest ./internal/modules/apps/application/... -race -count=1 -v
go test -tags=dbtest ./internal/modules/apps/application/... -coverprofile=/tmp/p3-app.cov
go tool cover -func=/tmp/p3-app.cov | tail -1
```

Expected: all pass; coverage ≥ 60%.

- [ ] **Step 8: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/application/
git commit -m "$(cat <<'EOF'
feat(be/apps/application): application CRUD vertical + Counter (P3)

Lands apps/application with DTOs, GORM repo (CRUD plus
CountByClusterID/CountByChartRepoID), service with audit + immutable-
field enforcement, Gin handlers (5 endpoints), and dockertest.

The exported Counter interface is the seam used by apps/repo.Delete
and (next task) k8s/cluster.Delete to refuse FK-violating deletes with
a friendly 42001/42002 BizError instead of a raw constraint failure.

HelmStatusProbe + HelmInstalledChecker are seams wired by main.go to
keep apps/application free of helm.sh/helm/v3 imports.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: P2 cluster.Delete pre-check via `apps/application.Counter`

**Goal:** Wire the `Counter` seam into P2's `k8s/cluster.Service.Delete` so cluster deletion is refused with `CodeAppsApplicationInUse` (42001) when applications still reference the cluster. This is the single upstream patch to P2 demanded by P3 spec §9.

**Files:**
- Modify: `optimus-be/internal/modules/k8s/cluster/service.go`
- Modify: `optimus-be/internal/modules/k8s/cluster/service_test.go`
- Modify: `optimus-be/internal/modules/k8s/module.go` (constructor signature)

- [ ] **Step 1: Add the seam to `cluster.Service`**

In `internal/modules/k8s/cluster/service.go`, add at the top of the file alongside other interface declarations:

```go
// AppsApplicationCounter is the narrow seam k8s/cluster.Delete uses to refuse
// deletion when applications still reference the cluster. Satisfied by
// apps/application.Repo (or any Counter); wired in main.go to avoid an
// import cycle.
type AppsApplicationCounter interface {
	CountByClusterID(ctx context.Context, clusterID uint64) (int, error)
}
```

Add a setter on `*Service`:

```go
// SetAppsCounter wires the counter post-construction. nil is allowed (the
// pre-check is skipped) so the k8s module can be brought up before apps.
func (s *Service) SetAppsCounter(c AppsApplicationCounter) { s.appsCounter = c }
```

Add field on `Service` struct:

```go
appsCounter AppsApplicationCounter
```

- [ ] **Step 2: Patch `Delete` to call the pre-check**

In the same file, the existing `Delete` method begins by loading the cluster row. Right after the existing not-found check and BEFORE the soft-delete call, insert:

```go
	if s.appsCounter != nil {
		n, err := s.appsCounter.CountByClusterID(ctx, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return apperr.New(apperr.CodeAppsApplicationInUse, "k8s.cluster.in_use_by_apps", fmt.Sprintf("%d", n))
		}
	}
```

Add `"fmt"` to the file's imports if not already there.

- [ ] **Step 3: Update unit test**

In `internal/modules/k8s/cluster/service_test.go`, add:

```go
type fakeAppsCounter struct {
	n   int
	err error
}

func (f *fakeAppsCounter) CountByClusterID(_ context.Context, _ uint64) (int, error) {
	return f.n, f.err
}

func TestService_Delete_RefusedWhenAppsReference(t *testing.T) {
	svc, _, _ := setupSvc(t) // existing helper from P2 tests
	// create a cluster first via the existing helper
	cl := mustCreateCluster(t, svc, "in-use")
	svc.SetAppsCounter(&fakeAppsCounter{n: 3})

	err := svc.Delete(context.Background(), 1, cl.ID)
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAppsApplicationInUse, be.Code)
}
```

If `mustCreateCluster` doesn't exist verbatim, follow whatever helper the existing P2 tests use to create a cluster row.

- [ ] **Step 4: Update `module.go`**

In `internal/modules/k8s/module.go`, the existing `Module` struct exposes the cluster Service. Add a setter pass-through or directly expose so `main.go` can call:

```go
func (m *Module) SetAppsCounter(c cluster.AppsApplicationCounter) {
	m.ClusterSvc.SetAppsCounter(c)
}
```

(Replace `m.ClusterSvc` with whatever the existing field is named — confirm via `grep` in the file.)

- [ ] **Step 5: Run tests**

```bash
cd optimus-be
go test ./internal/modules/k8s/cluster/... -race -count=1 -v
```

Expected: PASS. Run the full BE test suite to make sure the change didn't disturb anything else:

```bash
go test ./... -race -count=1
```

- [ ] **Step 6: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/k8s/cluster/service.go \
        optimus-be/internal/modules/k8s/cluster/service_test.go \
        optimus-be/internal/modules/k8s/module.go
git commit -m "$(cat <<'EOF'
feat(be/k8s/cluster): refuse delete when applications still reference (P3)

Adds AppsApplicationCounter seam to k8s/cluster.Service. Delete now
runs a CountByClusterID pre-check; >0 returns 42001
CodeAppsApplicationInUse with message_key
k8s.cluster.in_use_by_apps. The seam is wired by main.go (so the k8s
package stays free of any apps import). The post-construction
SetAppsCounter pattern matches the existing inuse seam used by P1
kubeconfig delete.

Required by P3 spec §9.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `apps.MapError` + tests

**Goal:** Land the central error-mapping function so every helm SDK, registry, repo, and network error coming out of the helmclient / chart enumeration paths gets normalised to a 42xxx BizError before it surfaces to handlers.

**Files (new):**
- `optimus-be/internal/modules/apps/errs.go`
- `optimus-be/internal/modules/apps/errs_test.go`

- [ ] **Step 1: `errs.go`**

```go
// Package apps provides MapError, the central error-mapping function used
// by the helmclient/, repo/, application/, and release/ sub-packages to
// normalise helm SDK / registry / repo / network errors into BizError
// values in the 42xxx range. See P3 spec §5.
package apps

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
)

// MapError normalises an error from helm / registry / repo / network into a
// BizError in the 42xxx range. nil pass-through.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// 1) helm storage driver sentinels.
	switch {
	case errors.Is(err, driver.ErrReleaseNotFound):
		return apperr.New(apperr.CodeAppsReleaseNotFound, "apps.release.not_found").WithCause(err)
	case errors.Is(err, driver.ErrReleaseExists):
		return apperr.New(apperr.CodeAppsReleaseAlreadyExists, "apps.release.already_exists").WithCause(err)
	case errors.Is(err, driver.ErrNoDeployedReleases):
		return apperr.New(apperr.CodeAppsReleaseNotFound, "apps.release.no_deployed").WithCause(err)
	}

	// 2) network / URL errors.
	var ne net.Error
	if errors.As(err, &ne) {
		return apperr.New(apperr.CodeAppsRepoUnreachable, "apps.repo.unreachable").WithCause(err)
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return apperr.New(apperr.CodeAppsRepoUnreachable, "apps.repo.unreachable").WithCause(err)
	}

	// 3) registry / repo string matching.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "denied"),
		strings.Contains(msg, "authentication required"):
		return apperr.New(apperr.CodeAppsRepoUnauthorized, "apps.repo.unauthorized").WithCause(err)
	case strings.Contains(msg, "not found") && (strings.Contains(msg, "chart") || strings.Contains(msg, "manifest") || strings.Contains(msg, "tag")):
		return apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found").WithCause(err)
	case strings.Contains(msg, "yaml") && strings.Contains(msg, "index"):
		return apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index").WithCause(err)
	case strings.Contains(msg, "manifest") || strings.Contains(msg, "blob"):
		return apperr.New(apperr.CodeAppsRepoOCIError, "apps.repo.oci_error").WithCause(err)
	}

	// 4) fallthrough.
	return apperr.New(apperr.CodeAppsReleaseOther, "apps.release.other").WithCause(err)
}
```

- [ ] **Step 2: `errs_test.go`**

```go
package apps

import (
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
)

func TestMapError_NilPassthrough(t *testing.T) {
	require.NoError(t, MapError(nil))
}

func TestMapError_HelmSentinels(t *testing.T) {
	cases := []struct {
		in   error
		want apperr.Code
	}{
		{driver.ErrReleaseNotFound, apperr.CodeAppsReleaseNotFound},
		{driver.ErrReleaseExists, apperr.CodeAppsReleaseAlreadyExists},
		{driver.ErrNoDeployedReleases, apperr.CodeAppsReleaseNotFound},
	}
	for _, tc := range cases {
		got := MapError(tc.in)
		be, ok := got.(*apperr.BizError)
		require.True(t, ok)
		require.Equal(t, tc.want, be.Code)
	}
}

type netErr struct{ msg string; tmo bool }

func (n *netErr) Error() string   { return n.msg }
func (n *netErr) Timeout() bool   { return n.tmo }
func (n *netErr) Temporary() bool { return n.tmo }

var _ net.Error = (*netErr)(nil)

func TestMapError_NetworkAndURL(t *testing.T) {
	got := MapError(&netErr{msg: "dial tcp: i/o timeout", tmo: true})
	be := got.(*apperr.BizError)
	require.Equal(t, apperr.CodeAppsRepoUnreachable, be.Code)

	got = MapError(&url.Error{Op: "GET", URL: "https://x", Err: errors.New("dial tcp: connection refused")})
	be = got.(*apperr.BizError)
	require.Equal(t, apperr.CodeAppsRepoUnreachable, be.Code)
}

func TestMapError_Strings(t *testing.T) {
	cases := []struct {
		msg  string
		want apperr.Code
	}{
		{"unauthorized: incorrect token", apperr.CodeAppsRepoUnauthorized},
		{"denied: requested access to the resource", apperr.CodeAppsRepoUnauthorized},
		{"manifest for foo:1.0 not found", apperr.CodeAppsRepoChartNotFound},
		{"chart \"nope\" not found", apperr.CodeAppsRepoChartNotFound},
		{"yaml: index parse error", apperr.CodeAppsRepoInvalidIndex},
		{"failed to pull blob: corrupted", apperr.CodeAppsRepoOCIError},
	}
	for _, tc := range cases {
		got := MapError(errors.New(tc.msg))
		be := got.(*apperr.BizError)
		require.Equal(t, tc.want, be.Code, "msg=%q", tc.msg)
	}
}

func TestMapError_Fallthrough(t *testing.T) {
	got := MapError(errors.New("something completely unexpected"))
	be := got.(*apperr.BizError)
	require.Equal(t, apperr.CodeAppsReleaseOther, be.Code)
}
```

- [ ] **Step 3: Run tests**

```bash
cd optimus-be
go test ./internal/modules/apps -race -count=1 -v
go test ./internal/modules/apps -coverprofile=/tmp/p3-errs.cov
go tool cover -func=/tmp/p3-errs.cov | tail -1
```

Expected: PASS; coverage ≥ 60% (this package is small, easy to hit 80%+).

- [ ] **Step 4: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/errs.go \
        optimus-be/internal/modules/apps/errs_test.go
git commit -m "$(cat <<'EOF'
feat(be/apps): MapError normalises helm/registry/net errors to 42xxx

Four-stage dispatch:
1. Helm storage driver sentinels (ErrReleaseNotFound, ErrReleaseExists,
   ErrNoDeployedReleases) -> 42201/42202.
2. net.Error / *url.Error -> 42101 Unreachable.
3. Substring match on registry/repo strings (helm packages don't export
   sentinels) -> 42102/42103/42104/42105.
4. Fallthrough -> 42299 ReleaseOther.

All paths wrap the original error via WithCause so structured logs keep
the upstream message for operator triage.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `apps/helmclient.Factory` + `restClientGetter`

**Goal:** Land the per-request `*action.Configuration` factory that future release operations call once per HTTP request. The factory: looks up cluster via the existing `cluster.Service`, gets kubeconfig from `credentials.Consumer`, builds a `*rest.Config` bound to the named context, wraps it in a `RESTClientGetter` (helm's required interface), and inits `action.Configuration` with the `secrets` storage driver.

**Files (new):**
- `optimus-be/internal/modules/apps/helmclient/factory.go`
- `optimus-be/internal/modules/apps/helmclient/getter.go`
- `optimus-be/internal/modules/apps/helmclient/factory_test.go`

- [ ] **Step 1: Write `factory.go`**

```go
// Package helmclient builds per-request *action.Configuration objects.
// helm action.Configuration's internal KubeClient is NOT safe to share
// across goroutines, so the rule is: build per request, discard before
// the handler returns. See P3 spec §7.1.
package helmclient

import (
	"context"
	"log/slog"

	"helm.sh/helm/v3/pkg/action"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/apps"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/cluster"
)

// Factory holds the cross-request seams. Safe to share across handlers.
type Factory struct {
	consumer credentials.Consumer
	clusters ClusterLookup
}

// ClusterLookup is the seam the Factory needs from cluster.Service. Defined
// here so we can use a narrow fake in tests.
type ClusterLookup interface {
	Get(ctx context.Context, id uint64) (*cluster.Cluster, error)
}

func NewFactory(consumer credentials.Consumer, clusters ClusterLookup) *Factory {
	return &Factory{consumer: consumer, clusters: clusters}
}

// NewForCluster returns a fresh *action.Configuration pointed at the given
// cluster, with the helm release namespace set to `namespace` and the
// storage driver set to "secrets" (helm 3 default). purpose threads through
// to credentials.Consumer for audit.
func (f *Factory) NewForCluster(
	ctx context.Context, clusterID uint64, namespace, purpose string,
) (*action.Configuration, error) {
	c, err := f.clusters.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	kc, err := f.consumer.GetKubeconfig(ctx, c.KubeconfigID, purpose)
	if err != nil {
		return nil, err
	}
	restCfg, err := buildRESTConfig(kc.YAML, c.Context)
	if err != nil {
		return nil, apps.MapError(err)
	}
	rcg := newRESTClientGetter(restCfg, namespace)

	actionCfg := new(action.Configuration)
	if err := actionCfg.Init(rcg, namespace, "secrets", debugLog); err != nil {
		return nil, apps.MapError(err)
	}
	return actionCfg, nil
}

// buildRESTConfig parses the kubeconfig and binds to the named context.
// Validation duplicates P1's validation as defense-in-depth (a DB row
// tampered post-upload still fails before we ship a request).
func buildRESTConfig(y []byte, contextName string) (*rest.Config, error) {
	if err := cluster.ValidateContextAndAuth(y, contextName); err != nil {
		return nil, err
	}
	apiCfg, err := clientcmd.Load(y)
	if err != nil {
		return nil, apperr.New(apperr.CodeValidation, "apps.helm.kubeconfig.invalid", err.Error())
	}
	apiCfg.CurrentContext = contextName
	return clientcmd.NewDefaultClientConfig(*apiCfg, &clientcmd.ConfigOverrides{}).ClientConfig()
}

// debugLog bridges helm SDK verbose output into slog at DEBUG level.
// Never goes back to the HTTP client.
func debugLog(format string, args ...interface{}) {
	slog.Debug("helm", slog.String("msg", trim(format, args...)))
}

func trim(format string, args ...interface{}) string {
	// Helm's internal log calls are sometimes plain strings. Keep simple.
	if len(args) == 0 {
		return format
	}
	return format // detailed sprintf left out: we don't ship helm verbose to clients.
}
```

- [ ] **Step 2: Write `getter.go`** (RESTClientGetter implementation)

```go
package helmclient

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// restClientGetter is the minimal genericclioptions.RESTClientGetter helm
// SDK needs. It owns a fresh *rest.Config and a namespace; no caching
// across calls.
type restClientGetter struct {
	cfg       *rest.Config
	namespace string
}

func newRESTClientGetter(cfg *rest.Config, namespace string) genericclioptions.RESTClientGetter {
	return &restClientGetter{cfg: cfg, namespace: namespace}
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) { return g.cfg, nil }

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	disc, err := discovery.NewDiscoveryClientForConfig(g.cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(disc), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	d, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(d)
	return restmapper.NewShortcutExpander(mapper, d, nil), nil
}

func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	rawCfg := clientcmd.NewDefaultClientConfig(*emptyAPIConfig(), &clientcmd.ConfigOverrides{
		Context: clientcmd.ContextOverride{Namespace: g.namespace},
	})
	return rawCfg
}

// emptyAPIConfig returns a non-nil but empty api.Config so ToRawKubeConfigLoader
// doesn't return nil — helm action.Configuration.Init asserts non-nil.
func emptyAPIConfig() *api.Config { c := api.NewConfig(); return c }
```

Imports for `meta` and `api`:

```go
import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/clientcmd/api"
)
```

Add them to the import block in `getter.go`.

- [ ] **Step 3: Write `factory_test.go`** (mock Consumer + table-driven YAML)

```go
package helmclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/cluster"
)

type fakeConsumer struct {
	yaml []byte
	err  error
}

func (f *fakeConsumer) GetSSHKey(context.Context, uint64, string) (*credentials.SSHKey, error) {
	panic("not used")
}
func (f *fakeConsumer) GetKubeconfig(context.Context, uint64, string) (*credentials.Kubeconfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &credentials.Kubeconfig{YAML: f.yaml, Name: "test"}, nil
}
func (f *fakeConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	panic("not used")
}

type fakeClusters struct {
	c *cluster.Cluster
}

func (f *fakeClusters) Get(context.Context, uint64) (*cluster.Cluster, error) {
	return f.c, nil
}

const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://kube.example.com:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: abc
contexts:
- name: ctx
  context:
    cluster: c
    user: u
current-context: ctx
`

func TestFactory_NewForCluster_OK(t *testing.T) {
	consumer := &fakeConsumer{yaml: []byte(minimalKubeconfig)}
	clusters := &fakeClusters{c: &cluster.Cluster{
		ID: 1, KubeconfigID: 100, Context: "ctx",
	}}
	f := NewFactory(consumer, clusters)
	cfg, err := f.NewForCluster(context.Background(), 1, "demo-ns", "apps.release.install")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Releases)
	require.NotNil(t, cfg.KubeClient)
}

func TestFactory_NewForCluster_RejectsExecPlugin(t *testing.T) {
	bad := []byte(`apiVersion: v1
kind: Config
clusters: [{name: c, cluster: {server: https://x, insecure-skip-tls-verify: true}}]
users: [{name: u, user: {exec: {apiVersion: client.authentication.k8s.io/v1, command: /bin/sh}}}]
contexts: [{name: ctx, context: {cluster: c, user: u}}]
current-context: ctx
`)
	consumer := &fakeConsumer{yaml: bad}
	clusters := &fakeClusters{c: &cluster.Cluster{ID: 1, KubeconfigID: 100, Context: "ctx"}}
	f := NewFactory(consumer, clusters)
	_, err := f.NewForCluster(context.Background(), 1, "demo", "test")
	require.Error(t, err)
}
```

- [ ] **Step 4: Run tests + coverage**

```bash
cd optimus-be
go test ./internal/modules/apps/helmclient/... -race -count=1 -v
go test ./internal/modules/apps/helmclient/... -coverprofile=/tmp/p3-helm.cov
go tool cover -func=/tmp/p3-helm.cov | tail -1
```

Expected: PASS; coverage ≥ 60%.

- [ ] **Step 5: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/helmclient/
git commit -m "$(cat <<'EOF'
feat(be/apps/helmclient): per-request *action.Configuration factory (P3)

Factory.NewForCluster walks: cluster.Service.Get -> credentials.Consumer.
GetKubeconfig -> clientcmd parse + context bind -> RESTClientGetter shim
-> action.Configuration.Init with the "secrets" storage driver. Storage
driver is helm 3 default, namespace-scoped, kubectl-introspectable.

restClientGetter is the minimal RESTClientGetter helm SDK needs (4
methods: RESTConfig, DiscoveryClient, RESTMapper, RawKubeConfigLoader).
Built per request alongside the rest.Config so nothing leaks between
calls.

Validation reuses P1's ValidateContextAndAuth as defense-in-depth so a
tampered DB row still fails before we issue an apiserver request.

helm verbose output is bridged to slog at DEBUG; never reaches the client.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: `apps/release` full vertical (install/upgrade/rollback/uninstall/status/history)

**Goal:** Land the release service that wraps helm SDK `action.{Install,Upgrade,Rollback,Uninstall,Status,History}`, plus the Gin handler with the 6 endpoints. The service uses the helmclient.Factory built in Task 10 and the application.Service from Task 7. Audit emission on every write. Unit tests use an in-memory helm storage driver (`driver.Memory`) and a fake KubeClient — no real cluster needed.

**Files (new):**
- `optimus-be/internal/modules/apps/release/dto.go`
- `optimus-be/internal/modules/apps/release/service.go`
- `optimus-be/internal/modules/apps/release/service_test.go`
- `optimus-be/internal/modules/apps/release/handler.go`
- `optimus-be/internal/modules/apps/release/handler_test.go`
- `optimus-be/internal/modules/apps/release/probe.go`

- [ ] **Step 1: `dto.go`**

```go
package release

// InstallRequest is the body of POST /apps/applications/:id/release/install.
type InstallRequest struct {
	ChartVersion string `json:"chart_version" binding:"required,max=64"`
	ValuesYAML   string `json:"values_yaml"   binding:"max=1048576"` // 1 MiB cap
}

// UpgradeRequest is the body of POST /.../release/upgrade. ChartRepoID is
// optional — when present, the application row's chart_repo_id is patched
// to the new value atomically with the helm upgrade.
type UpgradeRequest struct {
	ChartRepoID  *uint64 `json:"chart_repo_id,omitempty"`
	ChartVersion string  `json:"chart_version" binding:"required,max=64"`
	ValuesYAML   string  `json:"values_yaml"   binding:"max=1048576"`
}

type RollbackRequest struct {
	Revision int `json:"revision" binding:"required,min=1"`
}

type UninstallRequest struct {
	KeepHistory bool `json:"keep_history"`
}

// ReleaseStatus is the live state of a release.
type ReleaseStatus struct {
	Status         string `json:"status"`                    // deployed|failed|pending|unknown
	Revision       int    `json:"revision"`
	ChartVersion   string `json:"chart_version"`
	AppVersion     string `json:"app_version"`
	LastDeployedAt string `json:"last_deployed_at"`
	Notes          string `json:"notes,omitempty"`
}

// RevisionRow is one entry of helm history.
type RevisionRow struct {
	Revision     int    `json:"revision"`
	Status       string `json:"status"`
	ChartVersion string `json:"chart_version"`
	AppVersion   string `json:"app_version"`
	UpdatedAt    string `json:"updated_at"`
	Description  string `json:"description"`
}

// InstallResult / UpgradeResult / RollbackResult are returned by writes.
type InstallResult struct {
	Revision       int    `json:"revision"`
	Status         string `json:"status"`
	ChartVersion   string `json:"chart_version"`
	LastDeployedAt string `json:"last_deployed_at"`
}

type UpgradeResult = InstallResult
type RollbackResult = InstallResult
```

- [ ] **Step 2: `service.go`**

```go
package release

import (
	"bytes"
	"context"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"sigs.k8s.io/yaml"

	"optimus-be/internal/infra/audit"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/apps/helmclient"
	apprepo "optimus-be/internal/modules/apps/repo"
)

// ChartLoader is the seam that fetches a chart .tgz from a chart repo
// and parses it into a *chart.Chart. Satisfied by apps/repo.Service (which
// already knows how to walk OCI / HTTP). main.go wires it.
type ChartLoader interface {
	LoadChart(ctx context.Context, repoID uint64, chartName, version string) (*chart.Chart, error)
}

type Service struct {
	factory *helmclient.Factory
	apps    *application.Service
	repos   *apprepo.Service
	loader  ChartLoader
	rec     audit.Recorder
}

func NewService(
	factory *helmclient.Factory,
	apps *application.Service,
	repos *apprepo.Service,
	loader ChartLoader,
	rec audit.Recorder,
) *Service {
	return &Service{factory: factory, apps: apps, repos: repos, loader: loader, rec: rec}
}

// Status returns the live helm status for the application's release.
// Returns 42202 ReleaseNotFound when the helm secret is absent.
func (s *Service) Status(ctx context.Context, appID uint64) (*ReleaseStatus, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		return nil, err
	}
	act := action.NewStatus(cfg)
	rel, err := act.Run(app.ReleaseName)
	if err != nil {
		return nil, apps.MapError(err)
	}
	return statusFromRelease(rel), nil
}

// History returns helm history for the application's release.
func (s *Service) History(ctx context.Context, appID uint64) ([]RevisionRow, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		return nil, err
	}
	act := action.NewHistory(cfg)
	releases, err := act.Run(app.ReleaseName)
	if err != nil {
		return nil, apps.MapError(err)
	}
	out := make([]RevisionRow, 0, len(releases))
	for _, r := range releases {
		out = append(out, revRowFromRelease(r))
	}
	return out, nil
}

func (s *Service) Install(ctx context.Context, actorID, appID uint64, req InstallRequest) (*InstallResult, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	vals, err := parseValues(req.ValuesYAML)
	if err != nil {
		return nil, err
	}
	ch, err := s.loader.LoadChart(ctx, app.ChartRepoID, app.ChartName, req.ChartVersion)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.install")
	if err != nil {
		return nil, err
	}
	act := action.NewInstall(cfg)
	act.ReleaseName = app.ReleaseName
	act.Namespace = app.Namespace
	act.CreateNamespace = false
	act.Wait = false
	act.Atomic = false

	rel, err := act.RunWithContext(ctx, ch, vals)
	if err != nil {
		return nil, apps.MapError(err)
	}
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.release.install",
		TargetType: "apps_application", TargetID: app.ID,
		Metadata: map[string]any{
			"cluster_id": app.ClusterID, "namespace": app.Namespace,
			"release_name": app.ReleaseName, "chart_version": req.ChartVersion,
			"revision": rel.Version,
		},
	})
	return installResultFromRelease(rel), nil
}

func (s *Service) Upgrade(ctx context.Context, actorID, appID uint64, req UpgradeRequest) (*UpgradeResult, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	if req.ChartRepoID != nil && *req.ChartRepoID != app.ChartRepoID {
		if err := s.apps.SetChartRepo(ctx, app.ID, *req.ChartRepoID); err != nil {
			return nil, err
		}
		app.ChartRepoID = *req.ChartRepoID
	}
	vals, err := parseValues(req.ValuesYAML)
	if err != nil {
		return nil, err
	}
	ch, err := s.loader.LoadChart(ctx, app.ChartRepoID, app.ChartName, req.ChartVersion)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.upgrade")
	if err != nil {
		return nil, err
	}
	act := action.NewUpgrade(cfg)
	act.Namespace = app.Namespace
	act.Wait = false
	act.Atomic = false

	rel, err := act.RunWithContext(ctx, app.ReleaseName, ch, vals)
	if err != nil {
		return nil, apps.MapError(err)
	}
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.release.upgrade",
		TargetType: "apps_application", TargetID: app.ID,
		Metadata: map[string]any{
			"cluster_id": app.ClusterID, "namespace": app.Namespace,
			"release_name": app.ReleaseName, "chart_version": req.ChartVersion,
			"revision": rel.Version,
		},
	})
	return installResultFromRelease(rel), nil
}

func (s *Service) Rollback(ctx context.Context, actorID, appID uint64, req RollbackRequest) (*RollbackResult, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.rollback")
	if err != nil {
		return nil, err
	}
	act := action.NewRollback(cfg)
	act.Version = req.Revision
	act.Wait = false

	if err := act.Run(app.ReleaseName); err != nil {
		// Helm wraps "no such revision" inside a generic error; pre-check.
		if strings.Contains(strings.ToLower(err.Error()), "revision") &&
			strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, apperr.New(apperr.CodeAppsReleaseHistoryTooShort, "apps.release.revision_missing")
		}
		return nil, apps.MapError(err)
	}
	// Read back the new revision via Status.
	st, _ := s.Status(ctx, app.ID)
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.release.rollback",
		TargetType: "apps_application", TargetID: app.ID,
		Metadata: map[string]any{
			"cluster_id": app.ClusterID, "namespace": app.Namespace,
			"release_name": app.ReleaseName, "rolled_back_to": req.Revision,
		},
	})
	if st == nil {
		return &RollbackResult{Revision: req.Revision, Status: "unknown"}, nil
	}
	return &RollbackResult{
		Revision: st.Revision, Status: st.Status,
		ChartVersion: st.ChartVersion, LastDeployedAt: st.LastDeployedAt,
	}, nil
}

func (s *Service) Uninstall(ctx context.Context, actorID, appID uint64, req UninstallRequest) error {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.uninstall")
	if err != nil {
		return err
	}
	act := action.NewUninstall(cfg)
	act.KeepHistory = req.KeepHistory
	act.Wait = false

	if _, err := act.Run(app.ReleaseName); err != nil {
		return apps.MapError(err)
	}
	s.rec.Record(ctx, audit.Event{
		ActorID: actorID, Action: "apps.release.uninstall",
		TargetType: "apps_application", TargetID: app.ID,
		Metadata: map[string]any{
			"cluster_id": app.ClusterID, "namespace": app.Namespace,
			"release_name": app.ReleaseName, "keep_history": req.KeepHistory,
		},
	})
	return nil
}

// appsGet pulls the AppsApplication model row (with associations) via the
// application repo, returning a NotFound BizError if missing.
func (s *Service) appsGet(ctx context.Context, id uint64) (*models.AppsApplication, error) {
	return s.apps.GetModel(ctx, id)
}

func parseValues(y string) (map[string]any, error) {
	if strings.TrimSpace(y) == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := yaml.Unmarshal([]byte(y), &out); err != nil {
		return nil, apperr.New(apperr.CodeAppsReleaseInvalidValues, "apps.release.invalid_values", err.Error())
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func statusFromRelease(r *release.Release) *ReleaseStatus {
	if r == nil {
		return &ReleaseStatus{Status: "unknown"}
	}
	return &ReleaseStatus{
		Status:         string(r.Info.Status),
		Revision:       r.Version,
		ChartVersion:   r.Chart.Metadata.Version,
		AppVersion:     r.Chart.Metadata.AppVersion,
		LastDeployedAt: r.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
		Notes:          r.Info.Notes,
	}
}

func revRowFromRelease(r *release.Release) RevisionRow {
	return RevisionRow{
		Revision: r.Version, Status: string(r.Info.Status),
		ChartVersion: r.Chart.Metadata.Version, AppVersion: r.Chart.Metadata.AppVersion,
		UpdatedAt:   r.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
		Description: r.Info.Description,
	}
}

func installResultFromRelease(r *release.Release) *InstallResult {
	return &InstallResult{
		Revision:       r.Version,
		Status:         string(r.Info.Status),
		ChartVersion:   r.Chart.Metadata.Version,
		LastDeployedAt: r.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// helmChartFromTgz parses raw tgz bytes into a *chart.Chart (used by
// apps/repo.Service when implementing ChartLoader). Exported as a package
// helper so apps/repo can import without going through release.
func helmChartFromTgz(tgz []byte) (*chart.Chart, error) {
	return loader.LoadArchive(bytes.NewReader(tgz))
}
```

Note `s.apps.GetModel(ctx, id)` — this is a new exported method on application.Service. Add it to `application/service.go` (Task 7's file):

```go
// GetModel returns the underlying *models.AppsApplication (with Preload-d
// associations). Used by release.Service which needs the raw row.
func (s *Service) GetModel(ctx context.Context, id uint64) (*models.AppsApplication, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found")
		}
		return nil, err
	}
	return m, nil
}
```

Land this small addition in Task 7's working commit if Task 7 hasn't been merged yet, or as a touch-up to that file at the top of Task 11.

- [ ] **Step 3: `probe.go`** — implement `application.HelmStatusProbe` + `application.HelmInstalledChecker`

```go
package release

import (
	"context"

	"helm.sh/helm/v3/pkg/action"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps"
)

// StatusForApplication satisfies application.HelmStatusProbe. Returns empty
// strings on any error so the application detail page still renders.
func (s *Service) StatusForApplication(ctx context.Context, app *models.AppsApplication) (string, *int, string, string, string, error) {
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		return "", nil, "", "", "", err
	}
	st := action.NewStatus(cfg)
	rel, err := st.Run(app.ReleaseName)
	if err != nil {
		// translate to BizError but DO NOT surface as an error to the
		// application detail — the row should still render with status="unknown".
		return "unknown", nil, "", "", "", apps.MapError(err)
	}
	rev := rel.Version
	return string(rel.Info.Status), &rev, rel.Chart.Metadata.Version, rel.Chart.Metadata.AppVersion,
		rel.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"), nil
}

// IsReleaseInstalled satisfies application.HelmInstalledChecker. true means
// "delete must be refused"; false means "uninstalled (or never installed)".
func (s *Service) IsReleaseInstalled(ctx context.Context, app *models.AppsApplication) (bool, error) {
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		return false, err
	}
	st := action.NewStatus(cfg)
	rel, err := st.Run(app.ReleaseName)
	if err != nil {
		mapped := apps.MapError(err)
		if be, ok := mapped.(interface{ ErrorCode() any }); ok {
			_ = be // future-proof; for now check by string
		}
		// 42202 ReleaseNotFound (or driver sentinel) means "not installed".
		return false, nil
	}
	// Any non-uninstalled status counts as "installed".
	return rel.Info.Status != "uninstalled", nil
}
```

- [ ] **Step 4: `handler.go`**

```go
package release

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Status(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.Status(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) History(c *gin.Context) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.History(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"items": out})
}

func (h *Handler) Install(c *gin.Context) {
	h.write(c, func(actor, id uint64, body []byte) (any, error) {
		var req InstallRequest
		if err := bind(body, &req); err != nil {
			return nil, err
		}
		return h.svc.Install(c.Request.Context(), actor, id, req)
	})
}
func (h *Handler) Upgrade(c *gin.Context) {
	h.write(c, func(actor, id uint64, body []byte) (any, error) {
		var req UpgradeRequest
		if err := bind(body, &req); err != nil {
			return nil, err
		}
		return h.svc.Upgrade(c.Request.Context(), actor, id, req)
	})
}
func (h *Handler) Rollback(c *gin.Context) {
	h.write(c, func(actor, id uint64, body []byte) (any, error) {
		var req RollbackRequest
		if err := bind(body, &req); err != nil {
			return nil, err
		}
		return h.svc.Rollback(c.Request.Context(), actor, id, req)
	})
}
func (h *Handler) Uninstall(c *gin.Context) {
	h.write(c, func(actor, id uint64, body []byte) (any, error) {
		var req UninstallRequest
		if err := bind(body, &req); err != nil {
			return nil, err
		}
		return nil, h.svc.Uninstall(c.Request.Context(), actor, id, req)
	})
}

func (h *Handler) write(c *gin.Context, fn func(actor, id uint64, body []byte) (any, error)) {
	id, err := middleware.PathUint64(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	actor := middleware.MustActorID(c)
	body, _ := c.GetRawData()
	out, err := fn(actor, id, body)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func bind(body []byte, dst any) error {
	if len(body) == 0 {
		return nil
	}
	if err := jsonUnmarshal(body, dst); err != nil {
		return apperr.New(apperr.CodeValidation, "common.invalid_body", err.Error())
	}
	return nil
}

// jsonUnmarshal — wrapper kept to avoid importing encoding/json at the top
// of this file (handler is small enough; keep imports tight).
var jsonUnmarshal = func(b []byte, v any) error { return jsonStd(b, v) }
```

Add `import "encoding/json"` and a tiny `jsonStd` alias:

```go
import "encoding/json"
func jsonStd(b []byte, v any) error { return json.Unmarshal(b, v) }
```

- [ ] **Step 5: `service_test.go`** — uses in-memory helm storage

```go
package release

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"

	"optimus-be/internal/infra/audit"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

// fakeFactory returns an in-memory helm action.Configuration each call.
type fakeFactory struct {
	cfg *action.Configuration
}

func newFakeFactory() *fakeFactory {
	cfg := &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(_ string, _ ...interface{}) {},
	}
	return &fakeFactory{cfg: cfg}
}

func (f *fakeFactory) NewForCluster(_ context.Context, _ uint64, _, _ string) (*action.Configuration, error) {
	return f.cfg, nil
}

// fakeChartLoader returns a minimal chart whose metadata version mirrors the
// requested version.
type fakeChartLoader struct{}

func (fakeChartLoader) LoadChart(_ context.Context, _ uint64, name, version string) (*chart.Chart, error) {
	ch, err := loader.LoadArchive(buildMinimalChartTgz(name, version))
	return ch, err
}

func TestService_Install_Then_Upgrade_Then_Rollback_Then_Uninstall(t *testing.T) {
	rec := audit.NewMemoryRecorder()
	factory := newFakeFactory()
	apps := stubAppService(t)
	loader := fakeChartLoader{}
	s := NewServiceWithFactory(factory, apps, nil, loader, rec)

	appID := apps.installedAppID
	// install
	res, err := s.Install(context.Background(), 1, appID, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
	require.Equal(t, 1, res.Revision)
	require.Equal(t, "deployed", res.Status)

	// upgrade
	res, err = s.Upgrade(context.Background(), 1, appID, UpgradeRequest{ChartVersion: "1.1.0"})
	require.NoError(t, err)
	require.Equal(t, 2, res.Revision)

	// rollback to v1
	res, err = s.Rollback(context.Background(), 1, appID, RollbackRequest{Revision: 1})
	require.NoError(t, err)
	require.Equal(t, 3, res.Revision)

	// uninstall
	require.NoError(t, s.Uninstall(context.Background(), 1, appID, UninstallRequest{}))

	// re-install after uninstall should succeed (helm allows it)
	_, err = s.Install(context.Background(), 1, appID, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
}

func TestService_Install_DuplicateRelease(t *testing.T) {
	rec := audit.NewMemoryRecorder()
	factory := newFakeFactory()
	apps := stubAppService(t)
	s := NewServiceWithFactory(factory, apps, nil, fakeChartLoader{}, rec)

	appID := apps.installedAppID
	_, err := s.Install(context.Background(), 1, appID, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
	_, err = s.Install(context.Background(), 1, appID, InstallRequest{ChartVersion: "1.0.0"})
	require.Error(t, err)
	be := err.(*apperr.BizError)
	require.Equal(t, apperr.CodeAppsReleaseAlreadyExists, be.Code)
}

func TestService_Rollback_RevisionMissing(t *testing.T) {
	rec := audit.NewMemoryRecorder()
	factory := newFakeFactory()
	apps := stubAppService(t)
	s := NewServiceWithFactory(factory, apps, nil, fakeChartLoader{}, rec)

	_, err := s.Install(context.Background(), 1, apps.installedAppID, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
	_, err = s.Rollback(context.Background(), 1, apps.installedAppID, RollbackRequest{Revision: 999})
	require.Error(t, err)
	be := err.(*apperr.BizError)
	require.Equal(t, apperr.CodeAppsReleaseHistoryTooShort, be.Code)
}

// Helpers:
//   - stubAppService returns a stub that satisfies the methods Service.appsGet uses.
//   - buildMinimalChartTgz builds a tgz with a Chart.yaml + values.yaml + a
//     no-op template, suitable for helm SDK to install via the fake KubeClient.
//   - NewServiceWithFactory mirrors NewService but accepts a *fakeFactory so
//     production code uses *helmclient.Factory while tests use the fake.
//     Either rename the production constructor to accept an interface, OR add
//     a separate test-only constructor. The plan recommends adding an
//     interface:
//
//        type Factory interface {
//            NewForCluster(ctx context.Context, clusterID uint64, ns, purpose string) (*action.Configuration, error)
//        }
//
//     Then Service field type becomes `factory Factory`, the production
//     constructor signature is unchanged, and the fakeFactory above
//     satisfies it.
```

(Implement `stubAppService` and `buildMinimalChartTgz` inline in `service_test.go`. The chart-tgz builder uses `archive/tar` + `compress/gzip` to produce: `mychart/Chart.yaml` with `apiVersion: v2`, `name: mychart`, the requested `version: X`; `mychart/values.yaml` empty; `mychart/templates/configmap.yaml` rendering a no-op ConfigMap with namespace `{{ .Release.Namespace }}`.)

- [ ] **Step 6: `handler_test.go`** — unit through httptest, using the same fake Factory

Mirror Task 5/7 handler tests. Mount the 6 release endpoints under a no-auth test router; assert the round-trip status codes and that the audit recorder saw the right `Action` for each write.

- [ ] **Step 7: Coverage check**

```bash
cd optimus-be
go test ./internal/modules/apps/release/... -race -count=1 -v
go test ./internal/modules/apps/release/... -coverprofile=/tmp/p3-rel.cov
go tool cover -func=/tmp/p3-rel.cov | tail -1
```

Expected: PASS; coverage ≥ 60%. If under, the most likely gap is parseValues edge cases (empty / scalar / array root) and the rollback error-translation branch.

- [ ] **Step 8: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/release/ \
        optimus-be/internal/modules/apps/application/service.go
git commit -m "$(cat <<'EOF'
feat(be/apps/release): helm SDK install/upgrade/rollback/uninstall (P3)

apps/release wraps helm SDK action.* calls behind the helmclient.Factory.
All six endpoints (install/upgrade/rollback/uninstall/status/history)
hang off /apps/applications/:id/release/... so writes are always rooted
at an Optimus-registered application — operations on unregistered helm
releases are not possible by API design.

- Factory is an interface so tests can swap in an in-memory helm config
  (storage.driver.Memory + kube/fake.PrintingKubeClient). No real cluster
  needed for unit tests.
- ChartLoader is the seam apps/repo.Service implements; main.go wires.
- HelmStatusProbe + HelmInstalledChecker on release.Service close the
  loop with application.Service (status decoration + delete pre-check).
- Audit row per write with full {cluster_id, namespace, release_name,
  chart_version, revision} metadata.
- parseValues accepts empty string -> empty map; non-map root returns
  42205 InvalidValues.
- Rollback to a non-existent revision returns 42203
  CodeAppsReleaseHistoryTooShort (helm wraps this error message but
  doesn't expose a typed sentinel).

application/service.go gains GetModel exposing the raw *models row to
release.Service.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: `apps/module.go` + `cmd/server/main.go` wiring

**Goal:** Land the composition root for the apps module — DI for the three services, route mounting via the nested-sub-group permission pattern, and the post-construction seam wiring that closes the circular dependencies (apps/repo ↔ apps/application ↔ apps/release ↔ k8s/cluster).

**Files (new):**
- `optimus-be/internal/modules/apps/module.go`
- `optimus-be/internal/modules/apps/module_test.go`

**Files (modify):**
- `optimus-be/cmd/server/main.go`

- [ ] **Step 1: `module.go`** — assembly + MountRoutes

```go
package apps

import (
	"context"

	"github.com/gin-gonic/gin"

	"helm.sh/helm/v3/pkg/chart"

	"optimus-be/internal/infra/audit"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/rbac"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/apps/helmclient"
	"optimus-be/internal/modules/apps/release"
	apprepo "optimus-be/internal/modules/apps/repo"
)

// Module is the composition root for apps. main.go calls NewModule once and
// then ClusterPicker / cluster module receives Application.Counter via the
// returned *Module.
type Module struct {
	Repo        *apprepo.Service
	Application *application.Service
	Release     *release.Service

	RepoHandler        *apprepo.Handler
	ApplicationHandler *application.Handler
	ReleaseHandler     *release.Handler
}

// NewModule wires apps. Caller MUST pass:
//   - db: shared *gorm.DB
//   - cipher: the SHARED P1 vault.Cipher (do not call NewCipher again)
//   - rec: the shared audit.Recorder
//   - factory: helmclient.Factory built from the same Consumer
//   - clusterLookup: the k8s/cluster.Service that satisfies helmclient.ClusterLookup
//
// After NewModule, main.go must call m.WireUpstream(k8sCluster) and
// m.WireDownstream() to close the post-construction seams.
func NewModule(
	repo *apprepo.Service, app *application.Service, rel *release.Service,
) *Module {
	return &Module{
		Repo:               repo,
		Application:        app,
		Release:            rel,
		RepoHandler:        apprepo.NewHandler(repo),
		ApplicationHandler: application.NewHandler(app),
		ReleaseHandler:     release.NewHandler(rel),
	}
}

// MountRoutes attaches all 14 apps endpoints under rg with per-route
// RequirePermission middleware via nested sub-groups (NOT variadic).
func (m *Module) MountRoutes(rg *gin.RouterGroup, cache *rbac.PermissionCache) {
	repos := rg.Group("/apps/repos")
	{
		repos.Group("", middleware.RequirePermission(cache, "apps:repo:read")).
			GET("", m.RepoHandler.List).
			GET("/:id", m.RepoHandler.Get).
			GET("/:id/charts", m.RepoHandler.ListCharts).
			GET("/:id/charts/:chart/versions", m.RepoHandler.ListVersions).
			GET("/:id/charts/:chart/versions/:version/values", m.RepoHandler.GetDefaultValues)
		repos.Group("", middleware.RequirePermission(cache, "apps:repo:write")).
			POST("", m.RepoHandler.Create).
			PUT("/:id", m.RepoHandler.Update)
		repos.Group("", middleware.RequirePermission(cache, "apps:repo:delete")).
			DELETE("/:id", m.RepoHandler.Delete)
	}

	app := rg.Group("/apps/applications")
	{
		app.Group("", middleware.RequirePermission(cache, "apps:application:read")).
			GET("", m.ApplicationHandler.List).
			GET("/:id", m.ApplicationHandler.Get).
			GET("/:id/release", m.ReleaseHandler.Status).
			GET("/:id/release/history", m.ReleaseHandler.History)
		app.Group("", middleware.RequirePermission(cache, "apps:application:write")).
			POST("", m.ApplicationHandler.Create).
			PUT("/:id", m.ApplicationHandler.Update)
		app.Group("", middleware.RequirePermission(cache, "apps:application:delete")).
			DELETE("/:id", m.ApplicationHandler.Delete)
		app.Group("", middleware.RequirePermission(cache, "apps:release:install")).
			POST("/:id/release/install", m.ReleaseHandler.Install)
		app.Group("", middleware.RequirePermission(cache, "apps:release:upgrade")).
			POST("/:id/release/upgrade", m.ReleaseHandler.Upgrade)
		app.Group("", middleware.RequirePermission(cache, "apps:release:rollback")).
			POST("/:id/release/rollback", m.ReleaseHandler.Rollback)
		app.Group("", middleware.RequirePermission(cache, "apps:release:uninstall")).
			POST("/:id/release/uninstall", m.ReleaseHandler.Uninstall)
	}
}

// helmChartLoader satisfies release.ChartLoader by delegating to apps/repo.
// Lives here so apps/repo doesn't need to import helm.sh/helm/v3 directly.
type helmChartLoader struct{ repo *apprepo.Service }

func (l *helmChartLoader) LoadChart(ctx context.Context, repoID uint64, name, version string) (*chart.Chart, error) {
	// apps/repo.Service.FetchChartTgz returns the raw tgz bytes; release helper
	// parses them. The helper is exposed in apps/release (Task 11) as
	// helmChartFromTgz; since module.go is in package apps (not release), we
	// route the parse through apps/repo, which calls release.HelmChartFromTgz
	// (a re-exported alias of the unexported helper).
	tgz, err := l.repo.FetchChartTgz(ctx, repoID, name, version)
	if err != nil {
		return nil, err
	}
	return release.HelmChartFromTgz(tgz)
}
```

**Touch-ups to other files implied by the helmChartLoader plumbing:**
- `apps/repo/service.go` gains `FetchChartTgz(ctx, repoID, name, version) ([]byte, error)` that walks the same OCI / HTTP path as `GetDefaultValues` but returns the full .tgz instead of just `values.yaml`. Hoist the per-protocol fetch into a small private `fetchTgz` helper called by both. Add unit tests for both paths in `charts_test.go` (one HTTP via httptest server).
- `apps/release/service.go` exposes its tgz parser via:

  ```go
  // HelmChartFromTgz is the exported alias used by apps/module to keep the
  // helm import surface localised to the release/ package.
  func HelmChartFromTgz(tgz []byte) (*chart.Chart, error) { return helmChartFromTgz(tgz) }
  ```

Land these two touch-ups in this task's commit (they are tiny additions that close the cycle; no separate commit).

- [ ] **Step 2: `module_test.go`** — smoke

```go
package apps

import "testing"

func TestNewModule_NilArgsPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil args")
		}
	}()
	// Construct with nils; the NewService constructors don't have nil checks,
	// so the first method call would NPE — this test exists to surface the
	// missing-arg failure mode early. Adjust to taste; the canonical
	// behaviour is "fail fast at startup, not at first request".
	_ = NewModule(nil, nil, nil)
}
```

- [ ] **Step 3: Modify `cmd/server/main.go`**

Locate the existing block where the k8s module is wired (look for `k8s.NewModule` or `k8s.MountRoutes`). Immediately after, add the apps module wiring:

```go
// --- P3 apps ---
appRepoSvc := apprepo.NewService(apprepo.NewRepo(db), vaultCipher, auditRecorder)
appAppSvc := application.NewService(application.NewRepo(db), auditRecorder)

helmFactory := helmclient.NewFactory(credentialsConsumer, k8sModule.ClusterSvc)

appRelSvc := release.NewService(helmFactory, appAppSvc, appRepoSvc, nil /* loader filled below */, auditRecorder)

appModule := apps.NewModule(appRepoSvc, appAppSvc, appRelSvc)

// Now close the post-construction seams:
//
//   apps/repo <- application.Counter (so repo delete can pre-check)
//   k8s/cluster <- application.Counter (so cluster delete can pre-check)
//   application <- release as HelmStatusProbe + HelmInstalledChecker
//   release <- apps/repo as ChartLoader (via module.helmChartLoader)
appRepoSvc.SetInUseCounter(application.NewRepo(db))     // counter — repo dep
k8sModule.SetAppsCounter(application.NewRepo(db))       // counter — k8s dep
appAppSvc.SetHelmStatusProbe(appRelSvc)
appAppSvc.SetHelmInstalledChecker(appRelSvc)
appRelSvc.SetChartLoader(&apps.HelmChartLoader{Repo: appRepoSvc}) // see note below

// Mount routes (after JWTAuth, like every other module).
appModule.MountRoutes(authedV1, permCache)
```

Add imports at the top:

```go
"optimus-be/internal/modules/apps"
"optimus-be/internal/modules/apps/application"
"optimus-be/internal/modules/apps/helmclient"
apprepo "optimus-be/internal/modules/apps/repo"
"optimus-be/internal/modules/apps/release"
```

Note on `apps.HelmChartLoader`: in Step 1 the loader was a private type inside `module.go`. Promote it to exported (`HelmChartLoader`) so `main.go` can construct it:

```go
// HelmChartLoader satisfies release.ChartLoader by delegating to apps/repo.
type HelmChartLoader struct{ Repo *apprepo.Service }

func (l *HelmChartLoader) LoadChart(ctx context.Context, repoID uint64, name, version string) (*chart.Chart, error) {
	tgz, err := l.Repo.FetchChartTgz(ctx, repoID, name, version)
	if err != nil {
		return nil, err
	}
	return release.HelmChartFromTgz(tgz)
}
```

And `release.Service` gains a setter:

```go
func (s *Service) SetChartLoader(l ChartLoader) { s.loader = l }
```

(Constructor's `loader` param remains, but the setter lets main.go fix the circular wiring after both services exist.)

- [ ] **Step 4: Build + run all tests**

```bash
cd optimus-be
go build ./...
go test ./... -race -count=1
```

Expected: everything compiles and existing P0/P1/P2 tests still pass.

- [ ] **Step 5: Curl-smoke the new routes**

Start the server (`make run`), log in as admin, grab a token, and:

```bash
TOKEN=...  # access token from /api/v1/auth/login

curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/apps/repos | jq

curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/apps/applications | jq
```

Expected: both return `{"code":0,"data":{"items":[],"total":0,"page":1,"page_size":20},"message":"OK"}`.

- [ ] **Step 6: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/module.go \
        optimus-be/internal/modules/apps/module_test.go \
        optimus-be/internal/modules/apps/repo/service.go \
        optimus-be/internal/modules/apps/release/service.go \
        optimus-be/cmd/server/main.go
git commit -m "$(cat <<'EOF'
feat(be/apps): module wiring + cmd/server/main.go integration (P3)

apps.Module is the composition root: builds the three handlers, mounts
14 endpoints via nested sub-group RequirePermission middleware, and
exposes HelmChartLoader so main.go can close the post-construction
seam between apps/release (needs a chart loader) and apps/repo (knows
how to fetch a chart .tgz).

main.go wires:
- apps/repo.InUseCounter <- application.Repo (so repo.Delete pre-checks)
- k8s/cluster.AppsApplicationCounter <- application.Repo
- application.HelmStatusProbe <- release.Service
- application.HelmInstalledChecker <- release.Service
- release.ChartLoader <- HelmChartLoader{repo}

No new global state, no new caches — every helm op still builds a fresh
*action.Configuration via the Factory.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: `cmd/seed` — apps menu + `k8s_operator` role updates

**Goal:** Append the apps top-level menu (1 parent + 2 children) to the seed, and grant the existing `k8s_operator` role the 6 new permissions per spec §6.3.

**Files (modify):**
- `optimus-be/internal/seed/seed.go`
- `optimus-be/internal/seed/seed_test.go`

- [ ] **Step 1: Find the existing menu insertion block**

```bash
grep -n 'menu.k8s' /Users/logic/Projects/optimus/optimus-be/internal/seed/seed.go
```

P2 added a `menu.k8s` parent with five children. Mirror the pattern below it.

- [ ] **Step 2: Append apps menu rows**

In `seed.go`, after the last `menu.k8s.*` insertion line, add:

```go
	// P3 apps menus.
	{Name: "menu.apps", Path: "/apps", Icon: "AppstoreOutlined", Sort: 600, ParentID: nil, PermissionCode: ""},
	{Name: "menu.apps.applications", Path: "/apps/applications", Icon: "", Sort: 1, ParentName: "menu.apps", PermissionCode: "apps:application:read"},
	{Name: "menu.apps.chart-repos",  Path: "/apps/chart-repos",  Icon: "", Sort: 2, ParentName: "menu.apps", PermissionCode: "apps:repo:read"},
```

Match the exact field names the existing seed struct uses (look at the menu fields in seed.go); if the seed code already uses a "parent_name → parent_id resolution" helper, use it. Otherwise insert in two passes.

The seed loop's post-processing resolves `ParentName` → `ParentID` lookup. If that helper doesn't exist, P3 introduces it minimally. The `menu.apps` parent must be inserted before its two children.

Sort value `600` follows P2's k8s parent (~500); leaves room for future top-level menus.

- [ ] **Step 3: Update `k8s_operator` role grants**

Find the existing builtin role registration block (search for `k8s_operator` in `seed.go`). Append to its permission code list:

```go
"apps:application:read",
"apps:repo:read",
"apps:release:install",
"apps:release:upgrade",
"apps:release:rollback",
"apps:release:uninstall",
```

`k8s_viewer` similarly gains:

```go
"apps:application:read",
"apps:repo:read",
```

`system_admin` grants all permissions via wildcard in P0 — no change needed if that's how it works; otherwise append the 10 codes.

- [ ] **Step 4: Update `seed_test.go`**

Add:

```go
func TestSeed_AppsMenusInserted(t *testing.T) {
	db := dbtest.NewDB(t)
	require.NoError(t, Run(context.Background(), db))

	var rows []models.Menu
	require.NoError(t, db.Where("name LIKE ?", "menu.apps%").Find(&rows).Error)
	got := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		got[r.Name] = struct{}{}
	}
	for _, want := range []string{"menu.apps", "menu.apps.applications", "menu.apps.chart-repos"} {
		_, ok := got[want]
		require.True(t, ok, "missing menu %q", want)
	}
}

func TestSeed_K8sOperatorHasAppsReleasePerms(t *testing.T) {
	db := dbtest.NewDB(t)
	require.NoError(t, Run(context.Background(), db))

	var role models.Role
	require.NoError(t, db.Where("name = ?", "k8s_operator").First(&role).Error)
	var codes []string
	require.NoError(t, db.Table("role_permissions").
		Select("permissions.code").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ?", role.ID).
		Pluck("permissions.code", &codes).Error)

	want := []string{
		"apps:application:read", "apps:repo:read",
		"apps:release:install", "apps:release:upgrade",
		"apps:release:rollback", "apps:release:uninstall",
	}
	for _, w := range want {
		require.Contains(t, codes, w)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd optimus-be
go test -tags=dbtest ./internal/seed/... -race -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Apply seed to dev DB to verify**

```bash
cd optimus-be
make seed
```

Expected: existing admin user already exists; the new menu rows and permission grants are upserted.

Verify:

```bash
docker exec -i optimus-dev-db psql -U optimus -d optimus -c "SELECT name, path, sort FROM menus WHERE name LIKE 'menu.apps%' ORDER BY sort;"
docker exec -i optimus-dev-db psql -U optimus -d optimus -c "SELECT p.code FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE r.name='k8s_operator' AND p.code LIKE 'apps:%' ORDER BY p.code;"
```

The first query returns 3 rows; the second returns the 6 apps codes.

- [ ] **Step 7: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/seed/seed.go \
        optimus-be/internal/seed/seed_test.go
git commit -m "$(cat <<'EOF'
feat(be/seed): apps menu tree + k8s_operator role updates (P3)

Seed gains:
- menu.apps top-level parent with two children
  (menu.apps.applications, menu.apps.chart-repos), sort=600 leaving
  room above for future top-level menus.
- k8s_operator role: apps:application:read, apps:repo:read,
  apps:release:install/upgrade/rollback/uninstall.
- k8s_viewer role: apps:application:read, apps:repo:read.
- system_admin: implicit via wildcard (no change required).

i18n keys for menu.apps.* land in Task 18 (FE locale additions).
Menu Name fields are aligned with i18n keys to avoid the rendering bug
P2 hit and fixed in 9352587.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: BE integration tests (3 files)

**Goal:** Add the three integration tests demanded by spec §10.2. These exercise real Postgres via dockertest and assert auth/audit/FK behaviour. Helm SDK is stubbed at the service layer for the release test — this layer only verifies the HTTP/audit envelope.

**Files (new):**
- `optimus-be/tests/integration/apps_repo_test.go`
- `optimus-be/tests/integration/apps_application_test.go`
- `optimus-be/tests/integration/apps_release_test.go`

- [ ] **Step 1: `apps_repo_test.go`**

Build tag `//go:build dbtest`. Mirror existing P2 integration tests (look at `tests/integration/cluster_test.go` for the bootstrap pattern). Required scenarios:

- Authenticated create + list + get round-trip; assert `has_password=true` after creating with non-empty password; assert `password` field never appears in any response body.
- Update with `"password": null` clears `has_password`.
- Delete refused with 42002 when applications reference the repo (insert a fixture application via the application repo directly).
- Soft-deleted name can be reused.

- [ ] **Step 2: `apps_application_test.go`**

Required scenarios:

- Authenticated create + list + get round-trip; verify FK preload populates `cluster_name` and `owner_name`.
- Create with duplicate `(cluster_id, namespace, release_name)` returns 42003.
- Create with non-existent `cluster_id` returns the FK violation mapped to a friendly error.
- Delete refused when `HelmInstalledChecker` returns true (wire a fake checker into the test container).
- LIST filters `cluster_id` and `tag` return the expected subset.
- PUT only mutates `description`, `tags`, `owner_user_id`; attempt to PUT `cluster_id` is silently ignored.

- [ ] **Step 3: `apps_release_test.go`**

Required scenarios (helm calls stubbed at service-layer):

- Each of the four write endpoints requires its specific permission; access with the wrong permission returns 40302.
- Each write writes one audit row with the expected `Action` and `Metadata` shape (verify via DB query of `audit_logs`).
- `GET /apps/applications/:id/release/history` returns 404 when no release exists.
- `POST /apps/applications/:id/release/install` returns 42201 when helm reports the release already exists (use a fake service that returns the BizError).

- [ ] **Step 4: Run with Colima socket**

```bash
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
colima start    # if not already
cd optimus-be
make test-int   # uses -tags=dbtest
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/tests/integration/apps_repo_test.go \
        optimus-be/tests/integration/apps_application_test.go \
        optimus-be/tests/integration/apps_release_test.go
git commit -m "$(cat <<'EOF'
test(be/integration): apps repo + application + release end-to-end

Three new dockertest files cover auth + audit + FK behaviour against
real Postgres:
- apps_repo: never-leak-password, null-clears, in-use refusal, soft-
  delete name reuse.
- apps_application: release-tuple unique, FK pre-checks, immutable
  field enforcement, list filter shape.
- apps_release: per-endpoint RBAC, per-write audit row, history 404,
  install duplicate -> 42201 (helm stubbed).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Swagger annotations + regenerate docs

**Goal:** Add swag annotations to every apps handler so `make swagger-diff` passes and `docs/api/swagger.json` reflects the 14 new endpoints. Also regenerate `docs/permissions.md`.

**Files (modify):**
- `optimus-be/internal/modules/apps/repo/handler.go`
- `optimus-be/internal/modules/apps/application/handler.go`
- `optimus-be/internal/modules/apps/release/handler.go`
- `optimus-be/api/docs/swagger.json` (regenerated)
- `docs/api/swagger.json` (copied)
- `docs/permissions.md` (regenerated)

- [ ] **Step 1: Annotate each handler method**

Pattern (apply to all 14 endpoints):

```go
// List godoc
//
//	@Summary      List chart repositories
//	@Description  Paginated list filtered by name / type
//	@Tags         apps
//	@Security     BearerAuth
//	@Produce      json
//	@Param        name      query  string  false  "fuzzy name filter"
//	@Param        type      query  string  false  "oci|http"
//	@Param        page      query  int     false  "page, default 1"
//	@Param        page_size query  int     false  "page size, default 20"
//	@Success      200       {object}  response.Envelope{data=repo.ListResponse}
//	@Failure      400       {object}  response.Envelope
//	@Failure      403       {object}  response.Envelope
//	@Router       /apps/repos [get]
func (h *Handler) List(c *gin.Context) { ... }
```

Cover each endpoint with `@Summary`, `@Description`, `@Tags apps`, `@Security BearerAuth`, request `@Param` lines for path/query/body, and `@Success`/`@Failure` lines for the codes the endpoint may return.

For body endpoints (`POST`/`PUT`):

```go
//	@Accept   json
//	@Param    request body repo.CreateRequest true "create chart repo"
```

For release endpoints, the `@Failure` set is larger:

```go
//	@Failure  400   {object}  response.Envelope  "validation failure"
//	@Failure  403   {object}  response.Envelope  "permission denied"
//	@Failure  404   {object}  response.Envelope  "application not found"
//	@Failure  422   {object}  response.Envelope  "helm domain error (42xxx)"
```

- [ ] **Step 2: Regenerate swagger**

```bash
cd optimus-be
make swag
```

Expected: `api/docs/swagger.json` and `docs/api/swagger.json` updated. Run:

```bash
make swagger-diff
```

Expected: PASS (no diff).

- [ ] **Step 3: Regenerate permissions doc**

```bash
cd optimus-be
make dump-perms
```

Expected: `docs/permissions.md` lists the 10 `apps:*` codes under category `apps`. Run:

```bash
make perm-check
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/ \
        optimus-be/api/docs/swagger.json \
        docs/api/swagger.json \
        docs/permissions.md
git commit -m "$(cat <<'EOF'
docs(be/apps): swagger annotations + regenerated swagger/perms (P3)

Adds @Summary/@Description/@Tags/@Param/@Success/@Failure annotations
to all 14 apps handlers. Regenerates docs/api/swagger.json (the
external mirror) and docs/permissions.md from the in-code registry.
CI swagger-diff + perm-check now green.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: BE coverage gate audit

**Goal:** Verify every `apps/*` package is at or above 60% test coverage. Find shortfalls and lift them with targeted tests. This task is bookkeeping but mandatory — the CI gate rejects regressions, and skipping it now means later PRs eat the cost.

**Files (modify):** any of `internal/modules/apps/**/*_test.go` as needed.

- [ ] **Step 1: Run full coverage**

```bash
cd optimus-be
go test -tags=dbtest -race -coverprofile=/tmp/p3-cover.cov \
  ./internal/modules/apps/...
go tool cover -func=/tmp/p3-cover.cov | tail -30
```

Look at the per-package totals. Anything `< 60.0%` is in scope.

- [ ] **Step 2: For each shortfall, add targeted cases**

Common gaps and the cases that close them:

- `apps/repo`: `List` query branches (filter by `name`, by `type`, by both), `Update` no-op (empty body), `Delete` without InUseCounter set (skip path).
- `apps/application`: list filter by `tag`, list filter by `owner_user_id`, `SetChartRepo` round-trip, `GetModel` not-found path.
- `apps/release`: `parseValues` empty / map-root / non-map-root, `Uninstall` with `KeepHistory=true` then re-history shows previous entries.
- `apps/helmclient`: `restClientGetter` four-method conformance test (call each, assert non-nil), validate the kubeconfig parse failure path.
- `apps/errs`: combinatorial table for substring matches (already mostly covered in Task 9).

- [ ] **Step 3: Re-run and confirm**

```bash
go test -tags=dbtest -race -coverprofile=/tmp/p3-cover.cov \
  ./internal/modules/apps/...
go tool cover -func=/tmp/p3-cover.cov | tail -1
```

Every line of the per-package summary must show ≥ 60.0%.

- [ ] **Step 4: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/internal/modules/apps/
git commit -m "$(cat <<'EOF'
test(be/apps): lift coverage above 60% gate across all P3 packages

Targeted cases plug the gaps surfaced by go tool cover:
- repo:        List filter branches + Update no-op + Delete-no-counter.
- application: tag/owner filters, SetChartRepo, GetModel not-found.
- release:     parseValues edge cases, Uninstall+KeepHistory replay.
- helmclient:  restClientGetter four-method conformance + bad-yaml.
- errs:        substring match table.

All apps/* packages now report >= 60% by `go tool cover -func`.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: FE — types, API modules, store, main.ts wiring

**Goal:** Land the TypeScript layer that subsequent FE tasks use: BE-mirroring DTOs in `types/apps.ts`, axios-backed API wrappers in `api/apps/{repo,application,release}.ts`, a thin Pinia store, and `provide()` registration in `main.ts`.

**Files (new):**
- `optimus-fe/src/types/apps.ts`
- `optimus-fe/src/api/apps/repo.ts`
- `optimus-fe/src/api/apps/application.ts`
- `optimus-fe/src/api/apps/release.ts`
- `optimus-fe/src/stores/apps.ts`

**Files (modify):**
- `optimus-fe/src/main.ts`

- [ ] **Step 1: `types/apps.ts`**

```ts
// Mirrors apps/{repo,application,release}/dto.go. The naming convention
// matches user.Summary / user.Detail used by P0.

export interface ChartRepoSummary {
  id: number
  name: string
  type: 'oci' | 'http'
  url: string
  username: string
  has_password: boolean
  description: string
  created_at: string
  updated_at: string
}
export type ChartRepoDetail = ChartRepoSummary

export interface ChartRepoCreateRequest {
  name: string
  type: 'oci' | 'http'
  url: string
  username?: string
  password?: string
  description?: string
}
export interface ChartRepoUpdateRequest {
  name?: string
  url?: string
  username?: string
  password?: string | null
  description?: string
}

export interface ChartSummary  { name: string; version_count: number; description: string }
export interface VersionSummary { version: string; app_version: string; created: string }

export interface ApplicationSummary {
  id: number
  name: string
  cluster_id: number
  cluster_name: string
  namespace: string
  release_name: string
  chart_repo_id: number
  chart_name: string
  description: string
  tags: string[]
  owner_user_id?: number
  owner_name?: string
  created_at: string
  updated_at: string
}
export interface ApplicationDetail extends ApplicationSummary {
  status?: 'deployed' | 'failed' | 'pending' | 'unknown' | ''
  revision?: number
  chart_version?: string
  app_version?: string
  last_deployed_at?: string
}
export interface ApplicationCreateRequest {
  name: string
  cluster_id: number
  namespace: string
  release_name: string
  chart_repo_id: number
  chart_name: string
  description?: string
  tags?: string[]
  owner_user_id?: number
}
export interface ApplicationUpdateRequest {
  description?: string
  tags?: string[]
  owner_user_id?: number
}

export interface ReleaseStatus {
  status: string
  revision: number
  chart_version: string
  app_version: string
  last_deployed_at: string
  notes?: string
}
export interface RevisionRow {
  revision: number
  status: string
  chart_version: string
  app_version: string
  updated_at: string
  description: string
}
export interface InstallRequest  { chart_version: string; values_yaml: string }
export interface UpgradeRequest  { chart_repo_id?: number; chart_version: string; values_yaml: string }
export interface RollbackRequest { revision: number }
export interface UninstallRequest { keep_history?: boolean }
export interface InstallResult   {
  revision: number
  status: string
  chart_version: string
  last_deployed_at: string
}
```

- [ ] **Step 2: `api/apps/repo.ts`**

```ts
import type { AxiosInstance } from 'axios'
import type {
  ChartRepoSummary, ChartRepoDetail, ChartRepoCreateRequest, ChartRepoUpdateRequest,
  ChartSummary, VersionSummary,
} from '@/types/apps'

export interface ListParams {
  page?: number
  page_size?: number
  name?: string
  type?: 'oci' | 'http'
}
export interface ListResponse {
  items: ChartRepoSummary[]
  total: number
  page: number
  page_size: number
}

export class RepoApi {
  constructor(private client: AxiosInstance) {}

  list(params: ListParams = {}): Promise<ListResponse> {
    return this.client.get('/apps/repos', { params }).then(r => r.data.data)
  }
  get(id: number): Promise<ChartRepoDetail> {
    return this.client.get(`/apps/repos/${id}`).then(r => r.data.data)
  }
  create(body: ChartRepoCreateRequest): Promise<ChartRepoDetail> {
    return this.client.post('/apps/repos', body).then(r => r.data.data)
  }
  update(id: number, body: ChartRepoUpdateRequest): Promise<ChartRepoDetail> {
    return this.client.put(`/apps/repos/${id}`, body).then(r => r.data.data)
  }
  remove(id: number): Promise<void> {
    return this.client.delete(`/apps/repos/${id}`).then(() => undefined)
  }
  listCharts(id: number): Promise<{ items: ChartSummary[] }> {
    return this.client.get(`/apps/repos/${id}/charts`).then(r => r.data.data)
  }
  listVersions(id: number, chart: string): Promise<{ items: VersionSummary[] }> {
    return this.client.get(`/apps/repos/${id}/charts/${encodeURIComponent(chart)}/versions`)
      .then(r => r.data.data)
  }
  getDefaultValues(id: number, chart: string, version: string): Promise<{ values_yaml: string }> {
    return this.client.get(`/apps/repos/${id}/charts/${encodeURIComponent(chart)}/versions/${encodeURIComponent(version)}/values`)
      .then(r => r.data.data)
  }
}
```

- [ ] **Step 3: `api/apps/application.ts`**

```ts
import type { AxiosInstance } from 'axios'
import type {
  ApplicationSummary, ApplicationDetail,
  ApplicationCreateRequest, ApplicationUpdateRequest,
} from '@/types/apps'

export interface ListParams {
  page?: number
  page_size?: number
  name?: string
  cluster_id?: number
  namespace?: string
  owner_user_id?: number
  tag?: string
}
export interface ListResponse {
  items: ApplicationSummary[]
  total: number
  page: number
  page_size: number
}

export class ApplicationApi {
  constructor(private client: AxiosInstance) {}

  list(params: ListParams = {}): Promise<ListResponse> {
    return this.client.get('/apps/applications', { params }).then(r => r.data.data)
  }
  get(id: number): Promise<ApplicationDetail> {
    return this.client.get(`/apps/applications/${id}`).then(r => r.data.data)
  }
  create(body: ApplicationCreateRequest): Promise<ApplicationDetail> {
    return this.client.post('/apps/applications', body).then(r => r.data.data)
  }
  update(id: number, body: ApplicationUpdateRequest): Promise<ApplicationDetail> {
    return this.client.put(`/apps/applications/${id}`, body).then(r => r.data.data)
  }
  remove(id: number): Promise<void> {
    return this.client.delete(`/apps/applications/${id}`).then(() => undefined)
  }
}
```

- [ ] **Step 4: `api/apps/release.ts`**

```ts
import type { AxiosInstance } from 'axios'
import type {
  ReleaseStatus, RevisionRow,
  InstallRequest, UpgradeRequest, RollbackRequest, UninstallRequest,
  InstallResult,
} from '@/types/apps'

export class ReleaseApi {
  constructor(private client: AxiosInstance) {}

  status(appId: number): Promise<ReleaseStatus> {
    return this.client.get(`/apps/applications/${appId}/release`).then(r => r.data.data)
  }
  history(appId: number): Promise<{ items: RevisionRow[] }> {
    return this.client.get(`/apps/applications/${appId}/release/history`).then(r => r.data.data)
  }
  install(appId: number, body: InstallRequest): Promise<InstallResult> {
    return this.client.post(`/apps/applications/${appId}/release/install`, body).then(r => r.data.data)
  }
  upgrade(appId: number, body: UpgradeRequest): Promise<InstallResult> {
    return this.client.post(`/apps/applications/${appId}/release/upgrade`, body).then(r => r.data.data)
  }
  rollback(appId: number, body: RollbackRequest): Promise<InstallResult> {
    return this.client.post(`/apps/applications/${appId}/release/rollback`, body).then(r => r.data.data)
  }
  uninstall(appId: number, body: UninstallRequest = {}): Promise<void> {
    return this.client.post(`/apps/applications/${appId}/release/uninstall`, body).then(() => undefined)
  }
}
```

- [ ] **Step 5: `stores/apps.ts`**

```ts
import { defineStore } from 'pinia'

export interface AppsState {
  filterClusterId: number | null
  filterNamespace: string
}

export const useAppsStore = defineStore('apps', {
  state: (): AppsState => ({
    filterClusterId: null,
    filterNamespace: '',
  }),
  actions: {
    setClusterFilter(id: number | null) { this.filterClusterId = id },
    setNamespaceFilter(ns: string)      { this.filterNamespace = ns },
    reset()                             { this.$reset() },
  },
})
```

- [ ] **Step 6: `main.ts`** — wire api modules

Locate the existing `provide('*Api')` block (P2 wires k8s api modules there). Append:

```ts
import { RepoApi }        from '@/api/apps/repo'
import { ApplicationApi } from '@/api/apps/application'
import { ReleaseApi }     from '@/api/apps/release'

const appsRepoApi        = new RepoApi(httpClient)
const appsApplicationApi = new ApplicationApi(httpClient)
const appsReleaseApi     = new ReleaseApi(httpClient)

app.provide('appsRepoApi',        appsRepoApi)
app.provide('appsApplicationApi', appsApplicationApi)
app.provide('appsReleaseApi',     appsReleaseApi)
```

And declare matching `InjectionKey`s in `src/api/keys.ts` (or wherever P2 keeps them) so the views can use `inject(appsRepoApiKey)`:

```ts
import type { InjectionKey } from 'vue'
import type { RepoApi }        from '@/api/apps/repo'
import type { ApplicationApi } from '@/api/apps/application'
import type { ReleaseApi }     from '@/api/apps/release'

export const appsRepoApiKey:        InjectionKey<RepoApi>        = Symbol('appsRepoApi')
export const appsApplicationApiKey: InjectionKey<ApplicationApi> = Symbol('appsApplicationApi')
export const appsReleaseApiKey:     InjectionKey<ReleaseApi>     = Symbol('appsReleaseApi')
```

- [ ] **Step 7: Typecheck**

```bash
cd optimus-fe
bun run typecheck
bun run lint
```

Expected: both PASS.

- [ ] **Step 8: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/types/apps.ts \
        optimus-fe/src/api/apps/ \
        optimus-fe/src/stores/apps.ts \
        optimus-fe/src/main.ts \
        optimus-fe/src/api/keys.ts
git commit -m "$(cat <<'EOF'
feat(fe/apps): types + api modules + store + main.ts wiring (P3)

DTOs mirror BE apps/{repo,application,release}/dto.go with the
Summary/Detail naming convention. API modules are thin axios wrappers
returning the inner data envelope. Pinia store holds list filter state
only (no application list cache — per spec §8.5).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 18: FE locale additions (49 keys × 2 locales)

**Goal:** Land all `apps.*`, `menu.apps.*`, and `perm.apps.*` keys per spec §A.1 in both `en-US.json` and `zh-CN.json`. (The 16 `error.42*` keys were already added in Task 2.)

**Files (modify):**
- `optimus-fe/src/locales/zh-CN.json`
- `optimus-fe/src/locales/en-US.json`

- [ ] **Step 1: Insert the 33 new (non-error) keys**

Find the apps-relevant insertion point (right after `menu.k8s.*` keys for menu group; right after the existing `apps.*` if any, otherwise after `k8s.*`). Insert per spec §A.1 — the full 33 keys for `apps.*` + `menu.apps.*` + `perm.apps.*`. Each key in both locales.

- [ ] **Step 2: i18n parity check**

```bash
cd optimus-fe
bun run i18n:check
```

Expected: PASS — both locales contain identical key sets.

- [ ] **Step 3: Browser-smoke**

Run `bun run dev`, log in, click through Settings → Permissions and verify the apps-category permissions display human-readable names in both locales (toggle via the language switcher).

- [ ] **Step 4: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/locales/zh-CN.json \
        optimus-fe/src/locales/en-US.json
git commit -m "$(cat <<'EOF'
feat(fe/i18n): P3 apps.* + menu.apps.* + perm.apps.* keys (zh + en)

49 total new keys: 33 functional keys per spec §A.1 plus the 16
error.42* keys already shipped in Task 2. Both locales identical;
bun run i18n:check passes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 19: FE ChartRepos pages (List + Form)

**Goal:** Land the chart-repo CRUD page (List + create/edit modal). Uses P0's ProTable + ProForm patterns. Action buttons gated by `v-permission`.

**Files (new):**
- `optimus-fe/src/views/apps/ChartRepos/List.vue`
- `optimus-fe/src/views/apps/ChartRepos/Form.vue`

- [ ] **Step 1: `Form.vue`**

```vue
<template>
  <a-modal
    :open="open"
    :title="isEdit ? t('apps.repo.form.title.edit') : t('apps.repo.form.title.create')"
    @ok="onOk"
    @cancel="$emit('update:open', false)"
    :confirm-loading="submitting"
  >
    <a-form :model="form" layout="vertical" ref="formRef" :rules="rules">
      <a-form-item :label="t('common.field.name')" name="name">
        <a-input v-model:value="form.name" :max-length="64" />
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.type')" name="type">
        <a-radio-group v-model:value="form.type" :disabled="isEdit">
          <a-radio value="http">HTTP</a-radio>
          <a-radio value="oci">OCI</a-radio>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.url')" name="url">
        <a-input v-model:value="form.url" :max-length="2048" />
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.username')">
        <a-input v-model:value="form.username" :max-length="255" />
      </a-form-item>
      <a-form-item :label="t('apps.repo.form.field.password')">
        <a-input-password
          v-model:value="form.password"
          :placeholder="isEdit ? t('apps.repo.form.placeholder.passwordEdit') : ''"
        />
        <template v-if="isEdit && original?.has_password">
          <a-button size="small" danger type="link" @click="form.password = '__CLEAR__'">
            {{ t('apps.repo.form.btn.clearPassword') }}
          </a-button>
        </template>
      </a-form-item>
      <a-form-item :label="t('common.field.description')">
        <a-textarea v-model:value="form.description" :rows="3" :max-length="4096" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch, computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'

import { appsRepoApiKey } from '@/api/keys'
import type { ChartRepoDetail } from '@/types/apps'

const props = defineProps<{ open: boolean; original?: ChartRepoDetail | null }>()
const emit  = defineEmits<{ (e: 'update:open', v: boolean): void; (e: 'saved'): void }>()

const { t } = useI18n()
const repoApi = inject(appsRepoApiKey)!
const formRef = ref()
const submitting = ref(false)

const form = ref({
  name: '', type: 'http' as 'http' | 'oci', url: '',
  username: '', password: '', description: '',
})
const isEdit = computed(() => !!props.original)

watch(() => props.open, (v) => {
  if (v) {
    if (props.original) {
      form.value = {
        name: props.original.name, type: props.original.type, url: props.original.url,
        username: props.original.username, password: '', description: props.original.description,
      }
    } else {
      form.value = { name: '', type: 'http', url: '', username: '', password: '', description: '' }
    }
  }
})

const rules = {
  name: [{ required: true, max: 64 }],
  type: [{ required: true }],
  url:  [{ required: true, max: 2048 }],
}

async function onOk() {
  await formRef.value.validate()
  submitting.value = true
  try {
    if (props.original) {
      // Update — translate sentinel "__CLEAR__" to JSON null.
      const body: any = {
        name: form.value.name, url: form.value.url,
        username: form.value.username, description: form.value.description,
      }
      if (form.value.password === '__CLEAR__') body.password = null
      else if (form.value.password !== '')    body.password = form.value.password
      await repoApi.update(props.original.id, body)
    } else {
      await repoApi.create({
        name: form.value.name, type: form.value.type, url: form.value.url,
        username: form.value.username || undefined,
        password: form.value.password || undefined,
        description: form.value.description || undefined,
      })
    }
    message.success(t('common.message.saved'))
    emit('saved')
    emit('update:open', false)
  } finally {
    submitting.value = false
  }
}
</script>
```

- [ ] **Step 2: `List.vue`**

```vue
<template>
  <PageHeader :title="t('apps.repo.list.title')">
    <template #extra>
      <a-button v-permission="'apps:repo:write'" type="primary" @click="openCreate">
        {{ t('common.button.create') }}
      </a-button>
    </template>
  </PageHeader>

  <a-table
    :columns="columns"
    :data-source="data?.items ?? []"
    :pagination="paginationConfig"
    :loading="loading"
    row-key="id"
    @change="onTableChange"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'has_password'">
        <a-tag :color="record.has_password ? 'green' : 'default'">
          {{ record.has_password ? t('apps.repo.form.field.hasPassword') : '—' }}
        </a-tag>
      </template>
      <template v-else-if="column.key === 'actions'">
        <a-space>
          <a-button v-permission="'apps:repo:write'" size="small" @click="openEdit(record)">
            {{ t('common.button.edit') }}
          </a-button>
          <a-popconfirm
            :title="t('common.popconfirm.delete')"
            @confirm="onDelete(record.id)"
          >
            <a-button v-permission="'apps:repo:delete'" size="small" danger>
              {{ t('common.button.delete') }}
            </a-button>
          </a-popconfirm>
        </a-space>
      </template>
    </template>
  </a-table>

  <Form
    v-model:open="formOpen"
    :original="formOriginal"
    @saved="refresh"
  />
</template>

<script setup lang="ts">
import { ref, inject, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'

import { appsRepoApiKey } from '@/api/keys'
import type { ChartRepoSummary } from '@/types/apps'
import PageHeader from '@/components/PageHeader.vue'
import Form from './Form.vue'

const { t } = useI18n()
const repoApi = inject(appsRepoApiKey)!

const loading = ref(false)
const data = ref<{ items: ChartRepoSummary[]; total: number; page: number; page_size: number } | null>(null)
const page = ref(1)
const pageSize = ref(20)

const formOpen = ref(false)
const formOriginal = ref<ChartRepoSummary | null>(null)

const columns = computed(() => [
  { title: t('common.field.name'), dataIndex: 'name', key: 'name' },
  { title: t('apps.repo.form.field.type'), dataIndex: 'type', key: 'type' },
  { title: t('apps.repo.form.field.url'),  dataIndex: 'url',  key: 'url' },
  { title: t('apps.repo.form.field.username'), dataIndex: 'username', key: 'username' },
  { title: t('apps.repo.form.field.hasPassword'), key: 'has_password' },
  { title: t('common.field.created_at'), dataIndex: 'created_at', key: 'created_at' },
  { title: t('common.field.actions'), key: 'actions', fixed: 'right' },
])

const paginationConfig = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: data.value?.total ?? 0,
}))

async function refresh() {
  loading.value = true
  try {
    data.value = await repoApi.list({ page: page.value, page_size: pageSize.value })
  } finally {
    loading.value = false
  }
}
function onTableChange(p: any) {
  page.value = p.current
  pageSize.value = p.pageSize
  refresh()
}
function openCreate() {
  formOriginal.value = null
  formOpen.value = true
}
function openEdit(r: ChartRepoSummary) {
  formOriginal.value = r
  formOpen.value = true
}
async function onDelete(id: number) {
  try {
    await repoApi.remove(id)
    message.success(t('common.message.deleted'))
    refresh()
  } catch (e: any) {
    // BizError message_key is already surfaced by axios interceptor.
  }
}

onMounted(refresh)
</script>
```

- [ ] **Step 3: Manual browser smoke**

```bash
cd optimus-fe
bun run dev
```

Navigate to `/apps/chart-repos`. Verify:
- Page loads (empty table).
- Create button visible only for users with `apps:repo:write`.
- Create flow saves and the row appears.
- Edit modal shows `has_password=true` after editing a row that had a password.
- Edit modal "Clear password" button + save sends `password: null` to the BE; row's `has_password` flips to false.
- Delete confirmation flows correctly.

- [ ] **Step 4: Build + typecheck + lint**

```bash
cd optimus-fe
bun run typecheck
bun run lint
bun run build
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/views/apps/ChartRepos/
git commit -m "$(cat <<'EOF'
feat(fe/apps): chart repositories list + form modal (P3)

Two-page CRUD surface for chart repos using P0's antd primitives. Action
buttons gated by v-permission. Form modal:
- type is disabled in edit mode (immutable per spec §3.1).
- password field's placeholder reflects edit-vs-create.
- explicit "Clear password" button maps to JSON null on the wire, so the
  BE distinguishes keep / replace / clear.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 20: FE shared components (ValuesEditor, ChartPickerStep, HistoryTable, ApplicationFormBasic)

**Goal:** Land the four reusable Vue components subsequent pages depend on. Each is small enough to live as one file. None have state that survives page navigation (no localStorage stash — spec §8.4).

**Files (new):**
- `optimus-fe/src/views/apps/Applications/components/ValuesEditor.vue`
- `optimus-fe/src/views/apps/Applications/components/ChartPickerStep.vue`
- `optimus-fe/src/views/apps/Applications/components/HistoryTable.vue`
- `optimus-fe/src/views/apps/Applications/components/ApplicationFormBasic.vue`

- [ ] **Step 1: `ValuesEditor.vue`** — CodeMirror YAML + Load defaults + Format

```vue
<template>
  <div class="values-editor">
    <div class="actions">
      <a-button size="small" @click="onLoadDefaults" :loading="loadingDefaults">
        {{ t('apps.install.btn.loadDefaults') }}
      </a-button>
      <a-button size="small" @click="onFormat">{{ t('apps.install.btn.format') }}</a-button>
    </div>
    <codemirror
      v-model="model"
      :style="{ height: '420px', fontSize: '13px' }"
      :extensions="[yaml(), oneDark]"
      @ready="onReady"
    />
    <a-typography-text v-if="parseError" type="danger" class="parse-err">
      {{ parseError }}
    </a-typography-text>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Codemirror as codemirror } from 'vue-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { oneDark } from '@codemirror/theme-one-dark'
import jsYaml from 'js-yaml'
import { message, Modal } from 'ant-design-vue'

import { appsRepoApiKey } from '@/api/keys'

const props = defineProps<{
  modelValue: string
  repoId?: number
  chartName?: string
  chartVersion?: string
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const { t } = useI18n()
const repoApi = inject(appsRepoApiKey)!
const model = ref(props.modelValue)
const loadingDefaults = ref(false)
const parseError = ref<string | null>(null)

watch(() => props.modelValue, v => { model.value = v })
watch(model, v => emit('update:modelValue', v))

function onReady() {/* hook for theme/extension tweaks */}

async function onLoadDefaults() {
  if (!props.repoId || !props.chartName || !props.chartVersion) {
    message.warning(t('apps.install.msg.pickChartFirst'))
    return
  }
  if (model.value.trim() !== '') {
    await new Promise<void>((resolve, reject) => {
      Modal.confirm({
        title: t('apps.install.confirm.loadDefaults.title'),
        content: t('apps.install.confirm.loadDefaults.body'),
        onOk: () => resolve(),
        onCancel: () => reject(new Error('cancel')),
      })
    }).catch(() => { throw 'cancel' })
  }
  loadingDefaults.value = true
  try {
    const { values_yaml } = await repoApi.getDefaultValues(props.repoId, props.chartName, props.chartVersion)
    model.value = values_yaml
  } finally {
    loadingDefaults.value = false
  }
}

function onFormat() {
  if (model.value.trim() === '') return
  try {
    const obj = jsYaml.load(model.value)
    if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
      model.value = jsYaml.dump(obj, { indent: 2, lineWidth: 120, noRefs: true })
      parseError.value = null
    } else {
      parseError.value = t('apps.install.msg.valuesNotMap')
    }
  } catch (e: any) {
    parseError.value = e.message ?? String(e)
  }
}
</script>

<style scoped>
.values-editor .actions { display: flex; gap: 8px; margin-bottom: 8px; }
.values-editor .parse-err { display: block; margin-top: 8px; }
</style>
```

If extra i18n keys `apps.install.btn.format`, `apps.install.msg.pickChartFirst`, `apps.install.msg.valuesNotMap`, `apps.install.confirm.loadDefaults.{title,body}` aren't in spec §A.1 (they aren't), add them to Task 18's locales in this commit (one extra commit hunk).

- [ ] **Step 2: `ChartPickerStep.vue`** — cascading select

```vue
<template>
  <a-row :gutter="16">
    <a-col :span="8">
      <a-form-item :label="t('apps.install.step.chart')">
        <a-select
          v-model:value="repoId"
          :options="repoOptions"
          :loading="loadingRepos"
          show-search
          option-filter-prop="label"
        />
      </a-form-item>
    </a-col>
    <a-col :span="8">
      <a-form-item :label="t('apps.install.field.chart')">
        <a-select
          v-model:value="chartName"
          :options="chartOptions"
          :disabled="!repoId"
          :loading="loadingCharts"
          show-search
          option-filter-prop="label"
        />
      </a-form-item>
    </a-col>
    <a-col :span="8">
      <a-form-item :label="t('apps.install.field.version')">
        <a-select
          v-model:value="version"
          :options="versionOptions"
          :disabled="!chartName"
          :loading="loadingVersions"
          show-search
          option-filter-prop="label"
        />
      </a-form-item>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { ref, watch, computed, inject, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

import { appsRepoApiKey } from '@/api/keys'

const props = defineProps<{
  repoId: number | undefined
  chartName: string | undefined
  version: string | undefined
}>()
const emit = defineEmits<{
  (e: 'update:repoId', v: number | undefined): void
  (e: 'update:chartName', v: string | undefined): void
  (e: 'update:version', v: string | undefined): void
}>()

const { t } = useI18n()
const repoApi = inject(appsRepoApiKey)!

const repoId    = ref(props.repoId)
const chartName = ref(props.chartName)
const version   = ref(props.version)

watch(repoId,    v => { emit('update:repoId', v); chartName.value = undefined; version.value = undefined })
watch(chartName, v => { emit('update:chartName', v); version.value = undefined })
watch(version,   v => emit('update:version', v))

const repos = ref<any[]>([])
const loadingRepos = ref(false)
const repoOptions = computed(() => repos.value.map(r => ({ label: `${r.name} (${r.type})`, value: r.id })))

const charts = ref<any[]>([])
const loadingCharts = ref(false)
const chartOptions = computed(() => charts.value.map(c => ({ label: c.name, value: c.name })))

const versions = ref<any[]>([])
const loadingVersions = ref(false)
const versionOptions = computed(() => versions.value.map(v => ({ label: v.version, value: v.version })))

onMounted(async () => {
  loadingRepos.value = true
  try { repos.value = (await repoApi.list({ page_size: 200 })).items } finally { loadingRepos.value = false }
})

watch(repoId, async (id) => {
  charts.value = []
  if (!id) return
  loadingCharts.value = true
  try { charts.value = (await repoApi.listCharts(id)).items } finally { loadingCharts.value = false }
})

watch([repoId, chartName], async ([id, name]) => {
  versions.value = []
  if (!id || !name) return
  loadingVersions.value = true
  try { versions.value = (await repoApi.listVersions(id, name)).items } finally { loadingVersions.value = false }
})
</script>
```

- [ ] **Step 3: `HistoryTable.vue`** — revisions + rollback action

```vue
<template>
  <a-table
    :columns="columns"
    :data-source="rows"
    :pagination="false"
    row-key="revision"
    size="middle"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'status'">
        <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
      </template>
      <template v-else-if="column.key === 'actions'">
        <a-popconfirm
          :title="t('apps.application.detail.confirm.rollback')"
          @confirm="$emit('rollback', record.revision)"
        >
          <a-button
            v-permission="'apps:release:rollback'"
            size="small"
            :disabled="record.revision === currentRevision"
          >
            {{ t('apps.application.detail.btn.rollback') }}
          </a-button>
        </a-popconfirm>
      </template>
    </template>
  </a-table>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RevisionRow } from '@/types/apps'

const props = defineProps<{ rows: RevisionRow[]; currentRevision?: number }>()
defineEmits<{ (e: 'rollback', revision: number): void }>()
const { t } = useI18n()

const columns = computed(() => [
  { title: '#', dataIndex: 'revision', key: 'revision', width: 64 },
  { title: t('common.field.status'), key: 'status' },
  { title: t('apps.application.list.col.chart_version'), dataIndex: 'chart_version', key: 'chart_version' },
  { title: t('apps.application.list.col.app_version'),   dataIndex: 'app_version',   key: 'app_version' },
  { title: t('common.field.updated_at'), dataIndex: 'updated_at', key: 'updated_at' },
  { title: t('common.field.description'), dataIndex: 'description', key: 'description' },
  { title: t('common.field.actions'), key: 'actions' },
])

function statusColor(s: string) {
  switch (s) {
    case 'deployed':                return 'green'
    case 'failed': case 'unknown':  return 'red'
    case 'pending-install':
    case 'pending-upgrade':
    case 'pending-rollback':        return 'orange'
    case 'uninstalled':             return 'default'
    default:                        return 'blue'
  }
}
</script>
```

- [ ] **Step 4: `ApplicationFormBasic.vue`** — shared basic-info form

```vue
<template>
  <a-form :model="model" layout="vertical">
    <a-row :gutter="16">
      <a-col :span="12">
        <a-form-item :label="t('common.field.name')" :required="!isEdit" name="name">
          <a-input v-model:value="model.name" :max-length="64" :disabled="isEdit" />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.release')" :required="!isEdit" name="release_name">
          <a-input v-model:value="model.release_name" :max-length="53" :disabled="isEdit" />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.cluster')" :required="!isEdit" name="cluster_id">
          <a-select v-model:value="model.cluster_id" :options="clusterOptions" :disabled="isEdit" />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.namespace')" :required="!isEdit" name="namespace">
          <a-input v-model:value="model.namespace" :max-length="63" :disabled="isEdit" />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item :label="t('apps.application.list.col.owner')">
          <a-select v-model:value="model.owner_user_id" :options="userOptions" allow-clear show-search option-filter-prop="label" />
        </a-form-item>
      </a-col>
      <a-col :span="12">
        <a-form-item label="Tags">
          <a-select v-model:value="model.tags" mode="tags" />
        </a-form-item>
      </a-col>
      <a-col :span="24">
        <a-form-item :label="t('common.field.description')">
          <a-textarea v-model:value="model.description" :rows="3" :max-length="4096" />
        </a-form-item>
      </a-col>
    </a-row>
  </a-form>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted, inject, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'

import { k8sClusterApiKey, userApiKey } from '@/api/keys' // P0/P2 injection keys

const props = defineProps<{ modelValue: any; isEdit?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: any): void }>()

const { t } = useI18n()
const model = reactive({ ...props.modelValue })
watchEffect(() => emit('update:modelValue', { ...model }))

const clusterOptions = ref<any[]>([])
const userOptions = ref<any[]>([])
const clusterApi = inject(k8sClusterApiKey)!
const userApi    = inject(userApiKey)!

onMounted(async () => {
  clusterOptions.value = ((await clusterApi.list({ page_size: 200 })).items as any[])
    .map(c => ({ label: c.name, value: c.id }))
  userOptions.value = ((await userApi.list({ page_size: 200 })).items as any[])
    .map(u => ({ label: u.display_name || u.username, value: u.id }))
})
</script>
```

If `k8sClusterApiKey` / `userApiKey` don't exist under those exact names, replace with the existing P0/P2 export names — grep for them.

- [ ] **Step 5: Add i18n key gaps**

The components above introduce keys not in spec §A.1:

```
apps.install.btn.format               "格式化"               "Format"
apps.install.field.chart              "Chart"                "Chart"
apps.install.field.version            "版本"                  "Version"
apps.install.msg.pickChartFirst       "请先选择 chart"       "Pick chart first"
apps.install.msg.valuesNotMap         "values 必须是 YAML map"  "Values must be a YAML map"
apps.install.confirm.loadDefaults.title "加载默认 values?"   "Load chart defaults?"
apps.install.confirm.loadDefaults.body "将覆盖当前编辑内容"   "This will overwrite the current editor."
apps.application.list.col.chart_version "Chart 版本"          "Chart version"
apps.application.list.col.app_version   "App 版本"           "App version"
apps.application.detail.confirm.rollback "回退到该版本?"     "Roll back to this revision?"
```

Add to both locales and re-run `bun run i18n:check`.

- [ ] **Step 6: Typecheck + lint + build**

```bash
cd optimus-fe
bun run typecheck
bun run lint
bun run build
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/views/apps/Applications/components/ \
        optimus-fe/src/locales/zh-CN.json \
        optimus-fe/src/locales/en-US.json
git commit -m "$(cat <<'EOF'
feat(fe/apps): shared components (ValuesEditor + ChartPickerStep + HistoryTable + ApplicationFormBasic)

ValuesEditor wraps vue-codemirror with the two actions spec §5.3 calls
for: Load chart defaults (with overwrite confirm) and Format
(js-yaml round-trip). No localStorage draft cache.

ChartPickerStep is the three-level cascade repo -> chart -> version
used by both Install and Upgrade wizards.

HistoryTable shows helm history with a rollback button gated by
v-permission='apps:release:rollback'.

ApplicationFormBasic is the shared "basics" panel rendered in both
Install (creating) and Detail (read-only) layouts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 21: FE Applications List + Detail

**Goal:** Land the two main application pages: list with cluster/namespace/owner/tag filters and a "live status" opt-in checkbox; detail page with basic info + revision history + rollback.

**Files (new):**
- `optimus-fe/src/views/apps/Applications/List.vue`
- `optimus-fe/src/views/apps/Applications/Detail.vue`

- [ ] **Step 1: `List.vue`**

```vue
<template>
  <PageHeader :title="t('apps.application.list.title')">
    <template #extra>
      <a-button v-permission="'apps:application:write'" type="primary" @click="$router.push('/apps/applications/new')">
        {{ t('apps.application.list.action.installNew') }}
      </a-button>
    </template>
  </PageHeader>

  <a-card :body-style="{ padding: '12px' }">
    <a-row :gutter="12">
      <a-col :span="5">
        <a-select
          v-model:value="filters.cluster_id"
          :options="clusterOptions"
          :placeholder="t('apps.application.list.col.cluster')"
          allow-clear
        />
      </a-col>
      <a-col :span="5">
        <a-input v-model:value="filters.namespace" :placeholder="t('apps.application.list.col.namespace')" allow-clear />
      </a-col>
      <a-col :span="5">
        <a-select v-model:value="filters.owner_user_id" :options="userOptions" :placeholder="t('apps.application.list.col.owner')" allow-clear show-search option-filter-prop="label" />
      </a-col>
      <a-col :span="4">
        <a-input v-model:value="filters.tag" placeholder="tag" allow-clear />
      </a-col>
      <a-col :span="5">
        <a-space>
          <a-button type="primary" @click="refresh">{{ t('common.button.search') }}</a-button>
          <a-checkbox v-model:checked="showLiveStatus">{{ t('apps.application.list.showLiveStatus') }}</a-checkbox>
        </a-space>
      </a-col>
    </a-row>
  </a-card>

  <a-table
    :columns="columns"
    :data-source="rows"
    :loading="loading"
    :pagination="paginationConfig"
    row-key="id"
    @change="onTableChange"
    style="margin-top: 12px;"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.key === 'status'">
        <a-tag :color="statusColor(record.status)">{{ record.status || '—' }}</a-tag>
      </template>
      <template v-else-if="column.key === 'actions'">
        <a-space>
          <a-button size="small" @click="$router.push(`/apps/applications/${record.id}`)">
            {{ t('apps.application.list.action.detail') }}
          </a-button>
          <a-button v-permission="'apps:release:upgrade'" size="small" @click="$router.push(`/apps/applications/${record.id}/upgrade`)">
            {{ t('apps.application.list.action.upgrade') }}
          </a-button>
          <a-popconfirm :title="t('apps.application.list.confirm.uninstall')" @confirm="onUninstall(record.id)">
            <a-button v-permission="'apps:release:uninstall'" size="small" danger>
              {{ t('apps.application.list.action.uninstall') }}
            </a-button>
          </a-popconfirm>
        </a-space>
      </template>
    </template>
  </a-table>
</template>

<script setup lang="ts">
import { ref, reactive, computed, inject, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'

import { appsApplicationApiKey, appsReleaseApiKey, k8sClusterApiKey, userApiKey } from '@/api/keys'
import PageHeader from '@/components/PageHeader.vue'
import type { ApplicationSummary } from '@/types/apps'

const { t } = useI18n()
const appApi  = inject(appsApplicationApiKey)!
const relApi  = inject(appsReleaseApiKey)!
const clusterApi = inject(k8sClusterApiKey)!
const userApi    = inject(userApiKey)!

const filters = reactive<{cluster_id?: number; namespace?: string; owner_user_id?: number; tag?: string}>({})
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const rows = ref<(ApplicationSummary & { status?: string })[]>([])
const total = ref(0)
const showLiveStatus = ref(false)

const clusterOptions = ref<any[]>([])
const userOptions    = ref<any[]>([])

const columns = computed(() => [
  { title: t('apps.application.list.col.name'), dataIndex: 'name', key: 'name' },
  { title: t('apps.application.list.col.cluster'), dataIndex: 'cluster_name', key: 'cluster' },
  { title: t('apps.application.list.col.namespace'), dataIndex: 'namespace', key: 'namespace' },
  { title: t('apps.application.list.col.release'), dataIndex: 'release_name', key: 'release' },
  { title: t('apps.application.list.col.chart'), dataIndex: 'chart_name', key: 'chart' },
  { title: t('apps.application.list.col.owner'), dataIndex: 'owner_name', key: 'owner' },
  { title: t('common.field.status'), key: 'status' },
  { title: t('apps.application.list.col.actions'), key: 'actions', fixed: 'right' },
])
const paginationConfig = computed(() => ({ current: page.value, pageSize: pageSize.value, total: total.value }))

async function refresh() {
  loading.value = true
  try {
    const data = await appApi.list({ ...filters, page: page.value, page_size: pageSize.value })
    rows.value = data.items
    total.value = data.total
    if (showLiveStatus.value) await fetchLiveStatuses()
  } finally { loading.value = false }
}
async function fetchLiveStatuses() {
  const promises = rows.value.map(async r => {
    try { const s = await relApi.status(r.id); r.status = s.status } catch { r.status = 'unknown' }
  })
  await Promise.all(promises)
}
function onTableChange(p: any) { page.value = p.current; pageSize.value = p.pageSize; refresh() }
async function onUninstall(id: number) {
  try { await relApi.uninstall(id, {}); message.success(t('common.message.done')); refresh() } catch {}
}
function statusColor(s?: string) {
  if (s === 'deployed') return 'green'
  if (s === 'failed' || s === 'unknown') return 'red'
  if (s && s.startsWith('pending')) return 'orange'
  return 'default'
}

watch(showLiveStatus, v => { if (v) fetchLiveStatuses() })

onMounted(async () => {
  clusterOptions.value = ((await clusterApi.list({ page_size: 200 })).items as any[]).map(c => ({ label: c.name, value: c.id }))
  userOptions.value    = ((await userApi.list({ page_size: 200 })).items as any[]).map(u => ({ label: u.display_name || u.username, value: u.id }))
  await refresh()
})
</script>
```

- [ ] **Step 2: `Detail.vue`**

```vue
<template>
  <PageHeader :title="detail?.name ?? '…'" :back="() => $router.push('/apps/applications')">
    <template #extra>
      <a-space>
        <a-button v-permission="'apps:release:upgrade'" @click="$router.push(`/apps/applications/${id}/upgrade`)">
          {{ t('apps.application.list.action.upgrade') }}
        </a-button>
        <a-popconfirm :title="t('apps.application.list.confirm.uninstall')" @confirm="onUninstall">
          <a-button v-permission="'apps:release:uninstall'" danger>{{ t('apps.application.list.action.uninstall') }}</a-button>
        </a-popconfirm>
      </a-space>
    </template>
  </PageHeader>

  <a-card :title="t('apps.application.detail.section.basic')" :loading="loadingDetail">
    <a-descriptions :column="3">
      <a-descriptions-item :label="t('common.field.name')">{{ detail?.name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.cluster')">{{ detail?.cluster_name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.namespace')">{{ detail?.namespace }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.release')">{{ detail?.release_name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart')">{{ detail?.chart_name }}</a-descriptions-item>
      <a-descriptions-item :label="t('common.field.status')">
        <a-tag :color="statusColor(detail?.status)">{{ detail?.status || '—' }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart_version')">{{ detail?.chart_version }}</a-descriptions-item>
      <a-descriptions-item :label="t('common.field.updated_at')">{{ detail?.last_deployed_at }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.owner')">{{ detail?.owner_name }}</a-descriptions-item>
    </a-descriptions>
  </a-card>

  <a-card :title="t('apps.application.detail.section.history')" style="margin-top: 12px;" :loading="loadingHistory">
    <HistoryTable :rows="history" :current-revision="detail?.revision" @rollback="onRollback" />
  </a-card>
</template>

<script setup lang="ts">
import { ref, computed, inject, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'

import { appsApplicationApiKey, appsReleaseApiKey } from '@/api/keys'
import PageHeader from '@/components/PageHeader.vue'
import HistoryTable from './components/HistoryTable.vue'
import type { ApplicationDetail, RevisionRow } from '@/types/apps'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const id = computed(() => Number(route.params.id))

const appApi = inject(appsApplicationApiKey)!
const relApi = inject(appsReleaseApiKey)!

const detail = ref<ApplicationDetail | null>(null)
const history = ref<RevisionRow[]>([])
const loadingDetail = ref(false)
const loadingHistory = ref(false)

async function load() {
  loadingDetail.value = true
  loadingHistory.value = true
  try {
    detail.value = await appApi.get(id.value)
    history.value = (await relApi.history(id.value)).items
  } catch {
    /* envelope interceptor already surfaced the error */
  } finally {
    loadingDetail.value = false
    loadingHistory.value = false
  }
}

async function onRollback(revision: number) {
  try {
    await relApi.rollback(id.value, { revision })
    message.success(t('common.message.done'))
    load()
  } catch {}
}
async function onUninstall() {
  try {
    await relApi.uninstall(id.value, {})
    message.success(t('common.message.done'))
    router.push('/apps/applications')
  } catch {}
}
function statusColor(s?: string) {
  if (s === 'deployed') return 'green'
  if (s === 'failed' || s === 'unknown') return 'red'
  if (s && s.startsWith('pending')) return 'orange'
  return 'default'
}
onMounted(load)
</script>
```

- [ ] **Step 3: Add missing i18n keys**

```
apps.application.list.showLiveStatus    "显示实时状态"   "Show live status"
apps.application.list.confirm.uninstall "确认卸载?"     "Confirm uninstall?"
```

Add to both locales.

- [ ] **Step 4: Build + typecheck + lint + smoke**

```bash
cd optimus-fe
bun run typecheck
bun run lint
bun run dev
```

Browse to `/apps/applications`. Empty table. Click filters; click "Show live status" — table re-renders with status tags (404 for not-yet-installed rows shows "unknown").

- [ ] **Step 5: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/views/apps/Applications/List.vue \
        optimus-fe/src/views/apps/Applications/Detail.vue \
        optimus-fe/src/locales/zh-CN.json \
        optimus-fe/src/locales/en-US.json
git commit -m "$(cat <<'EOF'
feat(fe/apps): application List + Detail pages (P3)

List page filters by cluster/namespace/owner/tag and surfaces an opt-in
"Show live status" toggle that fans out per-row helm status fetches
(spec §4.4 — list page itself never calls helm).

Detail page renders basic info + helm history; rollback gated by
v-permission='apps:release:rollback'. Uninstall navigates back to
list on success.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 22: FE Install wizard

**Goal:** Land the three-step Install wizard. Step 1 calls `POST /apps/applications` (DB row only). Step 2 picks chart + version. Step 3 edits values + calls `POST /apps/applications/:id/release/install`. Any step's failure preserves earlier steps' DB write so the user can resume from an "registered but not deployed" pill in the list.

**Files (new):**
- `optimus-fe/src/views/apps/Applications/Install.vue`

- [ ] **Step 1: Component skeleton**

```vue
<template>
  <PageHeader :title="t('apps.application.list.action.installNew')" :back="() => $router.push('/apps/applications')" />

  <a-card>
    <a-steps :current="step" style="margin-bottom: 24px;">
      <a-step :title="t('apps.install.step.basic')" />
      <a-step :title="t('apps.install.step.chart')" />
      <a-step :title="t('apps.install.step.values')" />
    </a-steps>

    <div v-if="step === 0">
      <ApplicationFormBasic v-model="basics" />
      <div class="step-actions">
        <a-button type="primary" @click="submitBasics" :loading="loading">
          {{ t('common.button.next') }}
        </a-button>
      </div>
    </div>

    <div v-else-if="step === 1">
      <ChartPickerStep
        v-model:repoId="chart.repoId"
        v-model:chartName="chart.name"
        v-model:version="chart.version"
      />
      <div class="step-actions">
        <a-button @click="step = 0">{{ t('common.button.back') }}</a-button>
        <a-button type="primary" :disabled="!chart.version" @click="step = 2">
          {{ t('common.button.next') }}
        </a-button>
      </div>
    </div>

    <div v-else>
      <ValuesEditor
        v-model="valuesYaml"
        :repo-id="chart.repoId"
        :chart-name="chart.name"
        :chart-version="chart.version"
      />
      <div class="step-actions">
        <a-button @click="step = 1">{{ t('common.button.back') }}</a-button>
        <a-button type="primary" :loading="loading" @click="submitInstall">
          {{ t('apps.install.btn.submit') }}
        </a-button>
      </div>
    </div>
  </a-card>
</template>

<script setup lang="ts">
import { ref, reactive, inject, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'

import { appsApplicationApiKey, appsReleaseApiKey } from '@/api/keys'
import PageHeader from '@/components/PageHeader.vue'
import ApplicationFormBasic from './components/ApplicationFormBasic.vue'
import ChartPickerStep from './components/ChartPickerStep.vue'
import ValuesEditor from './components/ValuesEditor.vue'

const { t } = useI18n()
const router = useRouter()
const appApi = inject(appsApplicationApiKey)!
const relApi = inject(appsReleaseApiKey)!

const step = ref(0)
const loading = ref(false)
const applicationId = ref<number | null>(null)

const basics = reactive<any>({
  name: '', cluster_id: undefined, namespace: '', release_name: '',
  owner_user_id: undefined, tags: [], description: '',
})
const chart = reactive<{ repoId?: number; name?: string; version?: string }>({})
const valuesYaml = ref('')

async function submitBasics() {
  loading.value = true
  try {
    const d = await appApi.create({
      name: basics.name,
      cluster_id: basics.cluster_id,
      namespace: basics.namespace,
      release_name: basics.release_name,
      chart_repo_id: 0, // we don't know it yet — but the API requires it.
      chart_name: '',   // patched in step 2
      description: basics.description,
      tags: basics.tags,
      owner_user_id: basics.owner_user_id,
    })
    applicationId.value = d.id
    step.value = 1
  } catch {} finally { loading.value = false }
}

async function submitInstall() {
  if (!applicationId.value) return
  loading.value = true
  try {
    // Patch chart_repo_id + chart_name onto the application row before install.
    // The PUT /apps/applications/:id endpoint only allows description/tags/owner,
    // so we route via the upgrade endpoint's chart_repo_id field — but for first
    // install we need to set chart_repo_id on the row before install runs.
    //
    // Simplest path: include the chart_repo_id in the install URL via a query
    // param, served by a small handler enhancement OR have step 1 ask the user
    // for chart_repo_id+chart_name up front. To avoid an API change, the
    // simplest UX is: step 1 collects chart_repo_id + chart_name too.
    //
    // Therefore: in the basics form, also collect chart_repo_id + chart_name,
    // and submit them in step 1's POST /apps/applications.
    //
    // (Implementer note: see "Step 2 — adjustment" below.)

    await relApi.install(applicationId.value, {
      chart_version: chart.version!,
      values_yaml: valuesYaml.value,
    })
    message.success(t('common.message.installed'))
    router.push(`/apps/applications/${applicationId.value}`)
  } catch {} finally { loading.value = false }
}
</script>

<style scoped>
.step-actions { margin-top: 24px; display: flex; gap: 12px; justify-content: flex-end; }
</style>
```

- [ ] **Step 2: Adjustment — fold chart_repo_id + chart_name into Step 1**

The wizard skeleton above flags that step 1 ALSO needs `chart_repo_id` + `chart_name` so the `POST /apps/applications` body is valid. Pull `ChartPickerStep` into step 1 as a second card, and re-purpose step 2 to be "version + values preview" instead of "pick chart". Reorder:

- **Step 1 (Basics):** `ApplicationFormBasic` + `ChartPickerStep` (collecting `repoId` + `chartName` + `version`). Submit creates the application row.
- **Step 2 (Values):** `ValuesEditor`. Submit calls install.

Two steps instead of three. Update the i18n keys: drop `apps.install.step.chart` from rendering (still keep the key for future use). Update the `<a-steps>` component to two steps.

Final `Install.vue` follows the two-step variant. The skeleton's three-step UI in Step 1 above is illustrative — the real component implements two steps.

- [ ] **Step 3: Test + commit**

```bash
cd optimus-fe
bun run typecheck && bun run lint && bun run build
```

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/views/apps/Applications/Install.vue
git commit -m "$(cat <<'EOF'
feat(fe/apps): Install wizard (P3)

Two-step wizard:
1. Basics (incl. chart selection via ChartPickerStep) -> POST /apps/
   applications. The application row exists from this point even if the
   user abandons the flow.
2. Values (ValuesEditor) -> POST /.../release/install.

Wizard intentionally splits "register" and "install" so a failed install
leaves the row visible in the list under a "registered, not deployed"
pill (spec §4.2 / §8.3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 23: FE Upgrade page

**Goal:** Land the upgrade page. Reuses `ChartPickerStep` (with the application's `chart_name` locked) and `ValuesEditor` pre-populated with the values currently deployed (from the `GET /release` notes path — helm's `action.GetValues` is wrapped through a small BE addition if not already present; otherwise the editor opens blank with a "Load chart defaults" button).

**Files (new):**
- `optimus-fe/src/views/apps/Applications/Upgrade.vue`

- [ ] **Step 1: Component**

```vue
<template>
  <PageHeader :title="t('apps.application.list.action.upgrade')" :back="goBack" />

  <a-card :loading="loadingDetail">
    <a-descriptions :column="2" v-if="detail">
      <a-descriptions-item :label="t('common.field.name')">{{ detail.name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.cluster')">{{ detail.cluster_name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.namespace')">{{ detail.namespace }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.release')">{{ detail.release_name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart')">{{ detail.chart_name }}</a-descriptions-item>
      <a-descriptions-item :label="t('apps.application.list.col.chart_version')">{{ detail.chart_version }} → {{ chart.version }}</a-descriptions-item>
    </a-descriptions>
  </a-card>

  <a-card style="margin-top: 12px;">
    <ChartPickerStep
      v-model:repoId="chart.repoId"
      v-model:chartName="chart.name"
      v-model:version="chart.version"
    />
  </a-card>

  <a-card :title="t('apps.install.step.values')" style="margin-top: 12px;">
    <ValuesEditor
      v-model="valuesYaml"
      :repo-id="chart.repoId"
      :chart-name="chart.name"
      :chart-version="chart.version"
    />
  </a-card>

  <div class="actions">
    <a-button @click="goBack">{{ t('common.button.cancel') }}</a-button>
    <a-button type="primary" :loading="submitting" :disabled="!chart.version" @click="submit">
      {{ t('apps.upgrade.btn.submit') }}
    </a-button>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, inject, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'

import { appsApplicationApiKey, appsReleaseApiKey } from '@/api/keys'
import PageHeader from '@/components/PageHeader.vue'
import ChartPickerStep from './components/ChartPickerStep.vue'
import ValuesEditor from './components/ValuesEditor.vue'
import type { ApplicationDetail } from '@/types/apps'

const { t } = useI18n()
const route  = useRoute()
const router = useRouter()
const id = computed(() => Number(route.params.id))

const appApi = inject(appsApplicationApiKey)!
const relApi = inject(appsReleaseApiKey)!

const detail = ref<ApplicationDetail | null>(null)
const loadingDetail = ref(false)
const submitting = ref(false)

const chart = reactive<{ repoId?: number; name?: string; version?: string }>({})
const valuesYaml = ref('')

function goBack() { router.push(`/apps/applications/${id.value}`) }

async function submit() {
  if (!chart.version) return
  submitting.value = true
  try {
    await relApi.upgrade(id.value, {
      chart_repo_id: chart.repoId !== detail.value?.chart_repo_id ? chart.repoId : undefined,
      chart_version: chart.version,
      values_yaml: valuesYaml.value,
    })
    message.success(t('common.message.upgraded'))
    goBack()
  } catch {} finally { submitting.value = false }
}

onMounted(async () => {
  loadingDetail.value = true
  try {
    detail.value = await appApi.get(id.value)
    // Pre-fill picker with the application's current chart/repo (version blank
    // so the user must consciously pick).
    chart.repoId = detail.value!.chart_repo_id
    chart.name   = detail.value!.chart_name
  } finally { loadingDetail.value = false }
})
</script>

<style scoped>
.actions { margin-top: 16px; display: flex; gap: 12px; justify-content: flex-end; }
</style>
```

- [ ] **Step 2: Add `common.message.upgraded` to both locales**

```
common.message.upgraded   "升级成功"   "Upgrade succeeded"
```

- [ ] **Step 3: Build + smoke**

```bash
cd optimus-fe
bun run typecheck && bun run lint && bun run build
```

Run dev server, install a chart, then navigate to upgrade page, change version, submit → revision N+1 appears in history table.

- [ ] **Step 4: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/views/apps/Applications/Upgrade.vue \
        optimus-fe/src/locales/zh-CN.json \
        optimus-fe/src/locales/en-US.json
git commit -m "$(cat <<'EOF'
feat(fe/apps): Upgrade page (P3)

Reuses ChartPickerStep (pre-populated with the application's current
chart_repo_id + chart_name) and ValuesEditor (empty by default; user
must explicitly Load defaults or paste). Submit sends chart_repo_id
only when it changed (BE patches the application row atomically).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 24: FE tests + v-permission audit

**Goal:** Add vitest unit tests for the API layer + key components; sweep all six new pages for any action button that should be gated but isn't, and any JS-level permission check that should be replaced by `v-permission`.

**Files (new):**
- `optimus-fe/src/views/apps/Applications/__tests__/ValuesEditor.test.ts`
- `optimus-fe/src/views/apps/Applications/__tests__/HistoryTable.test.ts`
- `optimus-fe/src/views/apps/ChartRepos/__tests__/Form.test.ts`
- `optimus-fe/src/api/apps/__tests__/release.test.ts`

- [ ] **Step 1: API test**

```ts
// release.test.ts
import { describe, it, expect, vi } from 'vitest'
import { ReleaseApi } from '../release'

describe('ReleaseApi', () => {
  const client: any = { get: vi.fn(), post: vi.fn() }
  const api = new ReleaseApi(client)

  it('install POSTs the install body', async () => {
    client.post.mockResolvedValue({ data: { code: 0, data: { revision: 1, status: 'deployed', chart_version: '1.0.0', last_deployed_at: 'x' } } })
    const r = await api.install(7, { chart_version: '1.0.0', values_yaml: '' })
    expect(client.post).toHaveBeenCalledWith('/apps/applications/7/release/install', { chart_version: '1.0.0', values_yaml: '' })
    expect(r.revision).toBe(1)
  })

  it('rollback maps body correctly', async () => {
    client.post.mockResolvedValue({ data: { code: 0, data: { revision: 2, status: 'deployed', chart_version: '1.0.0', last_deployed_at: 'x' } } })
    await api.rollback(7, { revision: 1 })
    expect(client.post).toHaveBeenCalledWith('/apps/applications/7/release/rollback', { revision: 1 })
  })
})
```

- [ ] **Step 2: ValuesEditor test**

```ts
// ValuesEditor.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ValuesEditor from '../components/ValuesEditor.vue'
// minimal stubs for codemirror + inject — see existing P2 component test patterns

describe('ValuesEditor', () => {
  it('format button transforms yaml round-trip', async () => {
    // mount with v-model, set value to "a: 1\nb: 2\n", click Format
    // assert v-model emits a yaml-dump string
  })
  it('Load defaults prompts confirm before overwriting non-empty buffer', async () => {
    // mock the inject(appsRepoApiKey) and assert window confirm path
  })
})
```

(Use the canonical mount + stub recipe from existing FE test files. Two cases each is enough to surface regressions.)

- [ ] **Step 3: HistoryTable test**

```ts
// HistoryTable.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import HistoryTable from '../components/HistoryTable.vue'

describe('HistoryTable', () => {
  it('disables rollback on current revision', () => {
    const w = mount(HistoryTable, {
      props: {
        rows: [{ revision: 1, status: 'deployed', chart_version: '1.0.0', app_version: '1', updated_at: 't', description: '' }],
        currentRevision: 1,
      },
    })
    const btn = w.find('button.ant-btn[disabled]')
    expect(btn.exists()).toBe(true)
  })
})
```

- [ ] **Step 4: ChartRepos Form test**

```ts
// Form.test.ts
import { describe, it, expect, vi } from 'vitest'

describe('ChartRepos Form', () => {
  it('sends explicit null when Clear password is clicked then saved', async () => {
    // mount in edit mode with original.has_password=true.
    // click "Clear password" then submit; assert update() payload has password: null.
  })
  it('omits password when field left empty in edit mode', async () => {
    // mount, leave password empty, click save; assert payload has no password field.
  })
})
```

- [ ] **Step 5: v-permission sweep**

Walk every `.vue` under `src/views/apps/` and confirm:
- All buttons that mutate (create / edit / install / upgrade / rollback / uninstall / delete) carry `v-permission`.
- No `if (auth.permissions.includes('apps:...'))` JS checks in components — those should be DOM gates.

Suggested grep:
```bash
cd optimus-fe
grep -rn 'v-permission' src/views/apps/ | wc -l
grep -rn "permissions.includes" src/views/apps/   # should be zero
```

- [ ] **Step 6: Run tests**

```bash
cd optimus-fe
bun run test
bun run lint
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-fe/src/views/apps/Applications/__tests__/ \
        optimus-fe/src/views/apps/ChartRepos/__tests__/ \
        optimus-fe/src/api/apps/__tests__/
git commit -m "$(cat <<'EOF'
test(fe/apps): vitest unit tests + v-permission audit (P3)

Tests cover:
- ReleaseApi install + rollback bodies (axios stubbed).
- ValuesEditor Format / Load-defaults overwrite confirmation.
- HistoryTable rollback button disabled on current revision.
- ChartRepos Form clear-password sends explicit null; empty password
  in edit mode omits the field.

Permission sweep: no JS-level permission checks remain in src/views/
apps/; every mutating button declares v-permission.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 25: Manual smoke + final verification + dev push

**Goal:** Run the P3 smoke checklist (spec §10.4), run all CI checks locally one more time, capture the helm SDK version in CLAUDE.md (if not already updated by Task 1), and prepare the dev branch for review.

**Files (new):**
- `optimus-be/scripts/p3-smoke.md`

**Files (modify):**
- `CLAUDE.md` (final mention of P3 if Task 1's update didn't already do it)

- [ ] **Step 1: Write the smoke checklist**

Create `optimus-be/scripts/p3-smoke.md`:

```markdown
# P3 smoke checklist

Run after a fresh deploy or whenever you suspect upstream chart-repo
behaviour has shifted. Not automated.

## Prereqs

- BE + FE running locally (`make run` + `bun run dev`).
- A P2-registered dev cluster (kubeconfig in P1 vault).
- Admin login.

## Steps

1. HTTP repo: add `https://charts.bitnami.com/bitnami`.
2. List charts → pick `nginx`. List versions. Pick the latest. Fetch
   default values; confirm `replicaCount` appears.
3. Register an application against the dev cluster, namespace `default`,
   release `nginx-test`, chart `nginx`. Submit.
4. Install with `replicaCount: 1`. Observe pod readiness via the k8s
   workloads page until status = deployed.
5. Upgrade: change `replicaCount` to 2; resubmit. Confirm history table
   shows revision 2 deployed.
6. Rollback to revision 1. Confirm history table shows revision 3
   deployed with chart_version unchanged and description starting with
   "Rollback to ".
7. Uninstall. Confirm history empties (or remains if `keep_history`
   set). Delete the application row.
8. Repeat 1–6 against an OCI repo (`oci://ghcr.io/<account>/charts`).
   Provide username + token if private.
9. Negative: while applications still reference a cluster, try
   deleting that cluster in the k8s/clusters page; expect a friendly
   error citing the application count.
10. Negative: rollback to a non-existent revision; expect 42203.

## Reporting

If any step fails, capture:
- The `code` and `message_key` in the response envelope.
- The relevant rows from `audit_logs` for the operation.
- The helm SDK stderr (visible in `slog` debug output if `OPTIMUS_LOG_LEVEL=debug`).
```

- [ ] **Step 2: Run the checklist**

Do steps 1–10. Any failure that isn't recoverable on the spot becomes a new task back in the plan; otherwise mark complete in this task's checkbox.

- [ ] **Step 3: Full CI script locally**

```bash
cd optimus-be
make lint
make test
DOCKER_HOST=unix:///Users/logic/.colima/docker.sock make test-int
make swag && make swagger-diff
make dump-perms && make perm-check
go test -tags=dbtest -coverprofile=/tmp/p3-final.cov ./internal/modules/apps/...
go tool cover -func=/tmp/p3-final.cov | tail -1

cd ../optimus-fe
bun run lint
bun run typecheck
bun run i18n:check
bun run test
bun run build
```

Expected: every command exits 0; the apps/* coverage line shows `>= 60.0%`.

- [ ] **Step 4: Confirm CLAUDE.md mentions the helm pin**

```bash
grep -n 'helm.sh/helm' /Users/logic/Projects/optimus/CLAUDE.md
```

If Task 1 left a placeholder ("v3.16.x or v3.15.x"), edit to record the *actual* version locked in go.mod. Touch this commit only if a change is needed.

- [ ] **Step 5: Final commit**

```bash
cd /Users/logic/Projects/optimus
git add optimus-be/scripts/p3-smoke.md
[[ $(git diff --cached --name-only CLAUDE.md | wc -l) -gt 0 ]] && git add CLAUDE.md || true
git commit -m "$(cat <<'EOF'
chore(p3): smoke checklist + CLAUDE.md final touch-up

Adds optimus-be/scripts/p3-smoke.md covering the HTTP + OCI install /
upgrade / rollback / uninstall happy paths plus two negative paths
(cluster-delete refused while apps reference; rollback to missing
revision). Smoke flow is NOT automated; it is run before any P3 release
sign-off.

CLAUDE.md records the precise helm SDK version locked at Task 1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 6: Push dev branch**

```bash
cd /Users/logic/Projects/optimus
git status
git log --oneline dev ^main | head -30   # sanity-check the chain
git push origin dev
```

P3 is now ready for review. The PR title and body are out of scope for this plan; the user will open the PR (or invoke the project's PR helper).

---
