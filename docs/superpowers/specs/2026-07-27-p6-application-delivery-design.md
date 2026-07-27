# P6 — Application Delivery Orchestration Design

**Status**: Draft for user review  
**Date**: 2026-07-27  
**Owner**: P6 sub-project  
**Depends on**: P0 platform, P1 credentials vault, P3 applications  
**Related but not directly consumed**: P2 Kubernetes management, P4 assets  

## 1. Goal and product boundary

P6 delivers an application release orchestration MVP. It promotes one immutable
Helm chart artifact through an ordered set of environments, with approvals,
durable execution records, concurrency control, auditability, and status
display. Each environment points to an existing P3 application and therefore to
one existing Helm release in one cluster and namespace.

P6 is a governance and orchestration layer. P3 remains the only owner of the
Helm release lifecycle and the only module that constructs Helm clients or
consumes kubeconfigs. P6 decides when a release may advance and records what
happened; P3 decides how a Helm release is inspected and upgraded.

### 1.1 MVP capabilities

- Group existing P3 applications into a logical delivery project.
- Bind one existing P3 application to each project environment.
- Publish an ordered, versioned environment-promotion pipeline.
- Require a four-eyes approval before selected stages.
- Resolve and freeze a Helm chart version to a content digest when a run is
  created.
- Upgrade already-installed releases to the same digest, reusing each
  environment's current Helm values.
- Persist run and stage state, approvals, structured events, leases, and safe
  execution results.
- Provide cancellation, timeout, reconciliation, and manually initiated retry
  semantics without guessing an external operation's result.
- Prevent direct P3 mutations from bypassing delivery governance for managed
  applications.
- Display projects, pipelines, runs, approvals, and a live structured event
  timeline in the frontend.

### 1.2 Explicit non-goals

P6 MVP does not provide:

- Source checkout or source builds.
- SCM webhooks or API-token-triggered runs.
- GitHub Actions or GitLab CI integration.
- Artifact upload, artifact storage, or a general artifact repository.
- SSH deployment or direct use of P1 SSH keys.
- Direct Kubernetes mutation or direct use of P1 kubeconfigs.
- Direct use of P1 HTTP credentials.
- Direct use of P4 `assets.Consumer` or asset-selected deployment targets.
- A general shell runner, user-supplied command, arbitrary container, script,
  environment-variable injection, or extensible task plugin.
- A general DAG editor. Pipelines are ordered environment stages.
- First-time Helm install, uninstall, or automatic rollback.
- Helm values editing, templating, storage, or promotion.
- Cross-environment atomicity or automatic compensation.

External CI integration and native SSH/Kubernetes tasks require separate future
specifications. They must not be introduced by adding an unreviewed stage type
to this MVP.

## 2. Chosen approach

Three approaches were considered:

1. **Application delivery orchestration (chosen)**: Optimus owns approvals and
   a durable state machine, then delegates a constrained Helm upgrade to P3.
2. **External CI integration**: Optimus triggers GitHub Actions or GitLab CI and
   reconciles an external job state. This introduces vendor adapters, HTTP
   credentials, callback authentication, and two sources of execution truth.
3. **Native deployment tasks**: Optimus runs SSH or Kubernetes tasks. This
   creates a remote-code-execution platform and requires a substantially larger
   runner, isolation, credential, output, and supply-chain security design.

The chosen approach has the narrowest trust boundary, reuses P3, and supplies
the governance that P3 intentionally omitted. Its cost is that P6 is not a
general CI/CD platform and can promote only existing Helm releases.

## 3. Architecture and module boundaries

The backend module is named `delivery`, rather than `cicd`, to reflect its
deliberately narrow product boundary.

```text
User
  -> Delivery HTTP API
  -> durable run/state machine in PostgreSQL
  -> bounded in-process delivery worker
  -> narrow P3 release seam
  -> existing P3 Helm lifecycle implementation
  -> target Helm release
```

### 3.1 P6 components

- **Delivery control plane** manages projects, environment bindings, immutable
  pipeline versions, artifact selection, and run creation.
