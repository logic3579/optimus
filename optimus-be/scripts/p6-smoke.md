# P6 application delivery — disposable local smoke checklist

> Run only against disposable PostgreSQL, Kind, and chart-repository resources.
> Never paste or commit JWTs, kubeconfigs, chart values, manifests, Helm notes,
> authorization headers, or raw Helm errors. Examples below retain only IDs,
> states, revisions, digests, stable error codes, and correlation IDs.

## 1. Prerequisites and isolated workspace

Required commands: `docker`, `kind`, `kubectl`, `helm`, `curl`, `jq`, `openssl`,
`go`, and `make`. Start from `optimus-be/` with the normal disposable backend
configuration and migrations available.

```bash
export P6_SMOKE_DIR="$(mktemp -d /tmp/optimus-p6-smoke.XXXXXX)"
export P6_CLUSTER=optimus-p6-smoke
export P6_API=http://127.0.0.1:8080/api/v1
export P6_NAMESPACE=optimus-p6-smoke
kind create cluster --name "$P6_CLUSTER"
kubectl create namespace "$P6_NAMESPACE"
kubectl config view --raw > "$P6_SMOKE_DIR/kubeconfig"
chmod 600 "$P6_SMOKE_DIR/kubeconfig"
```

Start disposable PostgreSQL and Optimus using the repository compose file and
the built server binary. The compose file contains PostgreSQL only; Optimus is
a host process. Keep the disposable secrets and server log private.

```bash
docker compose up -d postgres
export OPTIMUS_JWT_SECRET="$(openssl rand -hex 32)"
export OPTIMUS_VAULT_MASTER_KEY="$(openssl rand -base64 32)"
make migrate-up
make seed
make build
umask 077
./bin/optimus-be >"$P6_SMOKE_DIR/server.log" 2>&1 &
export P6_SERVER_PID=$!
until curl --fail --silent http://127.0.0.1:8080/api/v1/health >/dev/null; do sleep 1; done
```

Record the disposable bootstrap password only in a shell variable, log in, and
keep authorization headers out of command tracing:

```bash
set +x
read -rsp 'Disposable admin password: ' P6_ADMIN_PASSWORD; echo
export P6_ADMIN_TOKEN="$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$P6_ADMIN_PASSWORD\"}" "$P6_API/auth/login" | jq -er '.data.access_token')"
p6_admin=(-H "Authorization: Bearer $P6_ADMIN_TOKEN" -H 'Content-Type: application/json')
```

Create separate initiator and approver users/roles through the normal RBAC API.
Grant the initiator project/pipeline/run permissions and the approver only
`delivery:approval:read` plus `delivery:approval:decide`. Export their IDs and
tokens as `P6_INITIATOR_ID`, `P6_APPROVER_ID`, `P6_INITIATOR_TOKEN`, and
`P6_APPROVER_TOKEN`; do not echo tokens. Confirm the ten delivery permissions:

```bash
curl --fail --silent "${p6_admin[@]}" "$P6_API/permissions?page_size=200" | jq -e '[.data.items[]|select(.code|startswith("delivery:"))]|length==10'
p6_initiator=(-H "Authorization: Bearer $P6_INITIATOR_TOKEN" -H 'Content-Type: application/json')
p6_approver=(-H "Authorization: Bearer $P6_APPROVER_TOKEN" -H 'Content-Type: application/json')
```

## 2. Two immutable chart versions and three installed P3 releases

Create a minimal `p6-smoke` chart with one Deployment, package versions `1.0.0`
and `2.0.0`, and serve the repository locally. The chart must contain no secret.

```bash
mkdir -p "$P6_SMOKE_DIR/charts/p6-smoke/templates" "$P6_SMOKE_DIR/repo"
cat >"$P6_SMOKE_DIR/charts/p6-smoke/Chart.yaml" <<'YAML'
apiVersion: v2
name: p6-smoke
type: application
version: 1.0.0
appVersion: "1"
YAML
cat >"$P6_SMOKE_DIR/charts/p6-smoke/templates/deployment.yaml" <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
spec:
  replicas: 1
  selector:
    matchLabels: { app.kubernetes.io/instance: "{{ .Release.Name }}" }
  template:
    metadata:
      labels: { app.kubernetes.io/instance: "{{ .Release.Name }}" }
    spec:
      containers:
        - name: smoke
          image: nginx:1.27-alpine
YAML
sed -i 's/^version:.*/version: 1.0.0/' "$P6_SMOKE_DIR/charts/p6-smoke/Chart.yaml"
helm package "$P6_SMOKE_DIR/charts/p6-smoke" --destination "$P6_SMOKE_DIR/repo"
sed -i 's/^version:.*/version: 2.0.0/' "$P6_SMOKE_DIR/charts/p6-smoke/Chart.yaml"
helm package "$P6_SMOKE_DIR/charts/p6-smoke" --destination "$P6_SMOKE_DIR/repo"
helm repo index "$P6_SMOKE_DIR/repo" --url http://127.0.0.1:18080
docker run -d --rm --name optimus-p6-chart-repo -p 18080:80 -v "$P6_SMOKE_DIR/repo:/usr/share/nginx/html:ro" nginx:1.27-alpine
curl --fail http://127.0.0.1:18080/index.yaml >/dev/null
```

