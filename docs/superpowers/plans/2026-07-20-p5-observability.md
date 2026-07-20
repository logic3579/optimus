# P5 — observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Prometheus-compatible metrics observation MVP with encrypted HTTP credentials, SSRF-safe bounded queries, built-in Kubernetes views, and saved custom dashboards, without alerts or local metric storage.

**Architecture:** P1 gains a generic encrypted HTTP credential vertical and extends `credentials.Consumer`; P5 adds `internal/modules/observability` with data-source, Prometheus transport/client, query, dashboard, built-in-template, and module-wiring packages. PostgreSQL stores configuration only, the browser queries only Optimus, and Prometheus remains the time-series system of record. P4 enrichment is optional and failure-tolerant through `assets.Consumer`.

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL/goose, existing AES-256-GCM vault/audit/RBAC/envelope infrastructure, Prometheus HTTP API v1 over `net/http`, Vue 3, Ant Design Vue, Pinia, Apache ECharts, Vitest, bun.

**Reference spec:** `docs/superpowers/specs/2026-07-20-p5-observability-design.md`

---

## Scope and invariants

1. There is no alert table, API, permission, worker, frontend route, or test. Alerts, logs, traces, APM, CloudWatch, and metric persistence are out of scope.
2. `credentials.Consumer.GetHTTPCredential` is the only path to Basic passwords and Bearer tokens. Fetch once per batch, `defer credentials.WipeHTTPCredential`, never cache plaintext.
3. The browser never contacts Prometheus and never receives authorization material.
4. Every upstream dial is checked by the SSRF-safe resolver/dialer. Form-time URL validation alone is insufficient.
5. Private targets are denied unless an exact destination matches `observability.allowed_private_cidrs`; metadata, loopback, link-local, multicast, unspecified, and documentation ranges remain denied.
6. One batch targets one data source, consumes at most one credential, contains at most 12 queries, and runs at most four upstream calls concurrently.
7. PostgreSQL stores configuration only. Do not add sample, series, chunk, retention, alert, or evaluation tables.
8. P4 is optional. Nil Consumer, no asset match, stale data, or enrichment error must preserve the core metric result.
9. Mutations and data-source tests audit; normal query/view traffic does not create P5 audit rows. P1 still records one credential-consume audit per authenticated batch.
10. Do not log full PromQL, authorization headers, secret bytes, custom CA bodies, or raw upstream response bodies. Log a query fingerprint and bounded counts.
11. All client errors use `apperr.New`/`apperr.Wrap` and the envelope. Do not leak raw `net`, DNS, TLS, or Prometheus error text.
12. Keep `go.mod` at `go 1.25.0`, the Kubernetes pins at `v0.30.14`, and Helm at `v3.15.4`.
13. Use bun only for frontend dependency work. Import ECharts modules selectively and dispose chart instances on unmount.
14. Frontend route meta and `v-permission` must match backend permission gates. Preserve `zh-CN`/`en-US` parity.
15. Code comments are English. One task equals one coherent commit; use exact-path `git add`, never `git add .` or `git add -A`.

## File map

| Task | Paths | Responsibility |
|---|---|---|
| 1 | `optimus-be/migrations/00022_p5_observability.sql`, `internal/models/{credential_http,observability}.go` | P1 HTTP credential and P5 configuration schema/models |
| 2 | `internal/infra/errors/codes.go`, `internal/infra/permissions/codes.go`, their tests, both locale files | 44xxx errors and P1/P5 permission registry |
| 3 | `internal/modules/credentials/httpcredential/*`, `credentials/{consume,module}.go` | Encrypted HTTP credential CRUD and Consumer seam |
| 4 | `internal/modules/observability/prometheus/{urlpolicy,transport}.go` and tests; config files | SSRF-safe URL policy, resolver/dialer, TLS/auth transport |
| 5 | `internal/modules/observability/prometheus/{dto,client,errors}.go` and tests | Prometheus API v1 client and normalized results |
| 6 | `internal/modules/observability/datasource/*` and tests | Data-source CRUD, validation, test, and in-use seams |
| 7 | `internal/modules/observability/query/*` and tests | Bounded batches, label metadata, cancellation, optional P4 enrichment |
| 8 | `internal/modules/observability/dashboard/*`, `builtin/*` and tests | Transactional dashboard aggregates and built-ins |
| 9 | `internal/modules/observability/module/*`, `cmd/server/main.go`, P1/P2 deletion guards | DI, exact routes, cross-module in-use checks |
| 10 | integration tests, Swagger, permissions docs | Real-Postgres and HTTP-contract coverage; generated artifacts |
| 11 | `optimus-fe/package.json`, `bun.lock`, observability types/APIs/store/main | ECharts dependency and FE data layer |
| 12 | P1 HTTP credential FE files and tests | Credential management page |
| 13 | observability data-source FE files and tests | Data-source management/test page |
| 14 | panel components, custom dashboard views/editor and tests | ECharts panels and saved dashboards |
| 15 | Kubernetes built-in view, seed menus, locale files, permission tests | Built-in monitoring navigation and i18n |
| 16 | security/coverage tests, `scripts/p5-smoke.md`, `CLAUDE.md` | Hardening, full verification, handoff |

---

## Task 1: Create the P1/P5 schema and GORM models

**Files:**
- Create: `optimus-be/migrations/00022_p5_observability.sql`
- Create: `optimus-be/internal/models/credential_http.go`
- Create: `optimus-be/internal/models/observability.go`
- Modify: `optimus-be/tests/dbtest/seed.go`
- Test: `optimus-be/tests/integration/p5_schema_test.go`

- [ ] **Step 1: Write the failing schema integration test**

Add `TestP5SchemaConstraints` using the existing integration DB harness. It must insert a user, encrypted HTTP credential, cluster, data source, dashboard, and panel, then assert: active-name duplicates fail; `none` auth rejects a credential; `basic` requires one; panel type outside `time_series|stat|table` fails; width outside `6|12` fails; deleting a referenced credential, cluster, or data source is restricted.

```go
func TestP5SchemaConstraints(t *testing.T) {
    _, db := setupServer(t)
    user := dbtest.SeedUser(t, db, "p5-schema")
    credential := dbtest.SeedHTTPCredential(t, db, user.ID, "prom-basic", "basic")
    cluster := dbtest.SeedCluster(t, db, "p5-cluster")
    datasource := dbtest.SeedObservabilityDatasource(t, db, credential.ID, cluster.ID)
    dashboard := dbtest.SeedObservabilityDashboard(t, db, user.ID)
    dbtest.SeedObservabilityPanel(t, db, dashboard.ID, datasource.ID, "time_series", 12)

    require.Error(t, db.Exec(`DELETE FROM credentials_http_credentials WHERE id = ?`, credential.ID).Error)
    require.Error(t, db.Exec(`DELETE FROM clusters WHERE id = ?`, cluster.ID).Error)
    require.Error(t, db.Exec(`DELETE FROM observability_datasources WHERE id = ?`, datasource.ID).Error)
    require.Error(t, db.Exec(`INSERT INTO observability_panels
        (dashboard_id,datasource_id,title,panel_type,promql,sort_order,width)
        VALUES (?,?,?,?,?,?,?)`, dashboard.ID, datasource.ID, "bad", "gauge", "up", 99, 5).Error)
}
```

Add the five named `dbtest.Seed...` helpers in `optimus-be/tests/dbtest/seed.go`; each helper inserts the corresponding model with the minimum valid fields and calls `t.Helper()` plus `require.NoError`. This makes later P5 integration tests reuse one concrete fixture path.

- [ ] **Step 2: Run the test and verify the migration is missing**

Run from `optimus-be/`:

```bash
rtk go test -tags=dbtest ./tests/integration -run TestP5SchemaConstraints -count=1
```

Expected: FAIL because `credentials_http_credentials` does not exist.

- [ ] **Step 3: Add the goose migration from spec §§3–4**

