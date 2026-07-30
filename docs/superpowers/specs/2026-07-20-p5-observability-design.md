# P5 — observability Design

**Status**: Approved
**Date**: 2026-07-20
**Owner**: P5 sub-project
**Depends on**: P0 platform-skeleton, P1 credentials-vault, P2 k8s-mgmt
**Optional dependency**: P4 assets, through the internal `assets.Consumer` seam only

---

## 1. Goal and scope

P5 delivers a **unified metrics-observation MVP** for Optimus. Operators register
Prometheus-compatible data sources, run bounded PromQL queries through the
Optimus backend, and view built-in Kubernetes dashboards or saved custom
dashboards. Prometheus remains the metrics system of record; Optimus owns data
source configuration, query policy, visualization metadata, RBAC, and audit.

The primary user stories are:

1. An administrator registers and tests a Prometheus-compatible data source.
2. An operator opens a built-in Kubernetes overview and sees cluster, node, and
   workload CPU/memory metrics.
3. An operator runs an instant or range PromQL query without giving the browser
   direct network access to Prometheus.
4. An authorized user saves a dashboard made of time-series, stat, and table
   panels configured with forms rather than a drag-and-drop designer.
5. Where metrics expose a bounded set of instance/private-IP labels, P5 may add
   P4 asset context. Missing P4 data never makes a metrics query fail.

### 1.1 In scope

- Prometheus HTTP API v1-compatible data sources.
- No authentication, HTTP Basic authentication, and Bearer-token authentication.
- A small P1 extension for encrypted generic HTTP credentials.
- Data-source CRUD, connectivity tests, label metadata, instant queries, range
  queries, and same-data-source batch queries.
- Optional binding of a data source to one P2 Kubernetes cluster.
- Built-in Kubernetes overview dashboards based on common
  kube-prometheus-stack/kube-state-metrics conventions.
- User-saved dashboards and panels.
- Apache ECharts rendering for time-series, stat, and table panels.
- Optional, failure-tolerant P4 instance enrichment through `assets.Consumer`.
- Backend-enforced SSRF controls and query resource limits.
- Seven P5 permissions, mutation auditing, credential-consumption auditing,
  Swagger, generated permission documentation, and bilingual frontend strings.

### 1.2 Explicitly out of scope

- Alerts, alert rules, alert state, notifications, and alert evaluation workers.
- Logs, traces, APM, Sentry, OpenTelemetry collection, and correlation views.
- AWS CloudWatch or any use of P1 cloud credentials by P5.
- Storing, downsampling, compacting, or replaying metric samples in PostgreSQL.
- Deploying, configuring, or discovering Prometheus servers automatically.
- Grafana compatibility, dashboard import/export, and arbitrary plugin panels.
- OAuth2, cloud-provider request signing, and mTLS client certificates.
- A drag-and-drop dashboard builder or freely positioned/resizable panels.
- Offline dashboards when the upstream Prometheus endpoint is unavailable.
- Per-data-source or per-dashboard ACLs; P5 uses P0's medium-grained RBAC.

---

## 2. Dependency boundaries

### 2.1 Required dependencies

P0 remains the platform boundary for authentication, error envelopes, audit,
RBAC, menus, i18n, configuration, and deployment. P1 remains the sole owner of
encrypted secret material. P2 supplies the optional cluster identity used to
label a data source and select the built-in Kubernetes experience.

A P5 data source may be independent of P2. `cluster_id` is nullable, so an
operator can register a Prometheus endpoint that is not associated with an
Optimus-managed Kubernetes cluster. If a data source references a cluster, P2
cluster deletion is rejected until that reference is removed.

P5 does not use a kubeconfig to reach Prometheus. A kubeconfig and a Prometheus
HTTP credential are separate security and lifecycle objects.

### 2.2 Optional P4 enrichment

P4 is an enhancement, not a hard dependency:

- P5 receives an optional `assets.Consumer` at module wire time.
- P5 never imports P4 repositories or queries P4 tables directly.
- P5 never triggers an asset synchronization.
- P5 may call `LookupInstanceByPrivateIP` for a bounded set of unique, valid IP
  label values returned by a metrics query.
