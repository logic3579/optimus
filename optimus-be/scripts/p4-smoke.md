# P4 assets — manual smoke checklist

> Run on a Docker-enabled workstation with a disposable, read-only AWS
> credential. This is a manual release sign-off; it is not an automated test
> because it talks to a real AWS account.

Run after a clean `make migrate-up` and `make seed` against Postgres.

## IAM policy for the smoke credential

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeVpcs",
        "ec2:DescribeSubnets",
        "rds:DescribeDBInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

## Steps

1. Sign in as an admin and create an AWS cloud key at
   `/credentials/cloud-keys`. Use a smoke-only access key and select a region
   with known resources.
2. Create a Cloud Account at `/assets/cloud-accounts`. Select that cloud key,
   enable the matching region, and leave the account enabled.
3. Click **Sync now** (or wait for the configured cron). Confirm the request
   returns immediately and the row becomes available for another sync after
   the worker completes.
4. Open `/assets/sync-runs`. Expect `instance`, `network`, and `database`
   runs with status `success`; verify `items_seen` matches the account's
   discovered resources. A failed run exposes its error code and reveals the
   detailed error only on the error control's tooltip.
5. Open `/assets/instances`, `/assets/vpcs`, and `/assets/databases`.
   Check account/region/search filters and pagination. Open a VPC and verify
   its subnet list; verify keyboard Enter and Space also open a selected VPC.
6. Remove one enabled region from the Cloud Account. With **Include deleted**
   enabled, verify resources in that region become soft-deleted; with it
   disabled, verify they are hidden.
7. Simulate drift in the smoke account (for example, remove an EC2 instance),
   run another sync, and confirm the missing resource is soft-deleted only
   after a successful full sweep.
8. While the Cloud Account exists, try deleting its cloud key. Expect error
   `43001` (`assets.cloud_account.in_use`). Delete the Cloud Account and
   verify the response reports its cascaded soft-delete count; retrying the
   cloud-key delete should now succeed.

## Reporting a failure

Capture the response envelope's `code` and `message_key`, the related
`assets_sync_runs` and `audit_logs` rows, and backend logs at debug level.