Create four tables in dependency order: `credentials_http_credentials`, `observability_datasources`, `observability_dashboards`, `observability_panels`. The `CREATE TABLE` bodies, checks, partial unique indexes, and secondary indexes are the SQL blocks in approved spec §§3.1 and 4.1–4.3, wrapped in the repository's required markers:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE credentials_http_credentials (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  auth_type VARCHAR(16) NOT NULL,
  username VARCHAR(256),
  secret_ciphertext BYTEA NOT NULL,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT credentials_http_auth_type_check CHECK (auth_type IN ('basic', 'bearer')),
  CONSTRAINT credentials_http_username_check CHECK (
    (auth_type = 'basic' AND username IS NOT NULL AND username <> '') OR
    (auth_type = 'bearer' AND username IS NULL)
  )
);
CREATE UNIQUE INDEX credentials_http_name_unique
  ON credentials_http_credentials(name) WHERE deleted_at IS NULL;

CREATE TABLE observability_datasources (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  base_url TEXT NOT NULL,
  auth_type VARCHAR(16) NOT NULL DEFAULT 'none',
  http_credential_id BIGINT REFERENCES credentials_http_credentials(id) ON DELETE RESTRICT,
  cluster_id BIGINT REFERENCES clusters(id) ON DELETE RESTRICT,
  tls_skip_verify BOOLEAN NOT NULL DEFAULT FALSE,
  custom_ca_pem TEXT,
  description TEXT NOT NULL DEFAULT '',
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT observability_datasource_auth_type_check CHECK (auth_type IN ('none', 'basic', 'bearer')),
  CONSTRAINT observability_datasource_credential_check CHECK (
    (auth_type = 'none' AND http_credential_id IS NULL) OR
    (auth_type IN ('basic', 'bearer') AND http_credential_id IS NOT NULL)
  )
);
CREATE UNIQUE INDEX observability_datasource_name_unique
  ON observability_datasources(name) WHERE deleted_at IS NULL;
CREATE INDEX observability_datasource_cluster_idx
  ON observability_datasources(cluster_id) WHERE deleted_at IS NULL;
CREATE INDEX observability_datasource_credential_idx
  ON observability_datasources(http_credential_id) WHERE deleted_at IS NULL;

CREATE TABLE observability_dashboards (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  refresh_interval_s INTEGER NOT NULL DEFAULT 30,
  time_range VARCHAR(16) NOT NULL DEFAULT '1h',
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT observability_dashboard_refresh_check CHECK (refresh_interval_s BETWEEN 15 AND 3600),
  CONSTRAINT observability_dashboard_range_check CHECK (time_range IN ('15m', '1h', '6h', '24h', '7d'))
);
CREATE UNIQUE INDEX observability_dashboard_name_unique
  ON observability_dashboards(name) WHERE deleted_at IS NULL;