- No match, a stale asset, P4 unavailability, or a nil Consumer produces an
  unenriched result rather than a failed metrics query.
- Enrichment is opt-in per query (`enrich_assets=true`) and limited to 100
  unique addresses per batch response.

The P5 composition root therefore accepts P4 conditionally. The core
data-source, query, and dashboard services must be testable and operable with
no P4 module wired.

### 2.3 Data flow

```text
Vue dashboard
    |
    | POST /api/v1/observability/query-range (queries[])
    v
P5 query service
    |-- load data-source metadata from PostgreSQL
    |-- fetch HTTP credential once from credentials.Consumer (when configured)
    |-- validate/dial the destination through the SSRF-safe transport
    |-- execute bounded Prometheus API v1 requests
    |-- normalize results and upstream errors
    |-- optionally enrich bounded IP labels through assets.Consumer
    v
Envelope<BatchQueryResult>
```

The browser never connects directly to Prometheus and never receives a data
source credential.

---

## 3. P1 extension: generic HTTP credentials

P1 currently has SSH-key, kubeconfig, and cloud-key credentials. None is a
valid container for a Prometheus Basic password or Bearer token. P5 therefore
requires a deliberately small P1 extension rather than storing encrypted
secrets inside the observability module.

### 3.1 Table

```sql
CREATE TABLE credentials_http_credentials (
  id                  BIGSERIAL PRIMARY KEY,
  name                VARCHAR(128) NOT NULL,
  auth_type           VARCHAR(16)  NOT NULL,
  username            VARCHAR(256),
  secret_ciphertext   BYTEA        NOT NULL,
  created_by_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at          TIMESTAMPTZ,
  CONSTRAINT credentials_http_auth_type_check
    CHECK (auth_type IN ('basic', 'bearer')),
  CONSTRAINT credentials_http_username_check
    CHECK ((auth_type = 'basic' AND username IS NOT NULL AND username <> '') OR
           (auth_type = 'bearer' AND username IS NULL))
);

CREATE UNIQUE INDEX credentials_http_name_unique
  ON credentials_http_credentials(name) WHERE deleted_at IS NULL;
```

The password/token is encrypted by the existing AES-256-GCM vault. HTTP read
responses expose only id, name, auth type, username where applicable, creator,
and timestamps. They never expose `secret_ciphertext`, a password, or a token.

### 3.2 Consumer seam

P1 extends its public internal seam with a type equivalent to:

```go
type HTTPCredential struct {
    Name     string
    AuthType string // basic | bearer
    Username string // basic only
    Secret   []byte // password or bearer token
}

type Consumer interface {
    // existing methods remain unchanged
    GetHTTPCredential(ctx context.Context, id uint64, purpose string) (*HTTPCredential, error)
}

func WipeHTTPCredential(c *HTTPCredential)
```

Every successful consume follows P1's existing audit contract. P5 uses purposes
such as `observability.datasource.test`, `observability.query.instant`, and
`observability.query.range`. One batch request consumes the credential once,
then uses it for all queries in that batch. The query service defers the wipe
immediately after a successful consume and never caches plaintext credentials.

P1 adds `credentials:http:{read,write,delete,use}` permissions and CRUD UI/API
following the existing credential modules. Deleting an HTTP credential is
rejected while an active observability data source references it. The in-use
counter integration must remain nil-safe when P5 is not wired, following the
existing P1/P4 integration pattern.

---

## 4. P5 data model

P5 persists configuration only. Metric samples stay in Prometheus.

### 4.1 `observability_datasources`

