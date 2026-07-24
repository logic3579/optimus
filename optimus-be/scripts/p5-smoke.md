# P5 observability MVP — manual smoke checklist

> This checklist uses disposable local Prometheus containers. It requires no
> production credential, cloud account, Kubernetes cluster, or production
> Prometheus. Never point it at production.

Run it on a Docker-enabled workstation after a clean migration and seed. Keep
the printed bootstrap password only for this disposable environment.

## 1. Start disposable unauthenticated and authenticated targets

Create a temporary directory outside the repository:

```bash
SMOKE_DIR=$(mktemp -d /tmp/optimus-p5-smoke.XXXXXX)
cd "$SMOKE_DIR"
openssl passwd -apr1 'smoke-pass' > htpasswd.hash
printf 'smoke:%s\n' "$(cat htpasswd.hash)" > htpasswd
```

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 5s
scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["127.0.0.1:9090"]
```

Create `nginx.conf`:

```nginx
events {}
http {
  server {
    listen 9091;
    auth_basic "smoke";
    auth_basic_user_file /etc/nginx/htpasswd;
    location / {
      proxy_pass http://172.31.77.10:9090;
    }
  }
}
```

Create `compose.yaml`:

```yaml
services:
  prometheus:
    image: prom/prometheus:v2.53.3
    command: ["--config.file=/etc/prometheus/prometheus.yml"]
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
    networks:
      smoke:
        ipv4_address: 172.31.77.10
  authenticated:
    image: nginx:1.27-alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./htpasswd:/etc/nginx/htpasswd:ro
    networks:
      smoke:
        ipv4_address: 172.31.77.11
networks:
  smoke:
    ipam:
      config:
        - subnet: 172.31.77.0/24
```

Start it and confirm both paths:

```bash
docker compose up -d
curl --fail http://172.31.77.10:9090/-/ready
curl --fail -u smoke:smoke-pass http://172.31.77.11:9091/-/ready
```

Configure Optimus with the two exact target addresses, not the broad Docker
subnet:

```bash
export OPTIMUS_OBSERVABILITY_ALLOWED_PRIVATE_CIDRS='172.31.77.10/32,172.31.77.11/32'
```

Restart the backend after changing the allowlist. A broad allowlist such as
`0.0.0.0/0` is not an acceptable smoke configuration.

## 2. Authenticate to Optimus

```bash
export API=http://127.0.0.1:8080/api/v1
export ADMIN_PASSWORD='<password printed by the disposable seed>'
export TOKEN=$(
  curl --fail --silent --show-error \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" \
    "$API/auth/login" | jq -er '.data.access_token'
)
auth=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
```

## 3. Create both data-source paths

Create the unauthenticated source:

```bash
export OPEN_DS=$(
  curl --fail --silent --show-error "${auth[@]}" \
    -d '{"name":"smoke-open","base_url":"http://172.31.77.10:9090","auth_type":"none","tls_skip_verify":false,"description":"disposable P5 smoke"}' \
    "$API/observability/datasources" | jq -er '.data.id'
)
```

Create a Basic HTTP credential and authenticated source:

```bash
export HTTP_CREDENTIAL=$(
  curl --fail --silent --show-error "${auth[@]}" \
    -d '{"name":"smoke-basic","auth_type":"basic","username":"smoke","secret":"smoke-pass"}' \
    "$API/credentials/http-credentials" | jq -er '.data.id'
)
export AUTH_DS=$(
  curl --fail --silent --show-error "${auth[@]}" \
    -d "{\"name\":\"smoke-auth\",\"base_url\":\"http://172.31.77.11:9091\",\"auth_type\":\"basic\",\"http_credential_id\":$HTTP_CREDENTIAL,\"tls_skip_verify\":false,\"description\":\"disposable authenticated P5 smoke\"}" \
    "$API/observability/datasources" | jq -er '.data.id'
)
```

For both IDs, call `POST /observability/datasources/{id}/test`. Expect
`reachable: true` and a Prometheus version:

```bash
curl --fail --silent "${auth[@]}" -X POST "$API/observability/datasources/$OPEN_DS/test" | jq
curl --fail --silent "${auth[@]}" -X POST "$API/observability/datasources/$AUTH_DS/test" | jq
```

## 4. Exercise metadata and metric queries

```bash
curl --fail --silent "${auth[@]}" "$API/observability/datasources/$OPEN_DS/labels" | jq
curl --fail --silent "${auth[@]}" \
  "$API/observability/datasources/$OPEN_DS/label-values?label=job" | jq

curl --fail --silent "${auth[@]}" \
  -d "{\"datasource_id\":$OPEN_DS,\"enrich_assets\":false,\"queries\":[{\"ref_id\":\"up\",\"promql\":\"up\"},{\"ref_id\":\"build\",\"promql\":\"prometheus_build_info\"}]}" \
  "$API/observability/query" | jq

