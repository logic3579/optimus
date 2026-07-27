# P6 Application Delivery Orchestration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a durable, approval-gated environment-promotion service that upgrades existing P3 Helm releases to one verified chart digest without introducing a generic runner or direct P6 credential access.

**Architecture:** A new `delivery` backend module persists immutable pipeline versions, runs, stage snapshots, approvals, leases, and structured events in PostgreSQL. A bounded in-process worker advances a transactional state machine and delegates digest-verified, reuse-values upgrades to new narrow P3 seams; Vue pages expose project configuration, runs, live events, and approvals.

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL, Helm SDK v3.15.4, Vue 3, TypeScript, Pinia, Ant Design Vue, authenticated SSE, Vitest, bun.

---

## Implementation constraints

- Read `docs/superpowers/specs/2026-07-27-p6-application-delivery-design.md` before Task 1.
- Keep Go at 1.25, Kubernetes modules at v0.30.14, and Helm at v3.15.4.
- Use `credentials.Consumer` only inside P3's existing Helm factory path.
- Do not import another module's private repository or query its table.
- Do not add shell, command, script, arbitrary container, arbitrary URL, arbitrary environment variable, values YAML, manifest, Helm notes, or raw upstream error fields.
- Run backend commands from `optimus-be/` and frontend commands from `optimus-fe/`.
- Use bun only for frontend work.
- Keep all code comments in English.

## File map

### Backend domain and persistence

- `optimus-be/migrations/00023_p6_delivery.sql`: all P6 tables, checks, foreign keys, partial indexes, and down migration.
- `optimus-be/internal/models/delivery.go`: GORM models and closed state/executor constants.
- `optimus-be/internal/modules/delivery/errs/codes.go`: P6 aliases and message keys for the `45xxx` range.
- `optimus-be/internal/modules/delivery/project/{dto,repo,service,handler}.go`: projects and environment bindings.
- `optimus-be/internal/modules/delivery/pipeline/{dto,repo,service,handler}.go`: immutable pipeline publication and reads.
- `optimus-be/internal/modules/delivery/run/{dto,repo,service,handler,state}.go`: run creation, snapshots, commands, and transition rules.
- `optimus-be/internal/modules/delivery/approval/{dto,repo,service,handler}.go`: pending approvals and immutable decisions.
- `optimus-be/internal/modules/delivery/event/{repo,service,handler}.go`: append-only events, SSE, and pruning.
- `optimus-be/internal/modules/delivery/orchestrator/{worker,reconciler}.go`: leases, stage advancement, execution, and recovery.
- `optimus-be/internal/modules/delivery/module/wire.go`: construction, routes, lifecycle, and exported P3 policy seams.

### P3 extensions

- `optimus-be/internal/modules/apps/repo/artifact.go`: chart archive digest resolution and verified loading.
- `optimus-be/internal/modules/apps/release/delivery.go`: safe application/artifact/release seam and reuse-values upgrade.
- `optimus-be/internal/modules/apps/release/coordinator.go`: database-backed per-application operation lease/idempotency.
- `optimus-be/internal/modules/apps/release/governance.go`: delivery-managed mutation policy.
- `optimus-be/internal/modules/apps/application/service.go`: delivery in-use deletion pre-check.
- `optimus-be/internal/modules/apps/module/module.go`: expose adapters and accept injected policy.

### Composition, platform, and generated artifacts

- `optimus-be/internal/infra/config/config.go`, `optimus-be/configs/config.yaml`: bounded delivery worker/SSE/retention settings.
- `optimus-be/internal/infra/errors/codes.go`: P6 numeric errors.
- `optimus-be/internal/infra/permissions/codes.go`: ten P6 permissions.
- `optimus-be/internal/seed/seed.go`: delivery menu only; no automatic `k8s_operator` grants.
- `optimus-be/cmd/server/main.go`: seam wiring and graceful worker lifecycle.
- `optimus-be/scripts/p6-smoke.md`: disposable local release-promotion checklist.
- `docs/api/swagger.json`, `optimus-be/api/docs/*`, `docs/permissions.md`: regenerated artifacts.

### Frontend

- `optimus-fe/src/types/delivery.ts`: API contracts and closed states.
- `optimus-fe/src/api/delivery/{project,pipeline,run,approval}.ts`: HTTP factories.
- `optimus-fe/src/api/delivery/events.ts`: authenticated SSE parser and cursor resume.
- `optimus-fe/src/stores/delivery.ts`: run-detail abort generation, event merge, and reset.
- `optimus-fe/src/views/delivery/projects/List.vue`: project list and create/edit/delete controls.
- `optimus-fe/src/views/delivery/projects/Detail.vue`: environment, pipeline, artifact, and history tabs.
- `optimus-fe/src/views/delivery/projects/components/{EnvironmentForm,PipelineForm,RunForm}.vue`: typed forms.
- `optimus-fe/src/views/delivery/runs/Detail.vue`: stage timeline and recovery commands.
- `optimus-fe/src/views/delivery/approvals/List.vue`: approval queue and decisions.
- `optimus-fe/src/main.ts`, locale JSON files, and seed-backed dynamic menus: registration and parity.

### Task 1: Add the delivery schema and models

**Files:**
- Create: `optimus-be/migrations/00023_p6_delivery.sql`
- Create: `optimus-be/internal/models/delivery.go`
- Modify: `optimus-be/migrations/embed_test.go`
- Test: `optimus-be/tests/integration/delivery_schema_test.go`

- [ ] **Step 1: Write the failing schema integration test**

Create a `dbtest` test that migrates a real PostgreSQL database, inserts one project, environment, pipeline, run, stage, approval, and event, then asserts duplicate active application binding and duplicate active project run both fail. Use constants `models.DeliveryRunQueued` and `models.DeliveryExecutorHelmUpgradeExistingRelease` so the model file is required to compile.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `go test -tags=dbtest ./tests/integration -run TestDeliverySchemaConstraints -race -count=1`  
Expected: FAIL because `models.DeliveryProject` and migration 00023 do not exist.

- [ ] **Step 3: Add the migration and model types**

Define these tables in dependency order:

```sql
delivery_projects
delivery_environments
delivery_pipelines
delivery_pipeline_stages
delivery_runs
delivery_run_stages
delivery_approvals
delivery_run_events
apps_release_operations
```

Use `BIGSERIAL`, `TIMESTAMPTZ`, explicit checks for every closed state, and partial unique indexes for active project names, active application bindings, current pipeline version, active project runs, and active application operations. `apps_release_operations` belongs to P3 coordination and stores `application_id`, `operation_id`, `kind`, `state`, `lease_owner`, `lease_expires_at`, safe result revision/digest, and timestamps. Its payload must not contain values or errors.