```sql
CREATE TABLE observability_datasources (
  id                  BIGSERIAL PRIMARY KEY,
  name                VARCHAR(128) NOT NULL,
  base_url            TEXT         NOT NULL,
  auth_type           VARCHAR(16)  NOT NULL DEFAULT 'none',
  http_credential_id  BIGINT REFERENCES credentials_http_credentials(id)
                             ON DELETE RESTRICT,
  cluster_id          BIGINT REFERENCES clusters(id) ON DELETE RESTRICT,
  tls_skip_verify     BOOLEAN      NOT NULL DEFAULT FALSE,
  custom_ca_pem       TEXT,
  description         TEXT         NOT NULL DEFAULT '',
  created_by_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at          TIMESTAMPTZ,
  CONSTRAINT observability_datasource_auth_type_check
    CHECK (auth_type IN ('none', 'basic', 'bearer')),
  CONSTRAINT observability_datasource_credential_check
    CHECK ((auth_type = 'none' AND http_credential_id IS NULL) OR
           (auth_type IN ('basic', 'bearer') AND http_credential_id IS NOT NULL))
);

CREATE UNIQUE INDEX observability_datasource_name_unique
  ON observability_datasources(name) WHERE deleted_at IS NULL;
CREATE INDEX observability_datasource_cluster_idx
  ON observability_datasources(cluster_id) WHERE deleted_at IS NULL;
CREATE INDEX observability_datasource_credential_idx
  ON observability_datasources(http_credential_id) WHERE deleted_at IS NULL;
```

`auth_type` must match the referenced credential's type; this cross-row rule is
validated in the service. `base_url` contains only scheme, authority, and an
optional fixed path prefix. Query strings, fragments, and URL user information
are rejected. Only `http` and `https` schemes are accepted.

`tls_skip_verify` is allowed only for an administrator with data-source write
permission and is displayed prominently in the UI. `custom_ca_pem` stores only
public CA certificates, not client keys. Invalid PEM or a CA bundle containing
non-certificate material is rejected.

Data sources use soft deletion. A delete is rejected while an active custom
dashboard panel references the data source. Built-in dashboards are code
templates and do not create a database reference.

### 4.2 `observability_dashboards`

```sql
CREATE TABLE observability_dashboards (
  id                  BIGSERIAL PRIMARY KEY,
  name                VARCHAR(128) NOT NULL,
  description         TEXT         NOT NULL DEFAULT '',
  refresh_interval_s  INTEGER      NOT NULL DEFAULT 30,
  time_range          VARCHAR(16)  NOT NULL DEFAULT '1h',
  created_by_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at          TIMESTAMPTZ,
  CONSTRAINT observability_dashboard_refresh_check
    CHECK (refresh_interval_s BETWEEN 15 AND 3600),
  CONSTRAINT observability_dashboard_range_check
    CHECK (time_range IN ('15m', '1h', '6h', '24h', '7d'))
);

CREATE UNIQUE INDEX observability_dashboard_name_unique
  ON observability_dashboards(name) WHERE deleted_at IS NULL;
```

### 4.3 `observability_panels`

```sql
CREATE TABLE observability_panels (
  id             BIGSERIAL PRIMARY KEY,
  dashboard_id   BIGINT NOT NULL REFERENCES observability_dashboards(id)
                        ON DELETE CASCADE,
  datasource_id  BIGINT NOT NULL REFERENCES observability_datasources(id)
                        ON DELETE RESTRICT,
  title          VARCHAR(128) NOT NULL,
  panel_type     VARCHAR(16)  NOT NULL,
  promql         TEXT         NOT NULL,
  unit           VARCHAR(32)  NOT NULL DEFAULT 'none',
  legend         VARCHAR(128) NOT NULL DEFAULT '',
  sort_order     INTEGER      NOT NULL,
  width          SMALLINT     NOT NULL DEFAULT 12,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  CONSTRAINT observability_panel_type_check
    CHECK (panel_type IN ('time_series', 'stat', 'table')),
  CONSTRAINT observability_panel_promql_check
    CHECK (length(btrim(promql)) BETWEEN 1 AND 8192),
  CONSTRAINT observability_panel_width_check CHECK (width IN (6, 12)),
  CONSTRAINT observability_panel_order_unique UNIQUE (dashboard_id, sort_order)
);

CREATE INDEX observability_panel_datasource_idx
  ON observability_panels(datasource_id);
```

