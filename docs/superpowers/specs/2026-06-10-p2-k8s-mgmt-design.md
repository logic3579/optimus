# P2 — k8s-mgmt Design

**Status**: Spec
**Date**: 2026-06-10
**Owner**: P2 sub-project
**Depends on**: P0 platform-skeleton (merged `c1149d9`, 2026-06-09 on `main`), P1 credentials-vault (merged `315ab0b`, 2026-06-10 on `main`)
**Downstream**: P3 applications (will consume the cluster registry + resource read APIs)

---

## 1. Goal and scope

P0 shipped auth / RBAC / users / roles / menus / audit / i18n / generic CRUD UI / deploy. P1 shipped the encrypted credentials vault with a Go-level `credentials.Consumer` seam that returns a decrypted kubeconfig YAML on demand.

P2 builds the **read-only Kubernetes management console** on top of P0 + P1. Operators can register Kubernetes clusters (each cluster row points at a P1 kubeconfig + a specific context inside it), browse a curated set of core API resources, stream Pod logs, and probe cluster health — all without ever leaving Optimus or shelling out to `kubectl`. P2 makes no `kubectl exec`, port-forward, write, or apply calls; those are explicitly deferred (§11).

What P2 ships:

1. A new `clusters` table + CRUD HTTP surface, with foreign-key to `credentials_kubeconfigs` (FK semantics in §3).
2. Per-request, no-cache Kubernetes clientset construction — every list/get/log call obtains a fresh `*rest.Config` from `credentials.Consumer.GetKubeconfig` and discards it after the response.
3. A typed read seam over 13 core resource kinds across four categories (workloads / network / config / cluster-scoped / pod logs); see §4 for the kind list.
4. Pod log streaming via SSE — one connection per pod+container, with manual reconnection only (no `EventSource` auto-reconnect).
5. 9 new permission codes under category `k8s` (§7), wired through P0's `RequirePermission` middleware.
6. Per-cluster on-demand health probe (`Discovery().ServerVersion()`) — no background poller.
7. A FE module under `src/views/k8s/` with 5 page surfaces, a global cluster picker in the layout header, and a CodeMirror-based YAML viewer.
8. A small refactor extracting the P0 single-flight refresh helper from `src/api/client.ts` into a Pinia store action so both axios (P0) and `fetch`-based streaming (P2) can share it.
9. One **upstream fix** to P1: tighten `kubeconfig` validation to reject `exec` / `auth-provider` plugins (§3 — kubeconfig RCE attack surface).

What P2 does **not** ship (§11):

- Any write / edit / apply / delete-of-k8s-objects path
- `kubectl exec` terminal, ephemeral debug containers, `kubectl debug`, port-forward, `kubectl cp`
- Watch / informer-based real-time list updates (manual refresh only)
- Background health polling
- CRDs / custom resources (only typed core APIs)
- Storage (PV / PVC / StorageClass), RBAC (Role / RoleBinding / SA), NetworkPolicy, HPA, PDB
- Metrics (CPU / mem) — defer to P5 observability
- Helm — defer to P3 applications
- SSE auto-reconnect, log replay, `--since=<duration>`

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ optimus-be                                                            │
│                                                                       │
│   internal/modules/k8s/                                              │
│     cluster/         ←── clusters table CRUD (model/repo/svc/handler) │
│     client/          ←── per-request Factory: rest.Config + clientset │
│     workload/        ←── List/Get for 7 workload kinds                │
│     network/         ←── List/Get for Service, Ingress                │
│     config/          ←── List/Get for ConfigMap, Secret, Secret/data  │
│     clusterscoped/   ←── List/Get for Namespace, Node, Event          │
│     log/             ←── SSE handler for Pod logs                     │
│     yaml/            ←── one endpoint, dispatch by ?kind              │
│     errs.go          ←── mapAPIError: apierrors → BizError 41xxx      │
│     module.go        ←── MountRoutes; assembles all of the above      │
│                                                                       │
│   cmd/server/main.go ←── wire k8s module after credentials module    │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
              │ imports credentials.Consumer (P1 seam)
              ▼
┌──────────────────────────────────────────────────────────────────────┐
│ credentials.Consumer.GetKubeconfig(ctx, kubeconfig_id, purpose)       │
│   purpose = "k8s.workload.list" / "k8s.log.stream" / "k8s.health.ping"│
│   side-effect: writes `credentials.consume` audit row per call        │
└──────────────────────────────────────────────────────────────────────┘
```

Each HTTP request walks: `gin handler → JWTAuth → RequirePermission → resource service → client.Factory.Clientset(clusterID, purpose) → credentials.Consumer.GetKubeconfig → clientcmd.RESTConfigFromKubeConfig → kubernetes.NewForConfig → typed clientset call → mapAPIError → DTO`. No state survives the request.

The FE module shape (`src/views/k8s/`) mirrors P0/P1: per-page `List.vue` + optional `Detail.vue` / `components/`. A new Pinia store `useK8sStore` holds the currently-selected cluster ID and a TTL-cached namespace list. The header layout adds a cluster picker dropdown visible only to users with any `k8s:*` permission.

---

## 3. Data model

### 3.1 New table: `clusters`

```sql
CREATE TABLE clusters (
  id              BIGSERIAL PRIMARY KEY,
  name            VARCHAR(64)  NOT NULL,
  kubeconfig_id   BIGINT       NOT NULL
                  REFERENCES credentials_kubeconfigs(id) ON DELETE RESTRICT,
  context         VARCHAR(128) NOT NULL,
  description     TEXT         NOT NULL DEFAULT '',
  tags            JSONB        NOT NULL DEFAULT '[]'::JSONB,
  last_health_at  TIMESTAMPTZ,
  last_health_ok  BOOLEAN,
  last_health_msg TEXT,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at      TIMESTAMPTZ,
  CONSTRAINT clusters_tags_is_array
    CHECK (jsonb_typeof(tags) = 'array'),
  CONSTRAINT clusters_name_unique
    UNIQUE (name) WHERE deleted_at IS NULL,
  CONSTRAINT clusters_kubeconfig_context_unique
    UNIQUE (kubeconfig_id, context) WHERE deleted_at IS NULL
);
CREATE INDEX clusters_kubeconfig_id_idx
  ON clusters(kubeconfig_id) WHERE deleted_at IS NULL;
CREATE INDEX clusters_tags_gin
  ON clusters USING GIN (tags) WHERE deleted_at IS NULL;