END=$(date -u +%Y-%m-%dT%H:%M:%SZ)
START=$(date -u -d '15 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
curl --fail --silent "${auth[@]}" \
  -d "{\"datasource_id\":$AUTH_DS,\"start\":\"$START\",\"end\":\"$END\",\"step\":\"30s\",\"enrich_assets\":false,\"queries\":[{\"ref_id\":\"up\",\"promql\":\"up\"}]}" \
  "$API/observability/query-range" | jq
```

Expect ordered per-`ref_id` results. Repeat one request with invalid PromQL and
confirm its item has a normalized error without exposing the raw upstream
response or credential.

## 5. Built-in and custom dashboards

List built-ins, fetch each definition, and open
`/observability/kubernetes` in the UI:

```bash
curl --fail --silent "${auth[@]}" "$API/observability/builtin-dashboards" | jq
curl --fail --silent "${auth[@]}" \
  "$API/observability/builtin-dashboards/kubernetes-cluster" | jq
```

Select `smoke-open`, run the built-in view, switch definitions while a query is
running, and verify stale panels disappear.

Create, read, replace, and delete a custom dashboard:

```bash
export DASHBOARD=$(
  curl --fail --silent "${auth[@]}" \
    -d "{\"name\":\"smoke-dashboard\",\"description\":\"disposable\",\"refresh_interval_s\":30,\"time_range\":\"15m\",\"panels\":[{\"datasource_id\":$OPEN_DS,\"title\":\"Up\",\"panel_type\":\"stat\",\"promql\":\"up\",\"unit\":\"none\",\"legend\":\"\",\"sort_order\":0,\"width\":6}]}" \
    "$API/observability/dashboards" | jq -er '.data.id'
)
curl --fail --silent "${auth[@]}" "$API/observability/dashboards/$DASHBOARD" | jq
curl --fail --silent "${auth[@]}" -X PUT \
  -d "{\"name\":\"smoke-dashboard-updated\",\"description\":\"disposable\",\"refresh_interval_s\":60,\"time_range\":\"1h\",\"panels\":[{\"datasource_id\":$AUTH_DS,\"title\":\"Build\",\"panel_type\":\"table\",\"promql\":\"prometheus_build_info\",\"unit\":\"none\",\"legend\":\"\",\"sort_order\":0,\"width\":12}]}" \
  "$API/observability/dashboards/$DASHBOARD" | jq
curl --fail --silent "${auth[@]}" -X DELETE "$API/observability/dashboards/$DASHBOARD" | jq
```

## 6. Verify RBAC personas

Create disposable roles/users through the normal admin UI or APIs:

- Metric operator: only `observability:metric:read`. It can open the built-in
  view, list the minimal query-source picker, and run queries, but receives
  403 for data-source and dashboard administration.
- Dashboard viewer: `observability:dashboard:read` plus
  `observability:metric:read`. It can view dashboards but cannot create,
  update, or delete them.
- Data-source operator: data-source read/write without delete. It can create,
  edit, and test sources but cannot delete them.
- Administrator: all P5 permissions. It can complete every step above.

Verify both direct API 403 responses and absence of the corresponding UI
controls. A metric-only operator must not generate a rejected call to
`GET /observability/datasources`.

## 7. Inspect audits and secret boundaries

Inspect recent `audit_logs` rows in the disposable database. Confirm:

- data-source create/update/test/delete and dashboard CRUD actions exist;
- HTTP credential consumption records the purpose but never the secret;
- no payload contains `smoke-pass`, `secret_ciphertext`, `custom_ca_pem`, CA
  PEM material, a base URL containing userinfo, or full PromQL;
- dashboard audit payloads contain bounded PromQL fingerprints only;
- API responses and backend logs do not contain credentials or authorization
  headers.

Example read-only checks:

```sql
SELECT action, target_type, target_id, payload
FROM audit_logs
WHERE action LIKE 'observability.%'
   OR action LIKE 'credentials.%http_credential%'
ORDER BY id DESC;

SELECT count(*) FROM audit_logs
WHERE payload::text LIKE '%smoke-pass%'
   OR payload::text LIKE '%custom_ca_pem%'
   OR payload::text LIKE '%prometheus_build_info%';
```

The second query must return zero.

## 8. Confirm denied destinations and redirects

Attempt to create/test sources for all of the following and expect normalized
destination-denied or invalid-URL errors:

- `http://169.254.169.254/latest/meta-data/` even if a deliberately broad
  `0.0.0.0/0` allowlist is tried;
- `http://[::ffff:169.254.169.254]/`;
- a hostname resolving to one allowed address and one loopback/private address;
- an allowed local endpoint that redirects to metadata, loopback, another
  origin, or outside the configured base-path prefix.

The redirect target must receive no request, and no Authorization header may
be forwarded. These cases are also enforced by automated URL-policy and
transport tests; do not weaken the allowlist to make a smoke step pass.

## 9. Teardown

Delete the smoke dashboards and both data sources before deleting the HTTP
credential. Remove the disposable RBAC users/roles. Then:

```bash
cd "$SMOKE_DIR"
docker compose down --volumes --remove-orphans
cd /
rm -rf -- "$SMOKE_DIR"
unset TOKEN ADMIN_PASSWORD HTTP_CREDENTIAL OPEN_DS AUTH_DS DASHBOARD
unset OPTIMUS_OBSERVABILITY_ALLOWED_PRIVATE_CIDRS
```

Confirm no smoke containers, credentials, data sources, dashboards, roles, or
users remain.