The dashboard create/update API accepts panels as one aggregate. The service
validates the complete aggregate and writes the dashboard plus replacement
panel set in one transaction. Panels do not have independent public mutation
routes in the MVP.

Allowed `unit` values are a small code registry (`none`, `percent`, `bytes`,
`bytes_per_second`, `cores`, `seconds`, `requests_per_second`) shared by BE and
FE validation. Arbitrary HTML, JavaScript formatters, and ECharts option blobs
are not accepted.

### 4.4 Built-in dashboards

Built-in dashboards are versioned Go/TypeScript definitions rather than rows in
the user tables. The MVP includes:

- Kubernetes cluster overview: CPU usage, memory usage, node readiness, pod
  phase counts, and pod restart rate.
- Kubernetes node overview: per-node CPU/memory and readiness.
- Kubernetes workload overview: namespace/workload CPU and memory, with
  deployment/statefulset/daemonset filters where the source labels permit it.

The templates assume common kube-prometheus-stack and kube-state-metrics names.
Before querying, the UI/backend may probe required metric names. Missing
metrics render a localized unsupported/no-data state per panel; they do not
mark the data source unhealthy.

---

## 5. Backend architecture

P5 follows the existing module layering and is wired once from
`cmd/server/main.go`:

```text
internal/modules/observability/
  datasource/   dto.go repo.go service.go handler.go routes.go
  dashboard/    dto.go repo.go service.go handler.go routes.go
  prometheus/   client.go transport.go dto.go errors.go
  query/        service.go handler.go routes.go enrichment.go
  builtin/      dashboards.go queries.go
  module/       wire.go routes.go
```

The module wire input includes `*gorm.DB`, `credentials.Consumer`,
`*audit.Recorder`, configuration, and an optional `assets.Consumer`. It returns
in-use counters for HTTP credential and P2 cluster deletion checks. P5 owns no
goroutines, scheduler, worker lifecycle, or in-memory state that affects
correctness.

### 5.1 Prometheus client lifecycle

A fresh logical client/transport is built per API request from immutable data
source metadata and the single consumed credential. It is discarded at request
completion. The transport:

- sets Basic or Bearer auth without placing secrets in URLs;
- applies the configured CA bundle and TLS policy;
- disables automatic decompression bombs by enforcing compressed and decoded
  response limits;
- validates every dial target and every redirect target;
- obeys request-context cancellation and a server-side timeout;
- never logs authorization headers or full upstream response bodies.

Connection pooling may use Go's transport internals only within that request;
P5 does not cache a client containing credential material across requests.

### 5.2 SSRF policy

Prometheus URLs are administrator-controlled but remain an SSRF boundary. P5
uses a dedicated resolver/dialer rather than validation at form-submit time
alone.

Required controls:

1. Accept only `http` and `https`; reject URL userinfo, fragments, and query
   strings in `base_url`.
2. Resolve the hostname for every connection attempt and validate every
   returned IP before dialing.
3. Reject unspecified, loopback, multicast, link-local, documentation, and
   otherwise non-routable addresses unconditionally.
4. Reject RFC1918, unique-local IPv6, and other private ranges unless the exact
   destination is covered by the deployment's
   `observability.allowed_private_cidrs` configuration.
5. Reject cloud metadata ranges even if a broad private CIDR is allowlisted.
6. Disable redirects by default. If same-origin redirects are enabled by a
   future configuration, resolve and validate the new target again and strip
   authorization on any origin change.
7. Dial the validated IP while preserving the original hostname for TLS SNI,
   preventing DNS rebinding between validation and connection.
8. Apply the same policy to data-source tests, label endpoints, and queries.

The default allowlist is empty. A typical internal deployment explicitly adds
only the Prometheus network, for example `10.20.30.0/24`, rather than all RFC1918
ranges.

### 5.3 Query limits

All limits are server configuration with conservative defaults; request values
may reduce but never expand them:

| Limit | Default |
|---|---:|
| Queries per batch | 12 |
| PromQL length per query | 8 KiB |
| Upstream timeout per batch | 15 s |
| Maximum range | 7 days |
| Minimum range-query step | 15 s |
| Maximum points per series | 11,000 |
| Maximum returned series per query | 1,000 |
| Maximum decoded upstream response | 16 MiB |
| Maximum asset-enrichment IPs | 100 |

The service validates `range / step` before the upstream call. Queries in a
batch execute with bounded concurrency (default 4). A failure is represented
per query so one invalid expression does not erase successful siblings, while
an authentication, connection, or SSRF failure applying to the data source may
fail the whole batch. Client cancellation cancels all remaining upstream work.

P5 does not attempt semantic PromQL cost analysis. Deployment owners must still
configure Prometheus-side query limits for defense in depth.

### 5.4 Result normalization

P5 accepts Prometheus API result types `vector`, `matrix`, `scalar`, and
`string`, and returns a stable Optimus DTO instead of passing the upstream JSON
envelope through verbatim. Samples use timestamp plus string value to preserve
Prometheus representation; the FE validates finite numeric values before
charting. Labels are string maps with count and key/value length limits.

Upstream warnings are returned as bounded strings. Upstream raw error bodies,
URLs containing credentials, authorization headers, Go errors, and TLS internals
never reach the client.

---

## 6. HTTP API

All endpoints use the P0 `{code,data,message,message_key?}` envelope and live
under `/api/v1/observability`.

### 6.1 Data sources

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/datasources` | `observability:datasource:read` | Paginated list |
| POST | `/datasources` | `observability:datasource:write` | Create |
| GET | `/datasources/{id}` | `observability:datasource:read` | Detail without secrets |
| PUT | `/datasources/{id}` | `observability:datasource:write` | Replace editable fields |
| DELETE | `/datasources/{id}` | `observability:datasource:delete` | Soft delete if unused |
| POST | `/datasources/{id}/test` | `observability:datasource:write` | Connectivity/build-info test |
| GET | `/datasources/{id}/labels` | `observability:metric:read` | Label names |
| GET | `/datasources/{id}/label-values?label=...` | `observability:metric:read` | Values for one validated label name |

List filters are `q`, `cluster_id`, `auth_type`, `page`, and `page_size`. The
connectivity test calls a low-cost Prometheus endpoint, returns version when
available, and never mutates `last_health_*` columns because the MVP stores no
health history or denormalized health state.

### 6.2 Queries

```http
POST /api/v1/observability/query
POST /api/v1/observability/query-range
Permission: observability:metric:read
```

Instant request shape:

```json
{
  "datasource_id": 12,
  "time": "2026-07-20T08:00:00Z",
  "enrich_assets": false,
  "queries": [
    {"ref_id": "cpu", "promql": "sum(rate(container_cpu_usage_seconds_total[5m]))"}
  ]
}
```

Range requests replace `time` with RFC3339 `start`, `end`, and a duration
`step`. `ref_id` is unique within the batch and is returned with its individual
result/error. The entire batch uses one data source; cross-data-source batches
are rejected so one request consumes at most one credential.

The API accepts PromQL because users with `metric:read` are explicitly allowed
to query metrics. It never accepts an arbitrary upstream path, HTTP method,
header, URL, or body.

### 6.3 Dashboards

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/dashboards` | `observability:dashboard:read` | Paginated custom-dashboard list |
| POST | `/dashboards` | `observability:dashboard:write` | Create dashboard plus panels |
| GET | `/dashboards/{id}` | `observability:dashboard:read` | Custom dashboard aggregate |
| PUT | `/dashboards/{id}` | `observability:dashboard:write` | Transactionally replace aggregate |
| DELETE | `/dashboards/{id}` | `observability:dashboard:delete` | Soft delete dashboard; hard-delete its panels transactionally |
| GET | `/builtin-dashboards` | `observability:metric:read` | List code-defined dashboards |
| GET | `/builtin-dashboards/{code}` | `observability:metric:read` | Built-in definition |