In `delivery.go`, define typed constants rather than free strings:

```go
type DeliveryRunState string
const (
    DeliveryRunQueued DeliveryRunState = "queued"
    DeliveryRunRunning DeliveryRunState = "running"
    DeliveryRunWaitingApproval DeliveryRunState = "waiting_approval"
    DeliveryRunCancelRequested DeliveryRunState = "cancel_requested"
    DeliveryRunReconciling DeliveryRunState = "reconciling"
    DeliveryRunSucceeded DeliveryRunState = "succeeded"
    DeliveryRunFailed DeliveryRunState = "failed"
    DeliveryRunRejected DeliveryRunState = "rejected"
    DeliveryRunCanceled DeliveryRunState = "canceled"
    DeliveryRunTimedOut DeliveryRunState = "timed_out"
    DeliveryRunOutcomeUnknown DeliveryRunState = "outcome_unknown"
)
const DeliveryExecutorHelmUpgradeExistingRelease = "helm_upgrade_existing_release"
```

Add `TableName()` methods for every model to make table ownership explicit.

- [ ] **Step 4: Verify migration embedding and constraints**

Run: `go test ./migrations -race`  
Expected: PASS and the embed test includes `00023_p6_delivery.sql`.  
Run: `go test -tags=dbtest ./tests/integration -run TestDeliverySchemaConstraints -race -count=1`  
Expected: PASS with both duplicate inserts rejected.

- [ ] **Step 5: Commit**

```bash
git add migrations/00023_p6_delivery.sql migrations/embed_test.go internal/models/delivery.go tests/integration/delivery_schema_test.go
git commit -m "feat(delivery): add orchestration schema"
```

### Task 2: Register P6 errors, permissions, and menus

**Files:**
- Modify: `optimus-be/internal/infra/errors/codes.go`
- Create: `optimus-be/internal/modules/delivery/errs/codes.go`
- Modify: `optimus-be/internal/infra/permissions/codes.go`
- Modify: `optimus-be/internal/seed/seed.go`
- Modify: `optimus-be/internal/seed/seed_test.go`
- Test: `optimus-be/internal/modules/delivery/errs/codes_test.go`

- [ ] **Step 1: Write failing registry tests**

Assert that all ten codes from the design exist exactly once, category is `delivery`, admin receives them, no special operator grant is created, and menus are:

```text
delivery
delivery.projects -> delivery/projects/List -> delivery:project:read
delivery.approvals -> delivery/approvals/List -> delivery:approval:read
```

Assert representative error aliases equal `45001`, `45101`, `45201`, and `45301` and message keys begin with `delivery.`.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/infra/permissions ./internal/seed ./internal/modules/delivery/errs -race`  
Expected: FAIL because P6 registries and menus are missing.

- [ ] **Step 3: Add exact registries**

Add constants for project read/write/delete, pipeline read/write, run read/create/cancel, and approval read/decide. Add `45xxx` errors for not found/name conflict/application bound/pipeline invalid, active run/idempotency conflict/invalid state, self approval/already decided, operation busy/artifact drift/reconciliation required/outcome unknown. Expose aliases and message keys from `delivery/errs`.

Seed only the parent and two child menus. Do not add a `k8s_operator` role or grant.

- [ ] **Step 4: Verify registries**

Run: `go test ./internal/infra/permissions ./internal/seed ./internal/modules/delivery/errs -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/errors/codes.go internal/modules/delivery/errs internal/infra/permissions/codes.go internal/seed/seed.go internal/seed/seed_test.go
git commit -m "feat(delivery): register errors permissions and menus"
```

### Task 3: Add bounded delivery configuration

**Files:**
- Modify: `optimus-be/internal/infra/config/config.go`
- Modify: `optimus-be/internal/infra/config/config_test.go`
- Modify: `optimus-be/configs/config.yaml`

- [ ] **Step 1: Write failing defaults and validation tests**

Cover defaults and invalid relationships for:

```go
type DeliveryConfig struct {
    WorkerConcurrency     int
    LeaseDuration         time.Duration
    LeaseRenewInterval    time.Duration
    DefaultStageTimeout   time.Duration
    MaxStageTimeout       time.Duration
    ReconcileInterval     time.Duration
    EventRetentionDays    int
    SSEHeartbeat          time.Duration
    SSEMaxConnections     int
}
```

Require all values positive, renew interval below lease duration, and default timeout no greater than max timeout.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/infra/config -run 'TestLoadDeliveryDefaults|TestValidateDelivery' -race`  
Expected: FAIL because `Config.Delivery` is missing.

- [ ] **Step 3: Implement defaults and strict validation**

Use defaults: concurrency 4, lease 30s, renew 10s, default timeout 10m, max timeout 30m, reconcile 15s, retention 180 days, heartbeat 20s, and max SSE connections 100. Add matching YAML keys and `OPTIMUS_DELIVERY_*` support through existing Viper mapping.

- [ ] **Step 4: Verify config**

Run: `go test ./internal/infra/config -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/config_test.go configs/config.yaml
git commit -m "feat(delivery): add bounded worker configuration"
```

### Task 4: Implement projects and environment bindings

**Files:**
- Create: `optimus-be/internal/modules/delivery/project/dto.go`
- Create: `optimus-be/internal/modules/delivery/project/repo.go`
- Create: `optimus-be/internal/modules/delivery/project/service.go`
- Test: `optimus-be/internal/modules/delivery/project/repo_test.go`
- Test: `optimus-be/internal/modules/delivery/project/service_test.go`

- [ ] **Step 1: Write failing service tests**

