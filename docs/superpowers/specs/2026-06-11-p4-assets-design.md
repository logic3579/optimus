# P4 — assets Design

**Status**: Spec
**Date**: 2026-06-11
**Owner**: P4 sub-project
**Depends on**: P0 platform-skeleton (merged), P1 credentials-vault (merged)
**Downstream**: P5 observability, P6 cicd (both consume via the §7 Go seam)

---

## 1. Goal and scope

P0/P1/P2/P3 already shipped. P1 stored cloud access keys with the explicit downstream promise: "**used by P4 (asset discovery)**" (`docs/superpowers/specs/2026-06-10-p1-credentials-vault-design.md` §1, line 17). P4 fulfills that promise.

P4 is a **read-only cloud-asset discovery console**: a background worker consumes P1 cloudkeys to enumerate AWS resources, persists snapshots in Postgres, and exposes them as filterable list pages. It introduces no write/mutate path against the cloud, mirroring P2's "read-only k8s console" philosophy.

**Scope**:

1. A new user-CRUD entity `CloudAccount` binding `(cloudkey, enabled_regions[])` — the user-facing configuration unit.
2. Background sync engine driven by an in-process `robfig/cron` v3 scheduler. Each tick sweeps every enabled `CloudAccount` × each `enabled_region` × three resource types: **EC2 instances**, **VPCs (+ subnets)**, **RDS databases**.
3. AWS SDK Go v2 integration with per-request client factory (no client caching across rounds). Error normalization via `MapError` into 7 codes in the 43xxx segment.
4. Sweep semantics: **authoritative full-page sweep + soft-delete by `last_seen_at`**. Partial sweeps never soft-delete.
5. Manual sync trigger endpoint (`POST /api/v1/assets/cloud-accounts/{id}/sync`) — async; handler returns 200 immediately, FE polls `sync_runs`.
6. `sync_runs` history table + read-only page for operability.
7. Five permission codes (`assets:account:{read,write,delete}` + `assets:resource:read` + `assets:sync:read`); built-in `admin`/`viewer` roles pick up via existing wildcard/LIKE grants.
8. Fifteen new error codes in segment 43xxx (cloud-account domain 43001-43099 — 8 codes; sync/AWS 43100-43199 — 7 codes).
9. Five FE pages (cloud-accounts + 3 resource lists + sync-runs) reusing P3 components/hooks.
10. Internal `assets.Consumer` Go seam (no HTTP) for P5/P6 to look up instances by IP, by ID, by VPC.
11. P1 patch: `credentials/cloudkey.Service.Delete` refuses while a `CloudAccount` references the key (mirrors P1/P2 `kubeconfig.Delete` ↔ `k8s.cluster.inuse` pattern).

**Out of scope** (deferred to P4.x or other sub-projects):

- GCP / Azure providers — `provider` column accepts `aws` only at MVP; 43005 `CodeAssetsProviderUnsupported` returned otherwise. Schema/enum reserved.
- Resource types beyond instance/vpc/database (S3, IAM, ELB, Route53, ECR, Lambda, …).
- Write/manage operations on AWS (start/stop instance, create VPC, …). P4 is read-only by design.
- Cost/billing surfaces.
- Cross-account/cross-region search aggregation beyond simple filter UI.
- Tag editing — tags are sync-from-AWS only, never written back.
- Real-time event-driven updates (EventBridge / CloudTrail ingestion) — purely poll-based.
- LocalStack-based integration tests for fetchers (decision: unit-test fetchers with fake SDK clients; integration tests cover DB layer only).

---

## 2. Architecture

### 2.1 High-level data flow

```
                ┌─────────────────────────────────────────────────────┐
                │              FE (Vue3 + AntdV)                      │
                │  /assets/cloud-accounts  (CRUD)                     │
                │  /assets/instances       (read-only list)           │
                │  /assets/vpcs            (read-only list + subnets) │
                │  /assets/databases       (read-only list)           │
                │  /assets/sync-runs       (read-only list)           │
                └────────────────┬────────────────────────────────────┘
                                 │ HTTPS Envelope<T>
                                 ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │   /api/v1/assets/cloud-accounts        (CRUD + POST :id/sync)    │
   │   /api/v1/assets/instances?account=&region=&q=                   │
   │   /api/v1/assets/vpcs?...     /api/v1/assets/vpcs/{id}/subnets   │
   │   /api/v1/assets/databases?...                                   │
   │   /api/v1/assets/sync-runs?account=&type=&status=                │
   │                                                                  │
   │   internal/modules/assets/                                       │
   │     account/   instance/   vpc/   database/                      │
   │           \         \       |       /                            │
   │            \         \      |      /                             │
   │             ▼         ▼     ▼     ▼                              │
   │           sync/  (engine + scheduler + fetchers + sync_runs)     │
   │             │                                                    │
   │             ▼                                                    │
   │     awsclient/  (per-request ec2.Client / rds.Client factory)    │
   │             │                                                    │
   │             ▼                                                    │
   │   credentials.Consumer.GetCloudKey()  ← P1 seam（唯一获取凭证路径）│
   │                                                                  │
   │   assets.Consumer (new seam)  →  P5/P6 import to query assets    │
   └──────────────────────────────────────────────────────────────────┘
                                 │
   ┌─────────────────────────────┴────────────────────────────────────┐
   │   PG: cloud_accounts, aws_instances, aws_vpcs, aws_subnets,      │
   │       aws_databases, assets_sync_runs                            │
   └──────────────────────────────────────────────────────────────────┘
```

The in-process `robfig/cron` v3 scheduler runs in a goroutine started from `cmd/server/main.go`, with a 30-second startup delay so the BE health endpoint stabilises first. Default schedule `*/15 * * * *` (configurable via `OPTIMUS_ASSETS_SYNC_CRON`).

### 2.2 BE module layout