- **Orchestrator** advances the persisted state machine and creates approval or
  execution work.
- **Worker** claims queued stages through database leases. Its only executor is
  the closed enum value `helm_upgrade_existing_release`.
- **Reconciler** determines the actual release revision and digest after a
  timeout, cancellation, lost lease, process crash, or ambiguous P3 result.
- **Run event service** appends structured events and serves an authenticated
  SSE stream.
- **Governance provider** tells P3 whether an application is delivery-managed
  and whether an internal orchestration operation is authorized.
- **In-use provider** prevents deletion of a P3 application that remains bound
  to a delivery environment.

### 3.2 P3 responsibilities

P3 continues to own:

- P3 application metadata and release identity.
- Chart repository access and chart-version enumeration.
- Chart content resolution and digest verification.
- Per-request kubeconfig consumption through `credentials.Consumer`.
- Per-request Helm client construction.
- Helm upgrade, status, revision, and history behavior.
- Helm and Kubernetes error normalization.
- Human-initiated rollback through the existing P3 API and RBAC gate.

P6 must not import a P3 private repository, query a P3 table, construct a Helm
client, read a Helm Secret, or use an internal HTTP call in place of a Go seam.

### 3.3 Cross-module seams

Interfaces are defined by their consumers and wired only in
`cmd/server/main.go`.

P6 consumes narrow P3 capabilities for:

- Reading minimal application identity and safe display metadata.
- Listing and resolving a chart version to repository, chart name, version,
  and digest.
- Inspecting whether an existing release is installed and reading its safe
  status, revision, and deployed digest.
- Upgrading an existing release to a verified digest while reusing its current
  values.
- Acquiring the P3-owned release operation coordinator with a stable operation
  ID.

P3 consumes narrow P6 capabilities for:

- Determining whether an application is delivery-managed.
- Authorizing a managed application's internal delivery operation.
- Counting active environment bindings before application deletion.

If the governance lookup fails, mutations on a known delivery-managed
application fail closed. An ordinary unbound P3 application retains its prior
behavior.

## 4. Credentials and trust boundaries

P6 does not hold or call `credentials.Consumer` in the MVP.

- P3 obtains kubeconfig material through `credentials.Consumer` for each Helm
  request and discards it as before.
- P6 never references an SSH key or HTTP credential.
- P6 never reads, decrypts, caches, records, or audits credential material.
- P4 `assets.Consumer` is not a dependency.

The P3 credential-consumption purpose is a bounded identifier such as
`delivery.run.<run-id>.stage.<stage-id>`. The original run initiator remains
available for credential-access attribution, while delivery state events
identify the actual advancing actor as the system worker.

The worker is part of the trusted Optimus control plane, not a remote runner.
It accepts only validated database work and a fixed typed request containing
the P3 application ID, immutable artifact reference, run/stage/operation IDs,
timeout, and initiator ID. No user-controlled command, script, image, URL,
mount, environment variable, or Helm flag can be represented.

An internal orchestration authorization is carried in process through a typed
context or capability object. It is not accepted from HTTP, is not exposed to
the frontend, and is not persisted as a reusable bearer string.

## 5. Users and principal workflow

P6 uses composable permissions rather than hard-coded business roles. The
expected duties are:

- **Pipeline administrator**: manages projects, environment bindings, and
  pipeline versions.
- **Release operator**: resolves an artifact and creates a run.
- **Approver**: approves or rejects an eligible stage.
- **Observer**: reads configuration, run history, and safe event timelines.

The principal workflow is:

1. An administrator creates a delivery project.
2. The administrator binds existing, already-installed P3 applications to
   environments such as development, staging, and production.
3. The administrator publishes an ordered pipeline version and marks the
   stages that require approval.
4. An operator selects a chart version. P3 resolves it to a content digest and
   validates that every target uses the same chart identity.
5. P6 creates an immutable run and stage snapshots.
6. Each stage either waits for approval or becomes executable immediately.
7. The worker asks P3 to upgrade the existing release to the fixed digest while
   reusing that environment's current values.