Define an `ApplicationReader` consumer interface returning only ID, name, chart repo ID, chart name, installed flag, cluster ID, namespace, and release name. Cover create/update/delete, duplicate names, application-not-found, uninstalled release, duplicate active binding, chart-name mismatch, active-run unbind denial, and outcome-unknown unbind denial.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/project -race`  
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement repository and service**

Expose concrete methods:

```go
CreateProject(ctx context.Context, actor uint64, ip, ua string, req CreateProjectRequest) (*ProjectDetail, error)
UpdateProject(ctx context.Context, actor uint64, ip, ua string, id uint64, req UpdateProjectRequest) (*ProjectDetail, error)
DeleteProject(ctx context.Context, actor uint64, ip, ua string, id uint64) error
BindEnvironment(ctx context.Context, actor uint64, ip, ua string, projectID uint64, req BindEnvironmentRequest) (*Environment, error)
UpdateEnvironment(ctx context.Context, actor uint64, ip, ua string, projectID, environmentID uint64, req UpdateEnvironmentRequest) (*Environment, error)
UnbindEnvironment(ctx context.Context, actor uint64, ip, ua string, projectID, environmentID uint64) error
```

Normalize environment keys to lowercase kebab-case, keep display names separate, and audit only safe IDs/names/chart identity.

- [ ] **Step 4: Verify project domain**

Run: `go test ./internal/modules/delivery/project -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/project
git commit -m "feat(delivery): manage projects and environments"
```

### Task 5: Implement immutable pipeline publication

**Files:**
- Create: `optimus-be/internal/modules/delivery/pipeline/dto.go`
- Create: `optimus-be/internal/modules/delivery/pipeline/repo.go`
- Create: `optimus-be/internal/modules/delivery/pipeline/service.go`
- Test: `optimus-be/internal/modules/delivery/pipeline/repo_test.go`
- Test: `optimus-be/internal/modules/delivery/pipeline/service_test.go`

- [ ] **Step 1: Write failing publication tests**

Cover empty stages, duplicate environment, non-contiguous input order normalization, missing binding, unsupported executor, non-positive timeout, timeout above configured maximum, immutable old version, and concurrent version publication producing one version 2 and one retryable conflict.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/pipeline -race`  
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement publish transaction**

Use this request shape:

```go
type PublishRequest struct {
    Stages []StageInput `json:"stages" binding:"required,min=1,max=20,dive"`
}
type StageInput struct {
    EnvironmentID  uint64        `json:"environment_id" binding:"required"`
    ApprovalRequired bool        `json:"approval_required"`
    Timeout          time.Duration `json:"timeout"`
}
```

Always persist executor `helm_upgrade_existing_release`, lock the project row, insert a new version plus ordered stages, and atomically switch the current marker. Audit one `delivery.pipeline.publish` event containing only version and stage/environment IDs.

- [ ] **Step 4: Verify pipeline domain**

Run: `go test ./internal/modules/delivery/pipeline -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/pipeline
git commit -m "feat(delivery): publish immutable pipelines"
```

### Task 6: Add P3 artifact digest resolution and verified loading

**Files:**
- Create: `optimus-be/internal/modules/apps/repo/artifact.go`
- Test: `optimus-be/internal/modules/apps/repo/artifact_test.go`
- Modify: `optimus-be/internal/modules/apps/repo/charts.go`

- [ ] **Step 1: Write failing HTTP and OCI artifact tests**

For HTTP, serve an index and chart archive, then assert SHA-256 is calculated from the exact downloaded bytes. For OCI, inject a pull function and assert the returned chart bytes produce the digest. Assert `LoadVerifiedChart` rejects a mismatched digest before Helm receives the chart.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/apps/repo -run 'TestResolveArtifact|TestLoadVerifiedChart' -race`  
Expected: FAIL because artifact APIs do not exist.

- [ ] **Step 3: Implement the safe artifact API**

Add:

```go
type Artifact struct { RepoID uint64; ChartName, Version, Digest string }
func (s *Service) ResolveArtifact(ctx context.Context, repoID uint64, chartName, version string) (*Artifact, error)
func (s *Service) LoadVerifiedChart(ctx context.Context, artifact Artifact) (*chart.Chart, error)
```

Use lowercase `sha256:<hex>`. Download once per call, hash before parse, compare with `subtle.ConstantTimeCompare`, wipe the byte slice after parsing, and map mismatch to a stable P6-safe sentinel that the delivery adapter maps to `453xx`. Do not log repository credentials or raw pull errors.

- [ ] **Step 4: Verify artifact behavior**

Run: `go test ./internal/modules/apps/repo -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/apps/repo/artifact.go internal/modules/apps/repo/artifact_test.go internal/modules/apps/repo/charts.go
git commit -m "feat(apps): resolve verified chart artifacts"
```

### Task 7: Add the P3 release operation coordinator

**Files:**
- Create: `optimus-be/internal/modules/apps/release/coordinator.go`
- Test: `optimus-be/internal/modules/apps/release/coordinator_test.go`
- Test: `optimus-be/tests/integration/apps_release_operation_test.go`

- [ ] **Step 1: Write failing coordinator tests**

Cover first acquire, same-operation replay, different-operation busy conflict, lease renewal by owner, lost-owner renewal denial, definite completion, lease-expiry takeover requiring reconciliation, and concurrent acquisition across two repository instances.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/apps/release -run TestCoordinator -race`  
Expected: FAIL because `Coordinator` does not exist.

- [ ] **Step 3: Implement database-backed coordination**

Expose:

```go
Acquire(ctx context.Context, applicationID uint64, operationID, kind, owner string, lease time.Duration) (AcquireResult, error)
Renew(ctx context.Context, operationID, owner string, until time.Time) error
Complete(ctx context.Context, operationID, owner string, result SafeOperationResult) error
Inspect(ctx context.Context, operationID string) (*Operation, error)
```

Use transactions and row locks. Never store upstream error strings, values, or chart bytes. A stale active row returns `NeedsReconciliation=true`; it does not authorize another mutation.

- [ ] **Step 4: Verify unit and DB concurrency**

Run: `go test ./internal/modules/apps/release -run TestCoordinator -race`  
Expected: PASS.  
Run: `go test -tags=dbtest ./tests/integration -run TestAppsReleaseOperationConcurrency -race -count=1`  
Expected: PASS with only one acquisition winner.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/apps/release/coordinator.go internal/modules/apps/release/coordinator_test.go tests/integration/apps_release_operation_test.go
git commit -m "feat(apps): coordinate release mutations"
```

### Task 8: Enforce delivery governance in every P3 mutation

**Files:**
- Create: `optimus-be/internal/modules/apps/release/governance.go`
- Modify: `optimus-be/internal/modules/apps/release/service.go`
- Modify: `optimus-be/internal/modules/apps/release/service_test.go`
- Modify: `optimus-be/internal/modules/apps/application/service.go`
- Modify: `optimus-be/internal/modules/apps/application/service_test.go`

- [ ] **Step 1: Write failing policy and deletion tests**

Inject a fake policy that marks an application managed. Assert direct install, upgrade, and uninstall fail before chart loading or Helm construction; direct rollback remains allowed. Assert an internal capability authorizes only matching application, operation ID, and action. Assert application deletion is blocked when delivery binding count is nonzero.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/apps/release ./internal/modules/apps/application -run 'Delivery|Governance|InUse' -race`  
Expected: FAIL because policy seams are absent.