```
optimus-be/internal/modules/assets/
├── errs/codes.go               # 43xxx error codes + message keys
├── awsclient/
│   ├── factory.go              # For(cloudkey, region) (*Clients, error); fresh per request
│   └── maperror.go             # MapError(error) (code int, key string, msg string)
├── account/
│   ├── dto.go
│   ├── repo.go                 # GORM; soft-delete aware
│   ├── service.go              # CRUD + manual-sync trigger
│   ├── handler.go              # HTTP; inline ParseUint; c.GetUint64(CtxKeyUserID)
│   └── inuse/inuse.go          # CountByCloudKeyID (consumed by P1 cloudkey.Delete)
├── instance/
│   ├── dto.go
│   ├── repo.go                 # read-only + include_deleted toggle
│   ├── service.go              # List/Get; no mutations
│   └── handler.go
├── vpc/
│   ├── dto.go
│   ├── repo.go                 # vpc + subnet (one repo handles both tables)
│   ├── service.go              # List vpcs / List subnets of vpc
│   └── handler.go
├── database/                   # symmetric structure
│   ├── dto.go
│   ├── repo.go
│   ├── service.go
│   └── handler.go
├── sync/
│   ├── engine.go               # RunAll / RunAccount + in-memory tryLock
│   ├── scheduler.go            # robfig/cron registration + 30s startup delay + retention sweep
│   ├── ec2_fetcher.go          # paginate DescribeInstances → []Instance
│   ├── vpc_fetcher.go          # paginates VPC + Subnet in one unit
│   ├── rds_fetcher.go          # paginate DescribeDBInstances
│   ├── sweep.go                # generic upsert + last_seen-based soft-delete
│   └── runs/
│       ├── dto.go
│       ├── repo.go
│       ├── service.go          # Insert/Finish/List + 90-day retention DELETE
│       └── handler.go          # GET /sync-runs only
├── consume.go                  # assets.Consumer interface + implementation
└── module/
    └── wire.go                 # Mount routes, register cron entries, build Consumer
```

The composition root remains `cmd/server/main.go`. `assets.module.Wire(...)` is called after P3's wiring; it receives `*gorm.DB`, `credentials.Consumer`, `*audit.Recorder`, the cron scheduler, and config, and returns the `assets.Consumer` (currently unused — P5/P6 will inject later).

---

## 3. Data model

Migration `optimus-be/migrations/00021_p4_assets.sql` (goose). All tables follow the project's `id BIGSERIAL PK / created_at / updated_at` convention; soft-delete columns explicit per table below. No foreign keys point AT external AWS-side IDs (those are strings unique only within `(cloud_account_id, region)`).

### 3.1 `cloud_accounts`

User-CRUD entity. Soft-deleted.

```sql
CREATE TABLE cloud_accounts (
  id              BIGSERIAL PRIMARY KEY,
  name            VARCHAR(128) NOT NULL,
  provider        VARCHAR(16) NOT NULL,                  -- 'aws' at MVP
  cloudkey_id     BIGINT NOT NULL REFERENCES credential_cloud_keys(id),
  enabled_regions TEXT[] NOT NULL,                       -- ['us-east-1','ap-northeast-1']
  enabled         BOOLEAN NOT NULL DEFAULT true,         -- false → cron skips this account
  description     TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_cloud_accounts_name_alive
    ON cloud_accounts(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_cloud_accounts_cloudkey ON cloud_accounts(cloudkey_id);
```

Notes:
- `enabled_regions` validation: every entry must match the AWS regional regex `^[a-z]{2}-[a-z]+-\d$`. Validation lives in `account.Service.Create/Update` (returns 43004 `CodeAssetsRegionInvalid`).
- `provider` validation: MVP accepts only `aws` (returns 43005 `CodeAssetsProviderUnsupported`).
- Partial unique index allows reusing a name after soft-delete (matches P0 menu/role pattern).

### 3.2 `aws_instances`

```sql
CREATE TABLE aws_instances (
  id                BIGSERIAL PRIMARY KEY,
  cloud_account_id  BIGINT NOT NULL REFERENCES cloud_accounts(id),
  region            VARCHAR(32) NOT NULL,
  instance_id       VARCHAR(32) NOT NULL,                -- i-0123...
  name              TEXT NOT NULL DEFAULT '',            -- from tag:Name
  instance_type     VARCHAR(32) NOT NULL DEFAULT '',
  state             VARCHAR(16) NOT NULL DEFAULT '',     -- running/stopped/...
  private_ip        INET,
  public_ip         INET,
  vpc_id            VARCHAR(32) NOT NULL DEFAULT '',     -- soft-link to aws_vpcs.vpc_id
  subnet_id         VARCHAR(32) NOT NULL DEFAULT '',
  availability_zone VARCHAR(32) NOT NULL DEFAULT '',
  launch_time       TIMESTAMPTZ,
  tags              JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at      TIMESTAMPTZ NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_instances_keytuple
    ON aws_instances(cloud_account_id, region, instance_id);
CREATE INDEX idx_aws_instances_vpc        ON aws_instances(vpc_id);
CREATE INDEX idx_aws_instances_private_ip ON aws_instances(private_ip);
CREATE INDEX idx_aws_instances_tags_gin   ON aws_instances USING GIN (tags);
CREATE INDEX idx_aws_instances_last_seen  ON aws_instances(last_seen_at);
```

### 3.3 `aws_vpcs`

```sql
CREATE TABLE aws_vpcs (
  id                BIGSERIAL PRIMARY KEY,
  cloud_account_id  BIGINT NOT NULL REFERENCES cloud_accounts(id),
  region            VARCHAR(32) NOT NULL,
  vpc_id            VARCHAR(32) NOT NULL,
  name              TEXT NOT NULL DEFAULT '',
  cidr_block        CIDR,
  is_default        BOOLEAN NOT NULL DEFAULT false,
  state             VARCHAR(16) NOT NULL DEFAULT '',
  tags              JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at      TIMESTAMPTZ NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_vpcs_keytuple ON aws_vpcs(cloud_account_id, region, vpc_id);
CREATE INDEX idx_aws_vpcs_last_seen      ON aws_vpcs(last_seen_at);
```

### 3.4 `aws_subnets`

```sql
CREATE TABLE aws_subnets (
  id                BIGSERIAL PRIMARY KEY,
  cloud_account_id  BIGINT NOT NULL REFERENCES cloud_accounts(id),
  region            VARCHAR(32) NOT NULL,
  subnet_id         VARCHAR(32) NOT NULL,
  vpc_id            VARCHAR(32) NOT NULL,
  cidr_block        CIDR,
  availability_zone VARCHAR(32) NOT NULL DEFAULT '',
  name              TEXT NOT NULL DEFAULT '',
  tags              JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at      TIMESTAMPTZ NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_subnets_keytuple ON aws_subnets(cloud_account_id, region, subnet_id);
CREATE INDEX idx_aws_subnets_vpc            ON aws_subnets(vpc_id);
CREATE INDEX idx_aws_subnets_last_seen      ON aws_subnets(last_seen_at);
```

### 3.5 `aws_databases`