Custom dashboard reads require `dashboard:read`; rendering their panels also
requires `metric:read` at the query endpoint. Built-in dashboards require only
`metric:read` because they are part of metric viewing, not user-managed
configuration.

---

## 7. RBAC

P5 registers exactly seven permissions in
`internal/infra/permissions/codes.go`:

| Code | Meaning |
|---|---|
| `observability:datasource:read` | View data-source metadata |
| `observability:datasource:write` | Create, update, and test data sources |
| `observability:datasource:delete` | Delete data sources |
| `observability:metric:read` | Read labels, run PromQL, and view built-ins |
| `observability:dashboard:read` | View custom dashboards |
| `observability:dashboard:write` | Create and update custom dashboards |
| `observability:dashboard:delete` | Delete custom dashboards |

Metric queries require `metric:read`, not `datasource:read`. This allows an
operator to use an approved data source without seeing its administrative
configuration. A connectivity test is administrative and requires
`datasource:write`.

The built-in `admin` role receives all P5 and new P1 HTTP-credential
permissions. The built-in `viewer` receives P5 read permissions only. Internal
credential consumption trusts the P5 route boundary and does not perform a
second `credentials:http:use` check, matching the existing P1 Consumer design.

Route middleware remains the only HTTP authorization gate. Frontend route
metadata and `v-permission` controls must use the same codes.

---

## 8. Audit and secret handling

P5 records business audit events for configuration mutations:

| Action | Payload policy |
|---|---|
| `observability.datasource.create` | after-state without CA body or credential secret |
| `observability.datasource.update` | bounded before/after diff without secrets |
| `observability.datasource.delete` | prior non-secret metadata |
| `observability.datasource.test` | datasource id, purpose, success/failure class; no raw upstream error |
| `observability.dashboard.create` | dashboard metadata and panel count; no full PromQL |
| `observability.dashboard.update` | bounded diff and panel count; no full PromQL |
| `observability.dashboard.delete` | dashboard metadata and panel count |

Ordinary metric queries and dashboard views do not write P5 business audit
rows. Authenticated batches still produce one P1
`credentials.consume.http_credential` audit event because P1 records every
successful secret consume. That event stores data-source purpose and credential
identity, not password/token or PromQL. Unauthenticated data sources produce no
credential-consumption event.

Full PromQL is excluded from audit because label matchers may contain tenant,
host, or application identifiers. Application logs contain query `ref_id`,
duration, result counts, and a one-way query fingerprint rather than the full
expression. Authorization headers and secret bytes are redacted at the
transport boundary. Returned `HTTPCredential.Secret` bytes are wiped after the
batch; strings are not used for secret storage where avoidable.

Audit writes for mutations are transactionally coupled with the database
change where the existing `Recorder.WithTx` pattern permits it. A data-source
test writes its audit event after completion because it has no mutation
transaction.

---

## 9. Frontend design

P5 adds a top-level Observability menu and lowercase/kebab-case paths:

```text
/observability/datasources
/observability/dashboards
/observability/dashboards/:id
/observability/kubernetes
```

The frontend module follows existing API factory, Pinia, router, permission,
Ant Design Vue, and i18n patterns:

```text
src/api/observability/
src/stores/observability.ts
src/types/observability.ts
src/views/observability/
  datasources/
  dashboards/
  kubernetes/
  components/
    TimeSeriesPanel.vue
    StatPanel.vue
    TablePanel.vue
    PanelState.vue
```

Apache ECharts is added through bun and imported modularly so only required
charts/components are bundled. Panel components share a normalized series
adapter and render explicit loading, empty, unsupported-metric, partial-batch,
upstream-unavailable, and permission-denied states.

The dashboard editor is a form-based ordered list. Users choose a data source,
panel type, title, PromQL, unit, legend, and width (half/full). They can reorder
panels with move-up/move-down controls. The server owns validation; the FE
mirrors limits for immediate feedback. No raw ECharts option editor exists.