Create one P1 kubeconfig credential, P2 cluster, P3 HTTP chart repository, and
three applications. The kubeconfig enters JSON directly and is never printed:

```bash
export P6_KUBECONFIG_ID="$(jq -n --rawfile kubeconfig "$P6_SMOKE_DIR/kubeconfig" '{name:"p6-smoke",description:"disposable P6 smoke",default_namespace:"optimus-p6-smoke",kubeconfig:$kubeconfig}' | curl --fail --silent "${p6_admin[@]}" --data-binary @- "$P6_API/credentials/kubeconfigs" | jq -er '.data.id')"
export P6_CLUSTER_ID="$(curl --fail --silent "${p6_admin[@]}" -d "{\"name\":\"p6-smoke\",\"description\":\"disposable P6 smoke\",\"kubeconfig_id\":$P6_KUBECONFIG_ID,\"context\":\"kind-$P6_CLUSTER\",\"tags\":[\"p6-smoke\"]}" "$P6_API/k8s/clusters" | jq -er '.data.id')"
export P6_REPO_ID="$(curl --fail --silent "${p6_admin[@]}" -d '{"name":"p6-smoke","description":"disposable P6 smoke","type":"http","url":"http://127.0.0.1:18080"}' "$P6_API/apps/repos" | jq -er '.data.id')"
create_app(){ env="$1"; curl --fail --silent "${p6_admin[@]}" -d "{\"name\":\"p6-smoke-$env\",\"description\":\"disposable P6 smoke\",\"cluster_id\":$P6_CLUSTER_ID,\"namespace\":\"$P6_NAMESPACE\",\"release_name\":\"p6-smoke-$env\",\"chart_repo_id\":$P6_REPO_ID,\"chart_name\":\"p6-smoke\",\"tags\":[\"p6-smoke\"]}" "$P6_API/apps/applications" | jq -er '.data.id'; }
export P6_DEV_APP="$(create_app dev)" P6_STAGING_APP="$(create_app staging)" P6_PROD_APP="$(create_app prod)"
for id in "$P6_DEV_APP" "$P6_STAGING_APP" "$P6_PROD_APP"; do curl --fail --silent "${p6_admin[@]}" -d '{"chart_version":"1.0.0","values_yaml":""}' "$P6_API/apps/applications/$id/release/install" | jq -e '.code==0'; done
for id in "$P6_DEV_APP" "$P6_STAGING_APP" "$P6_PROD_APP"; do curl --fail --silent "${p6_admin[@]}" "$P6_API/apps/applications/$id/release/status" | jq -e '.data.status=="deployed" and .data.chart_version=="1.0.0"'; done
```

## 3. Project, bindings, pipeline, and artifact resolution

```bash
export P6_PROJECT_ID="$(curl --fail --silent "${p6_initiator[@]}" -d '{"name":"p6-smoke","description":"disposable local smoke"}' "$P6_API/delivery/projects" | jq -er '.data.id')"
for row in "dev:Development:$P6_DEV_APP" "staging:Staging:$P6_STAGING_APP" "prod:Production:$P6_PROD_APP"; do IFS=: read -r key name app_id <<<"$row"; curl --fail --silent "${p6_initiator[@]}" -d "{\"environment_key\":\"$key\",\"display_name\":\"$name\",\"application_id\":$app_id}" "$P6_API/delivery/projects/$P6_PROJECT_ID/environments" | jq -e '.code==0'; done
export P6_ENV_IDS="$(curl --fail --silent "${p6_initiator[@]}" "$P6_API/delivery/projects/$P6_PROJECT_ID/environments" | jq -c '[.data[].id]')"
export P6_PIPELINE_BODY="$(jq -cn --argjson ids "$P6_ENV_IDS" '{stages:[$ids|to_entries[]|{environment_id:.value,approval_required:(.key==1),timeout:"10m"}]}')"
curl --fail --silent "${p6_initiator[@]}" -X PUT -d "$P6_PIPELINE_BODY" "$P6_API/delivery/projects/$P6_PROJECT_ID/pipeline" | jq -e '.data.version==1'
export P6_ARTIFACT="$(curl --fail --silent "${p6_initiator[@]}" "$P6_API/delivery/projects/$P6_PROJECT_ID/artifacts" | jq -cer '.data[]|select(.version=="2.0.0")|{chart_repo_id,chart_name,chart_version:.version}')"
export P6_RESOLVED="$(curl --fail --silent "${p6_initiator[@]}" -d "$P6_ARTIFACT" "$P6_API/delivery/projects/$P6_PROJECT_ID/artifacts/resolve" | jq -cer '.data|select(.digest|test("^sha256:[0-9a-f]{64}$"))')"
```

## 4. Run, SSE, approval, restart, and terminal evidence