```sql
CREATE TABLE aws_databases (
  id                  BIGSERIAL PRIMARY KEY,
  cloud_account_id    BIGINT NOT NULL REFERENCES cloud_accounts(id),
  region              VARCHAR(32) NOT NULL,
  db_instance_id      VARCHAR(64) NOT NULL,              -- DBInstanceIdentifier
  engine              VARCHAR(32) NOT NULL DEFAULT '',
  engine_version      VARCHAR(32) NOT NULL DEFAULT '',
  instance_class      VARCHAR(32) NOT NULL DEFAULT '',
  status              VARCHAR(32) NOT NULL DEFAULT '',
  endpoint            TEXT NOT NULL DEFAULT '',
  port                INT,
  multi_az            BOOLEAN NOT NULL DEFAULT false,
  publicly_accessible BOOLEAN NOT NULL DEFAULT false,
  storage_gb          INT,
  tags                JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at        TIMESTAMPTZ NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_databases_keytuple
    ON aws_databases(cloud_account_id, region, db_instance_id);
CREATE INDEX idx_aws_databases_last_seen ON aws_databases(last_seen_at);
```

### 3.6 `assets_sync_runs`

Append-only log; not soft-deleted (retention via 90-day DELETE cron — §5.3).

```sql
CREATE TABLE assets_sync_runs (
  id                   BIGSERIAL PRIMARY KEY,
  cloud_account_id     BIGINT NOT NULL REFERENCES cloud_accounts(id),
  region               VARCHAR(32) NOT NULL,
  resource_type        VARCHAR(16) NOT NULL,             -- 'instance' / 'network' / 'database'
  started_at           TIMESTAMPTZ NOT NULL,
  finished_at          TIMESTAMPTZ,
  status               VARCHAR(16) NOT NULL,             -- 'running' / 'success' / 'failed' / 'skipped'
  items_seen           INT NOT NULL DEFAULT 0,
  items_softdeleted    INT NOT NULL DEFAULT 0,
  error                TEXT NOT NULL DEFAULT '',
  error_code           INT NOT NULL DEFAULT 0,
  trigger              VARCHAR(16) NOT NULL,             -- 'cron' / 'manual' / 'test'
  triggered_by_user_id BIGINT REFERENCES users(id),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_assets_sync_runs_account_type
    ON assets_sync_runs(cloud_account_id, resource_type, started_at DESC);
CREATE INDEX idx_assets_sync_runs_status
    ON assets_sync_runs(status, started_at DESC);
```

`resource_type` value `'network'` covers the joint VPC + Subnet sweep (a single sync_run row represents one network sweep, with `items_seen` = vpcs + subnets summed and `items_softdeleted` likewise).

### 3.7 Model package

`internal/models/assets.go` defines the GORM struct counterparts. `name` of structs follows the no-stutter convention (`models.CloudAccount`, `models.AWSInstance`, `models.AWSVPC`, `models.AWSSubnet`, `models.AWSDatabase`, `models.AssetsSyncRun`).

---

## 4. Sync engine

### 4.1 Entry points

```go
package sync

// RunAll iterates all enabled, non-deleted CloudAccounts and runs every region × type
// sweep that is due. trigger is "cron" or "test".
func (e *Engine) RunAll(ctx context.Context, trigger string) error

// RunAccount runs every region × type sweep for a single account. trigger is
// "manual" (handler call) or "test". triggeredByUser is nil for cron/test.
func (e *Engine) RunAccount(ctx context.Context, accountID int64, trigger string, triggeredByUser *uint64) error
```

The scheduler calls `RunAll(ctx, "cron")`. The HTTP handler calls `RunAccount(ctx, id, "manual", &userID)` in a detached goroutine after acquiring the lock (§4.3).

### 4.2 Sweep semantics

For each `(account, region, resource_type)`:

1. `runID := runs.Insert(status="running", trigger=..., started_at=now())`.
2. `clients, err := awsclient.For(cloudkey, region)` — fresh per sweep.
3. `items, err := fetcher.FetchAll(ctx, clients)` — paginates internally until `nextToken == nil`. Any error → return with empty items.
4. **Authoritative gate**: only on `err == nil` do we proceed to step 5/6; otherwise step 7 finalises as `failed`.
5. `tx`: upsert each item ON CONFLICT update fields + `last_seen_at = sweep_start` + `deleted_at = NULL`. Within the same tx, `softdel := UPDATE … SET deleted_at = now() WHERE cloud_account_id=$ AND region=$ AND last_seen_at < sweep_start AND deleted_at IS NULL`. Commit.
6. VPC sweep variant: both `aws_vpcs` and `aws_subnets` upserted/soft-deleted in the **same tx**. Both must succeed; either failing rolls back both. (Why: prevents subnet rows pointing to a vpc_id that hasn't yet appeared in `aws_vpcs`.)
7. `runs.Finish(runID, finished_at=now(), status=..., items_seen=len(items), items_softdeleted=softdel, error=msg, error_code=code)`.

`status` mapping at step 7:
- `err == nil` → `success`.
- `err != nil` and any items upserted → `success` is incorrect; we never partially commit so this branch doesn't exist.
- `err != nil` (any fetch error) → `failed`, `items_seen=0`, `items_softdeleted=0`.
- `tryLock` failed at outer level → `skipped`, no fetch attempted.

### 4.3 In-memory per-account lock

```go
type Engine struct {
    locks sync.Map // map[int64]bool
}

func (e *Engine) tryLock(accountID int64) bool {
    _, loaded := e.locks.LoadOrStore(accountID, true)
    return !loaded
}

func (e *Engine) unlock(accountID int64) { e.locks.Delete(accountID) }
```

A `cron` tick that finds the lock held by an in-flight `manual` (or vice-versa) writes one `assets_sync_runs` row with `status="skipped"` for the first `(region, resource_type)` of the account and returns. (Single skipped row per account-per-tick, not per region — minimises log noise.)

`manual` trigger when busy: handler returns 409 `CodeAssetsSyncBusy` (43101); no sync_runs row written.

### 4.4 Manual trigger handler

```
POST /api/v1/assets/cloud-accounts/{id}/sync         perm: assets:account:write
  body: empty
  200: {code:0, data:{queued: true, started_at: "RFC3339"}}
  404: account not found / soft-deleted
  409: CodeAssetsSyncBusy
  422: account.enabled=false (CodeAssetsCloudAccountDisabled, 43006)
```

Flow:

1. Handler loads the account row; 404 / 422 short-circuits (not found / disabled) return immediately.
2. Handler calls `engine.tryLock(accountID)`; if it returns false, respond 409 `CodeAssetsSyncBusy` without spawning anything.
3. Handler writes the `assets.cloud_account.sync_trigger` audit row synchronously.
4. Handler spawns a goroutine and returns 200. **The goroutine owns the lock from this point**: it `defer`s `engine.unlock(accountID)` immediately as its first statement.
5. Goroutine context: `ctx := context.WithoutCancel(c.Request.Context())` so closing the HTTP connection doesn't cancel the sweep; then `ctx, cancel := context.WithTimeout(ctx, cfg.AWS_REQUEST_TIMEOUT * time.Duration(len(regions)) * 3)` as the outer ceiling; `defer cancel()`.
6. Goroutine calls `engine.RunAccount(ctx, accountID, "manual", &userID)`. Inside `RunAccount`, the lock is **assumed already held** (handler acquired it); `RunAccount` skips its own `tryLock`. A separate internal entry point `engine.runAccountLocked(ctx, ...)` is used by both the manual-handler path (after handler-side lock) and the `RunAll` loop (after per-account lock); the public `RunAccount` is reserved for tests and itself acquires the lock + delegates to `runAccountLocked`.

### 4.5 Scheduler

In `sync/scheduler.go`:

```go
func StartScheduler(ctx context.Context, cfg Config, engine *Engine, logger *slog.Logger) *cron.Cron {
    c := cron.New(cron.WithSeconds()) // 5-field or 6-field both accepted
    time.AfterFunc(cfg.StartupDelay, func() {
        c.AddFunc(cfg.SyncCron, func() {
            if err := engine.RunAll(context.Background(), "cron"); err != nil {
                logger.Error("assets.sync.cron.err", "err", err)
            }
        })
        c.AddFunc("0 3 * * *", func() {                  // 03:00 UTC daily
            engine.PruneSyncRuns(context.Background(), cfg.SyncRunRetentionDays)
        })
        c.Start()
    })
    go func() { <-ctx.Done(); c.Stop() }()
    return c
}
```

`engine.PruneSyncRuns(ctx, days)` runs a single SQL: `DELETE FROM assets_sync_runs WHERE started_at < now() - INTERVAL '<days> days'`.

### 4.6 Validation that sweep is "good enough" to soft-delete

A sweep counts as **authoritative** when:
- `fetcher.FetchAll` returns `err == nil`, AND
- the AWS SDK call completed paginating (no `NextToken` left).

This is reified by the contract of `FetchAll`: it iterates internally and only returns `(items, nil)` after pagination is exhausted. Any partial state surfaces as `err != nil`. Soft-delete therefore happens exactly when `err == nil` — no separate flag needed.

---

## 5. AWS SDK integration

### 5.1 Dependencies

Added to `optimus-be/go.mod`:

```
github.com/aws/aws-sdk-go-v2                    (latest v1.x compatible with go 1.25)
github.com/aws/aws-sdk-go-v2/config             (latest)
github.com/aws/aws-sdk-go-v2/credentials        (latest; for StaticCredentialsProvider)
github.com/aws/aws-sdk-go-v2/service/ec2        (latest)
github.com/aws/aws-sdk-go-v2/service/rds        (latest)
github.com/robfig/cron/v3                       (v3.0.1)
```

**Constraint**: every chosen version must keep `go.mod`'s `go` directive at `1.25` (matches the P2 client-go pin gotcha, `CLAUDE.md` "Conventions"). If a transitive dep tries to push it to 1.26, the offending pkg gets pinned (precedent: `helm.sh/helm/v3` pinned at v3.15.4 for the same reason).

### 5.2 Client factory

```go
// internal/modules/assets/awsclient/factory.go
package awsclient

type Clients struct {
    EC2 *ec2.Client
    RDS *rds.Client
}

func For(ctx context.Context, ck *credentials.CloudKey, region string, timeout time.Duration) (*Clients, error) {
    if ck.Provider != "aws" {
        return nil, apperr.New(errs.CodeAssetsProviderUnsupported, "assets.provider.unsupported",
            fmt.Sprintf("provider %q not supported", ck.Provider))
    }
    cfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion(region),
        config.WithCredentialsProvider(credentials_aws.NewStaticCredentialsProvider(
            ck.AccessKeyID, ck.SecretAccessKey, "")),
        config.WithRetryMaxAttempts(3),
        config.WithHTTPClient(&http.Client{Timeout: timeout}),
    )
    if err != nil {
        return nil, apperr.Wrap(err, errs.CodeAssetsAWSConfig, "assets.aws.config",
            "failed to build AWS SDK config")
    }
    return &Clients{EC2: ec2.NewFromConfig(cfg), RDS: rds.NewFromConfig(cfg)}, nil
}
```

The clientset is **never cached** between sweeps; constructed fresh each time. Mirrors P2 `k8s/client/factory.go`.

### 5.3 Error mapping

```go
// internal/modules/assets/awsclient/maperror.go
package awsclient

// MapError normalises an AWS SDK error into a 43xxx code + message_key + human message.
// Callers (fetchers / scheduler) store the triple in assets_sync_runs.
func MapError(err error) (code int, key string, msg string)
```

Mapping table:

| Match | Code | Key |
|---|---|---|
| `errors.As(*smithy.GenericAPIError)` with code `AuthFailure` / `InvalidClientTokenId` / `SignatureDoesNotMatch` / `ExpiredToken` | 43102 `CodeAssetsAWSUnauthorized` | `assets.aws.unauthorized` |
| Generic code `UnauthorizedOperation`, `AccessDenied`, `AccessDeniedException` | 43103 `CodeAssetsAWSForbidden` | `assets.aws.forbidden` |
| `errors.Is(err, context.DeadlineExceeded)`; net.Error / DNS error / TCP reset; smithy `RequestCanceled` | 43104 `CodeAssetsAWSUnreachable` | `assets.aws.unreachable` |
| Generic code `Throttling`, `ThrottlingException`, `RequestLimitExceeded`, `TooManyRequestsException` | 43105 `CodeAssetsAWSThrottled` | `assets.aws.throttled` |
| Any other API error | 43106 `CodeAssetsAWSOther` | `assets.aws.other` |
| SDK config failure (`LoadDefaultConfig` itself) | 43107 `CodeAssetsAWSConfig` | `assets.aws.config` |

The fetcher does NOT itself retry throttling beyond SDK's built-in `WithRetryMaxAttempts(3)`. If that exhausts, the run is marked `failed`; cron picks it up next tick. Throttle bursts that span multiple ticks are visible in `sync_runs` history.

### 5.4 Fetchers

Each fetcher is a struct with a `FetchAll(ctx, *Clients) ([]X, error)` method. Examples:

```go
// ec2_fetcher.go
type EC2Fetcher struct{}
func (EC2Fetcher) FetchAll(ctx context.Context, c *awsclient.Clients) ([]Instance, error) {
    var out []Instance
    p := ec2.NewDescribeInstancesPaginator(c.EC2, &ec2.DescribeInstancesInput{
        MaxResults: aws.Int32(100),
    })
    for p.HasMorePages() {
        page, err := p.NextPage(ctx)
        if err != nil { return nil, err }              // raw err; MapError applied by caller
        for _, r := range page.Reservations {
            for _, i := range r.Instances {
                out = append(out, instanceFromSDK(i))   // pure helper; extracts tag:Name, IPs, ...
            }
        }
    }
    return out, nil
}
```

`vpc_fetcher` returns `(vpcs []VPC, subnets []Subnet, error)` — both populated only on full success; otherwise both nil. Failing either DescribeVpcs or DescribeSubnets aborts the unit.

`rds_fetcher` paginates DescribeDBInstances similarly.

Tag extraction helpers live in `sync/tags.go`:
- `tagMap(tags []ec2types.Tag) map[string]string` (likewise for RDS Tag list).
- `tagName(tags) string` returns `tags["Name"]` or `""`.

---

## 6. assets.Consumer Go seam

Mirrors `credentials.Consumer` but **no `purpose` parameter, no per-call audit** (resource lookups are not security-sensitive, called per-request by P5/P6; auditing them would explode log volume).

### 6.1 Types

```go
// internal/modules/assets/consume.go
package assets

type Instance struct {
    AccountID   int64
    AccountName string
    Region      string
    InstanceID  string
    Name        string
    InstanceType string
    State       string
    PrivateIP   netip.Addr      // zero value if NULL
    PublicIP    netip.Addr
    VPCID       string
    SubnetID    string
}

type Consumer interface {
    LookupInstanceByPrivateIP(ctx context.Context, ip netip.Addr) (*Instance, error)
    LookupInstanceByID(ctx context.Context, accountID int64, region, instanceID string) (*Instance, error)
    ListInstancesByVPC(ctx context.Context, accountID int64, region, vpcID string) ([]Instance, error)
}
```

### 6.2 Behaviour

- All lookups exclude soft-deleted rows (`deleted_at IS NULL`).
- `LookupInstanceByPrivateIP` returns `ErrAssetsInstanceNotFound` (sentinel error in `assets/errs/`) when no row matches; ambiguous match (multiple accounts share the same private IP — possible across VPCs) returns the first by `last_seen_at DESC`. Callers needing strict uniqueness should pass account+region.
- Implementation joins `aws_instances` to `cloud_accounts` to populate `AccountName`.
- No HTTP endpoint. Constructed once in `cmd/server/main.go` and provided to downstream (P5/P6) modules at wire time.

### 6.3 Smoke test

`internal/modules/assets/consume_smoke_test.go` (build-tagged `dbtest`) seeds one cloud_account + one aws_instance and exercises all three lookup methods. Permanent integration test (mirrors P1 `consume_smoke_test.go`).

---

## 7. HTTP API surface

All routes mounted under `/api/v1/assets/`. Each registered via the nested-group `RequirePermission` middleware pattern (`cmd/server/main.go` precedent, `CLAUDE.md` "Architecture — backend"). Envelope shape: `{code, data, message, message_key?}` — no deviations.

### 7.1 CloudAccount

```
GET    /cloud-accounts                                                 assets:account:read
  query: q (name LIKE), provider, enabled (bool), include_deleted (bool, default false),
         page, size
  200: {items: [Summary], total}

GET    /cloud-accounts/{id}                                            assets:account:read
  200: {data: Detail}; 404 CodeAssetsCloudAccountNotFound (43002)

POST   /cloud-accounts                                                 assets:account:write
  body: { name, provider:"aws", cloudkey_id, enabled_regions: [...], enabled?, description? }
  200: {data: Detail}
  422: 43003 CodeAssetsCloudAccountNameConflict / 43004 CodeAssetsRegionInvalid /
       43005 CodeAssetsProviderUnsupported / 43008 CodeAssetsCloudKeyNotFound

PUT    /cloud-accounts/{id}                                            assets:account:write
  body: { name?, enabled_regions?, enabled?, description? }
        (provider and cloudkey_id are immutable post-create — change requires delete+recreate
         because the bind point is the credential identity)
  200: {data: Detail}
  422 same as POST
  Side-effect on enabled_regions shrinkage: when a region present on the prior row is absent
  in the new enabled_regions, the service soft-deletes (deleted_at=now()) all aws_instances /
  aws_vpcs / aws_subnets / aws_databases rows where (cloud_account_id=$id AND region=$removed).
  Reason: future sweeps will not visit removed regions, so the engine's last_seen_at-based
  soft-delete never fires for them; without this explicit cleanup, removed-region resources
  linger as live rows forever. Audit payload includes regions_removed:[...] in the changed_fields.

DELETE /cloud-accounts/{id}                                            assets:account:delete
  Tx: soft-delete account + cascade soft-delete all aws_instances / aws_vpcs / aws_subnets /
      aws_databases rows where cloud_account_id = $.
      No effect on assets_sync_runs (append-only history).
  200: {data: {cascaded_resources_count: N}}

POST   /cloud-accounts/{id}/sync                                       assets:account:write
  Async — see §4.4.
```

`Summary` vs `Detail`: Detail includes `enabled_regions`, `description`, `cloudkey_name` (joined), `last_sync_at` (max started_at across resource types), `last_sync_status` (status of the most recent sync_run); Summary has the same fields minus `enabled_regions` array (just `regions_count`).

### 7.2 Resource lists

```
GET /instances                                                         assets:resource:read
  query: account_id, region, state, vpc_id, q (matches name / instance_id / private_ip / tag values),
         include_deleted (bool, default false), page, size
  Order: deleted_at IS NULL DESC, last_seen_at DESC.

GET /vpcs                                                              assets:resource:read
  query: account_id, region, q (matches name / vpc_id), include_deleted, page, size

GET /vpcs/{id}/subnets                                                 assets:resource:read
  Returns subnets belonging to the VPC referenced by the row at id (lookup vpc_id +
  cloud_account_id + region from aws_vpcs, then list aws_subnets matching).
  200: {items: [Subnet], total}
  404 CodeAssetsVPCNotFound (43009)

GET /databases                                                         assets:resource:read
  query: account_id, region, engine, status, q (matches db_instance_id / endpoint),
         include_deleted, page, size
```

### 7.3 sync_runs

```
GET /sync-runs                                                         assets:sync:read
  query: account_id, resource_type, status, started_after (RFC3339), page, size
  Order: started_at DESC.
```

No POST/PUT/DELETE on sync-runs.

---

## 8. Permission codes

Appended to `internal/infra/permissions/codes.go`:

```go
PermAssetsAccountRead   = "assets:account:read"
PermAssetsAccountWrite  = "assets:account:write"   // create / update / manual-sync trigger
PermAssetsAccountDelete = "assets:account:delete"
PermAssetsResourceRead  = "assets:resource:read"   // covers instance, vpc, database, subnet
PermAssetsSyncRead      = "assets:sync:read"
```

Five codes total. Registered via `permissions.Register(ctx, db, permissions.All)` at startup as usual.

**Role auto-pickup**:
- `admin` role grants wildcard `*` → all five codes.
- `viewer` role grants `%:read` LIKE → picks up `assets:account:read`, `assets:resource:read`, `assets:sync:read`.
- `assets:account:write` and `assets:account:delete` are admin-only by default.

No new built-in roles created. (Sticks with CLAUDE.md note that built-in roles are `admin` + `viewer` only.)

`make dump-perms` regenerates `docs/permissions.md`; CI `make perm-check` enforces parity.

---

## 9. Error codes (segment 43xxx)

Fifteen codes total. Defined in `internal/modules/assets/errs/codes.go` and re-exported from `internal/infra/errors/codes.go` for handler use.

### 9.1 Domain — cloud account (43001-43099)

| Code | Constant | Message key | HTTP | When |
|---|---|---|---|---|
| 43001 | CodeAssetsCloudAccountInUse | assets.cloud_account.in_use | 409 | P1 cloudkey.Delete blocked because a CloudAccount references the key |
| 43002 | CodeAssetsCloudAccountNotFound | assets.cloud_account.not_found | 404 | Any account ID lookup miss |
| 43003 | CodeAssetsCloudAccountNameConflict | assets.cloud_account.name_conflict | 422 | Create/Update name collides with alive row |
| 43004 | CodeAssetsRegionInvalid | assets.region.invalid | 422 | An enabled_regions entry fails the regex |
| 43005 | CodeAssetsProviderUnsupported | assets.provider.unsupported | 422 | provider != "aws" |
| 43006 | CodeAssetsCloudAccountDisabled | assets.cloud_account.disabled | 422 | Manual-sync invoked on enabled=false account |
| 43008 | CodeAssetsCloudKeyNotFound | assets.cloudkey.not_found | 422 | Create/Update references missing cloudkey_id |
| 43009 | CodeAssetsVPCNotFound | assets.vpc.not_found | 404 | `/vpcs/{id}/subnets` row lookup miss |

Gap at 43007 reserved.

### 9.2 Sync / AWS (43100-43199)

| Code | Constant | Message key | HTTP | When |
|---|---|---|---|---|
| 43101 | CodeAssetsSyncBusy | assets.sync.busy | 409 | Manual sync invoked while account lock held |
| 43102 | CodeAssetsAWSUnauthorized | assets.aws.unauthorized | n/a (internal) | Sweep got AuthFailure / InvalidClientTokenId / etc. |
| 43103 | CodeAssetsAWSForbidden | assets.aws.forbidden | n/a | IAM denied the API call |
| 43104 | CodeAssetsAWSUnreachable | assets.aws.unreachable | n/a | Timeout / DNS / network |
| 43105 | CodeAssetsAWSThrottled | assets.aws.throttled | n/a | Throttling exhausted SDK retries |
| 43106 | CodeAssetsAWSOther | assets.aws.other | n/a | Any other API error |
| 43107 | CodeAssetsAWSConfig | assets.aws.config | n/a | SDK config.LoadDefaultConfig failed |

"n/a (internal)" codes are written to `assets_sync_runs.error_code` and surfaced in the FE Sync Runs list / tooltip; they are not returned as HTTP error envelopes.

43200+ reserved for future P4.x provider expansion (GCP / Azure).

---

## 10. Audit

Existing shared `audit.Recorder` (do not construct a second one — `CLAUDE.md` "Conventions"). Service mutation methods pass `(ctx, actorID, ip, ua, ...)` per the P3 pattern.

| Action | TargetType | TargetID | Payload |
|---|---|---|---|
| `assets.cloud_account.create` | `cloud_account` | account_id (string) | `{name, provider, cloudkey_id, regions:[...]}` |
| `assets.cloud_account.update` | `cloud_account` | account_id | `{changed_fields:[...], regions:[...] if changed}` |
| `assets.cloud_account.delete` | `cloud_account` | account_id | `{name, cascaded_resources_count: N}` |
| `assets.cloud_account.sync_trigger` | `cloud_account` | account_id | `{regions:[...], trigger:"manual"}` |

`audit.Event.UserID` is `*uint64`; cron and test triggers leave it nil (no audit event written at all for cron). Manual sweeps record `sync_trigger` once per manual click (not per region/type). Per-sweep granular records live in `assets_sync_runs`, not audit.

The `credentials.Consumer.GetCloudKey(ctx, id, "assets.sync")` call inside the sweep generates a `credentials.consume.cloud_key` audit row from P1's existing audit plumbing — no duplication.

---

## 11. FE pages and menu

### 11.1 Module wiring

- `optimus-fe/src/types/assets.ts` — DTOs mirroring BE (no class wrappers).
- `optimus-fe/src/api/assets/{account,resource,sync}.ts` — factory functions `makeAssetsAccountApi(client)` etc. (`CLAUDE.md` "FE injects use string keys ... API modules are functional factories").
- `optimus-fe/src/stores/assets.ts` — single Pinia store with sub-areas: `accounts`, `instances`, `vpcs`, `databases`, `syncRuns`. State + actions only; no view logic.
- `optimus-fe/src/main.ts` — register the three API factories and `provide` with string keys `'assetsAccountApi'`, `'assetsResourceApi'`, `'assetsSyncApi'`.

### 11.2 Routing + views

Lowercase/kebab directories (Linux-case-sensitive — P3 lesson):

```
src/views/assets/
├── cloud-accounts/
│   ├── List.vue       ProTable + row-level "Sync now" button (v-permission="assets:account:write")
│   └── Form.vue       Modal: name + provider("aws" only) + cloudkey selector + region multi-select +
│                      enabled toggle + description
├── instances/List.vue ProTable; columns: account / region / instance_id / name / type / state /
│                      private_ip / public_ip / vpc_id / tags; filters: account / region / state /
│                      vpc_id / include_deleted; search box maps to q
├── vpcs/
│   ├── List.vue       account / region / vpc_id / name / cidr / is_default / state / tags; row-
│   │                  click → Detail
│   └── Detail.vue     vpc summary header + Subnet list table (q-filtered)
├── databases/List.vue account / region / db_instance_id / engine / version / class / status /
│                      endpoint / multi_az
└── sync-runs/List.vue ProTable; columns: started / finished / cloud_account / region / type /
                       status (colored) / items_seen / items_softdeleted / error (truncated tooltip);
                       filters: account / type / status / started_after
```

Static-route imports added in `src/router/static-routes.ts` for any pages referenced from `/me/menus`. The dynamic-routes mechanism (`registerDynamicRoutes`) handles per-menu mounting; the menu records seeded in §11.4 carry `Component:` strings matching these lowercase paths.

### 11.3 Permission gates

- Route-level `to.meta.permission`:
  - `/assets/cloud-accounts` → `assets:account:read`
  - `/assets/cloud-accounts/{id}/edit` → `assets:account:write` (form modal isn't a route, but if any standalone editor page is added, this is the gate)
  - `/assets/instances` / `/assets/vpcs` / `/assets/databases` → `assets:resource:read`
  - `/assets/sync-runs` → `assets:sync:read`
- DOM-level `v-permission`:
  - "Create CloudAccount" button → `assets:account:write`
  - "Edit" / "Delete" row buttons → `assets:account:write` / `assets:account:delete`
  - "Sync now" row button → `assets:account:write`
- The Assets **menu group** itself is gated by `assets:resource:read` (the most commonly-held perm — viewer auto-picks it). If a user lacks all `assets:*` perms, the group is hidden.

### 11.4 Menu seed (BE)

`internal/seed/menu.go` appended (sits next to existing menu seed rows). New rows:

```
{id_key:"menu.assets_group",       parent:nil,               i18n_key:"menu.assets",                 category:"assets", order: <next>, permission:"assets:resource:read", path:"/assets",                  component:"Layout"}
{id_key:"menu.assets.cloud_accounts", parent:"menu.assets_group", i18n_key:"menu.assets.cloud_accounts", category:"assets", order: 1,    permission:"assets:account:read",  path:"/assets/cloud-accounts",   component:"assets/cloud-accounts/List"}
{id_key:"menu.assets.instances",   parent:"menu.assets_group", i18n_key:"menu.assets.instances",      category:"assets", order: 2,    permission:"assets:resource:read", path:"/assets/instances",        component:"assets/instances/List"}
{id_key:"menu.assets.vpcs",        parent:"menu.assets_group", i18n_key:"menu.assets.vpcs",           category:"assets", order: 3,    permission:"assets:resource:read", path:"/assets/vpcs",             component:"assets/vpcs/List"}
{id_key:"menu.assets.databases",   parent:"menu.assets_group", i18n_key:"menu.assets.databases",      category:"assets", order: 4,    permission:"assets:resource:read", path:"/assets/databases",        component:"assets/databases/List"}
{id_key:"menu.assets.sync_runs",   parent:"menu.assets_group", i18n_key:"menu.assets.sync_runs",      category:"assets", order: 5,    permission:"assets:sync:read",     path:"/assets/sync-runs",        component:"assets/sync-runs/List"}
```

(`<next>` = the integer immediately after the largest existing top-level order — picked at implementation time, not hard-coded here.)

### 11.5 i18n

Nested keys in `src/locales/{zh-CN,en-US}.json`:

```
assets: {
  account: { ... fields, table headers, form labels, status, actions ... },
  resource: { instance: {...}, vpc: {...}, subnet: {...}, database: {...} },
  sync: { run_status: { running, success, failed, skipped }, ... }
},
menu: { assets: "资产管理"/"Assets", assets_group: ..., ... },
perm: { assets: { account: { read, write, delete }, resource: { read }, sync: { read } } },
error: { "43001": "...", "43002": "...", ..., "43107": "..." }
```

`bun run i18n:check` enforces parity; new keys must be added in both locales.

### 11.6 Components reused

- `ProTable`, `ProForm`, `PageHeader`, `ConfirmButton` from `src/components/`.
- `useTable<T,F>` from `@/hooks/useTable`.
- `useI18n` from `@/hooks/useI18n` (NOT `vue-i18n` directly — `CLAUDE.md` gotcha).
- v-permission directive from `src/directives/`.

### 11.7 No new vendor deps

P4 FE adds **no new npm packages**. All UI built from AntdV components already in the lockfile.

---

## 12. P1 reverse-reference patch

A small change to P1 `credentials/cloudkey/service.go` to prevent dangling references:

```go
// inside cloudkey.Service.Delete, after permission check, before delete:
if s.assetsAccountsInUse != nil {
    n, err := s.assetsAccountsInUse.CountByCloudKeyID(ctx, id)
    if err != nil { return err }
    if n > 0 {
        return apperr.New(errs.CodeAssetsCloudAccountInUse,
            "assets.cloud_account.in_use",
            fmt.Sprintf("cloud key referenced by %d cloud account(s)", n))
    }
}
```

- `assetsAccountsInUse` is a new interface field on `cloudkey.Service`, optional (nil-safe) so P1 still compiles standalone.
- `cmd/server/main.go` constructs the assets `inuse.Counter` and injects it into the cloudkey service before mounting routes. (Same wiring pattern as P2 cluster.Service ↔ kubeconfig.Service.)
- Interface lives in `assets/account/inuse/`:

```go
package inuse

type Counter interface {
    CountByCloudKeyID(ctx context.Context, cloudKeyID uint64) (int64, error)
}
```

- The P1 unit test for `cloudkey.Service.Delete` is updated to cover the new branch (counter returns > 0 → returns `CodeAssetsCloudAccountInUse`).

---

## 13. Testing strategy

Coverage target ≥ 60% per module (matches P0/P1/P2/P3 standard).

### 13.1 Unit (`*_test.go`, no build tags)

- `account/repo_test.go` — sqlmock-backed; verifies SQL shape, soft-delete predicate, name conflict detection.
- `account/service_test.go` — fake repo + fake audit recorder; verifies create/update/delete pathways, region validation, manual-sync invocation, audit payload shape.
- `instance/repo_test.go`, `vpc/repo_test.go`, `database/repo_test.go` — list filters + include_deleted toggle + ordering.
- `sync/engine_test.go` — fake fetcher + fake repo; verifies:
  - authoritative gate (err != nil → no soft-delete)
  - tryLock CAS / concurrent triggers
  - sync_runs row written in all four status branches
  - 90-day pruning
- `sync/ec2_fetcher_test.go`, `vpc_fetcher_test.go`, `rds_fetcher_test.go` — table-driven with fake AWS SDK clients (interface-via-narrow-type or `aws.Endpoint` stub); verifies pagination, tag extraction, network field extraction, error pass-through.
- `awsclient/maperror_test.go` — every branch of the mapping table.

### 13.2 Integration (`-tags=dbtest`, `tests/integration/assets_*_test.go`)

`tests/integration/`:
- `assets_account_test.go` — CloudAccount CRUD against real PG via dockertest; cascade-soft-delete verification on DELETE.
- `assets_sync_run_test.go` — engine.RunAll against PG with a stubbed fetcher (no real AWS); verifies upsert + soft-delete invariants end-to-end; verifies pruning.
- `assets_consume_smoke_test.go` — `assets.Consumer` lookups.

**No LocalStack**: AWS SDK calls are mocked at the fetcher boundary in unit tests; integration tests stop at the DB boundary.

### 13.3 FE (vitest)

- `cloud-accounts/List.vue` + `Form.vue` — render, validation, v-permission gating.
- `instances/List.vue` / `vpcs/List.vue` / `vpcs/Detail.vue` / `databases/List.vue` — render with mocked store; filter state behaviour.
- `sync-runs/List.vue` — status coloring + tooltip.
- v-permission audit unit test enumerating every page (sticks to P3 precedent).
- Store actions hit a stubbed API; HTTP not exercised here.

### 13.4 CI gates

`make swag` (re-runs swagger generation; `make swagger-diff` fails on drift), `make dump-perms` (`make perm-check` fails on drift), `bun run i18n:check`, `bun run lint --max-warnings=0`, `bun run typecheck`, `make lint`, `make test`, `make test-int` (gated on Docker availability — `CLAUDE.md` "Gotchas" Colima section).

---

## 14. Configuration

`optimus-be/configs/config.yaml` (dev) gains:

```yaml
assets:
  sync_cron: "*/15 * * * *"        # Standard 5-field cron
  sync_startup_delay: 30s
  sync_run_retention_days: 90
  aws_request_timeout: 30s
```

Env overrides (prod, via Viper `OPTIMUS_*` chain):

```
OPTIMUS_ASSETS_SYNC_CRON
OPTIMUS_ASSETS_SYNC_STARTUP_DELAY
OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS
OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT
```

`deploy/.env.example` annotated to explain each. Default values match the dev YAML; production is expected to keep `*/15 * * * *` unless the user has many accounts.

No new Docker images / no new compose services / no new healthcheck. The sync engine runs inside the existing `optimus-be` container.

---

## 15. Documentation and dogfood

- `optimus-be/scripts/p4-smoke.md` — manual smoke checklist:
  1. Generate or fetch a real AWS access key with read-only IAM (sample policy provided inline).
  2. Create the cloudkey via P1 FE (`/credentials/cloud-keys`).
  3. Create a CloudAccount via P4 FE with one region (e.g. `us-east-1`).
  4. Trigger "Sync now"; verify `assets_sync_runs` rows go `running → success`.
  5. Verify `aws_instances` / `aws_vpcs` / `aws_subnets` / `aws_databases` populate.
  6. Simulate a deleted resource by toggling its visibility (e.g., stop and terminate an EC2 in AWS); next sweep should soft-delete it; FE list with `include_deleted=true` shows it.
  7. Delete the cloudkey → expect 43001 `CodeAssetsCloudAccountInUse`.
  8. Soft-delete the CloudAccount → confirm resource rows cascade-soft-delete.
- `CLAUDE.md` updated: new "Architecture — assets (P4)" section appended after the P3 section, ≤ 200 words, captures the load-bearing invariants (no client caching, authoritative sweep gate, soft-delete-only, manual-sync async, `assets.Consumer` for downstream, P1 patch).
- `docs/permissions.md` regenerated.
- `docs/api/swagger.json` regenerated.

---

## 16. Risk register

| Risk | Mitigation |
|---|---|
| AWS SDK transitively raises go.mod's `go` directive past 1.25 | Pin SDK module versions explicitly in `go.mod`; verified via `go mod tidy` in CI. Document in CLAUDE.md "Conventions" as a new pin gotcha if it actually happens. |
| `robfig/cron/v3` blocks shutdown | `c.Stop()` returns a `context.Context` that completes when running jobs finish; main.go waits with a 30s timeout before forcing exit. |
| In-memory lock lost on BE restart while sweep was active | Acceptable: the next cron tick re-runs; partial sweep wasn't committed (transactional). No "ghost lock" possible. |
| AWS throttling causes ⇉ all sweeps fail simultaneously across regions | SDK already retries up to 3× with backoff. Sequential per-account / per-region sweeps spread the load. If a user has many accounts → many sweeps → spread is sufficient at < 50-user team scale. If not, the user can lengthen `sync_cron`. |
| Soft-delete cascades on CloudAccount DELETE are slow with millions of rows | Acceptable scale: < 50-user team has at most ~10s of thousands of resource rows per account. UPDATE on indexed `cloud_account_id` is sub-second at that scale. |
| Tag JSONB queries are slow without per-tag index | GIN index on `tags` column covers `?` / `@>` operators. Beyond that, P4.x can add functional indexes if needed. |
| sync_runs table grows unbounded | 90-day retention DELETE cron at 03:00 UTC daily. |

---

## 17. Implementation order hint (not the plan)

The implementation plan (separate document, written by `superpowers:writing-plans`) will sequence tasks. The natural dependency order is:

1. Migrations + models + errs codes + permissions registration
2. P1 patch (interface + nil-safe injection — no behaviour change yet)
3. CloudAccount module (CRUD, repo, service, handler, inuse)
4. awsclient (factory + MapError)
5. sync engine (engine, scheduler, sweep — fetcher stubs first, behavior tests pass without AWS)
6. Resource modules (instance, vpc, database — read-only repo/service/handler)
7. Real fetchers (ec2, vpc, rds) with unit tests
8. sync_runs module
9. assets.Consumer + smoke test
10. cmd/server/main.go wiring (mount routes, register cron entries, build Consumer)
11. Seed updates (menu rows)
12. FE: types + api factories + store + main.ts wiring
13. FE: cloud-accounts pages → resource list pages → sync-runs page
14. FE: i18n keys (both locales) + v-permission audit
15. Swagger + perm dump + smoke checklist + CLAUDE.md update

Single-PR delivery (sticks with P3 precedent — bundled PR over many small ones).

---

**End of spec.**