8. Success advances to the next environment. Failure, rejection, cancellation,
   timeout, or an unknown outcome stops the run.
9. Recovery uses P3's existing rollback flow, P6 reconciliation, and, when
   allowed, a new linked retry run.

If a stage requires approval, the run initiator may not approve it. An
administrator is not exempt. Environments that do not require separation may
be configured without an approval stage.

## 6. Data model

### 6.1 `delivery_projects`

Represents one logical deployable service across environments.

Principal fields are `id`, `name`, `description`, nullable owner user ID,
timestamps, and `deleted_at`. Active names are unique. This is a delivery
aggregate only; it does not introduce a platform-wide tenant or project model.

### 6.2 `delivery_environments`

Represents a named environment within a project. Principal fields are `id`,
`project_id`, stable environment key, display name, `application_id`,
timestamps, and `deleted_at`.

Active environment keys are unique within a project. An active P3 application
may be bound to at most one active delivery environment globally. Binding
requires the application to exist, have an installed release, and have a chart
identity compatible with the project.

### 6.3 `delivery_pipelines`

Represents one immutable published pipeline version. Principal fields are
`id`, `project_id`, monotonically increasing version, creator ID, publication
time, and whether it is the project's current version.

One project has exactly one current published version after initial setup.
Publishing a new version never modifies an older version.

### 6.4 `delivery_pipeline_stages`

Represents an ordered stage in one pipeline version. Principal fields are
`id`, `pipeline_id`, `environment_id`, unique contiguous order, executor type,
approval-required flag, and bounded timeout.

The only allowed executor type is `helm_upgrade_existing_release`. One
environment may appear at most once in a pipeline version.

### 6.5 `delivery_runs`

Represents one immutable promotion attempt. Principal fields include:

- Project ID and pipeline ID/version.
- Chart repository identity, chart name, chart version, and content digest.
- Initiator ID.
- Idempotency key and request fingerprint.
- Overall state and timestamps.
- Optional `retry_of_run_id`.
- Safe error code/message key and correlation ID.

The idempotency key is unique in the initiator-plus-project scope. Repeating an
identical request returns the existing run. Reusing the key with a different
fingerprint returns a conflict.

### 6.6 `delivery_run_stages`

Contains the immutable execution snapshot for each stage: environment display
fields, P3 application ID and release identity, order, approval rule, timeout,
state, stable operation ID, lease owner/expiry, safe result revision/digest,
timestamps, and safe error fields.

Changes to environments or later pipeline versions cannot change a run-stage
snapshot.

### 6.7 `delivery_approvals`

Represents one stage approval request and its terminal decision. Principal
fields include run/stage IDs, request time, decision, decision-maker ID,
bounded comment, and decision time.

A decision is immutable. The first valid decision wins. A repeated identical
command returns the existing decision; a conflicting decision returns a
conflict.

### 6.8 `delivery_run_events`

An append-only event timeline with a monotonically increasing ID, run/stage
IDs, event type, old/new state, actor type and nullable actor ID, timestamp,
safe error code/message key, correlation ID, and strictly bounded structured
metadata.

### 6.9 Persistence invariants

- At most one active run exists per project.
- At most one active P3 release mutation exists per application.
- One active application binding exists globally per P3 application.
- A run's pipeline snapshot and artifact reference never change.
- Approval decisions and terminal run history are never overwritten.
- Soft deletion and snapshot display fields preserve historical readability.
- No P6 table contains Helm values, a credential, a script, a command, an
  arbitrary image, an arbitrary environment variable, a manifest, Helm notes,
  or raw executor output.

## 7. Artifact and values semantics

Run creation uses P3 to resolve a selected chart version into:

```text
repository identity + chart name + chart version + content digest
```

All run stages use the same digest. P3 re-resolves or verifies the digest before
each upgrade so a mutable OCI tag or replaced HTTP chart cannot silently change
the promoted content. A chart-name mismatch or digest mismatch stops the run
before mutation.