CREATE TABLE observability_panels (
  id BIGSERIAL PRIMARY KEY,
  dashboard_id BIGINT NOT NULL REFERENCES observability_dashboards(id) ON DELETE CASCADE,
  datasource_id BIGINT NOT NULL REFERENCES observability_datasources(id) ON DELETE RESTRICT,
  title VARCHAR(128) NOT NULL,
  panel_type VARCHAR(16) NOT NULL,
  promql TEXT NOT NULL,
  unit VARCHAR(32) NOT NULL DEFAULT 'none',
  legend VARCHAR(128) NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL,
  width SMALLINT NOT NULL DEFAULT 12,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT observability_panel_type_check CHECK (panel_type IN ('time_series', 'stat', 'table')),
  CONSTRAINT observability_panel_promql_check CHECK (length(btrim(promql)) BETWEEN 1 AND 8192),
  CONSTRAINT observability_panel_width_check CHECK (width IN (6, 12)),
  CONSTRAINT observability_panel_order_unique UNIQUE (dashboard_id, sort_order)
);
CREATE INDEX observability_panel_datasource_idx ON observability_panels(datasource_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS observability_panels;
DROP TABLE IF EXISTS observability_dashboards;
DROP TABLE IF EXISTS observability_datasources;
DROP TABLE IF EXISTS credentials_http_credentials;
-- +goose StatementEnd
```

Do not create a separate down file: this repository stores Up and Down in one goose migration. Do not add any other P5 tables.

- [ ] **Step 4: Verify the reverse section order**

```sql
DROP TABLE IF EXISTS observability_panels;
DROP TABLE IF EXISTS observability_dashboards;
DROP TABLE IF EXISTS observability_datasources;
DROP TABLE IF EXISTS credentials_http_credentials;
```

Exercise down/up only through the repository's disposable integration migration helper. Do not run a manual rollback command against a developer or production database.

- [ ] **Step 5: Add exact GORM models**

```go
type HTTPCredential struct {
    ID              uint64         `gorm:"primaryKey"`
    Name            string
    AuthType        string
    Username        *string
    SecretCiphertext []byte
    CreatedByUserID *uint64
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       gorm.DeletedAt `gorm:"index"`
}
func (HTTPCredential) TableName() string { return "credentials_http_credentials" }

type ObservabilityDatasource struct {
    ID uint64 `gorm:"primaryKey"`
    Name, BaseURL, AuthType, Description string
    HTTPCredentialID, ClusterID *uint64
    TLSSkipVerify bool
    CustomCAPEM *string
    CreatedByUserID *uint64
    CreatedAt, UpdatedAt time.Time
    DeletedAt gorm.DeletedAt `gorm:"index"`
}

type ObservabilityDashboard struct {
    ID uint64 `gorm:"primaryKey"`
    Name, Description, TimeRange string
    RefreshIntervalS int
    CreatedByUserID *uint64
    CreatedAt, UpdatedAt time.Time
    DeletedAt gorm.DeletedAt `gorm:"index"`
}

type ObservabilityPanel struct {
    ID, DashboardID, DatasourceID uint64
    Title, PanelType, PromQL, Unit, Legend string
    SortOrder, Width int
    CreatedAt, UpdatedAt time.Time
}
func (ObservabilityDatasource) TableName() string { return "observability_datasources" }
func (ObservabilityDashboard) TableName() string { return "observability_dashboards" }
func (ObservabilityPanel) TableName() string { return "observability_panels" }
```

- [ ] **Step 6: Run focused integration tests**

Run: `rtk go test -tags=dbtest ./tests/integration -run TestP5SchemaConstraints -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-be/migrations/00022_p5_observability.sql optimus-be/internal/models/credential_http.go optimus-be/internal/models/observability.go optimus-be/tests/dbtest/seed.go optimus-be/tests/integration/p5_schema_test.go
rtk git commit -m "feat(be/observability): add P5 configuration schema"
```

## Task 2: Register error codes, permissions, and locale contracts

**Files:**
- Modify: `optimus-be/internal/infra/errors/codes.go`
- Modify: `optimus-be/internal/infra/errors/codes_test.go`
- Modify: `optimus-be/internal/infra/permissions/codes.go`
- Modify: `optimus-be/internal/infra/permissions/registry_test.go`
- Modify: `optimus-fe/src/locales/zh-CN.json`
- Modify: `optimus-fe/src/locales/en-US.json`

- [ ] **Step 1: Write failing registry tests**

Assert all 17 exact 44xxx codes from spec §10 are non-zero/distinct and the permission registry contains:

```go
want := []string{
    "credentials:http:read", "credentials:http:write",
    "credentials:http:delete", "credentials:http:use",
    "observability:datasource:read", "observability:datasource:write",
    "observability:datasource:delete", "observability:metric:read",
    "observability:dashboard:read", "observability:dashboard:write",
    "observability:dashboard:delete",
}
```

Also assert no registered permission starts with `observability:alert:`.

- [ ] **Step 2: Verify the tests fail**

Run:

```bash
rtk go test ./internal/infra/errors ./internal/infra/permissions -count=1
```

Expected: FAIL on missing constants/registry entries.

- [ ] **Step 3: Add the 44xxx constants and message keys**

Add exactly `44001–44006`, `44101–44107`, and `44201–44204` with the HTTP/message-key mapping in spec §10. Do not allocate alert codes.

- [ ] **Step 4: Add the eleven permission entries**

Use categories `credentials` and `observability`; names must be nested locale keys such as `perm.observability.datasource.read`. The viewer's existing `%:read` grant must naturally pick up datasource, metric, and dashboard reads.

- [ ] **Step 5: Add matching bilingual locale keys**

Add error text and permission labels to both nested JSON files. Preserve every existing key and identical key shape between languages.

- [ ] **Step 6: Verify registries and i18n**

Run:

```bash
rtk go test ./internal/infra/errors ./internal/infra/permissions -count=1
cd ../optimus-fe && rtk bun run i18n:check
```

Expected: both PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-be/internal/infra/errors/codes.go optimus-be/internal/infra/errors/codes_test.go optimus-be/internal/infra/permissions/codes.go optimus-be/internal/infra/permissions/registry_test.go optimus-fe/src/locales/zh-CN.json optimus-fe/src/locales/en-US.json
rtk git commit -m "feat(observability): register P5 errors and permissions"
```

## Task 3: Implement encrypted generic HTTP credentials and Consumer support

**Files:**
- Create: `optimus-be/internal/modules/credentials/httpcredential/dto.go`
- Create: `optimus-be/internal/modules/credentials/httpcredential/repo.go`
- Create: `optimus-be/internal/modules/credentials/httpcredential/service.go`
- Create: `optimus-be/internal/modules/credentials/httpcredential/handler.go`
- Create: `optimus-be/internal/modules/credentials/httpcredential/{repo,service,handler}_test.go`
- Modify: `optimus-be/internal/modules/credentials/consume.go`
- Modify: `optimus-be/internal/modules/credentials/consume_smoke_test.go`
- Modify: `optimus-be/internal/modules/credentials/module.go`
- Modify: `optimus-be/internal/modules/credentials/module_test.go`

- [ ] **Step 1: Write failing service tests**

Cover Basic/Bearer validation, sealing before persistence, secret omission from DTOs, update-without-secret preserving ciphertext, consume audit purpose, wrong auth fields, soft delete, and a nil in-use counter. Use a recording cipher that fails the test if plaintext reaches a response DTO.

```go
func TestServiceCreateBasicSealsAndRedacts(t *testing.T) {
    svc, repo, rec := setupHTTPService(t)
    got, err := svc.Create(context.Background(), 7, "127.0.0.1", "test", CreateRequest{
        Name: "prom", AuthType: "basic", Username: ptr("reader"), Secret: "password",
    })
    require.NoError(t, err)
    require.Equal(t, "reader", deref(got.Username))
    row, err := repo.GetByID(context.Background(), got.ID)
    require.NoError(t, err)
    require.NotEqual(t, []byte("password"), row.SecretCiphertext)
    require.NotContains(t, marshalJSON(t, got), "password")
    require.Len(t, rec.Events(), 1)
}
```

- [ ] **Step 2: Run and verify failure**

Run: `rtk go test ./internal/modules/credentials/httpcredential -count=1`

Expected: FAIL because package/files do not exist.

- [ ] **Step 3: Implement DTO/repo/service contracts**

Use these request/public shapes:

```go
type CreateRequest struct {
    Name string `json:"name" binding:"required,max=128"`
    AuthType string `json:"auth_type" binding:"required,oneof=basic bearer"`
    Username *string `json:"username,omitempty"`
    Secret string `json:"secret" binding:"required,max=16384"`
}
type UpdateRequest struct {
    Name *string `json:"name,omitempty"`
    Username *string `json:"username,omitempty"`
    Secret *string `json:"secret,omitempty"`
}
type Detail struct {
    ID uint64 `json:"id"`
    Name, AuthType string
    Username *string `json:"username,omitempty"`
    CreatedByUserID *uint64 `json:"created_by_user_id,omitempty"`
    CreatedAt, UpdatedAt time.Time
}
```

`AuthType` is immutable on update. Basic requires a trimmed non-empty username; Bearer requires nil username. Name/secret validation happens before `Cipher.Seal`. Mutations audit `credentials.http_credential.{create,update,delete}` and never include the secret/ciphertext.

- [ ] **Step 4: Add the HTTP handlers and routes**

Mount `/api/v1/credentials/http-credentials` with list/get/create/update/delete and the four `credentials:http:*` permissions. Copy existing handler binding, pagination, actor, IP/UA, response, and Swagger patterns exactly.

- [ ] **Step 5: Extend the Consumer and wipe helper**

```go
type HTTPCredential struct {
    Name, AuthType, Username string
    Secret []byte
}
type Consumer interface {
    // existing methods unchanged
    GetHTTPCredential(context.Context, uint64, string) (*HTTPCredential, error)
}
func WipeHTTPCredential(c *HTTPCredential) {
    if c == nil { return }
    for i := range c.Secret { c.Secret[i] = 0 }
    c.Secret = nil
}
```

`GetHTTPCredential` validates purpose with the existing Consumer rules, opens ciphertext, copies plaintext into `[]byte`, writes `credentials.consume.http_credential`, and wipes intermediate bytes on every exit.

- [ ] **Step 6: Add module/Consumer smoke coverage**

Update constructor wiring and exact-route tests. Extend `consume_smoke_test.go` with one Basic and one Bearer round trip, audit assertion, cancellation, invalid system purpose, and wipe assertion.

- [ ] **Step 7: Run focused tests**

Run:

```bash
rtk go test ./internal/modules/credentials/... -count=1
```

Expected: PASS and no existing credential test regression.

- [ ] **Step 8: Commit**

```bash
rtk git add optimus-be/internal/modules/credentials/httpcredential optimus-be/internal/modules/credentials/consume.go optimus-be/internal/modules/credentials/consume_smoke_test.go optimus-be/internal/modules/credentials/module.go optimus-be/internal/modules/credentials/module_test.go
rtk git commit -m "feat(be/credentials): add encrypted HTTP credentials"
```

## Task 4: Build the SSRF-safe URL policy and authenticated transport

**Files:**
- Create: `optimus-be/internal/modules/observability/prometheus/urlpolicy.go`
- Create: `optimus-be/internal/modules/observability/prometheus/urlpolicy_test.go`
- Create: `optimus-be/internal/modules/observability/prometheus/transport.go`
- Create: `optimus-be/internal/modules/observability/prometheus/transport_test.go`
- Modify: `optimus-be/internal/infra/config/config.go`
- Modify: `optimus-be/internal/infra/config/config_test.go`
- Modify: `optimus-be/configs/config.yaml`

- [ ] **Step 1: Write table-driven URL-policy tests**

The table must cover HTTPS public success; `file`, `ftp`, userinfo, fragment,
and base query rejection; IPv4/IPv6 loopback; link-local; multicast;
unspecified; RFC1918/ULA without allowlist; permitted exact private CIDR;
IPv4-mapped IPv6; `169.254.169.254`; `fd00:ec2::254`; documentation ranges;
mixed DNS answers; and canceled resolution.

```go
func TestPolicyValidateResolved(t *testing.T) {
    tests := []struct{name, raw string; ips []netip.Addr; cidrs []string; wantCode int}{
        {"public", "https://prom.example.com", addrs("8.8.8.8"), nil, 0},
        {"private denied", "http://prom.internal", addrs("10.2.3.4"), nil, 44101},
        {"private allowed", "http://prom.internal", addrs("10.2.3.4"), []string{"10.2.3.0/24"}, 0},
        {"metadata always denied", "http://meta", addrs("169.254.169.254"), []string{"0.0.0.0/0"}, 44101},
    }
    // Construct policy, parse URL, validate every resolved IP, assert BizError code.
}
```

Use an actually routable public fixture address for the success case; documentation ranges are denial cases.

- [ ] **Step 2: Run and verify failure**

Run: `rtk go test ./internal/modules/observability/prometheus -run 'TestPolicy|TestTransport' -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Add configuration with strict defaults**

```go
type ObservabilityConfig struct {
    AllowedPrivateCIDRs []string      `mapstructure:"allowed_private_cidrs"`
    QueryTimeout        time.Duration `mapstructure:"query_timeout"`
    MaxBatchQueries     int           `mapstructure:"max_batch_queries"`
    MaxConcurrent       int           `mapstructure:"max_concurrent"`
    MaxRange            time.Duration `mapstructure:"max_range"`
    MinStep             time.Duration `mapstructure:"min_step"`
    MaxPointsPerSeries  int           `mapstructure:"max_points_per_series"`
    MaxSeries           int           `mapstructure:"max_series"`
    MaxResponseBytes    int64         `mapstructure:"max_response_bytes"`
    MaxEnrichmentIPs    int           `mapstructure:"max_enrichment_ips"`
}
```

Set YAML defaults to `[]`, `15s`, `12`, `4`, `168h`, `15s`, `11000`, `1000`, `16777216`, `100`. Startup validates positive limits, parses all CIDRs with `netip.ParsePrefix`, and rejects a CIDR that contains a metadata address.

- [ ] **Step 4: Implement URL parsing and destination classification**

Expose:

```go
type Resolver interface { LookupNetIP(context.Context, string, string) ([]netip.Addr, error) }
type Policy struct { allowed []netip.Prefix; resolver Resolver }
func NewPolicy([]string, Resolver) (*Policy, error)
func (p *Policy) ParseBaseURL(raw string) (*url.URL, error)
func (p *Policy) ResolveAllowed(context.Context, string) ([]netip.Addr, error)
```

Normalize `addr.Unmap()`. Deny special ranges before consulting the private allowlist. All DNS answers must be allowed; do not silently select only a favorable answer.

- [ ] **Step 5: Implement a validate-and-dial transport**

`TransportFactory.New(baseURL, tlsConfig, auth)` returns an `*http.Client` whose `DialContext` resolves through `Policy.ResolveAllowed` and dials a selected already-validated IP while preserving the original TLS `ServerName`. Set `CheckRedirect` to `http.ErrUseLastResponse`. Set Basic/Bearer headers in a redacting round tripper; never put them in the URL.

```go
type Auth struct { Type, Username string; Secret []byte }
type TLSOptions struct { SkipVerify bool; CustomCAPEM []byte }
func (f *TransportFactory) New(base *url.URL, tlsOpt TLSOptions, auth Auth) (*http.Client, error)
```

- [ ] **Step 6: Add rebinding and redirect transport tests**

Use fake resolvers/dialers: assert the dialer receives the validated IP, not the hostname; a second resolution returning metadata is denied; 3xx is returned without follow; Basic/Bearer headers reach the test server; captured errors/debug strings never contain the secret.

- [ ] **Step 7: Run focused tests**

Run:

```bash
rtk go test ./internal/modules/observability/prometheus ./internal/infra/config -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
rtk git add optimus-be/internal/modules/observability/prometheus/urlpolicy.go optimus-be/internal/modules/observability/prometheus/urlpolicy_test.go optimus-be/internal/modules/observability/prometheus/transport.go optimus-be/internal/modules/observability/prometheus/transport_test.go optimus-be/internal/infra/config/config.go optimus-be/internal/infra/config/config_test.go optimus-be/configs/config.yaml
rtk git commit -m "feat(be/observability): add SSRF-safe Prometheus transport"
```

## Task 5: Implement the normalized Prometheus API v1 client

**Files:**
- Create: `optimus-be/internal/modules/observability/prometheus/dto.go`
- Create: `optimus-be/internal/modules/observability/prometheus/client.go`
- Create: `optimus-be/internal/modules/observability/prometheus/client_test.go`
- Create: `optimus-be/internal/modules/observability/prometheus/errors.go`
- Create: `optimus-be/internal/modules/observability/prometheus/errors_test.go`

- [ ] **Step 1: Write failing decode/request tests**

Use `httptest.Server` fixtures for vector, matrix, scalar, string, warnings, API error, malformed JSON, oversized body, 401, 500, and timeout. Assert query values are form encoded and never concatenated into a URL.

```go
func TestClientQueryRangeMatrix(t *testing.T) {
    server := prometheusFixture(t, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"pod":"api-0"},"values":[[1,"2.5"],[2,"3.5"]]}]}}`)
    c := NewClient(server.Client(), mustURL(server.URL), 1<<20, 1000)
    got, err := c.QueryRange(context.Background(), "rate(http_requests_total[5m])", time.Unix(1,0), time.Unix(2,0), time.Second)
    require.NoError(t, err)
    require.Equal(t, "matrix", got.ResultType)
    require.Equal(t, "api-0", got.Series[0].Labels["pod"])
}
```