- [ ] **Step 3: Add fail-closed seams**

Define:

```go
type Governance interface {
    AuthorizeMutation(ctx context.Context, applicationID uint64, action string) error
}
type DeliveryApplicationCounter interface {
    CountByApplicationID(ctx context.Context, applicationID uint64) (int, error)
}
```

Call governance at the top of every release mutation. The default unbound provider permits existing behavior; a managed lookup failure denies mutation. Use a private typed context capability for P6 upgrade, never an HTTP field. Add a nil-safe delivery counter to application deletion.

- [ ] **Step 4: Verify P3 protection**

Run: `go test ./internal/modules/apps/release ./internal/modules/apps/application -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/apps/release/governance.go internal/modules/apps/release/service.go internal/modules/apps/release/service_test.go internal/modules/apps/application/service.go internal/modules/apps/application/service_test.go
git commit -m "feat(apps): protect delivery managed releases"
```

### Task 9: Add the constrained P3 delivery executor seam

**Files:**
- Create: `optimus-be/internal/modules/apps/release/delivery.go`
- Test: `optimus-be/internal/modules/apps/release/delivery_test.go`
- Modify: `optimus-be/internal/modules/apps/release/dto.go`
- Modify: `optimus-be/internal/modules/apps/release/service.go`

- [ ] **Step 1: Write failing execution tests**

Assert that delivery execution requires an installed release, verifies repo/name/version/digest, sets Helm `ReuseValues=true`, passes an empty values map, uses `RunWithContext`, records only safe audit metadata, and returns revision/status/digest. Assert values, notes, and raw errors never appear in result or audit.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/apps/release -run TestDeliveryUpgrade -race`  
Expected: FAIL because `UpgradeForDelivery` does not exist.

- [ ] **Step 3: Implement the closed request**

```go
type DeliveryUpgradeRequest struct {
    ApplicationID uint64
    OperationID   string
    RepoID        uint64
    ChartName     string
    ChartVersion  string
    Digest        string
    InitiatorID   uint64
    Purpose       string
}
type DeliveryUpgradeResult struct { Revision int; Status, Digest string }
```

Acquire the coordinator, add the internal governance capability, load the verified chart, build the request-scoped Helm configuration, set `ReuseValues`, run with context, inspect status, and complete the operation with safe result fields. Ambiguous errors leave the operation reconcilable instead of completed as failure.

- [ ] **Step 4: Verify execution contract**

Run: `go test ./internal/modules/apps/release -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/apps/release/delivery.go internal/modules/apps/release/delivery_test.go internal/modules/apps/release/dto.go internal/modules/apps/release/service.go
git commit -m "feat(apps): expose constrained delivery upgrade"
```

### Task 10: Create run snapshots and idempotent run creation

**Files:**
- Create: `optimus-be/internal/modules/delivery/run/state.go`
- Create: `optimus-be/internal/modules/delivery/run/dto.go`
- Create: `optimus-be/internal/modules/delivery/run/repo.go`
- Create: `optimus-be/internal/modules/delivery/run/service.go`
- Test: `optimus-be/internal/modules/delivery/run/state_test.go`
- Test: `optimus-be/internal/modules/delivery/run/service_test.go`

- [ ] **Step 1: Write failing state and create tests**

Table-drive the exact states in the design. Cover no current pipeline, active run conflict, artifact mismatch across environments, identical idempotent replay, changed fingerprint conflict, immutable stage snapshot, and first-stage transition to waiting approval or queued.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/run -race`  
Expected: FAIL because the run package does not exist.

- [ ] **Step 3: Implement canonical creation**

Define `CanTransitionRun(from,to)` and `CanTransitionStage(from,to)` as explicit maps. Canonicalize the fingerprint from project ID, pipeline version, chart identity/version/digest, and retry origin using deterministic JSON plus SHA-256. In one transaction lock the project, reject active or unknown runs, insert the run/stage snapshots, create the first approval when required, and append the first run events.

- [ ] **Step 4: Verify run creation**

Run: `go test ./internal/modules/delivery/run -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/run
git commit -m "feat(delivery): create immutable idempotent runs"
```

### Task 11: Implement approvals with live RBAC and four-eyes rules

**Files:**
- Create: `optimus-be/internal/modules/delivery/approval/dto.go`
- Create: `optimus-be/internal/modules/delivery/approval/repo.go`
- Create: `optimus-be/internal/modules/delivery/approval/service.go`
- Test: `optimus-be/internal/modules/delivery/approval/service_test.go`
- Test: `optimus-be/tests/integration/delivery_approval_test.go`

- [ ] **Step 1: Write failing approval tests**

Cover pending queue visibility, missing live permission, initiator self-approval, required comment length, approve transition to queued, reject transition to rejected, identical replay, opposite-decision conflict, and two approvers racing.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/approval -race`  
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement immutable decision transaction**

Accept a narrow permission checker:

```go
type PermissionChecker interface { Has(ctx context.Context, userID uint64, code string) (bool, error) }
```

Lock the approval and stage rows, recheck permission, reject `userID == run.InitiatorID`, persist the first decision, transition stage/run, append events, and audit only decision plus comment-present/hash. Never copy comment text into audit or events.

- [ ] **Step 4: Verify decisions and race**

Run: `go test ./internal/modules/delivery/approval -race`  
Expected: PASS.  
Run: `go test -tags=dbtest ./tests/integration -run TestDeliveryApprovalFirstDecisionWins -race -count=1`  
Expected: PASS with one terminal decision.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/approval tests/integration/delivery_approval_test.go
git commit -m "feat(delivery): add four eyes approvals"
```

### Task 12: Implement transactional events and authenticated SSE

**Files:**
- Create: `optimus-be/internal/modules/delivery/event/repo.go`
- Create: `optimus-be/internal/modules/delivery/event/service.go`
- Create: `optimus-be/internal/modules/delivery/event/handler.go`
- Test: `optimus-be/internal/modules/delivery/event/service_test.go`
- Test: `optimus-be/internal/modules/delivery/event/handler_test.go`

- [ ] **Step 1: Write failing event and stream tests**

Assert ordered cursor reads, `Last-Event-ID` resume, heartbeat comments, request cancellation, write-deadline clearing, per-event size rejection, run ownership by permission middleware, and absence of forbidden keys (`values`, `manifest`, `notes`, `credential`, `raw_error`).

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/event -race`  
Expected: FAIL because the event package does not exist.

- [ ] **Step 3: Implement bounded persisted SSE**

Use the P2 `http.NewResponseController` pattern. Encode events as:

```text
id: <numeric-id>
event: delivery
data: <bounded-json>