```bash
export P6_KEY="$(openssl rand -hex 16)"
export P6_RUN_ID="$(curl --fail --silent "${p6_initiator[@]}" -H "Idempotency-Key: $P6_KEY" -d "$P6_ARTIFACT" "$P6_API/delivery/projects/$P6_PROJECT_ID/runs" | jq -er '.data.id')"
timeout 20s curl --no-buffer --silent -H "Authorization: Bearer $P6_INITIATOR_TOKEN" "$P6_API/delivery/runs/$P6_RUN_ID/events" | jq --unbuffered -R 'select(startswith("data:"))|ltrimstr("data:")|fromjson|{id,event_type,old_state,new_state,correlation_id}'
export P6_STAGE_ID="$(curl --fail --silent "${p6_approver[@]}" "$P6_API/delivery/approvals/pending" | jq -er ".data[]|select(.run_id==$P6_RUN_ID)|.run_stage_id")"
curl --fail --silent "${p6_approver[@]}" -d '{"comment":"approved by disposable P6 smoke"}' "$P6_API/delivery/run-stages/$P6_STAGE_ID/approve" | jq -e '.data.decision=="approved"'
kill -TERM "$P6_SERVER_PID"
wait "$P6_SERVER_PID"
./bin/optimus-be >"$P6_SMOKE_DIR/server-restarted.log" 2>&1 &
export P6_SERVER_PID=$!
until curl --fail --silent http://127.0.0.1:8080/api/v1/health >/dev/null; do sleep 1; done
until curl --fail --silent "${p6_initiator[@]}" "$P6_API/delivery/runs/$P6_RUN_ID" | jq -e '.data.state=="succeeded" and ([.data.stages[]|.result_revision>0 and (.result_digest|test("^sha256:[0-9a-f]{64}$"))]|all)'; do sleep 2; done
```

Expected safe example: run `succeeded`; revisions are positive integers; run
and stage digests are `sha256:` plus 64 lowercase hex characters.

## 5. Required negative and recovery checks

Repeat each request while capturing only HTTP status and envelope code/key.

```bash
safe_status(){ file="$P6_SMOKE_DIR/response.json"; status="$(curl --silent --output "$file" --write-out '%{http_code}' "$@")"; jq -c --arg status "$status" '{http_status:$status,code,message_key,correlation_id}' "$file"; }
safe_status "${p6_initiator[@]}" -X POST -d "$P6_ARTIFACT" "$P6_API/apps/applications/$P6_DEV_APP/release/upgrade" # managed mutation denied
safe_status "${p6_initiator[@]}" -X POST "$P6_API/apps/applications/$P6_DEV_APP/release/uninstall" # managed mutation denied
safe_status "${p6_initiator[@]}" -d '{"comment":"self decision must fail"}' "$P6_API/delivery/run-stages/$P6_STAGE_ID/approve" # self-approval denied
export P6_DUPLICATE_ID="$(curl --fail --silent "${p6_initiator[@]}" -H "Idempotency-Key: $P6_KEY" -d "$P6_ARTIFACT" "$P6_API/delivery/projects/$P6_PROJECT_ID/runs" | jq -er '.data.id')"; test "$P6_DUPLICATE_ID" = "$P6_RUN_ID"
safe_status "${p6_initiator[@]}" -H "Idempotency-Key: $P6_KEY" -d "$(jq '.chart_version="1.0.0"' <<<"$P6_ARTIFACT")" "$P6_API/delivery/projects/$P6_PROJECT_ID/runs" # idempotency conflict
```

Create a second run while the first is deliberately paused at approval and
expect `delivery.run.active`. Force one disposable chart upgrade to fail, verify
only its stable error key/correlation ID, use the existing P3 application detail
rollback flow, then call reconciliation for `outcome_unknown` or retry for a
terminal failed/timed-out/canceled/rejected run:

```bash
safe_status "${p6_initiator[@]}" -X POST "$P6_API/delivery/runs/$P6_RUN_ID/reconcile"
export P6_RETRY_KEY="$(openssl rand -hex 16)"
curl --fail --silent "${p6_initiator[@]}" -H "Idempotency-Key: $P6_RETRY_KEY" -X POST "$P6_API/delivery/runs/$P6_RUN_ID/retry" | jq -e '.data.retry_of_run_id=='"$P6_RUN_ID"
```

Scan only captured safe API/SSE/audit projections. This must produce no output:

```bash
rg -ni 'values_yaml|kubeconfig|authorization|manifest|helm notes|raw_error|shell|script|container_image' "$P6_SMOKE_DIR"/*.safe.json "$P6_SMOKE_DIR"/*.safe.ndjson
```

## 6. Teardown

```bash
set +e
kill -TERM "$P6_SERVER_PID"
wait "$P6_SERVER_PID"
docker rm -f optimus-p6-chart-repo
kind delete cluster --name "$P6_CLUSTER"
docker compose down -v
find "$P6_SMOKE_DIR" -type f -exec chmod 600 {} +
rm -rf -- "$P6_SMOKE_DIR"
unset P6_ADMIN_PASSWORD P6_ADMIN_TOKEN P6_INITIATOR_TOKEN P6_APPROVER_TOKEN OPTIMUS_JWT_SECRET OPTIMUS_VAULT_MASTER_KEY
```