Every target application must already have an installed Helm release. P6 does
not install a missing release. P3 upgrades with reuse-values semantics so the
environment retains its existing configuration. P6 never accepts, stores, or
displays values YAML. Environment-specific configuration and Secret references
are established through P3 before the application enters delivery governance.

## 8. Pipeline and state machine

### 8.1 Run states

Non-terminal states are:

- `queued`
- `running`
- `waiting_approval`
- `cancel_requested`
- `reconciling`

Terminal states are:

- `succeeded`
- `failed`
- `rejected`
- `canceled`
- `timed_out`
- `outcome_unknown`

### 8.2 Stage states

Normal progression is:

```text
pending -> waiting_approval -> queued -> running -> reconciling -> succeeded
```

An approval-free stage skips `waiting_approval`. `reconciling` is entered only
when an external result needs confirmation and may transition to a definite
terminal result. Exceptional terminal stage states are `failed`, `rejected`,
`canceled`, `timed_out`, and `outcome_unknown`.

All transitions use an expected-old-state conditional update. The state change
and corresponding event append occur in the same database transaction. SSE
publishes only committed events.

### 8.3 Durable work and leases

The worker claims a queued stage by transactionally setting a short database
lease. It renews the lease while working. A process that loses the lease must
stop claiming ownership of the result. After lease expiry, another worker may
claim the stage only by entering reconciliation before considering another
mutation.

The P3 operation coordinator applies to both direct P3 mutations and P6 calls.
It uses the stable delivery operation ID to detect repeats and serializes
mutations per application across Optimus instances.

Global worker concurrency, lease duration, renewal interval, default/max stage
timeout, and reconciliation interval are bounded configuration values validated
at startup.

## 9. Approval semantics

A stage marked `approval_required` creates one approval request. One authorized
decision is sufficient.

- Approval and rejection both require `delivery:approval:decide` at decision
  time.
- The run initiator cannot decide any approval in that run.
- Approval and rejection require a bounded comment.
- The first valid decision is terminal and immutable.
- Rejection terminates the stage and run.
- A retry run creates fresh approvals even when the original stage was already
  approved.

The MVP uses global Optimus RBAC. It does not provide per-project or
per-environment approver assignment.

## 10. Cancellation, timeout, reconciliation, and retry

Cancellation is cooperative:

- A pending, waiting, or queued stage can be canceled without contacting P3.
- A running stage receives context cancellation, but P6 does not assume that
  Helm or the API server stopped the mutation.
- A running cancellation, deadline, connection loss, lease loss, or process
  crash enters reconciliation.

Reconciliation asks P3 for the installed revision and digest:

- If the target digest is deployed, the stage succeeds.
- If P3 can prove the operation did not take effect, the stage reaches the
  appropriate failed, canceled, or timed-out terminal state.
- If the result cannot be determined reliably, the stage and run become
  `outcome_unknown`.

An `outcome_unknown` run blocks new runs for the project and further P6
mutation of the target application until reconciliation produces a definite
result. P6 never lets an operator edit the old run to force success.

P6 performs no automatic retry. An authorized user may create a linked retry
run after state reconciliation. It uses the original digest, starts at the
failed environment, excludes already-successful earlier environments, and
requires all applicable approvals again.

## 11. Failure recovery and rollback

Any exceptional stage terminal state stops later environments. Earlier
environments that succeeded remain at the promoted version. P6 does not model
cross-environment atomic deployment and does not automatically compensate.

Rollback remains a P3 operation. The P6 run detail links to the existing P3
release detail and rollback flow. A P3 rollback does not rewrite the P6 run.
P6 reconciliation later records the observed revision/digest and reports drift
from the run target.

For a delivery-managed application:

- Direct P3 install, upgrade, and uninstall are denied.
- A delivery upgrade requires a valid in-process orchestration capability and
  stable operation ID.
- Direct P3 rollback remains available as the explicitly audited, RBAC-gated
  recovery path.

Unbinding an environment requires no active run, no unresolved unknown outcome,
and `delivery:project:write`. After unbinding, normal P3 behavior resumes.

## 12. API design