```

### 3.2 Foreign-key semantics

`ON DELETE RESTRICT`: a `credentials_kubeconfigs` row cannot be deleted while any `clusters` row references it. The P1 kubeconfig delete handler must surface this as `CodeConflict` (40901) + message key `credentials.kubeconfig.in_use` (see §9.2). FE's P1 kubeconfig list view adds a "referenced by N clusters" column (data via `GET /api/v1/k8s/clusters?kubeconfig_id=N`); FE blocks the delete button when N > 0 and shows the cluster names in the disabled-tooltip.

`ON UPDATE` is irrelevant — `credentials_kubeconfigs.id` is `BIGSERIAL` and immutable.

### 3.3 Health denormalization

Health is reflected in three columns on the cluster row, not a separate history table. Each `POST /clusters/{id}/ping` overwrites them. v1 keeps no history; the only consumers are the cluster list dot indicator and the cluster detail page. Add a `cluster_health_events` table if/when users ask for incident history (v2).

### 3.4 What is **not** stored

- `endpoint`, `ca_cert`, `server_version`, `node_count` — all are runtime-derived from `apiserver` and would be stale within seconds. Computed lazily by the detail page.
- `region` / `provider` — kubeconfig already encodes whatever auth/endpoint identifies the cluster; reproducing those fields drifts.
- `tags` is stored but is a free-form `JSONB` array of strings; no enum. Tags participate in list filtering only (`?tag=prod`).

### 3.5 Upstream patch to P1: kubeconfig validation

P1's `kubeconfig.Service.Create/Update` currently calls `clientcmd.Load` and asserts at least one context. It does **not** reject `users[].user.exec` or `users[].user.auth-provider`. These fields would cause `client-go` to spawn an external binary on the Optimus BE host whenever a clientset is built — an arbitrary-code-execution surface for anyone with `credentials:kubeconfig:write`.

P2's plan includes a small P1 patch: extend `kubeconfig.validateYAML` to walk `apiConfig.AuthInfos` and reject any entry with non-nil `Exec` or `AuthProvider`. The error is raised with the existing `CodeValidation` (40002) + sub-message keys `credentials.kubeconfig.exec_forbidden` / `credentials.kubeconfig.authprovider_forbidden`, matching P1's existing convention of "reuse codes + distinguish by `message_key`". The validation is also duplicated in P2's `client.Factory.RestConfig` as defense-in-depth — credentials that pre-date the P1 tightening (or were uploaded via a future bypass) still fail at first use.

---

## 4. Resource catalogue

P2 v1 supports these kinds. Each gets a Summary (List item) and Detail (Get response) DTO. Unsupported kinds in any endpoint return `CodeBadRequest` (40001) + `k8s.yaml.unsupported_kind`.

| Category | Kind | Namespaced? | Notes |
|---|---|---|---|
| workloads | Deployment | yes | summary: replicas {desired,ready,updated,available}, strategy |
| workloads | StatefulSet | yes | summary: replicas {desired,ready}, service name |
| workloads | DaemonSet | yes | summary: {desired,current,ready,available,misscheduled} |
| workloads | Job | yes | summary: completions, succeeded, failed, start/end time |
| workloads | CronJob | yes | summary: schedule, last schedule time, active jobs count |
| workloads | ReplicaSet | yes | summary: replicas {desired,ready}, owner ref |
| workloads | Pod | yes | summary: phase, ready N/M, restarts, node, IP, age, status reason |
| network | Service | yes | summary: type, cluster IP, external IPs, ports |
| network | Ingress | yes | summary: ingressClass, rules count, load balancer IPs |
| config | ConfigMap | yes | summary: data keys count, age |
| config | Secret | yes | summary: type, data keys count (names visible, **no values** via this endpoint) |
| cluster-scoped | Namespace | no | summary: phase, age |
| cluster-scoped | Node | no | summary: ready, schedulable, roles, kubelet, cpu/mem capacity, age |
| cluster-scoped | Event | yes/all | summary: type, reason, object, message, count, lastTimestamp |

Pod logs are streamed via SSE (§5.7), not a kind in the resource catalogue.

---

## 5. HTTP API

All under `/api/v1/k8s/`. Each route declares its permission via `RequirePermission` middleware (P0 pattern). The YAML endpoint is the lone exception — it dispatches permission internally based on `?kind=` (§5.6).

### 5.1 Cluster CRUD

| Method | Path | Perm | Description |
|---|---|---|---|
| GET | `/k8s/clusters?search=&tag=&kubeconfig_id=` | `k8s:cluster:read` | List clusters, ProTable-paginated |
| POST | `/k8s/clusters` | `k8s:cluster:write` | Create cluster row; validates `context` exists in referenced kubeconfig YAML, validates no `exec`/`auth-provider` (defense in depth) |
| GET | `/k8s/clusters/{id}` | `k8s:cluster:read` | Get cluster row + joined kubeconfig name |
| PUT | `/k8s/clusters/{id}` | `k8s:cluster:write` | Update name / description / tags / context / kubeconfig_id; revalidates same as POST |
| DELETE | `/k8s/clusters/{id}` | `k8s:cluster:write` | Soft delete |
| POST | `/k8s/clusters/{id}/ping` | `k8s:cluster:read` | Probe (§5.8) |

Cluster CRUD is the only category that writes to the DB. Everything else is read-through to apiserver.

### 5.2 Resource list/get endpoints

Each runs `Factory.Clientset(clusterID, purpose)` → typed clientset call → DTO mapping. All accept `?namespace=` (empty = cluster-wide for namespaced kinds), `?limit=` (default 500), `?continue=`, `?search=` (FE-side filter passthrough; ignored by BE).

```
GET /k8s/clusters/{id}/namespaces                                  perm: k8s:cluster_resource:read
GET /k8s/clusters/{id}/nodes [/{name}]                             perm: k8s:cluster_resource:read
GET /k8s/clusters/{id}/events?namespace=                           perm: k8s:cluster_resource:read

GET /k8s/clusters/{id}/workloads/{kind} [/{ns}/{name}]             perm: k8s:workload:read
    kind ∈ {deployments, statefulsets, daemonsets, jobs, cronjobs, replicasets, pods}

GET /k8s/clusters/{id}/network/{kind} [/{ns}/{name}]               perm: k8s:network:read
    kind ∈ {services, ingresses}

GET /k8s/clusters/{id}/config/configmaps [/{ns}/{name}]            perm: k8s:config:read