Auto-refresh is opt-in per opened dashboard using the saved interval, pauses
when the tab is hidden, cancels the previous request before a new refresh, and
does not retry authentication or validation failures in a tight loop. Panels
sharing a data source are grouped into a single batch query. A dashboard with
multiple data sources issues one bounded batch per source.

All new locale keys must exist in both `zh-CN` and `en-US`. Route permission
checks and DOM permission directives must remain aligned.

---

## 10. Error contract

P5 uses the 44xxx domain segment. All client responses remain inside the P0
envelope and are created with `apperr.New`/`apperr.Wrap`.

| Code | HTTP | `message_key` | Meaning |
|---:|---:|---|---|
| 44001 | 404 | `observability.datasource.not_found` | Data source not found |
| 44002 | 409 | `observability.datasource.name_taken` | Active name conflict |
| 44003 | 409 | `observability.datasource.in_use` | Dashboard panels reference it |
| 44004 | 400 | `observability.datasource.invalid_url` | Invalid base URL |
| 44005 | 400 | `observability.datasource.auth_mismatch` | Auth type/credential mismatch |
| 44006 | 400 | `observability.datasource.invalid_tls` | Invalid CA/TLS configuration |
| 44101 | 403 | `observability.query.destination_denied` | SSRF policy denied target |
| 44102 | 502 | `observability.query.upstream_unreachable` | Connect/DNS/TLS failure |
| 44103 | 504 | `observability.query.upstream_timeout` | Upstream timed out |
| 44104 | 502 | `observability.query.upstream_rejected` | Upstream API-level error |
| 44105 | 502 | `observability.query.invalid_response` | Malformed/oversized response |
| 44106 | 400 | `observability.query.limit_exceeded` | Batch/range/step/query limit |
| 44107 | 400 | `observability.query.invalid_request` | Invalid ref, time, label, or PromQL shape |
| 44201 | 404 | `observability.dashboard.not_found` | Dashboard not found |
| 44202 | 409 | `observability.dashboard.name_taken` | Active name conflict |
| 44203 | 400 | `observability.dashboard.invalid_panel` | Invalid aggregate/panel |
| 44204 | 404 | `observability.dashboard.builtin_not_found` | Unknown built-in code |

Per-query Prometheus expression errors are returned as bounded batch-item
errors using `44104` semantics; they do not change the outer envelope to a
failure when sibling queries succeeded. Whole-data-source failures use the
normal error envelope. Internal diagnostics are logged with request ID and
redaction, while client messages remain stable and localized.

The P1 HTTP-credential extension reuses P1's existing generic validation,
not-found, conflict, crypto, and in-use conventions rather than allocating P5
codes for credential failures.

---

## 11. Testing strategy

### 11.1 Backend unit tests

- Prometheus instant/range/batch request construction and normalized decoding
  for vector, matrix, scalar, and string results.
- Context cancellation, timeout, bounded concurrency, partial batch failure,
  response-size, series-count, point-count, step, range, and PromQL limits.
- Basic/Bearer/no-auth header construction and proof that secrets do not appear
  in logs, errors, DTOs, or audit payloads.
- SSRF cases: invalid schemes, URL userinfo, loopback, link-local, private
  address without allowlist, metadata endpoints, IPv4-mapped IPv6, multi-answer
  DNS, DNS rebinding, and redirects.
- Custom CA and `tls_skip_verify` validation.
- Built-in dashboard definitions, required metric probes, unit registry, panel
  validation, and PromQL variable substitution.
- P4 enrichment success, invalid IP, missing asset, nil Consumer, cancellation,
  deduplication, and the 100-address limit.
- Exact `(method,path,permission)` route snapshot for P5 and P1 additions.
- P2/P1 in-use counters, including nil-safe behavior when P5 is not wired.

### 11.2 Backend integration tests

- PostgreSQL CRUD, soft deletion, partial unique indexes, JSON/nullable fields,
  dashboard aggregate replacement, panel cascade, and data-source FK RESTRICT.
- P2 cluster deletion and P1 HTTP-credential deletion while referenced.
- HTTP credential encrypt/decrypt round-trip, consume audit, purpose validation,
  and wiping behavior.