```

Poll committed rows after the cursor with a fixed page limit, emit heartbeat comments, stop on request context, and set `Cache-Control: no-cache` plus `X-Accel-Buffering: no`. Do not start a goroutine that can outlive the request.

- [ ] **Step 4: Verify event service**

Run: `go test ./internal/modules/delivery/event -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/event
git commit -m "feat(delivery): stream structured run events"
```

### Task 13: Implement worker leasing and stage advancement

**Files:**
- Create: `optimus-be/internal/modules/delivery/orchestrator/worker.go`
- Create: `optimus-be/internal/modules/delivery/orchestrator/executor.go`
- Test: `optimus-be/internal/modules/delivery/orchestrator/worker_test.go`
- Test: `optimus-be/tests/integration/delivery_worker_test.go`

- [ ] **Step 1: Write failing worker tests**

Use a fake clock and executor. Cover bounded concurrency, one claimant, periodic renewal, graceful stop, successful stage advancement, next approval creation, final run success, failed stage stop, and lease loss forcing reconciliation instead of a terminal guess.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/orchestrator -run TestWorker -race`  
Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the bounded loop**

Define a closed executor interface:

```go
type Executor interface {
    UpgradeExisting(ctx context.Context, req UpgradeRequest) (UpgradeResult, error)
}
```

Claim queued rows with `FOR UPDATE SKIP LOCKED`, hold a semaphore of configured size, renew leases, and use conditional state transitions. On success persist safe revision/digest and enqueue the next stage. On a definite classified failure stop the run. On context, lease, or transport ambiguity enter reconciliation.

- [ ] **Step 4: Verify worker concurrency**

Run: `go test ./internal/modules/delivery/orchestrator -race`  
Expected: PASS.  
Run: `go test -tags=dbtest ./tests/integration -run TestDeliveryWorkersClaimOnce -race -count=1`  
Expected: PASS with one P3 execution.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/orchestrator tests/integration/delivery_worker_test.go
git commit -m "feat(delivery): execute leased promotion stages"
```

### Task 14: Implement cancel, reconciliation, and linked retry

**Files:**
- Create: `optimus-be/internal/modules/delivery/orchestrator/reconciler.go`
- Test: `optimus-be/internal/modules/delivery/orchestrator/reconciler_test.go`
- Modify: `optimus-be/internal/modules/delivery/run/service.go`
- Modify: `optimus-be/internal/modules/delivery/run/service_test.go`

- [ ] **Step 1: Write failing recovery tests**

Cover immediate cancellation before execution, cooperative running cancellation, target digest observed as success, previous digest proving timeout/cancel, ambiguous inspection producing `outcome_unknown`, project blocking, later definite reconciliation, P3 rollback drift, retry fixed to original digest, retry beginning at failed environment, and renewed approvals.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/orchestrator ./internal/modules/delivery/run -run 'Cancel|Reconcile|Retry' -race`  
Expected: FAIL because recovery commands are absent.

- [ ] **Step 3: Implement explicit recovery commands**

Add:

```go
Cancel(ctx context.Context, actor uint64, ip, ua string, runID uint64) (*Detail, error)
RequestReconcile(ctx context.Context, actor uint64, ip, ua string, runID uint64) (*Detail, error)
Retry(ctx context.Context, actor uint64, ip, ua string, runID uint64, idempotencyKey string) (*Detail, error)
```

Never update an old terminal run during retry. Reconciliation may resolve an unknown state only from P3 inspection evidence. Keep the project blocked while any run remains unknown.

- [ ] **Step 4: Verify recovery behavior**

Run: `go test ./internal/modules/delivery/orchestrator ./internal/modules/delivery/run -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/orchestrator/reconciler.go internal/modules/delivery/orchestrator/reconciler_test.go internal/modules/delivery/run/service.go internal/modules/delivery/run/service_test.go
git commit -m "feat(delivery): reconcile cancel and retry runs"
```

### Task 15: Add event pruning and lifecycle tests

**Files:**
- Create: `optimus-be/internal/modules/delivery/event/pruner.go`
- Test: `optimus-be/internal/modules/delivery/event/pruner_test.go`

- [ ] **Step 1: Write failing retention tests**

Insert events just before, at, and after a 180-day cutoff. Assert only older detailed events are deleted, run/stage/approval summaries remain, pruning is bounded by batch size, context cancellation stops batches, and a second pass is idempotent.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/event -run TestPruner -race`  
Expected: FAIL because pruning does not exist.

- [ ] **Step 3: Implement the bounded pruner**

Delete event IDs selected by cutoff in batches of 500 inside short transactions. Run on startup after the configured reconcile delay and then daily. Log only deleted count and safe failure category.

- [ ] **Step 4: Verify retention**

Run: `go test ./internal/modules/delivery/event -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery/event/pruner.go internal/modules/delivery/event/pruner_test.go
git commit -m "feat(delivery): prune detailed run events"
```

### Task 16: Add delivery HTTP handlers and route permissions

**Files:**
- Create: `optimus-be/internal/modules/delivery/project/handler.go`
- Create: `optimus-be/internal/modules/delivery/project/handler_test.go`
- Create: `optimus-be/internal/modules/delivery/pipeline/handler.go`
- Create: `optimus-be/internal/modules/delivery/pipeline/handler_test.go`
- Create: `optimus-be/internal/modules/delivery/run/handler.go`
- Create: `optimus-be/internal/modules/delivery/run/handler_test.go`
- Create: `optimus-be/internal/modules/delivery/approval/handler.go`
- Create: `optimus-be/internal/modules/delivery/approval/handler_test.go`
- Create: `optimus-be/internal/modules/delivery/module/wire.go`
- Test: `optimus-be/internal/modules/delivery/module/wire_test.go`

- [ ] **Step 1: Write failing contract and permission tests**

Test every design endpoint, exact permission, bad ID/binding handling, `Idempotency-Key` requirement, envelope shape, comment limits, pagination limits, no raw error leakage, SSE permission, and absence of any install/uninstall/script endpoint.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/modules/delivery/... -run 'Handler|Routes' -race`  
Expected: FAIL because handlers and routes are absent.

- [ ] **Step 3: Implement handlers and module assembly**