GET /k8s/clusters/{id}/secrets [/{ns}/{name}]                       perm: k8s:secret:read
GET /k8s/clusters/{id}/secrets/{ns}/{name}/data                     perm: k8s:secret:reveal
```

### 5.3 Pagination

`limit` + `continue` are passed straight to apiserver `metav1.ListOptions`. Response envelope:

```json
{
  "code": 0,
  "data": {
    "items": [...],
    "continue": "eyJ..." | "",
    "truncated": true | false
  }
}
```

`truncated == true` ⇔ `limit` was reached and apiserver returned a non-empty continue token. FE v1 displays a hint banner ("showing first 500; narrow by namespace or search to see more") but does **not** expose a "next page" button. The continue token is plumbed through the envelope so a future v2 button can wire up without API changes.

### 5.4 Item DTO shape

Each kind has `K8sXxxSummary` (List item) and `K8sXxxDetail` (Get response). Detail = Summary + `conditions[]`, `labels`, `annotations`, `owner_references`, and kind-specific subtree (e.g., Pod's `containers[]`, Deployment's `strategy`). Detail does **not** include full spec — for full YAML, use §5.6.

### 5.5 Secret data revelation

```
GET /k8s/clusters/{id}/secrets/{ns}/{name}/data
    perm: k8s:secret:reveal
    →   {"data": {"key": "<decoded plaintext UTF-8 string>", ...}}
```

Keys that aren't valid UTF-8 are returned as base64 with a `"<key>:base64": true` sibling marker. No audit row beyond the underlying `credentials.consume(purpose=k8s.secret.reveal)`.

### 5.6 Universal YAML endpoint

```
GET /k8s/clusters/{id}/yaml?kind=&namespace=&name=
    Content-Type: text/yaml
```

The handler maintains a hardcoded `yamlKindPerm: map[string]string` (see §7.3) and calls `cache.Check(userID, perm)` itself; the route is NOT wrapped in `RequirePermission`. Unsupported kinds → `CodeBadRequest` (40001) + `k8s.yaml.unsupported_kind`. For `kind=secret`, **`k8s:secret:read` is sufficient** — the YAML response includes base64-encoded `data`. The product position is: "see decoded plaintext requires `:reveal`; see base64 in YAML does not." Operators who want to gate base64 too should limit `k8s:secret:read` accordingly. This boundary is intentional and called out in §11.5.

### 5.7 Pod log SSE

```
GET /k8s/clusters/{id}/pods/{ns}/{name}/log
    ?container=&follow=true&tailLines=200&previous=false
    perm: k8s:log:read
    Content-Type: text/event-stream
```

Headers:

```
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

Wire format: one line per log entry as `data: <text>\n\n`. Keep-alive every 30s as a SSE comment line: `: keepalive\n\n`. Stream end (either `follow=false` natural end, pod termination, or apiserver disconnect) closes the response with no special marker — the client's `fetch().body.getReader()` sees `done=true`.

The Factory's 10s request-level timeout does **not** apply to SSE: the handler builds its own `rest.Config` for log routes (no `cfg.Timeout`), and the gin `http.Server.WriteTimeout` is set to `0` for paths under `/api/v1/k8s/*/log` via a per-route timeout override (P0's current global is 60s; we add a fan-out).

### 5.8 Health probe

```
POST /k8s/clusters/{id}/ping
    perm: k8s:cluster:read
    → {"ok": true|false, "server_version": "v1.30.5", "message": ""}
```

5-second timeout. Probe = `cs.Discovery().ServerVersion()` (no auth scope needed beyond what kubeconfig provides). Result is persisted to the cluster row's `last_health_*` columns synchronously before responding. There is **no** background poller in v1; FE's cluster list page offers a "refresh all" action that fan-outs in parallel.

### 5.9 Endpoint authentication for SSE

P0's `JWTAuth` middleware accepts **only** `Authorization: Bearer <token>` headers. The SSE endpoint is no exception — but `EventSource` in browsers cannot set headers. **P2 does not change the auth middleware contract.** Instead, FE uses `fetch()` + `ReadableStream` with a manual SSE parser (`useLogStream` composable, §8.3); this keeps the JWT in the header and out of URLs / access logs.

---

## 6. Backend internals

### 6.1 Client factory (`internal/modules/k8s/client/factory.go`)

```go
type Factory struct {
    consumer credentials.Consumer
    repo     cluster.ReadRepo
}

func (f *Factory) RestConfig(ctx context.Context, clusterID uint64, purpose string) (*rest.Config, error) {
    c, err := f.repo.Get(ctx, clusterID)
    if err != nil { return nil, err }                               // CodeNotFound / k8s.cluster.not_found
    kc, err := f.consumer.GetKubeconfig(ctx, c.KubeconfigID, purpose)
    if err != nil { return nil, err }                               // bubble P1 errors
    return buildRestConfig(kc.YAML, c.Context)                      // CodeValidation w/ sub-keys
}

func (f *Factory) Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error) {
    cfg, err := f.RestConfig(ctx, clusterID, purpose)
    if err != nil { return nil, err }
    cfg.Timeout = 10 * time.Second
    return kubernetes.NewForConfig(cfg)
}
```

`buildRestConfig` (excerpt) — error codes follow P0/P1 convention of reusing existing numeric codes + sub-message-key for granularity:

```go
func buildRestConfig(y []byte, contextName string) (*rest.Config, error) {
    apiCfg, err := clientcmd.Load(y)
    if err != nil { return nil, apperr.New(apperr.CodeValidation, "k8s.kubeconfig.invalid", err.Error()) }
    for name, u := range apiCfg.AuthInfos {
        if u.Exec != nil {
            return nil, apperr.New(apperr.CodeValidation, "k8s.kubeconfig.exec_forbidden", "user "+name+" uses exec auth plugin")
        }
        if u.AuthProvider != nil {
            return nil, apperr.New(apperr.CodeValidation, "k8s.kubeconfig.authprovider_forbidden", "user "+name+" uses auth-provider plugin")
        }
    }
    if _, ok := apiCfg.Contexts[contextName]; !ok {
        return nil, apperr.New(apperr.CodeValidation, "k8s.kubeconfig.context_not_found", "context "+contextName+" not found")
    }
    apiCfg.CurrentContext = contextName
    return clientcmd.NewDefaultClientConfig(*apiCfg, &clientcmd.ConfigOverrides{}).ClientConfig()
}
```