- [ ] **Step 2: Run and verify failure**

Run: `rtk go test ./internal/modules/observability/prometheus -run 'TestClient|TestMap' -count=1`

Expected: FAIL on undefined client.

- [ ] **Step 3: Define stable normalized DTOs**

```go
type Sample struct { Timestamp float64 `json:"timestamp"`; Value string `json:"value"` }
type Series struct { Labels map[string]string `json:"labels"`; Samples []Sample `json:"samples"` }
type Result struct {
    ResultType string `json:"result_type"`
    Series []Series `json:"series,omitempty"`
    Scalar *Sample `json:"scalar,omitempty"`
    Text *string `json:"text,omitempty"`
    Warnings []string `json:"warnings,omitempty"`
}
```

Reject label maps over 128 entries, keys over 256 bytes, values over 4096 bytes, series over configured maximum, non-finite timestamps, and invalid sample tuple shapes.

- [ ] **Step 4: Implement only the approved API methods**

```go
func (c *Client) Query(ctx context.Context, promql string, at time.Time) (Result, error)
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) (Result, error)
func (c *Client) Labels(ctx context.Context) ([]string, error)
func (c *Client) LabelValues(ctx context.Context, label string) ([]string, error)
func (c *Client) BuildInfo(ctx context.Context) (map[string]string, error)
```

Use `io.LimitReader(max+1)`, check overflow before JSON decoding, validate label names with Prometheus label syntax, and join paths beneath the fixed base prefix without accepting caller-supplied paths.

- [ ] **Step 5: Map upstream failures**

Map destination denial to `44101`, dial/DNS/TLS to `44102`, deadline to `44103`, Prometheus error/401/403/5xx to bounded `44104`, decode/size to `44105`, and caller validation to `44107`. Preserve the cause with `apperr.Wrap`, but use stable client messages.

- [ ] **Step 6: Run focused tests**

Run: `rtk go test ./internal/modules/observability/prometheus -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-be/internal/modules/observability/prometheus/dto.go optimus-be/internal/modules/observability/prometheus/client.go optimus-be/internal/modules/observability/prometheus/client_test.go optimus-be/internal/modules/observability/prometheus/errors.go optimus-be/internal/modules/observability/prometheus/errors_test.go
rtk git commit -m "feat(be/observability): add Prometheus API client"
```

## Task 6: Implement the data-source vertical

**Files:**
- Create: `optimus-be/internal/modules/observability/datasource/dto.go`
- Create: `optimus-be/internal/modules/observability/datasource/repo.go`
- Create: `optimus-be/internal/modules/observability/datasource/repo_test.go`
- Create: `optimus-be/internal/modules/observability/datasource/service.go`
- Create: `optimus-be/internal/modules/observability/datasource/service_test.go`
- Create: `optimus-be/internal/modules/observability/datasource/handler.go`
- Create: `optimus-be/internal/modules/observability/datasource/handler_test.go`
- Create: `optimus-be/internal/modules/observability/datasource/inuse/inuse.go`
- Create: `optimus-be/internal/modules/observability/datasource/inuse/inuse_test.go`