Follow existing `response.Success/Error` and nested `Group("", RequirePermission(...))` patterns. `PUT /projects/:id/pipeline` publishes a new version. `POST /runs/:id/reconcile` and `/retry` use run create; cancel uses run cancel. Parse and cap `Last-Event-ID` in the event handler.

Annotate every JSON handler for Swagger with stable DTOs and envelope results. Do not add Swagger examples containing chart credentials, values, or raw errors.

- [ ] **Step 4: Verify backend HTTP surface**

Run: `go test ./internal/modules/delivery/... -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/delivery
git commit -m "feat(delivery): expose orchestration APIs"
```

### Task 17: Wire P3 and P6 with graceful lifecycle

**Files:**
- Modify: `optimus-be/internal/modules/apps/module/module.go`
- Modify: `optimus-be/internal/modules/apps/module/module_test.go`
- Modify: `optimus-be/cmd/server/main.go`
- Test: `optimus-be/cmd/server/main_test.go`

- [ ] **Step 1: Write failing composition tests**

Assert main constructs one shared P3 coordinator, injects the delivery governance and application counter, gives delivery only P3 public adapters, mounts `/delivery`, starts worker/reconciler/pruner with the server context, and waits for graceful shutdown without marking an in-flight stage failed.

- [ ] **Step 2: Verify failure**

Run: `go test ./cmd/server ./internal/modules/apps/module -race`  
Expected: FAIL because delivery wiring is absent.

- [ ] **Step 3: Implement composition-root wiring**

Construct P3 first, construct delivery with P3 adapter interfaces, then inject delivery policy/count seams back into P3. Start background components only after routes and dependencies are ready. On shutdown cancel the shared context, stop claims, wait within `Server.ShutdownTimeout`, and leave uncertain work reconcilable.

- [ ] **Step 4: Verify composition**

Run: `go test ./cmd/server ./internal/modules/apps/module ./internal/modules/delivery/module -race`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/apps/module/module.go internal/modules/apps/module/module_test.go cmd/server/main.go cmd/server/main_test.go
git commit -m "feat(delivery): wire orchestration lifecycle"
```

### Task 18: Add backend end-to-end integration coverage

**Files:**
- Create: `optimus-be/tests/integration/delivery_flow_test.go`
- Create: `optimus-be/tests/integration/delivery_recovery_test.go`

- [ ] **Step 1: Write the full flow test against PostgreSQL and a fake P3 adapter**

Exercise project creation, three bindings, pipeline publication, idempotent run creation, no-approval first stage, approval wait, self-approval denial, authorized approval, final success, event ordering, and managed-application guard state.

- [ ] **Step 2: Write recovery integration scenarios**

Exercise concurrent run creation, two workers, lease expiry, cancellation during execution, outcome unknown, project blocking, definite reconciliation, P3 rollback drift, and linked retry from the failed environment.

- [ ] **Step 3: Verify integration tests**

Run: `go test -tags=dbtest ./tests/integration -run 'TestDeliveryFlow|TestDeliveryRecovery' -race -count=1`  
Expected: PASS.

- [ ] **Step 4: Run the complete backend test suite**

Run: `make test`  
Expected: PASS with no race failure.  
Run: `make test-int`  
Expected: PASS including all delivery integration tests.

- [ ] **Step 5: Commit**

```bash
git add tests/integration/delivery_flow_test.go tests/integration/delivery_recovery_test.go
git commit -m "test(delivery): cover promotion and recovery flows"
```

### Task 19: Add frontend delivery contracts and API factories

**Files:**
- Create: `optimus-fe/src/types/delivery.ts`
- Create: `optimus-fe/src/api/delivery/project.ts`
- Create: `optimus-fe/src/api/delivery/pipeline.ts`
- Create: `optimus-fe/src/api/delivery/run.ts`
- Create: `optimus-fe/src/api/delivery/approval.ts`
- Test: `optimus-fe/src/api/delivery/__tests__/api.test.ts`
- Modify: `optimus-fe/src/main.ts`

- [ ] **Step 1: Write failing API serialization tests**

Assert exact paths, query names, envelopes, `Idempotency-Key`, no values field, closed state unions, pipeline duration serialization, and injection keys `deliveryProjectApi`, `deliveryPipelineApi`, `deliveryRunApi`, `deliveryApprovalApi`.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/api/delivery/__tests__/api.test.ts`  
Expected: FAIL because delivery types and APIs do not exist.

- [ ] **Step 3: Implement typed APIs**

Define unions matching backend constants, including:

```ts
export type RunState='queued'|'running'|'waiting_approval'|'cancel_requested'|'reconciling'|'succeeded'|'failed'|'rejected'|'canceled'|'timed_out'|'outcome_unknown'
export type StageState='pending'|'waiting_approval'|'queued'|'running'|'reconciling'|'succeeded'|'failed'|'rejected'|'canceled'|'timed_out'|'outcome_unknown'
```

Expose only design-approved DTO fields. Register factories in `main.ts`.

- [ ] **Step 4: Verify frontend contracts**

Run: `bun x vitest run src/api/delivery/__tests__/api.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/types/delivery.ts src/api/delivery src/main.ts
git commit -m "feat(fe/delivery): add typed delivery APIs"
```

### Task 20: Add authenticated event streaming and stale-generation protection

**Files:**
- Create: `optimus-fe/src/api/delivery/events.ts`
- Test: `optimus-fe/src/api/delivery/__tests__/events.test.ts`
- Create: `optimus-fe/src/stores/delivery.ts`
- Test: `optimus-fe/src/stores/delivery.test.ts`
- Modify: `optimus-fe/src/main.ts`

- [ ] **Step 1: Write failing parser and store tests**

Cover split UTF-8 chunks, multiline SSE frames, event IDs, reconnect cursor, authorization header, abort, polling fallback, duplicate event merge, old-generation rejection, reset on logout, and no use of browser `EventSource`.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/api/delivery/__tests__/events.test.ts src/stores/delivery.test.ts`  
Expected: FAIL because stream and store do not exist.

- [ ] **Step 3: Implement stream and store**

Use `fetch`, the current access token, `ReadableStreamDefaultReader`, and an abort controller per run. The store increments a generation for every run selection and checks both generation and run ID before committing a snapshot, event, reconnect, or poll result.

- [ ] **Step 4: Verify streaming state**

Run: `bun x vitest run src/api/delivery/__tests__/events.test.ts src/stores/delivery.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/api/delivery/events.ts src/api/delivery/__tests__/events.test.ts src/stores/delivery.ts src/stores/delivery.test.ts src/main.ts
git commit -m "feat(fe/delivery): stream run events safely"
```

### Task 21: Build the delivery project list

**Files:**
- Create: `optimus-fe/src/views/delivery/projects/List.vue`
- Test: `optimus-fe/src/views/delivery/projects/__tests__/List.test.ts`

- [ ] **Step 1: Write failing component permission tests**

Mount with read-only, write, and delete permission sets. Assert read absence suppresses fetch, write controls create/edit, delete controls delete, pagination uses `useTable`, errors use normalized messages, and navigation opens `/delivery/projects/:id`.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/views/delivery/projects/__tests__/List.test.ts`  
Expected: FAIL because the page does not exist.