No clientset caching. Reasoning: <50 users + on-demand-refresh UX = at most a few requests per second peak; TLS handshake cost is sub-100ms. A future LRU cache keyed by `(cluster_id, last_kubeconfig_updated_at)` is a one-file addition that doesn't change the Factory's external API.

### 6.2 Error mapping (`internal/modules/k8s/errs.go`)

```go
func mapAPIError(err error) error {
    switch {
    case err == nil:                       return nil
    case apierrors.IsNotFound(err):        return apperr.New(apperr.CodeNotFound,             "k8s.apiserver.not_found",    err.Error())
    case apierrors.IsForbidden(err):       return apperr.New(apperr.CodeAPIServerForbidden,   "k8s.apiserver.forbidden",    err.Error())
    case apierrors.IsUnauthorized(err):    return apperr.New(apperr.CodeAPIServerUnauthorized,"k8s.apiserver.unauthorized", err.Error())
    case apierrors.IsTimeout(err):         return apperr.New(apperr.CodeClusterUnreachable,   "k8s.cluster.unreachable",    err.Error())
    case isNetworkErr(err):                return apperr.New(apperr.CodeClusterUnreachable,   "k8s.cluster.unreachable",    err.Error())
    case apierrors.IsServerTimeout(err):   return apperr.New(apperr.CodeClusterUnreachable,   "k8s.cluster.unreachable",    err.Error())
    default:                               return apperr.New(apperr.CodeAPIServerOther,       "k8s.apiserver.other",        err.Error())
    }
}
```

`isNetworkErr` recognises `net.Error` (dial/EOF/refused), `x509.UnknownAuthorityError`, `tls.RecordHeaderError`, and `*url.Error` whose underlying is one of those. `CodeAPIServerForbidden`, `CodeAPIServerUnauthorized`, `CodeAPIServerOther`, and `CodeClusterUnreachable` are the four new numeric codes P2 adds (see §9).

### 6.3 SSE log handler (`internal/modules/k8s/log/handler.go`)

```go
func (h *Handler) Stream(c *gin.Context) {
    clusterID := mustParseID(c, "id")
    ns, pod := c.Param("ns"), c.Param("name")
    opts := corev1.PodLogOptions{
        Container:  c.Query("container"),
        Follow:     c.Query("follow") == "true",
        TailLines:  ptr(parseInt64(c.Query("tailLines"), 200)),
        Previous:   c.Query("previous") == "true",
        Timestamps: true,
    }

    cs, err := h.factory.ClientsetForStream(c.Request.Context(), clusterID, "k8s.log.stream")
    if err != nil { response.Error(c, err); return }

    streamCtx, cancel := context.WithCancel(c.Request.Context())
    defer cancel()
    rc, err := cs.CoreV1().Pods(ns).GetLogs(pod, &opts).Stream(streamCtx)
    if err != nil { response.Error(c, mapAPIError(err)); return }
    defer rc.Close()

    c.Writer.Header().Set("Content-Type", "text/event-stream")
    c.Writer.Header().Set("Cache-Control", "no-cache")
    c.Writer.Header().Set("Connection", "keep-alive")
    c.Writer.Header().Set("X-Accel-Buffering", "no")
    c.Writer.WriteHeader(http.StatusOK)
    c.Writer.Flush()

    sc := bufio.NewScanner(rc)
    sc.Buffer(make([]byte, 1<<20), 1<<20)
    done := make(chan struct{})
    go func() {
        defer close(done)
        for sc.Scan() {
            fmt.Fprintf(c.Writer, "data: %s\n\n", sc.Text())
            c.Writer.Flush()
        }
    }()

    ka := time.NewTicker(30 * time.Second)
    defer ka.Stop()
    for {
        select {
        case <-c.Request.Context().Done(): return
        case <-done: return
        case <-ka.C:
            fmt.Fprint(c.Writer, ": keepalive\n\n")
            c.Writer.Flush()
        }
    }
}
```

`ClientsetForStream` is a Factory variant that omits `cfg.Timeout` (regular streams must not deadline on the 10s request timeout).

### 6.4 SSE bypass of server write timeout

Current `http.Server.WriteTimeout` is driven by `cfg.Server.WriteTimeout` (default **15s** per `configs/config.yaml`). Any handler that streams for more than 15s after the first write will be killed mid-response. SSE log streaming with `follow=true` will inevitably trip this.

The simplest, modern fix: use `http.ResponseController` (Go 1.20+) to clear the per-request write deadline inside the SSE handler. No changes required to server-level timeout, no new middleware, no `Hijacker` ceremony.

```go
func (h *Handler) Stream(c *gin.Context) {
    rc := http.NewResponseController(c.Writer)
    if err := rc.SetWriteDeadline(time.Time{}); err != nil {
        // SetWriteDeadline returned ErrNotSupported — should not happen with
        // net/http's default ResponseWriter; log and continue (15s timeout will apply)
        slog.WarnContext(c, "sse: cannot clear write deadline", "err", err)
    }
    // ... rest of SSE handler from §6.3
}
```

Server-level `cfg.Server.WriteTimeout` stays unchanged. The single line `rc.SetWriteDeadline(time.Time{})` in the SSE handler is the only "exemption" — no global middleware redesign.

---

## 7. RBAC

### 7.1 New permission codes (`internal/infra/permissions/codes.go`)

```go
{Code: "k8s:cluster:read",           Name: "perm.k8s.cluster.read",           Category: "k8s", Description: "Read clusters"},
{Code: "k8s:cluster:write",          Name: "perm.k8s.cluster.write",          Category: "k8s", Description: "Create/update/delete clusters"},
{Code: "k8s:workload:read",          Name: "perm.k8s.workload.read",          Category: "k8s", Description: "Read workload resources"},
{Code: "k8s:network:read",           Name: "perm.k8s.network.read",           Category: "k8s", Description: "Read services and ingresses"},
{Code: "k8s:config:read",            Name: "perm.k8s.config.read",            Category: "k8s", Description: "Read configmaps"},
{Code: "k8s:secret:read",            Name: "perm.k8s.secret.read",            Category: "k8s", Description: "Read secret metadata and keys"},
{Code: "k8s:secret:reveal",          Name: "perm.k8s.secret.reveal",          Category: "k8s", Description: "Decode secret data values"},
{Code: "k8s:cluster_resource:read",  Name: "perm.k8s.cluster_resource.read",  Category: "k8s", Description: "Read namespaces, nodes, and events"},
{Code: "k8s:log:read",               Name: "perm.k8s.log.read",               Category: "k8s", Description: "Stream pod logs"},
```