- End-to-end handlers against `httptest.Server` implementations of Prometheus
  API v1; tests never require a real Prometheus process.
- Mutation audit rows and explicit assertions that secrets, CA bodies, and full
  PromQL are absent.
- Exact envelope/error mappings for upstream and validation failures.

### 11.3 Frontend tests

- API factories and Pinia state for data sources, batches, and dashboards.
- Data-source form auth/TLS conditional fields and connectivity-test states.
- Dashboard aggregate editor validation and ordering.
- Time-series/stat/table mapping, partial results, missing metrics, empty data,
  cancellation, hidden-tab pause, and refresh cleanup on unmount.
- Query grouping: panels sharing a data source produce one batch.
- Route-meta and `v-permission` consistency.
- `zh-CN`/`en-US` key parity.

### 11.4 Full verification and smoke

Backend verification includes `make test`, `make test-int`, `make lint`,
`make swag`, `make swagger-diff`, `make dump-perms`, and `make perm-check`.
Frontend verification includes `bun run lint`, `bun run typecheck`,
`bun run i18n:check`, `bun run test`, and `bun run build`.

The manual smoke checklist uses a disposable local Prometheus-compatible
instance and verifies unauthenticated plus one authenticated path, connection
test, labels, instant/range batching, a built-in Kubernetes dashboard when
sample metrics are available, custom dashboard CRUD, RBAC, audit redaction,
and denied SSRF targets. Production credentials and production clusters are not
required for CI or normal implementation verification.

---

## 12. Risks and mitigations

| Risk | Mitigation / accepted limitation |
|---|---|
| Backend becomes an SSRF proxy | Dedicated validated dialer, empty private-CIDR allowlist by default, metadata denylist, no arbitrary upstream paths, strict redirect policy |
| Expensive PromQL overloads Prometheus or Optimus | Batch/range/step/size/series limits, bounded concurrency, cancellation, upstream-side limits still required |
| Dashboard refresh floods credential audit | Group panels by data source; consume once and write one P1 audit event per batch |
| Secrets leak through logs/errors/audit | Byte-oriented secret type, deferred wipe, structured redaction, no raw upstream bodies, explicit negative tests |
| Prometheus variants differ | Support only documented API v1 shapes; normalize results; treat vendor extensions as out of contract |
| Built-in K8s metrics are absent or renamed | Target common kube-prometheus conventions and show per-panel unsupported/no-data states |
| P4 data is stale or unavailable | Optional bounded enrichment; core metrics result always survives enrichment failure |
| No local time-series copy | Accepted MVP limitation: upstream outage means no historical display |
| Large ECharts bundle or leaking chart instances | Modular imports, lazy routes, dispose on unmount, build-size observation |
| P2/P1 deletions break P5 | DB `RESTRICT` plus user-facing in-use checks; nil-safe cross-module wiring |
| `tls_skip_verify` weakens transport security | Explicit admin-only setting, visible UI warning, audited changes, default false |
| Single-server assumptions change | No scheduler or correctness-critical memory; query path remains stateless for future horizontal scale |

---

## 13. Delivery boundary and acceptance criteria

The P5 design is complete when this document is approved. Implementation must
not begin until a separate task-by-task plan is written and approved.

The eventual P5 MVP is acceptable when:

1. An authorized administrator can manage and test no-auth, Basic, and Bearer
   Prometheus-compatible data sources without exposing secrets.
2. An authorized viewer can execute bounded instant/range batches and render
   built-in or custom time-series/stat/table panels.
3. Independent Prometheus data sources work without P2/P4; cluster binding and
   P4 enrichment degrade safely when unavailable.
4. PostgreSQL contains configuration and audit state only, with no metric
   samples and no alert-related schema or code.
5. All routes enforce the seven confirmed P5 permissions and all mutation
   audits meet the redaction contract.
6. SSRF and resource-limit tests cover the negative cases in §11.
7. Swagger, permission docs, i18n, backend tests, frontend tests, lint,
   typecheck, and builds are clean.