- [ ] **Step 3: Implement the page**

Use Ant Design Vue table, bounded search, modal metadata form, confirmation delete, exact `v-permission`, and no deployment controls on the list page.

- [ ] **Step 4: Verify project list**

Run: `bun x vitest run src/views/delivery/projects/__tests__/List.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/views/delivery/projects/List.vue src/views/delivery/projects/__tests__/List.test.ts
git commit -m "feat(fe/delivery): add project list"
```

### Task 22: Build environment and pipeline configuration

**Files:**
- Create: `optimus-fe/src/views/delivery/projects/Detail.vue`
- Create: `optimus-fe/src/views/delivery/projects/components/EnvironmentForm.vue`
- Create: `optimus-fe/src/views/delivery/projects/components/PipelineForm.vue`
- Test: `optimus-fe/src/views/delivery/projects/__tests__/Detail.test.ts`
- Test: `optimus-fe/src/views/delivery/projects/components/__tests__/PipelineForm.test.ts`

- [ ] **Step 1: Write failing form and permission tests**

Cover project-read and pipeline-read tabs independently, project-write environment actions, pipeline-write publication, application installed/chart compatibility errors, duplicate environment prevention, ordered stages, approval toggles, bounded timeout, and publish confirmation showing the next immutable version.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/views/delivery/projects/__tests__/Detail.test.ts src/views/delivery/projects/components/__tests__/PipelineForm.test.ts`  
Expected: FAIL because components do not exist.

- [ ] **Step 3: Implement typed configuration UI**

Use a reorderable button-based list, not a generic DAG or YAML editor. The form submits only environment IDs, approval booleans, and duration strings accepted by the backend. Display bound P3 application, cluster, namespace, release, and chart as read-only facts.

- [ ] **Step 4: Verify configuration UI**

Run: `bun x vitest run src/views/delivery/projects/__tests__/Detail.test.ts src/views/delivery/projects/components/__tests__/PipelineForm.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/views/delivery/projects/Detail.vue src/views/delivery/projects/components src/views/delivery/projects/__tests__/Detail.test.ts
git commit -m "feat(fe/delivery): configure promotion pipelines"
```

### Task 23: Add artifact confirmation and run history

**Files:**
- Create: `optimus-fe/src/views/delivery/projects/components/RunForm.vue`
- Test: `optimus-fe/src/views/delivery/projects/components/__tests__/RunForm.test.ts`
- Modify: `optimus-fe/src/views/delivery/projects/Detail.vue`
- Modify: `optimus-fe/src/views/delivery/projects/__tests__/Detail.test.ts`

- [ ] **Step 1: Write failing run creation tests**

Assert only run-create users fetch artifacts, confirmation shows repo/chart/version/digest plus environment order and approvals, a fresh idempotency key is generated once per submission intent, double-click reuses the key, success navigates to run detail, and no values or Helm flags are rendered.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/views/delivery/projects/components/__tests__/RunForm.test.ts`  
Expected: FAIL because the run form does not exist.

- [ ] **Step 3: Implement run form and history tab**

Use `crypto.randomUUID()` when available and a tested UUID fallback. Reset the key only after success or an explicit version change. Add paginated run history gated by run read and links to `/delivery/runs/:id`.

- [ ] **Step 4: Verify run creation UI**

Run: `bun x vitest run src/views/delivery/projects/components/__tests__/RunForm.test.ts src/views/delivery/projects/__tests__/Detail.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/views/delivery/projects/components/RunForm.vue src/views/delivery/projects/components/__tests__/RunForm.test.ts src/views/delivery/projects/Detail.vue src/views/delivery/projects/__tests__/Detail.test.ts
git commit -m "feat(fe/delivery): create immutable promotion runs"
```

### Task 24: Build run detail and recovery controls

**Files:**
- Create: `optimus-fe/src/views/delivery/runs/Detail.vue`
- Test: `optimus-fe/src/views/delivery/runs/__tests__/Detail.test.ts`

- [ ] **Step 1: Write failing timeline and action tests**

Cover snapshot load, structured event merge, reconnect, polling fallback, every state label, digest/revision display, approval result, safe error plus correlation ID, cancel permission, reconcile/retry permission, disabled actions in invalid states, and P3 rollback link. Assert no raw-log panel exists.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/views/delivery/runs/__tests__/Detail.test.ts`  
Expected: FAIL because the page does not exist.

- [ ] **Step 3: Implement run detail**

Render an Ant Design timeline with stage status tags, approval rows, and structured event descriptions. Use the delivery store's generation lifecycle on mount, route change, and unmount. Show the P3 release link only for failed/timed-out/unknown stages.

- [ ] **Step 4: Verify run detail**

Run: `bun x vitest run src/views/delivery/runs/__tests__/Detail.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/views/delivery/runs/Detail.vue src/views/delivery/runs/__tests__/Detail.test.ts
git commit -m "feat(fe/delivery): show promotion run timeline"
```

### Task 25: Build the approval queue

**Files:**
- Create: `optimus-fe/src/views/delivery/approvals/List.vue`
- Test: `optimus-fe/src/views/delivery/approvals/__tests__/List.test.ts`

- [ ] **Step 1: Write failing approval UI tests**

Cover approval-read fetch, decide buttons, self-run actions disabled, bounded required comment, approve/reject confirmation, identical replay refresh, conflict refresh, and run-detail navigation. Verify approval decide does not imply approval read in UI computation.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/views/delivery/approvals/__tests__/List.test.ts`  
Expected: FAIL because the page does not exist.

- [ ] **Step 3: Implement the queue**

Show project, environment, artifact digest, initiator, requested time, and run link. Keep comment text only in component state until submission; never log it or place it in route/query state.

- [ ] **Step 4: Verify approvals UI**

