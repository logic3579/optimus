# P3 smoke checklist

> **Run on a Docker-enabled workstation against a live dev cluster — not
> automated.** This checklist is the manual sign-off the team performs before
> tagging a P3 release. It is intentionally not wired into CI because each
> step talks to an external Helm chart repository (HTTP + OCI) whose
> availability and chart versions drift independently of our codebase.

Run after a fresh deploy or whenever you suspect upstream chart-repo
behaviour has shifted.

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