All JSON endpoints are under `/api/v1`, use the standard Optimus envelope, and
return stable numeric errors and i18n message keys.

### 12.1 Projects and environments

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/delivery/projects` | `delivery:project:read` | Paginated projects |
| POST | `/delivery/projects` | `delivery:project:write` | Create project |
| GET | `/delivery/projects/:id` | `delivery:project:read` | Project detail |
| PUT | `/delivery/projects/:id` | `delivery:project:write` | Update safe metadata |
| DELETE | `/delivery/projects/:id` | `delivery:project:delete` | Soft-delete eligible project |
| GET | `/delivery/projects/:id/environments` | `delivery:project:read` | List bindings |
| POST | `/delivery/projects/:id/environments` | `delivery:project:write` | Bind application |
| PUT | `/delivery/projects/:id/environments/:environmentId` | `delivery:project:write` | Update binding metadata |
| DELETE | `/delivery/projects/:id/environments/:environmentId` | `delivery:project:write` | Unbind eligible environment |

### 12.2 Pipeline and artifacts

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/delivery/projects/:id/pipeline` | `delivery:pipeline:read` | Current version and stages |
| PUT | `/delivery/projects/:id/pipeline` | `delivery:pipeline:write` | Validate and publish a new version |
| GET | `/delivery/projects/:id/artifacts` | `delivery:run:create` | List and resolve deployable versions |

Pipeline `PUT` never edits an existing version.