- [ ] **Step 1: Write failing repository/service tests**

Cover pagination/filters; active-name uniqueness; HTTP credential existence/type match; optional cluster existence; URL policy parsing; CA certificate-only validation; create/update/delete audits; `tls_skip_verify`; dashboard-panel deletion conflict; connection test consume purpose; no secret/raw error in test audit; and unauthenticated test without Consumer call.

```go
func TestServiceRejectsCredentialAuthMismatch(t *testing.T) {
    svc := setupDatasourceService(t, fakeCredentialMeta{ID: 9, AuthType: "bearer"})
    _, err := svc.Create(context.Background(), 1, "ip", "ua", CreateRequest{
        Name: "prom", BaseURL: "https://prom.example.com", AuthType: "basic", HTTPCredentialID: ptr(uint64(9)),
    })
    requireBizCode(t, err, 44005)
}
```

- [ ] **Step 2: Run and verify failure**

Run: `rtk go test ./internal/modules/observability/datasource/... -count=1`

Expected: FAIL because package is absent.

- [ ] **Step 3: Implement DTO/repository contracts**

Create/update requests mirror spec §4.1. Public DTO exposes credential ID/name and cluster ID/name but never CA body or credential fields. Repo methods are `List`, `GetByID`, `Create`, `Update`, `SoftDelete`, and `CountByHTTPCredentialID/CountByClusterID`, with transactional variants for deletion guards.

- [ ] **Step 4: Implement service validation and mutation audits**

Inject narrow interfaces:

```go
type CredentialMetadata interface { GetHTTPMetadata(context.Context, uint64) (HTTPMetadata, error) }
type ClusterExistence interface { Exists(context.Context, uint64) (bool, error) }
type PanelUsage interface { CountByDatasourceID(context.Context, uint64) (int64, error) }
type Tester interface { Test(context.Context, Detail, string) (map[string]string, error) }
```

Trim name/base URL; require auth/credential consistency; parse CA PEM blocks and require every block type `CERTIFICATE`; transact mutation plus `Recorder.WithTx`; omit CA body and full upstream error from audits.

- [ ] **Step 5: Implement handlers and Swagger annotations**

Add list/create/get/update/delete/test handlers under `/observability/datasources`. Test returns `{reachable:true, version?:string}` on success. Every ID uses inline `strconv.ParseUint`; mutations pass actor/IP/UA.

- [ ] **Step 6: Run focused tests**

Run: `rtk go test ./internal/modules/observability/datasource/... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-be/internal/modules/observability/datasource
rtk git commit -m "feat(be/observability): add data source management"
```

## Task 7: Implement bounded query batches, labels, and P4 enrichment

**Files:**
- Create: `optimus-be/internal/modules/observability/query/dto.go`
- Create: `optimus-be/internal/modules/observability/query/service.go`
- Create: `optimus-be/internal/modules/observability/query/service_test.go`
- Create: `optimus-be/internal/modules/observability/query/enrichment.go`
- Create: `optimus-be/internal/modules/observability/query/enrichment_test.go`
- Create: `optimus-be/internal/modules/observability/query/handler.go`
- Create: `optimus-be/internal/modules/observability/query/handler_test.go`

- [ ] **Step 1: Write failing limit/batch tests**

Test duplicate/empty ref IDs, 13 queries, PromQL over 8 KiB, range over seven days, step under 15s, points over 11,000, four-worker concurrency ceiling, request cancellation, one credential consume per batch, per-item PromQL errors, whole-batch auth/dial failures, no P5 query audit, and no full PromQL in logs.

```go
func TestRangeBatchConsumesCredentialOnce(t *testing.T) {
    consumer := &countingCredentialConsumer{}
    runner := &recordingRunner{}
    svc := newQueryService(consumer, runner, Limits{MaxBatch:12, MaxConcurrent:4, MaxRange:7*24*time.Hour, MinStep:15*time.Second, MaxPoints:11000})
    got, err := svc.Range(context.Background(), RangeRequest{DatasourceID:1, Start:now.Add(-time.Hour), End:now, Step:time.Minute, Queries: []Query{{RefID:"a", PromQL:"up"},{RefID:"b", PromQL:"rate(x[5m])"}}})
    require.NoError(t, err)
    require.Equal(t, 1, consumer.Calls())
    require.Len(t, got.Results, 2)
}
```

- [ ] **Step 2: Run and verify failure**

Run: `rtk go test ./internal/modules/observability/query -count=1`

Expected: FAIL because package is absent.

- [ ] **Step 3: Implement request/result types and validation**

```go
type Query struct { RefID string `json:"ref_id"`; PromQL string `json:"promql"` }
type InstantRequest struct { DatasourceID uint64; Time time.Time; EnrichAssets bool; Queries []Query }
type RangeRequest struct { DatasourceID uint64; Start, End time.Time; Step time.Duration; EnrichAssets bool; Queries []Query }
type ItemResult struct { RefID string; Result *prometheus.Result; Error *ItemError }
type BatchResult struct { Results []ItemResult; AssetContext map[string]AssetSummary `json:"asset_context,omitempty"` }
```

Keep output ordering identical to input. Use `errgroup.WithContext` plus a semaphore of four. Classify expression-level `44104` as an item error; destination/auth/transport failures cancel and fail the batch.

- [ ] **Step 4: Implement client construction and credential lifetime**

Load one data-source detail, consume at most one HTTP credential with purpose `observability.query.instant` or `.range`, immediately `defer WipeHTTPCredential`, build one request-scoped transport/client, execute bounded queries, then close idle connections.

- [ ] **Step 5: Implement optional enrichment**

Scan only configured candidate labels `private_ip`, `instance_ip`, `node_ip`; parse/unmap, deduplicate, sort for deterministic tests, cap at 100, and call `assets.Consumer.LookupInstanceByPrivateIP`. Ignore not-found and all enrichment errors after structured debug logging. Never mutate series labels.

- [ ] **Step 6: Add handlers for query and metadata routes**

Implement POST `/query`, POST `/query-range`, GET `/datasources/{id}/labels`, and GET `/datasources/{id}/label-values?label=...`. Metadata routes use purpose `observability.query.metadata` and the same safe client path.

- [ ] **Step 7: Run focused tests with race detection**

Run: `rtk go test -race ./internal/modules/observability/query -count=1`

Expected: PASS with no goroutine leak/race.

- [ ] **Step 8: Commit**

```bash
rtk git add optimus-be/internal/modules/observability/query
rtk git commit -m "feat(be/observability): add bounded metric queries"
```

## Task 8: Implement saved dashboards and built-in Kubernetes definitions

**Files:**
- Create: `optimus-be/internal/modules/observability/dashboard/dto.go`
- Create: `optimus-be/internal/modules/observability/dashboard/repo.go`
- Create: `optimus-be/internal/modules/observability/dashboard/repo_test.go`
- Create: `optimus-be/internal/modules/observability/dashboard/service.go`
- Create: `optimus-be/internal/modules/observability/dashboard/service_test.go`
- Create: `optimus-be/internal/modules/observability/dashboard/handler.go`
- Create: `optimus-be/internal/modules/observability/dashboard/handler_test.go`
- Create: `optimus-be/internal/modules/observability/builtin/dashboards.go`
- Create: `optimus-be/internal/modules/observability/builtin/dashboards_test.go`

- [ ] **Step 1: Write failing aggregate tests**

Cover create/get/list/update/delete; transaction rollback on invalid/repeated panel order; missing/soft-deleted data source; units outside registry; panel type/width; PromQL length; refresh/range enum; mutation audits excluding full PromQL; update replacing panels atomically; soft-deleting dashboard and hard-deleting panels.

- [ ] **Step 2: Write failing built-in snapshot tests**