Conventions follow P0/P1: snake_case for multi-word resource segments (`cluster_resource`), `Description` is short imperative English, `Name` is the FE i18n key.

### 7.2 Default role bindings

The seed's existing admin-role grant pulls all codes from the registry, so the 9 new codes are picked up automatically on next `make seed`. No additional seeded roles (P0 only seeds `admin`).

### 7.3 YAML endpoint dispatch

The hardcoded map in `internal/modules/k8s/yaml/handler.go`:

```go
var yamlKindPerm = map[string]string{
    "deployment":  "k8s:workload:read",
    "statefulset": "k8s:workload:read",
    "daemonset":   "k8s:workload:read",
    "pod":         "k8s:workload:read",
    "job":         "k8s:workload:read",
    "cronjob":     "k8s:workload:read",
    "replicaset":  "k8s:workload:read",
    "service":     "k8s:network:read",
    "ingress":     "k8s:network:read",
    "configmap":   "k8s:config:read",
    "secret":      "k8s:secret:read",       // see §5.6 + §11.5
    "namespace":   "k8s:cluster_resource:read",
    "node":        "k8s:cluster_resource:read",
    "event":       "k8s:cluster_resource:read",
}
```

---

## 8. Frontend

### 8.1 Routes (dynamic, registered from `/me/menus`)

```
/k8s/clusters                          → views/k8s/clusters/List.vue           perm: k8s:cluster:read
/k8s/clusters/:id                      → views/k8s/clusters/Detail.vue         perm: k8s:cluster:read
/k8s/workloads                         → views/k8s/workloads/List.vue          perm: k8s:workload:read
/k8s/network                           → views/k8s/network/List.vue            perm: k8s:network:read
/k8s/config                            → views/k8s/config/List.vue             perm: k8s:config:read OR k8s:secret:read (route guard accepts either)
/k8s/cluster-resources                 → views/k8s/cluster-resources/List.vue  perm: k8s:cluster_resource:read
```

Pages 3–6 check `useK8sStore().currentClusterId` in `onBeforeMount`; if null, redirect to `/k8s/clusters` with a transient antd `message.info`.

### 8.2 Pinia store (`src/stores/k8s.ts`)

```ts
export const useK8sStore = defineStore('k8s', () => {
  const currentClusterId = ref<number | null>(null)
  const currentClusterName = ref<string>('')
  const namespaces = ref<string[]>([])
  const namespacesFetchedAt = ref<number>(0)
  const currentNamespace = ref<string>('')

  function setCluster(id: number, name: string) { /* clears namespace cache on switch */ }
  async function ensureNamespaces(api: K8sNamespaceApi) { /* 5min TTL */ }

  return { currentClusterId, currentClusterName, namespaces, currentNamespace,
           setCluster, ensureNamespaces }
}, { persist: { paths: ['currentClusterId', 'currentClusterName'] } })
```

`pinia-plugin-persistedstate` is added if not already present (P0 used Pinia without persistence; this is the first persistent slice).

### 8.3 Log streaming composable (`src/composables/useLogStream.ts`)

`fetch()` + `ReadableStream` based SSE parser. No `EventSource` because:
- `EventSource` cannot set `Authorization` headers; the JWT would leak into URLs.
- Auto-reconnect from byte 0 is wrong UX for log tail; manual reconnect with sinceTime is better.
- `AbortController.abort()` integrates cleanly with `onScopeDispose`.

```ts
export interface LogStreamOpts {
  clusterId: number; namespace: string; pod: string
  container?: string; follow?: boolean; tailLines?: number; previous?: boolean
}

export function useLogStream() {
  const lines = ref<string[]>([])
  const status = ref<'idle' | 'open' | 'closed' | 'error'>('idle')
  let ctl: AbortController | null = null

  async function open(opts: LogStreamOpts) {
    close()
    ctl = new AbortController()
    status.value = 'open'
    const qs = new URLSearchParams({
      container: opts.container ?? '',
      follow: String(opts.follow ?? true),
      tailLines: String(opts.tailLines ?? 200),
      previous: String(opts.previous ?? false),
    })
    const url = `/api/v1/k8s/clusters/${opts.clusterId}/pods/${opts.namespace}/${opts.pod}/log?${qs}`
    const res = await fetch(url, {
      headers: { Authorization: `Bearer ${useAuthStore().accessToken}` },
      signal: ctl.signal,
    })
    if (res.status === 401) {
      await useAuthStore().refreshAccessTokenShared()                  // §8.5
      return open(opts)                                                // single retry
    }
    if (!res.ok || !res.body) { status.value = 'error'; return }
    const reader = res.body.getReader()
    const dec = new TextDecoder()
    let buf = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buf += dec.decode(value, { stream: true })
      let nl: number
      while ((nl = buf.indexOf('\n\n')) >= 0) {
        const chunk = buf.slice(0, nl); buf = buf.slice(nl + 2)
        if (chunk.startsWith('data: ')) lines.value.push(chunk.slice(6))
        // skip comment lines (": keepalive")
      }
    }
    status.value = 'closed'
  }

  function close() { ctl?.abort(); ctl = null }
  onScopeDispose(close)
  return { lines, status, open, close }
}
```

### 8.4 LogViewer component (`src/components/k8s/LogViewer.vue`)

Virtual-scrolled `<a-list>` of `lines`, with:
- Container picker (dropdown of `pod.spec.containers[].name + initContainers[].name`)
- `tailLines` chooser (100 / 500 / 2000 / 10000)
- `follow` toggle (also pauses auto-scroll-to-bottom when paused)
- `previous` toggle (visible only if pod has `lastTerminationState`)
- "Reconnect" button that calls `close()` then `open()`

Performance: cap `lines.value` at 50,000 entries (drop from head on overflow). One log line ≈ 200B → ~10MB peak in memory.

### 8.5 Refactor: shared single-flight refresh

P0's `src/api/client.ts` currently holds the `refreshing: Promise<TokenPair> | null` single-flight state inside the axios interceptor closure. P2 moves it to a `useAuthStore` action `refreshAccessTokenShared()` so both axios and `fetch`-based streams use the same in-flight promise. Behaviour preserved exactly: concurrent 401s share one refresh; an inner 401 on `/auth/refresh` calls `onLogout()`; on success, callers replay their original request once. Tests in `src/api/__tests__/refresh.test.ts` migrate from axios-mock to store-action mock.