### 12.3 Runs, events, and approvals

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/delivery/projects/:id/runs` | `delivery:run:read` | Paginated run history |
| POST | `/delivery/projects/:id/runs` | `delivery:run:create` | Create idempotent run |
| GET | `/delivery/runs/:id` | `delivery:run:read` | Run and stage detail |
| POST | `/delivery/runs/:id/cancel` | `delivery:run:cancel` | Request cancellation |
| POST | `/delivery/runs/:id/reconcile` | `delivery:run:create` | Request safe reconciliation |
| POST | `/delivery/runs/:id/retry` | `delivery:run:create` | Create linked retry run |
| GET | `/delivery/runs/:id/events` | `delivery:run:read` | Authenticated SSE timeline |
| GET | `/delivery/approvals/pending` | `delivery:approval:read` | User's actionable queue |
| POST | `/delivery/run-stages/:id/approve` | `delivery:approval:decide` | Approve stage |
| POST | `/delivery/run-stages/:id/reject` | `delivery:approval:decide` | Reject stage |

Creating a run requires an `Idempotency-Key` header. The server computes its
own canonical request fingerprint.

The SSE endpoint follows the P2 authenticated `fetch` plus `ReadableStream`
pattern, not `EventSource`. It supports an event cursor and `Last-Event-ID`, has
connection and payload limits, and falls back to bounded polling in the
frontend.

## 13. Frontend design

P6 adds four primary surfaces:

1. **Delivery projects**: paginated search, creation, and status summary.
2. **Project detail**: project metadata, environment bindings, pipeline
   publication, artifact selection, run creation, and run history.
3. **Run detail**: immutable artifact summary, stage timeline, approval history,
   safe errors, cancel/reconcile/retry controls, and links to P3 recovery.
4. **My approvals**: pending actionable approvals and prior decisions.

The pipeline editor is a typed ordered-stage form. It is not a YAML editor,
script editor, drag-and-drop DAG builder, or generic task designer. Publishing
uses a validate-and-confirm flow.

Before run creation, the user sees the fixed chart version/digest, target
environment order, and approval points. Route metadata and every concrete
control use their exact permissions through both the router gate and
`v-permission`. The backend remains authoritative.

SSE state uses a run-specific abort generation. An old definition request,
event stream, reconnect, or poll response cannot update a newer run view.
All user-facing strings preserve `zh-CN` and `en-US` parity.

## 14. RBAC

P6 adds these permissions:

- `delivery:project:read`
- `delivery:project:write`
- `delivery:project:delete`
- `delivery:pipeline:read`
- `delivery:pipeline:write`
- `delivery:run:read`
- `delivery:run:create`
- `delivery:run:cancel`
- `delivery:approval:read`
- `delivery:approval:decide`

Environment mutation uses project write. Reconcile and retry use run create.
Artifact enumeration also uses run create so a project observer cannot probe
deployable repository versions.

Run creation does not imply cancellation. Approval read does not imply decide.
No P6 permission is granted automatically to the existing `k8s_operator` role.
Administrators explicitly compose operator and approver roles. The current
global RBAC model applies to all delivery projects; resource-scoped ACL is
deferred.

## 15. Audit and sensitive-data handling

### 15.1 Human control-plane audit events

P0 audit records:

- `delivery.project.create/update/delete`
- `delivery.environment.bind/update/unbind`
- `delivery.pipeline.publish`
- `delivery.run.create/cancel/reconcile/retry`
- `delivery.approval.approve/reject`

Safe audit metadata may contain project, pipeline version, environment,
application, run, stage, chart version, and digest identifiers. An approval
comment is stored only in the access-controlled approval row. Audit stores only
whether it exists and a bounded SHA-256 fingerprint.

### 15.2 Machine state events

Run events represent queued, started, waiting approval, approved, rejected,
cancel requested, reconciling, succeeded, failed, canceled, timed out, and
unknown-outcome transitions. A worker event uses actor type `system`; it does
not impersonate the initiating user.

### 15.3 Forbidden data

The following must never enter P6 tables, run events, audit metadata, SSE,
Swagger examples, client errors, or ordinary application logs:

- Helm values.
- Kubernetes Secret data.
- Any P1 credential or kubeconfig.
- Authorization headers or registry tokens.
- Rendered manifests or Helm notes.
- Raw Helm, Kubernetes, network, or executor errors.

Client-visible failure data consists of a stable numeric code, i18n message key,
stage identity, and correlation ID. Internal code may inspect a raw upstream
error transiently to classify it, but must not persist or return the raw text.

## 16. Error taxonomy

P0 through P5 occupy domain ranges through `44xxx`; P6 reserves `45xxx`:

- `45001-45099`: project, environment, and pipeline validation or conflict.
- `45101-45199`: run state, active-run conflict, idempotency, and cancellation.
- `45201-45299`: approval missing, unauthorized self-approval, already decided,
  or decision conflict.
- `45301-45399`: execution busy, artifact drift, timeout, reconciliation, or
  unknown outcome.

P3 keeps its `42xxx` errors internally. P6 maps them to the safe P6 execution
taxonomy when storing a stage result. Unexpected handler errors use the
existing generic internal envelope behavior.

## 17. Event streaming and retention

P6 does not capture executor stdout or stderr. P3 returns structured results;
P6 streams only persisted structured state events.

The client first fetches the run snapshot, then consumes committed events. On
reconnect it supplies the last event ID and fills any gap. Each event has a
strict serialized-size bound, and the endpoint enforces connection,
heartbeat, rate, and duration limits.

Run, stage, and approval terminal summaries are retained as release history.
Detailed run events are retained for 180 days by a bounded background pruning
job. After pruning, the run detail still shows the artifact, revisions, stage
terminal states, approvals, timestamps, and safe errors. P0 audit retention is
independent and P6 never deletes P0 audit records. Per-project retention is not
part of the MVP.

## 18. Configuration and lifecycle

P6 introduces bounded global configuration for:

- Worker concurrency.
- Lease duration and renewal interval.
- Default and maximum stage timeout.
- Reconciliation interval.
- Run-event retention days.
- SSE connection and heartbeat limits.

Invalid relationships, such as a renewal interval greater than the lease
duration or a default timeout above the maximum, fail startup.

Graceful shutdown stops claiming new stages and cancels local execution
contexts. Any in-flight external operation whose result is not confirmed is
left for lease-expiry reconciliation, never marked failed merely because the
process stopped.

## 19. Testing strategy

### 19.1 Backend unit tests

- Table-drive every legal and illegal run/stage transition.
- Use fake clocks and fake P3 seams for approval, cancellation, timeout, lease
  loss, crash recovery, reconciliation, retry, and digest drift.
- Verify self-approval denial, real-time permission checks, concurrent approval
  decisions, and idempotency fingerprints.
- Verify error mapping and all sensitive-data redaction boundaries.
- Verify managed-application policy fails closed.

### 19.2 PostgreSQL integration tests

- Partial uniqueness for active projects, environments, runs, and mutations.
- Foreign keys, soft deletion, and historical snapshots.
- Two workers racing to claim a stage.
- Lease expiry and takeover.
- Stable operation-ID replay.
- Conditional state transitions under concurrency.
- Atomic state transition plus event append.

### 19.3 P3 seam and governance tests

- Resolve and verify chart digests.
- Reject missing releases and chart-identity mismatches.
- Reuse existing values without exposing them to P6.
- Serialize direct P3 and P6 mutations through one coordinator.
- Deny direct install, upgrade, and uninstall for managed applications.
- Preserve audited P3 rollback as the recovery path.
- Block deletion of a bound P3 application.

### 19.4 Frontend tests

- API/store and normalized state behavior.
- SSE resume, polling fallback, and stale-generation protection.
- Permission matrices and self-approval UI denial.
- Pipeline validation and publication.
- Run confirmation, approval, rejection, cancellation, reconciliation, and
  retry controls.
- `zh-CN`/`en-US` parity, lint, typecheck, Vitest, and production build.

### 19.5 Full verification gates

Backend gates remain:

```bash
make test
make test-int
make lint
make swag
make swagger-diff
make dump-perms
make perm-check
```

Frontend gates remain:

```bash
bun install --frozen-lockfile
bun run lint
bun run typecheck
bun run i18n:check
bun run test
bun run build
```

## 20. Manual smoke test

`optimus-be/scripts/p6-smoke.md` will use disposable PostgreSQL, a local
Kubernetes cluster, and a local test chart repository. It requires no
production credential, production cluster, SCM account, or external artifact
service.

The smoke procedure will:

1. Create and pre-install development, staging, and production releases through
   P3.
2. Bind them to a delivery project and publish an approval pipeline.
3. Resolve one chart digest and promote it through all environments.
4. Verify self-approval denial, approval, rejection, cancellation, idempotent
   replay, and concurrent-run rejection.
5. Restart the worker during an upgrade and verify lease takeover plus
   reconciliation.
6. Cause a failure, use P3 rollback, detect drift in P6, and create a linked
   retry run.
7. Verify managed applications cannot be upgraded or uninstalled directly
   through P3.
8. Inspect API, database, audit, logs, and SSE output for forbidden sensitive
   data.

## 21. Principal risks and mitigations

| Risk | Mitigation |
|---|---|
| Reimplementing P3 Helm lifecycle | P6 uses narrow P3 seams; all Helm behavior remains in P3. |
| Bypassing approval through P3 | Delivery-managed policy blocks direct install, upgrade, and uninstall. |
| Arbitrary code execution | Closed executor enum and a schema with no command, script, image, or arbitrary input fields. |
| Credential or values leakage | P6 directly consumes no credentials and never stores values, manifests, notes, or raw errors. |
| Mutable chart tag or replaced HTTP chart | Freeze digest at run creation and verify it before every stage. |
| Duplicate mutation after crash | Database lease, P3 operation coordinator, stable operation ID, and reconciliation. |
| Approval race or self-approval | Conditional updates, live RBAC check, immutable first decision, and initiator denial. |
| False terminal status after timeout | Reconcile actual release revision/digest; use `outcome_unknown` when proof is unavailable. |
| Manual P3 rollback creates drift | Keep immutable P6 history and surface drift through reconciliation. |
| Scope expands into generic CI/CD | Explicit non-goals and no extensible task representation in the MVP. |

The following are release-blocking design violations: direct P6 credential
consumption, access to another module's private repository/table, a second Helm
lifecycle implementation, an arbitrary execution field, a managed-application
approval bypass, non-durable state advancement, or an external result inferred
without reconciliation.