Require codes `kubernetes-cluster`, `kubernetes-nodes`, and `kubernetes-workloads`; unique panel ref IDs; only approved types/units; non-empty PromQL; and no mutation API. Snapshot the title key, variables, panel order, width, and expressions.

- [ ] **Step 3: Run and verify failure**

Run: `rtk go test ./internal/modules/observability/dashboard ./internal/modules/observability/builtin -count=1`

Expected: FAIL because packages are absent.

- [ ] **Step 4: Implement aggregate DTO/repository/service**

```go
type PanelInput struct { DatasourceID uint64; Title, PanelType, PromQL, Unit, Legend string; SortOrder, Width int }
type SaveRequest struct { Name, Description string; RefreshIntervalS int; TimeRange string; Panels []PanelInput }
```

Validate the entire request before opening the transaction. Within one transaction, create/update dashboard, delete old panels on update, insert all new panels, and write one bounded audit event with panel count and SHA-256 fingerprints only.

- [ ] **Step 5: Implement handlers**

Mount custom list/create/get/update/delete and built-in list/get. Built-ins require `metric:read`; custom reads require `dashboard:read`; writes/deletes use their exact permissions.

- [ ] **Step 6: Encode built-ins as immutable definitions**

Use common expressions for container CPU/memory, node readiness, pod phase count, restart rate, and namespace/workload filters. Variable substitution accepts only label-safe values and uses PromQL string escaping; never use raw string replacement.

- [ ] **Step 7: Run focused tests**

Run: `rtk go test ./internal/modules/observability/dashboard ./internal/modules/observability/builtin -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
rtk git add optimus-be/internal/modules/observability/dashboard optimus-be/internal/modules/observability/builtin
rtk git commit -m "feat(be/observability): add metric dashboards"
```

## Task 9: Wire the module and cross-module deletion guards

**Files:**
- Create: `optimus-be/internal/modules/observability/module/wire.go`
- Create: `optimus-be/internal/modules/observability/module/wire_test.go`
- Modify: `optimus-be/cmd/server/main.go`
- Modify: `optimus-be/internal/modules/credentials/httpcredential/service.go`
- Modify: `optimus-be/internal/modules/credentials/httpcredential/service_test.go`
- Modify: `optimus-be/internal/modules/k8s/cluster/service.go`
- Modify: `optimus-be/internal/modules/k8s/cluster/service_test.go`

- [ ] **Step 1: Write the failing exact-route snapshot**

Assert every method/path/permission triple in spec §6 and assert no path contains `alert`. Include the four P1 HTTP-credential CRUD permission groups.

- [ ] **Step 2: Write failing deletion-guard tests**

HTTP credential delete must refuse when the injected P5 counter returns >0 and remain safe when nil. Cluster delete must refuse when its existing app references or the new P5 data-source counter is >0; both checks must use the same transaction when supported.

- [ ] **Step 3: Run and verify failure**

Run:

```bash
rtk go test ./internal/modules/observability/module ./internal/modules/credentials/httpcredential ./internal/modules/k8s/cluster -count=1
```

Expected: FAIL on missing wire/counters.

- [ ] **Step 4: Implement module inputs/outputs**

```go
type Input struct {
    DB *gorm.DB
    Credentials credentials.Consumer
    Audit *audit.Recorder
    Config config.ObservabilityConfig
    Assets assets.Consumer // pass assetsModule.Consumer; may be nil in standalone tests
}
type Module struct {
    CredentialUsage datasource.CredentialUsageCounter
    ClusterUsage datasource.ClusterUsageCounter
    // private handlers
}
func Wire(in Input) (*Module, error)
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache)
```

Compile/validate CIDRs during `Wire`; invalid security configuration must fail startup.

- [ ] **Step 5: Wire in the composition root**

Construct P5 after P4 and pass the exported `assetsModule.Consumer` field. Inject P5's credential counter into the HTTP credential service and its cluster counter into the P2 cluster service before routes are mounted. Do not introduce background contexts or shutdown hooks.

- [ ] **Step 6: Run module and server tests**

Run:

```bash
rtk go test ./internal/modules/observability/module ./internal/modules/credentials/... ./internal/modules/k8s/cluster ./cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-be/internal/modules/observability/module optimus-be/cmd/server/main.go optimus-be/internal/modules/credentials/httpcredential/service.go optimus-be/internal/modules/credentials/httpcredential/service_test.go optimus-be/internal/modules/k8s/cluster/service.go optimus-be/internal/modules/k8s/cluster/service_test.go
rtk git commit -m "feat(be/observability): wire P5 module"
```

## Task 10: Add integration coverage and regenerate backend contracts

**Files:**
- Create: `optimus-be/tests/integration/http_credential_test.go`
- Create: `optimus-be/tests/integration/observability_datasource_test.go`
- Create: `optimus-be/tests/integration/observability_dashboard_test.go`
- Create: `optimus-be/tests/integration/observability_query_test.go`
- Modify: P5 and HTTP-credential handler Swagger annotations
- Modify: `optimus-be/api/docs/swagger.json`
- Modify: `docs/api/swagger.json`
- Modify: `docs/permissions.md`

- [ ] **Step 1: Add HTTP credential integration tests**

Against real PostgreSQL and the real vault cipher, prove Basic/Bearer CRUD, ciphertext-at-rest, redacted HTTP responses, consume audit, reference-restricted deletion, and deletion after data-source soft delete.

- [ ] **Step 2: Add data-source/dashboard integration tests**

Prove filters, partial uniqueness, cluster/credential restrictions, aggregate replacement, rollback on bad panel, data-source delete blocked by panels, and dashboard delete releasing the reference.

- [ ] **Step 3: Add end-to-end query handler tests**

Use a loopback `httptest.Server` only with a test policy explicitly allowing its exact `/32` or `/128`. Cover Basic/Bearer propagation, one credential audit for two expressions, instant/range normalization, per-item expression error, timeout, response limit, and denied metadata destination. Assert there are no P5 query audit rows.

- [ ] **Step 4: Run integration tests**

Run:

```bash
rtk go test -tags=dbtest ./tests/integration -run 'TestHTTPCredential|TestObservability' -race -count=1
```

Expected: PASS.

- [ ] **Step 5: Regenerate and verify contracts**

Run from `optimus-be/`:

```bash
rtk make swag
rtk make swagger-diff
rtk make dump-perms
rtk make perm-check
```

Expected: generation succeeds and both drift checks PASS. Inspect Swagger to confirm no secret response fields and no alert paths.

- [ ] **Step 6: Commit**

```bash
rtk git add optimus-be/tests/integration/http_credential_test.go optimus-be/tests/integration/observability_datasource_test.go optimus-be/tests/integration/observability_dashboard_test.go optimus-be/tests/integration/observability_query_test.go optimus-be/internal/modules/credentials/httpcredential/handler.go optimus-be/internal/modules/observability/datasource/handler.go optimus-be/internal/modules/observability/query/handler.go optimus-be/internal/modules/observability/dashboard/handler.go optimus-be/api/docs/swagger.json docs/api/swagger.json docs/permissions.md
rtk git commit -m "test(be/observability): cover P5 HTTP contracts"
```

## Task 11: Add ECharts and the frontend data layer

**Files:**
- Modify: `optimus-fe/package.json`
- Modify: `optimus-fe/bun.lock`
- Create: `optimus-fe/src/types/observability.ts`
- Create: `optimus-fe/src/api/observability/datasource.ts`
- Create: `optimus-fe/src/api/observability/query.ts`
- Create: `optimus-fe/src/api/observability/dashboard.ts`
- Create: `optimus-fe/src/api/observability/__tests__/api.test.ts`
- Create: `optimus-fe/src/stores/observability.ts`
- Create: `optimus-fe/src/stores/observability.test.ts`
- Modify: `optimus-fe/src/main.ts`

- [ ] **Step 1: Add the ECharts dependency with bun**

Run from `optimus-fe/`:

```bash
rtk bun add echarts
```

Expected: only `package.json` and `bun.lock` change; no npm/yarn/pnpm lockfile appears.

- [ ] **Step 2: Write failing API tests**

Mock the shared client and assert exact methods/paths/bodies for all spec §6 endpoints, including `queries[]`, duration step serialization, label query encoding, and aggregate dashboard writes.

```ts
it('posts one range batch for one data source', async () => {
  await makeQueryApi(client).range({ datasource_id: 3, start, end, step: '1m', enrich_assets: false, queries: [{ ref_id: 'cpu', promql: 'rate(x[5m])' }] })
  expect(client.post).toHaveBeenCalledWith('/observability/query-range', expect.objectContaining({ datasource_id: 3 }))
})
```

- [ ] **Step 3: Define exact frontend types**

Mirror backend public JSON only. Include `DatasourceSummary/Detail/SaveInput`, `Query/InstantBatch/RangeBatch`, normalized samples/series/results/item errors, `Dashboard/Panel/SaveDashboard`, and built-in definitions. Do not add alert types or secret fields.

- [ ] **Step 4: Implement functional API factories**

Use `makeObservabilityDatasourceApi`, `makeObservabilityQueryApi`, and `makeObservabilityDashboardApi`. Return unwrapped `data` exactly as existing APIs do; rely on shared envelope/error handling.

- [ ] **Step 5: Write then implement store tests**

Store only selected data-source/dashboard IDs, metadata lists, and request state. Do not cache metric results globally. Provide cancellation ownership per dashboard load and a `reset()` used by logout.

- [ ] **Step 6: Provide APIs in `main.ts` and run tests**

Use string injection keys `observabilityDatasourceApi`, `observabilityQueryApi`, and `observabilityDashboardApi`.

Run:

```bash
rtk bun run test -- src/api/observability src/stores/observability.test.ts
rtk bun run typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-fe/package.json optimus-fe/bun.lock optimus-fe/src/types/observability.ts optimus-fe/src/api/observability optimus-fe/src/stores/observability.ts optimus-fe/src/stores/observability.test.ts optimus-fe/src/main.ts
rtk git commit -m "feat(fe/observability): add metrics data layer"
```

## Task 12: Add the generic HTTP credential page

**Files:**
- Create: `optimus-fe/src/api/credentials/http-credential.ts`
- Create: `optimus-fe/src/views/credentials/http-credentials/List.vue`
- Create: `optimus-fe/src/views/credentials/http-credentials/components/HttpCredentialEditModal.vue`
- Create: `optimus-fe/src/views/credentials/http-credentials/__tests__/List.test.ts`
- Create: `optimus-fe/src/views/credentials/http-credentials/__tests__/HttpCredentialEditModal.test.ts`
- Modify: `optimus-fe/src/main.ts`

- [ ] **Step 1: Write failing component tests**

Test Basic username required, Bearer username hidden/cleared, secret required on create but optional on update, secret never rendered after save, read/write/delete permission gates, confirm delete, pagination, and API error preservation.

- [ ] **Step 2: Run and verify failure**

Run: `rtk bun run test -- src/views/credentials/http-credentials`

Expected: FAIL because components do not exist.

- [ ] **Step 3: Implement API and modal**

Use the existing credential page patterns. Form state is:

```ts
type HTTPForm = { name: string; auth_type: 'basic'|'bearer'; username?: string; secret?: string }
```

On auth-type change, clear username for Bearer. On modal close/success, overwrite then clear the secret ref. Never populate secret during edit.

- [ ] **Step 4: Implement list and permissions**

Columns: name, auth type, Basic username, updated time, actions. Gate create/edit with `credentials:http:write` and delete with `credentials:http:delete`. The page itself uses `credentials:http:read` through menu/route metadata.

- [ ] **Step 5: Run tests/typecheck**

Run:

```bash
rtk bun run test -- src/views/credentials/http-credentials
rtk bun run typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
rtk git add optimus-fe/src/api/credentials/http-credential.ts optimus-fe/src/views/credentials/http-credentials optimus-fe/src/main.ts
rtk git commit -m "feat(fe/credentials): manage HTTP credentials"
```

## Task 13: Add the data-source management page

**Files:**
- Create: `optimus-fe/src/views/observability/datasources/List.vue`
- Create: `optimus-fe/src/views/observability/datasources/components/DatasourceForm.vue`
- Create: `optimus-fe/src/views/observability/datasources/__tests__/List.test.ts`
- Create: `optimus-fe/src/views/observability/datasources/__tests__/DatasourceForm.test.ts`

- [ ] **Step 1: Write failing form/list tests**

Cover auth-dependent credential selector; optional cluster; CA input; visible danger warning for `tls_skip_verify`; URL validation; test button requiring write permission; test success/version; test failure localization; CRUD permissions; reference-conflict deletion; and proof no credential secret field exists.

- [ ] **Step 2: Run and verify failure**

Run: `rtk bun run test -- src/views/observability/datasources`

Expected: FAIL because files do not exist.

- [ ] **Step 3: Implement the form**

Use fields from public `SaveDatasourceInput`. Selecting `none` clears credential ID. Basic/Bearer credential choices are filtered by returned credential auth type. CA accepts pasted public PEM only. The form never sends URL userinfo/query/fragment.

- [ ] **Step 4: Implement list/test interactions**

Use `useTable`; columns are name, base URL, auth, cluster, TLS warning, updated time, actions. Test calls the dedicated endpoint and renders only normalized outcome/version.

- [ ] **Step 5: Run component tests, typecheck, and lint**

Run:

```bash
rtk bun run test -- src/views/observability/datasources
rtk bun run typecheck
rtk bun run lint
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
rtk git add optimus-fe/src/views/observability/datasources
rtk git commit -m "feat(fe/observability): add data source management"
```

## Task 14: Add reusable ECharts panels and custom dashboards

**Files:**
- Create: `optimus-fe/src/views/observability/components/chart-adapter.ts`
- Create: `optimus-fe/src/views/observability/components/chart-adapter.test.ts`
- Create: `optimus-fe/src/views/observability/components/TimeSeriesPanel.vue`
- Create: `optimus-fe/src/views/observability/components/StatPanel.vue`
- Create: `optimus-fe/src/views/observability/components/TablePanel.vue`
- Create: `optimus-fe/src/views/observability/components/PanelState.vue`
- Create: `optimus-fe/src/views/observability/components/__tests__/panels.test.ts`
- Create: `optimus-fe/src/views/observability/dashboards/List.vue`
- Create: `optimus-fe/src/views/observability/dashboards/Detail.vue`
- Create: `optimus-fe/src/views/observability/dashboards/Form.vue`
- Create: `optimus-fe/src/views/observability/dashboards/__tests__/{List,Detail,Form}.test.ts`

- [ ] **Step 1: Write failing adapter tests**

Test matrix/vector/scalar conversion; label-based legend; `NaN`, `+Inf`, `-Inf` as gaps; percent/bytes/cores/seconds/rate formatting; stable series ordering; and no mutation of API results.

- [ ] **Step 2: Implement the pure adapter**

Export only pure functions returning typed ECharts options. Use modular ECharts imports in the component, not the adapter. Never use `eval`, HTML formatters, or backend-provided ECharts option objects.

- [ ] **Step 3: Write failing panel lifecycle/state tests**

Assert loading/empty/unsupported/partial/error states; chart construction after mount; resize observer; option update; and `dispose()` plus observer cleanup on unmount.

- [ ] **Step 4: Implement three panel components**

Time-series uses line charts, stat renders a formatted latest value without a chart when possible, and table uses Ant Design Vue table. All share `PanelState` and accept normalized DTOs only.

- [ ] **Step 5: Write failing custom-dashboard tests**