### 8.6 YAML viewer (`src/components/k8s/YamlViewer.vue`)

Powered by `vue-codemirror` + `@codemirror/lang-yaml` + `@codemirror/theme-one-dark`. Read-only mode (no edit in v1). Bundle impact: ~300KB gzip total; acceptable. Not Monaco — kept off the table to avoid the ~2MB shipped weight and the Vite worker setup.

### 8.7 Cluster picker (header)

`src/components/layout/ClusterPicker.vue`, slotted into `DefaultLayout.vue` immediately left of the locale switcher. Renders nothing when the user has no `k8s:*` permission. Otherwise a dropdown of clusters with health dot + name + tags pill; selection triggers `useK8sStore().setCluster()` and, if the user is on a k8s subpage, refreshes the current view.

### 8.8 i18n key additions

Both `zh-CN.json` and `en-US.json` add the keys below; `bun run i18n:check` enforces parity in CI.

- `menu.k8s`, `menu.k8s.clusters`, `menu.k8s.workloads`, `menu.k8s.network`, `menu.k8s.config`, `menu.k8s.cluster-resources`
- `perm.category.k8s`, plus 9 `perm.k8s.*` keys matching §7.1
- `k8s.cluster.*` (form fields, list columns, ping result strings)
- `k8s.workload.<kind>.*` for each of the 7 workload kinds (status colour map, column headers)
- `k8s.network.*`, `k8s.config.*`, `k8s.secret.*`, `k8s.event.*`, `k8s.node.*`, `k8s.namespace.*`
- `k8s.log.*` (controls, status text, reconnect button)
- `k8s.cluster.not_found`, `k8s.cluster.name_taken`, `k8s.cluster.unreachable` (cluster-scope message keys from §9.2 / §9.1)
- `k8s.kubeconfig.{invalid,exec_forbidden,authprovider_forbidden,context_not_found}` — also referenced by P1's patched kubeconfig validator (single source of translation)
- `k8s.apiserver.{not_found,forbidden,unauthorized,other}` — runtime apiserver failure surface
- `k8s.yaml.unsupported_kind`, `k8s.log.unavailable`
- `credentials.kubeconfig.in_use` — raised from P1 delete handler; lives under `credentials.*` namespace because the user is in the P1 kubeconfig page when they see it

---

## 9. Errors

P0/P1 convention: reuse the existing numeric codes in `internal/infra/errors/codes.go` and distinguish granular semantics via the `message_key` (which the FE translates through the locale files). P2 follows this rule for all *client-facing validation / conflict / not-found / forbidden* cases, and adds **5 new numeric codes** only where the existing block has no good fit — specifically for **runtime apiserver failures** (kubeconfig RBAC denial, kubeconfig auth expired, generic apiserver `StatusError`) and **cluster reachability failures** (timeout / network).

### 9.1 New numeric codes (append to `internal/infra/errors/codes.go`)

```go
// 41xxx k8s runtime — runtime failures reaching or talking to apiserver.
// Distinct from 40xxx client errors because they encode upstream-dependency
// state, not malformed/unauthorized client requests.
CodeClusterUnreachable     Code = 41101 // network/timeout reaching apiserver
CodeAPIServerForbidden     Code = 41103 // kubeconfig user's RBAC denies the call
CodeAPIServerUnauthorized  Code = 41104 // kubeconfig credentials expired/invalid
CodeAPIServerOther         Code = 41105 // generic apiserver StatusError
CodeLogUnavailable         Code = 41202 // pod log unavailable (pending/init/no previous)
```

`apierrors.IsNotFound` from apiserver maps to existing `CodeNotFound` (40401) with `k8s.apiserver.not_found` — same numeric code as a 404 on cluster row, FE just renders the message key.

### 9.2 Reused codes + new message_keys

| Reused Code | Message key | Raised at |
|---|---|---|
| `CodeNotFound` (40401) | `k8s.cluster.not_found` | cluster repo Get |
| `CodeNotFound` (40401) | `k8s.apiserver.not_found` | mapAPIError when apiserver returns 404 |
| `CodeConflict` (40901) | `k8s.cluster.name_taken` | cluster repo Create on partial-unique-index violation |
| `CodeConflict` (40901) | `credentials.kubeconfig.in_use` | **raised from P1** kubeconfig delete handler when any cluster references it |
| `CodeValidation` (40002) | `k8s.kubeconfig.invalid` | buildRestConfig — clientcmd.Load failure |
| `CodeValidation` (40002) | `k8s.kubeconfig.exec_forbidden` | buildRestConfig — auth info has Exec plugin (also P1 validateYAML) |
| `CodeValidation` (40002) | `k8s.kubeconfig.authprovider_forbidden` | buildRestConfig — auth info has AuthProvider plugin (also P1 validateYAML) |
| `CodeValidation` (40002) | `k8s.kubeconfig.context_not_found` | buildRestConfig — context not present in YAML |
| `CodeBadRequest` (40001) | `k8s.yaml.unsupported_kind` | YAML handler when kind not in §7.3 map |

### 9.3 Cross-module dependency for `credentials.kubeconfig.in_use`

P1's kubeconfig delete handler must learn about P2's cluster references. Approach: P2 ships a lightweight read-only helper package `internal/modules/k8s/cluster/inuse` exposing one function:

```go
package inuse
func CountByKubeconfigID(ctx context.Context, db *gorm.DB, kubeconfigID uint64) (int64, error)
```

P1 imports this single function (no broader k8s coupling), and raises `apperr.New(apperr.CodeConflict, "credentials.kubeconfig.in_use", "kubeconfig is referenced by N clusters")` when count > 0. The function reads `clusters` filtered on `deleted_at IS NULL`. This is an intentional one-way dependency: P1 → tiny P2 read helper. P2 already depends on P1's Consumer in the other direction; the helper package is small enough to avoid creating a cycle.

---

## 10. Audit

Existing `audit_logs` schema is unchanged.

New action codes raised by P2. `target_type` uses the P1-established `<category>.<resource>` convention (`credentials.kubeconfig`, `credentials.ssh_key`, …) — so cluster rows are `k8s.cluster`:

| action | trigger | target_type | target_id | payload |
|---|---|---|---|---|
| `k8s.cluster.create` | `POST /clusters` | `k8s.cluster` | new ID | `{name, kubeconfig_id, context, tags}` |
| `k8s.cluster.update` | `PUT /clusters/{id}` | `k8s.cluster` | id | `{name, changed_fields:[...]}` |
| `k8s.cluster.delete` | `DELETE /clusters/{id}` | `k8s.cluster` | id | `{name}` (denormalized snapshot for post-delete audit display) |
| `k8s.cluster.ping` | `POST /clusters/{id}/ping` | `k8s.cluster` | id | `{name, ok, server_version, error}` |

Per-request resource reads do **not** emit a separate P2 audit row. They are covered by the `credentials.consume` row that P1 writes from `Consumer.GetKubeconfig`, with a P2-specific `purpose` string discriminator:

```
purpose=k8s.workload.list       /  k8s.workload.get
purpose=k8s.network.list        /  k8s.network.get
purpose=k8s.config.list         /  k8s.config.get
purpose=k8s.secret.list         /  k8s.secret.get  /  k8s.secret.reveal
purpose=k8s.cluster_resource.list / k8s.cluster_resource.get
purpose=k8s.log.stream
purpose=k8s.health.ping
purpose=k8s.yaml.get
```

`docs/permissions.md` (regenerated by `make dump-perms`) does not enumerate purposes; they're documented in this spec only.

---

## 11. Out of scope (defer to v2 / other sub-projects)

These are deliberately **not** in P2 v1. Each will land in a follow-up if/when needed; do not implement during P2 plan execution.

### 11.1 Real-time updates
Informer / watch caches. Manual refresh only.

### 11.2 Write operations
Scale, restart, rollout, delete pod, cordon/drain, edit YAML, apply YAML. Deferred to a future P2.v2 spec; requires dry-run + diff UI plus an audit-heavy write path.

### 11.3 Interactive shells
`kubectl exec` terminal (WebSocket SPDY proxy), ephemeral debug containers, `kubectl debug`. Deferred — would warrant its own sub-project given the WS + SPDY complexity.

### 11.4 Plumbing operations
`port-forward`, `kubectl cp`. Tied to the exec story.

### 11.5 Secret protection boundary
The decision in §5.6 means `k8s:secret:read` exposes base64 data via `/yaml?kind=secret`. To restrict base64 too, an operator must withhold `k8s:secret:read` entirely. A separate `k8s:secret:list-only` permission (returns name + meta but no `/secrets/{ns}/{name}` Get) is a viable v2 escalation but not implemented now.

### 11.6 Resources excluded from v1 catalogue
PV, PVC, StorageClass, NetworkPolicy, HPA, PDB, Role, RoleBinding, ClusterRole, ClusterRoleBinding, ServiceAccount, CRD-defined kinds. Add a kind = a new Summary/Detail DTO + one route registration + perm-map entry + DTO mapper test.

### 11.7 Observability
Workload CPU / memory / network metrics. Belongs in P5 observability.

### 11.8 Cluster lifecycle
Cluster provisioning (kops / kubeadm / cloud-managed), auto-discovery, federation. Out of platform scope.

### 11.9 Background workers
Periodic health polling, scheduled cluster snapshots, notification webhooks. v1 is fully request-driven.

### 11.10 SSE polish
Auto-reconnect with `sinceTime`, log replay window, `--since=<duration>`, multi-pod log aggregation, browser tab title unread counter. v1 supports a single follow stream per opened pod page.

---

## 12. Testing

### 12.1 BE unit + dockertest

| Package | Test form | Focus |
|---|---|---|
| `k8s/cluster` | dockertest (P1 vertical pattern) | CRUD, name uniqueness (partial unique index), `kubeconfig_id` FK RESTRICT, tags JSONB round-trip, `CountByKubeconfigID` for P1 delete |
| `k8s/client` | unit (mock `credentials.Consumer`) | YAML parse, context override, context-not-found → `CodeValidation` + `k8s.kubeconfig.context_not_found`, kubeconfig with `exec` → `CodeValidation` + `k8s.kubeconfig.exec_forbidden`, with `auth-provider` → `CodeValidation` + `k8s.kubeconfig.authprovider_forbidden` |
| `k8s/workload`, `network`, `config`, `clusterscoped` | unit (`k8s.io/client-go/kubernetes/fake.NewSimpleClientset`) | DTO mapping per kind, kind dispatch, `mapAPIError` cases |
| `k8s/log` | unit + `goleak` | SSE pump produces `data: x\n\n`, keepalive comment timing (use fake clock), ctx cancel exits cleanly with no leaked goroutines |
| `k8s/yaml` | unit | `kind → perm` map completeness against §4 catalogue, unknown kind → `CodeBadRequest` + `k8s.yaml.unsupported_kind` |
| `k8s` (module level) | unit | `MountRoutes` produces the exact `(method, path, permission)` triples expected by §5 (snapshot test) |

Coverage gate per package: **≥60%** matching P0/P1 §12.

### 12.2 FE unit

- `useLogStream.test.ts` — `vi.spyOn(globalThis, 'fetch')` returns a `Response` with a hand-rolled `ReadableStream` that emits multiple chunks (including chunked SSE events split across reads). Assert line buffering, AbortController cancellation, `onScopeDispose` cleanup.
- `useK8sStore.test.ts` — cluster switch clears namespace cache; TTL behavior with `vi.useFakeTimers()`.
- `api/__tests__/refresh.test.ts` — migrated from axios-only test to assert both axios and fetch consumers share the single-flight refresh (concurrent 401s issue exactly one refresh call).

P0/P1 convention of no per-view vitest is preserved.

### 12.3 Manual smoke (acceptance, not CI)

Documented in §13. Spinning up `kind` or `k3d` in CI is rejected — it would push CI duration past 15 minutes and add flakiness. The fake clientset gives sufficient assurance for resource handler logic.

### 12.4 Static checks

`make lint` (golangci-lint) and `make swag` (regenerate swagger) and `make perm-check` (regenerate `docs/permissions.md`) must all be clean before merge.

---

## 13. Acceptance

A run-through that exercises every shipped surface:

1. **Setup**: in a clean environment, `colima start --kubernetes` (or use an existing cluster). Export its kubeconfig.
2. **P0/P1**: log in as admin, upload the kubeconfig via P1 UI → expect successful create (the new `exec`/`auth-provider` validation does not reject vanilla kubeconfigs).
3. **Cluster create**: navigate to `/k8s/clusters`, click "新建", enter name, pick the uploaded kubeconfig, pick context, save. Expect HTTP 200; row appears with grey health dot.
4. **Cluster ping**: click the row's ping action. Expect green dot + apiserver version displayed.
5. **Pre-populate**: in another terminal, `kubectl create deployment hello --image=nginx --replicas=2; kubectl create service clusterip hello --tcp=80:80; kubectl create configmap demo --from-literal=key=value; kubectl create secret generic demo --from-literal=token=s3cret`.
6. **Workload list**: navigate to `/k8s/workloads`, switch tab to Deployments, confirm `hello` shows with `2/2 ready`. Switch to Pods tab, confirm two `hello-*` pods. Click one pod row.
7. **Detail drawer**: assert Overview / YAML / Events / Logs tabs render.
8. **Pod log SSE**: in the Logs tab, click Connect. Stream `kubectl logs hello-...` content. In a third terminal, `kubectl exec hello-... -- sh -c 'echo new-line'` — verify the new line shows up in the FE within ~1s.
9. **Log reconnect**: click Reconnect. Verify a fresh `fetch` is issued (devtools Network tab) and the new connection takes over without duplicating previous lines.
10. **Network / config**: confirm `hello` Service shows on `/k8s/network`, `demo` ConfigMap on `/k8s/config`.
11. **Secret reveal RBAC**: as admin, click Reveal on `demo` Secret — see `token: s3cret`. Create a `viewer` role with all `k8s:*:read` codes but **without** `k8s:secret:reveal`, assign to a new user, log in as them, retry Reveal — expect 403 with a clear message.
12. **YAML secret bypass note**: as the viewer user, open the secret's YAML tab — see base64-encoded data field. This is the §5.6 documented behaviour, not a bug.
13. **Cluster-scoped**: confirm Namespace / Node / Event tabs render under `/k8s/cluster-resources`.
14. **Cluster delete protection**: try to delete the uploaded P1 kubeconfig from `/credentials/kubeconfigs`. Expect `CodeConflict` (40901) + message key `credentials.kubeconfig.in_use` + clear UI message listing the dependent cluster.
15. **Cluster delete**: delete the cluster row → P1 delete now succeeds.
16. **Audit**: `/system/audit-logs` shows `k8s.cluster.create`, `k8s.cluster.ping`, `k8s.cluster.delete` (target_type `k8s.cluster`), and `credentials.consume` rows with k8s purposes from the run.
17. **Kubeconfig RCE attempt**: upload a kubeconfig containing `users[0].user.exec.command: /bin/sh`. Expect `CodeValidation` (40002) with `credentials.kubeconfig.exec_forbidden` at P1 upload (the patch) AND `CodeValidation` (40002) with `k8s.kubeconfig.exec_forbidden` at P2 cluster create (defense in depth) — both paths reject.
18. **CI**: `make test`, `make test-int`, `make lint`, `make swagger-diff`, `make perm-check`, `bun run lint`, `bun run typecheck`, `bun run test`, `bun run i18n:check` all green.

---

## 14. Risks and mitigations

| Risk | Mitigation |
|---|---|
| `bufio.Scanner` 1MB buffer overflow on huge JSON-encoded log lines | Document the limit in §6.3; lines exceeding are split mid-token. Acceptable for v1; consider line-aware reader if reports come in. |
| SSE blocked by intermediate proxy (nginx, corporate egress) buffering | `X-Accel-Buffering: no` header + 30s keepalive comment + deploy doc note to set `proxy_buffering off` for `/api/v1/k8s/*/log` in `deploy/fe/nginx.conf` |
| Kubeconfig with `insecure-skip-tls-verify: true` silently accepts MITM | FE displays a ⚠️ icon on the cluster list row; spec'd in §13 step 4 as a UX bullet, not a hard reject |
| `client-go` v0.30.14 pin from P1 must continue to satisfy P2 imports | Verified: all P2 imports (`kubernetes`, `clientcmd`, `apierrors`, `kubernetes/fake`) ship in v0.30.x. If a future bump requires v0.31+, also bump Dockerfile + CI `go-version` (P1 left this note in `project-p1-progress`). |
| `pinia-plugin-persistedstate` is a new FE dependency | Lightweight (1KB), well-maintained; alternative is hand-rolling `localStorage` sync in the store action |
| Single-flight refresh refactor accidentally breaks P0 axios path | Refactor is mechanical (move state from closure to store action with the same shape); P0 tests in `api/__tests__/` are migrated rather than rewritten |
| Operator confused by Secret base64 visibility | §5.6 + §11.5 + acceptance step 12 + dedicated i18n string in the YAML viewer header for Secret kind ("⚠️ data is base64-encoded; use Reveal to decode") |
| `EventSource`-less SSE means we lose browser-native reconnect-on-network-blip | Acceptable for v1; user clicks Reconnect. If pain reported, add a `useLogStream` retry loop with jittered backoff in a follow-up. |

---

## 15. Migration / rollout

- **Migration**: one goose file `00015_create_clusters.sql` with the §3.1 DDL. The P1 kubeconfig validation patch is a code-only change (no schema). No data migration.
- **Rollout sequence**:
  1. Patch P1 kubeconfig validation (additive — rejects exec/auth-provider with `CodeValidation` + message keys per §9.2). Existing rows already in the DB are not re-validated; if any contain `exec`, they fail at P2 cluster create via the defense-in-depth check in `buildRestConfig`. Spec accepts this — re-running the upload to refresh is operator action.
  2. Apply migration `00015_create_clusters.sql`.
  3. Boot server — `permissions.Register` upserts the 9 new codes.
  4. `make seed` — admin role automatically gains the 9 new codes.
  5. Restart FE — new dynamic routes load from `/me/menus` after the seed.

No feature flag. The whole module is gated behind the new perms; users without any `k8s:*` code see no menu entry and 403 on any direct URL hit.

---

## 16. Open items

- [ ] **`pinia-plugin-persistedstate` adoption** — confirm with the user before adding the dep. Alternative: 5-line manual `watch(currentClusterId, v => localStorage.setItem('k8s.cluster.id', String(v)))`.
- [ ] **`vue-codemirror` vs `@guolao/vue-monaco-editor` (a slim Monaco wrapper)** — defaulting to `vue-codemirror` per §8.6 decision, but spec leaves room to switch if reviewer prefers Monaco's autocomplete + diff capabilities (we'd never need them in read-only v1).

Both items are FE-only and reversible after merge.