Run: `bun x vitest run src/views/delivery/approvals/__tests__/List.test.ts`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/views/delivery/approvals/List.vue src/views/delivery/approvals/__tests__/List.test.ts
git commit -m "feat(fe/delivery): add approval queue"
```

### Task 26: Complete navigation, i18n, and frontend regression coverage

**Files:**
- Modify: `optimus-fe/src/locales/zh-CN.json`
- Modify: `optimus-fe/src/locales/en-US.json`
- Modify: `optimus-fe/src/router/dynamic-routes.test.ts`
- Modify: `optimus-fe/src/main.ts`
- Test: `optimus-fe/src/views/delivery/__tests__/v-permission.test.ts`

- [ ] **Step 1: Write failing route, reset, and parity tests**

Assert seeded component paths resolve on Linux, run detail is navigable as a non-menu child route, all concrete buttons carry exact permissions, logout aborts delivery streams and resets state, and both locales contain identical delivery keys.

- [ ] **Step 2: Verify failure**

Run: `bun x vitest run src/router/dynamic-routes.test.ts src/views/delivery/__tests__/v-permission.test.ts`  
Expected: FAIL until routes, reset, and locale labels are complete.

- [ ] **Step 3: Add complete bilingual labels and registration**

Add menu names, permission labels, entity fields, states, event descriptions, validation, confirmation, safe error, retry, reconciliation, and approval messages in both locale files. Reset `useDeliveryStore()` in the existing logout callback.

- [ ] **Step 4: Run all frontend gates**

Run: `bun run lint`  
Expected: PASS with zero warnings.  
Run: `bun run typecheck`  
Expected: PASS.  
Run: `bun run i18n:check`  
Expected: PASS with parity.  
Run: `bun run test`  
Expected: PASS.  
Run: `bun run build`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/locales/zh-CN.json src/locales/en-US.json src/router/dynamic-routes.test.ts src/main.ts src/views/delivery
git commit -m "feat(fe/delivery): complete navigation and localization"
```

### Task 27: Regenerate Swagger and permissions

**Files:**
- Modify: `optimus-be/api/docs/docs.go`
- Modify: `optimus-be/api/docs/swagger.json`
- Modify: `optimus-be/api/docs/swagger.yaml`
- Modify: `docs/api/swagger.json`
- Modify: `docs/permissions.md`

- [ ] **Step 1: Run artifact drift checks before regeneration**

Run: `make swagger-diff`  
Expected: FAIL because delivery handlers are absent from checked-in Swagger.  
Run: `make perm-check`  
Expected: FAIL because delivery permissions are absent from `docs/permissions.md`.

- [ ] **Step 2: Regenerate artifacts**

Run: `make swag`  
Expected: PASS and update both backend and repository Swagger artifacts.  
Run: `make dump-perms`  
Expected: PASS and list exactly ten delivery permissions.

- [ ] **Step 3: Scan generated output for forbidden content**

Run: `rg -n 'values_yaml|kubeconfig|authorization|manifest|helm notes|raw_error|shell|script|container_image' api/docs ../docs/api/swagger.json ../docs/permissions.md`  
Expected: no P6 endpoint or DTO exposes forbidden fields; pre-existing unrelated kubeconfig documentation may remain and must be reviewed by path, not deleted.

- [ ] **Step 4: Verify generated artifacts**

Run: `make swagger-diff`  
Expected: PASS.  
Run: `make perm-check`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/docs ../docs/api/swagger.json ../docs/permissions.md
git commit -m "docs(delivery): regenerate API and permissions"
```

### Task 28: Write and execute the disposable P6 smoke checklist

**Files:**
- Create: `optimus-be/scripts/p6-smoke.md`

- [ ] **Step 1: Write the executable checklist**

Document exact local prerequisites and commands for disposable PostgreSQL, a local Kubernetes cluster, a local HTTP chart repository, three P3 applications/releases, a version-1 and version-2 test chart, JWT login, role setup, project binding, pipeline publication, run creation, approvals, SSE, worker restart, failure, P3 rollback, reconciliation, retry, and teardown.

- [ ] **Step 2: Add negative security checks**

Include executable requests proving direct P3 upgrade/uninstall on a managed application are denied, self-approval is denied, duplicate idempotency returns the same run, conflicting idempotency is rejected, a second active run is rejected, and API/SSE/audit output contains none of the forbidden sensitive strings.

- [ ] **Step 3: Run the checklist against disposable resources**

Run each command in `scripts/p6-smoke.md`.  
Expected: every positive step succeeds, every negative assertion fails with the documented safe code, and all resources are disposable local resources.

- [ ] **Step 4: Record only safe outcomes in the checklist**

Update the checklist's expected status/revision/digest examples from the local test chart. Do not paste kubeconfig, tokens, values, manifests, notes, or raw Helm errors.

- [ ] **Step 5: Commit**

```bash
git add scripts/p6-smoke.md
git commit -m "docs(delivery): add local promotion smoke test"
```

### Task 29: Run final quality, security, and hygiene gates

**Files:**
- Modify only files required to fix a failing gate; keep each fix in its owning task area.

- [ ] **Step 1: Run complete backend gates**

Run: `make test`  
Expected: PASS.  
Run: `make test-int`  
Expected: PASS.  
Run: `make lint`  
Expected: PASS.  
Run: `make swagger-diff`  
Expected: PASS.  
Run: `make perm-check`  
Expected: PASS.

- [ ] **Step 2: Run complete frontend gates with the frozen lockfile**

Run: `bun install --frozen-lockfile`  
Expected: PASS without lockfile changes.  
Run: `bun run lint && bun run typecheck && bun run i18n:check && bun run test && bun run build`  
Expected: every command PASS.

- [ ] **Step 3: Run explicit architectural and leakage scans**

Run: `rg -n 'os/exec|exec\.Command|/bin/sh|bash -c|container_image|values_yaml|GetSSHKey|GetHTTPCredential|GetCloudKey|assets\.Consumer' internal/modules/delivery`  
Expected: no match.  
Run: `rg -n 'internal/modules/(apps|credentials|assets)/.*/repo|Table\("apps_|Table\("credentials_|Table\("assets_' internal/modules/delivery`  
Expected: no match.  
Run: `git diff --check`  
Expected: PASS.

- [ ] **Step 4: Review branch history and worktree**

Run: `git status --short --branch`  
Expected: clean worktree on the implementation branch.  
Run: `git log --oneline --decorate --max-count=35`  
Expected: one coherent commit per task slice with the P6 spec and plan as ancestors.

- [ ] **Step 5: Commit any gate-only correction**

If and only if a gate required a correction, stage the exact corrected files and commit:

```bash
git commit -m "fix(delivery): satisfy final quality gates"
```

If no correction was required, do not create an empty commit.