Cover list CRUD permissions, aggregate editor validation, move up/down order, widths 6/12, approved types/units only, grouping panels by data source into one batch, multiple data sources into separate batches, partial item errors, saved refresh interval, hidden-tab pause, cancel-before-refresh, and cleanup on unmount.

- [ ] **Step 6: Implement dashboard pages**

The form owns `PanelInput[]`; it never exposes drag/drop or raw chart options. Detail computes `Map<datasourceID, Query[]>`, issues one request per group, and maps `ref_id` back to panels. Use `document.visibilityState`, one timer, and one `AbortController` per refresh generation.

- [ ] **Step 7: Run focused tests and build**

Run:

```bash
rtk bun run test -- src/views/observability/components src/views/observability/dashboards
rtk bun run typecheck
rtk bun run build
```

Expected: PASS; inspect build output to ensure ECharts is lazy-loaded with the observability route rather than the login bundle.

- [ ] **Step 8: Commit**

```bash
rtk git add optimus-fe/src/views/observability/components optimus-fe/src/views/observability/dashboards
rtk git commit -m "feat(fe/observability): add metric dashboards"
```

## Task 15: Add built-in Kubernetes monitoring, menus, and complete i18n

**Files:**
- Create: `optimus-fe/src/views/observability/kubernetes/Index.vue`
- Create: `optimus-fe/src/views/observability/kubernetes/__tests__/Index.test.ts`
- Modify: `optimus-be/internal/seed/seed.go`
- Modify: `optimus-be/internal/seed/seed_test.go`
- Modify: `optimus-fe/src/locales/zh-CN.json`
- Modify: `optimus-fe/src/locales/en-US.json`
- Create: `optimus-fe/src/views/observability/__tests__/v-permission.test.ts`

- [ ] **Step 1: Write failing seed/menu tests**

Require one `observability` group with children for Kubernetes, custom dashboards, and data sources; add the HTTP credential child under credentials. Exact components/permissions:

```text
observability/kubernetes/Index       observability:metric:read
observability/dashboards/List        observability:dashboard:read
observability/datasources/List       observability:datasource:read
credentials/http-credentials/List   credentials:http:read
```

- [ ] **Step 2: Write failing Kubernetes-view tests**

Cover cluster-bound data-source selection, independent-source availability, built-in definition loading, namespace/workload variables, query grouping, no-data/unsupported panels, asset enrichment toggle, and metric-read permission.

- [ ] **Step 3: Implement seed rows and Kubernetes view**

Use lowercase paths/components. The view loads built-in definitions from the backend; do not duplicate PromQL in Vue. Cluster binding is a filter convenience, not a hard requirement.

- [ ] **Step 4: Add complete bilingual strings**

Add menus, forms, panel states, units, errors, TLS warning, query limits, built-in titles, and credential labels to both nested locale files. Do not add alert strings.

- [ ] **Step 5: Add permission audit test**

Scan observability views and assert mutating controls use exact write/delete codes, while query/view controls use metric/dashboard read codes. Assert route component casing resolves on Linux.

- [ ] **Step 6: Run seed, FE, and i18n tests**

Run:

```bash
cd optimus-be && rtk go test ./internal/seed -count=1
cd ../optimus-fe && rtk bun run test -- src/views/observability
rtk bun run i18n:check
rtk bun run typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add optimus-be/internal/seed/seed.go optimus-be/internal/seed/seed_test.go optimus-fe/src/views/observability/kubernetes optimus-fe/src/views/observability/__tests__/v-permission.test.ts optimus-fe/src/locales/zh-CN.json optimus-fe/src/locales/en-US.json
rtk git commit -m "feat(observability): add Kubernetes monitoring navigation"
```

## Task 16: Security audit, coverage, smoke guide, and final verification

**Files:**
- Modify: P5/P1 tests where coverage or security gaps are found
- Create: `optimus-be/scripts/p5-smoke.md`
- Modify: `CLAUDE.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Run the explicit forbidden-surface scan**

Run:

```bash
rtk rg -n 'observability.*alert|alert.*observability|cloudwatch|GetCloudKey|metric_samples|prometheus.*cache' optimus-be/internal/modules/observability optimus-fe/src/views/observability optimus-be/migrations/00022_p5_observability.sql
```

Expected: no alert/CloudWatch/sample-storage/client-cache implementation. Matches inside comments explicitly stating exclusions must be reviewed manually.

- [ ] **Step 2: Run secret and SSRF negative tests**

Add/fix tests until these invariants are explicit: broad `0.0.0.0/0` cannot permit metadata; mapped IPv6 cannot bypass checks; mixed DNS answer is denied; redirects are not followed; auth is absent from errors/log captures/audits/Swagger; custom CA is absent from audit; full PromQL is absent from audit/log captures.

Run:

```bash
cd optimus-be && rtk go test -race ./internal/modules/observability/... ./internal/modules/credentials/... -count=1
```

Expected: PASS.

- [ ] **Step 3: Audit package coverage**

Run:

```bash
rtk go test ./internal/modules/observability/... ./internal/modules/credentials/httpcredential -coverprofile=/tmp/p5-cover.out
rtk go tool cover -func=/tmp/p5-cover.out
```

Expected: every new package is at least 60%; transport, URL policy, credential service, and query service are at least 80%. Add focused behavioral tests, never assertion-free coverage calls.

- [ ] **Step 4: Write the manual smoke checklist**

`p5-smoke.md` must provide disposable local Prometheus setup, an explicit narrow CIDR allowlist, unauthenticated and one authenticated path, connection test, labels, instant/range batch, built-in view, custom dashboard CRUD, RBAC personas, audit/secret inspection, denied metadata/redirect tests, and teardown. It must state that production credentials/clusters are unnecessary.

- [ ] **Step 5: Update repository handoff guides**

Add P5 architecture, invariants, verification, and out-of-scope alert boundary to `CLAUDE.md` and concise operational rules to `AGENTS.md`. Update current phase to P5 implemented only after all verification passes; otherwise say P5 implementation is in progress.

- [ ] **Step 6: Run full backend verification**

Run from `optimus-be/`:

```bash
rtk make test
rtk make test-int
rtk make lint
rtk make swagger-diff
rtk make perm-check
```

Expected: all PASS. If Docker is unavailable, fix the environment/socket; do not weaken integration tests.

- [ ] **Step 7: Run full frontend verification**

Run from `optimus-fe/`:

```bash
rtk bun install --frozen-lockfile
rtk bun run lint
rtk bun run typecheck
rtk bun run i18n:check
rtk bun run test
rtk bun run build
```

Expected: all PASS.

- [ ] **Step 8: Verify repository hygiene**

Run from repository root:

```bash
rtk git diff --check
rtk git status --short
rtk rg -n 'FIXME|XXX|HACK' optimus-be/internal/modules/observability optimus-fe/src/views/observability optimus-be/scripts/p5-smoke.md
```

Expected: no whitespace errors, no generated drift, no unexpected files, and no unresolved markers.

- [ ] **Step 9: Commit the final hardening/handoff slice**

```bash
rtk git add optimus-be/internal/modules/observability optimus-be/internal/modules/credentials/httpcredential optimus-be/scripts/p5-smoke.md CLAUDE.md AGENTS.md
rtk git commit -m "chore(p5): harden and document observability MVP"
```

---

## Self-review checklist

- [ ] Every in-scope requirement in spec §§1–13 maps to at least one task.
- [ ] P1 HTTP credentials, P2 cluster guard, and optional P4 enrichment have explicit integration tasks.
- [ ] Data-source CRUD/test, labels, instant/range batch, custom dashboards, and built-ins have backend and frontend tasks.
- [ ] No task creates alerts, notifications, metric storage, CloudWatch, logs, traces, or APM.
- [ ] All code-changing steps name exact files, contracts, commands, and expected results.
- [ ] All introduced method/type/property names are consistent across tasks.
- [ ] Every implementation step provides concrete contracts and commands.
- [ ] Every task ends in a focused commit and preserves unrelated worktree changes.
