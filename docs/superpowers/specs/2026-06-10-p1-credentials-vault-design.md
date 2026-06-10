# P1 — credentials-vault Design

**Status**: Spec
**Date**: 2026-06-10
**Owner**: P1 sub-project
**Depends on**: P0 platform-skeleton (merged 2026-06-09 on `main`)
**Downstream**: P2 k8s-mgmt, P4 assets, P5 observability, P6 cicd (all consume via the §6 Go seam)

---

## 1. Goal and scope

P0 shipped the platform-skeleton: auth/RBAC/users/roles/menus/audit/i18n/CRUD UI/deploy. P1 builds on top of P0 the **credentials vault** — encrypted storage for the three credential types every downstream sub-project will need to operate on real infrastructure:

1. **SSH keys** — private key + optional passphrase + username, used by P2 (k8s exec/debug pods) and P6 (cicd remote deploys)
2. **kubeconfigs** — full YAML, used by P2 (cluster control plane access)
3. **Cloud access keys** — AWS / GCP / Azure access-key-id + secret-access-key, used by P4 (asset discovery) and P5 (cloud metric ingestion)

What P1 ships:

1. Three CRUD HTTP surfaces (one per type) following P0 Plan 2b's CRUD shape
2. Application-layer AES-256-GCM encryption with master key bootstrapped from env or file
3. An internal Go-level `Consumer` seam (no HTTP) that downstream sub-projects import to fetch decrypted credentials
4. Per-consume audit recording with a caller-supplied `purpose` string
5. A `optimus-vault-keygen` CLI for first-deploy master-key minting
6. New permission codes registered into P0's permission registry (12 codes — 3 types × {read, write, delete, use}, matching P0 `system:user|role|menu` shape)
7. Three new FE pages (`/credentials/ssh-keys`, `/credentials/kubeconfigs`, `/credentials/cloud-keys`) re-using P0 2b components

**Out of scope** (deferred to P1.x or later):

- In-app rotation API and CLI rotation script (master key change requires manual decrypt-re-encrypt; design accommodates but doesn't ship the script)
- Per-credential expiry / TTL
- External KMS integration (AWS KMS, HashiCorp Vault, etc.) — master key is local-only
- Per-credential ACLs / role-sharing — access model is flat (anyone with permission sees all credentials of that type)
- File upload for credential material (paste-into-textarea only)
- Per-credential usage analytics dashboards
- Secret reveal / copy-to-clipboard from the UI — secrets only leave the BE via the Go seam

---

## 2. Architecture

```
                ┌─────────────────────────────────────────┐
                │              FE (Vue3 + AntdV)          │
                │  /credentials/ssh-keys                  │
                │  /credentials/kubeconfigs               │
                │  /credentials/cloud-keys                │
                │  (CRUD pages — never display secrets)   │
                └────────────────┬────────────────────────┘
                                 │ HTTPS, Envelope<T>
                                 ▼
   ┌──────────────────────────────────────────────────────────────┐
   │                     BE (Go + Gin + GORM)                     │
   │                                                              │
   │   /api/v1/credentials/ssh-keys                               │
   │   /api/v1/credentials/kubeconfigs        (HTTP CRUD)         │
   │   /api/v1/credentials/cloud-keys                             │
   │                       │                                      │
   │                       ▼                                      │
   │      internal/modules/credentials/                           │
   │        sshkey/      kubeconfig/      cloudkey/               │
   │             \           |           /                        │
   │              \          |          /                         │
   │               ▼         ▼         ▼                          │
   │             vault/   (AES-256-GCM, master key)               │
   │                                                              │
   │      consume.go ←── Consumer interface (exported)            │
   │            ▲                                                 │
   └────────────┼─────────────────────────────────────────────────┘
                │ Go imports (no HTTP)
                │
   ┌────────────┴──────────────────────────────────┐
   │  P2 k8s-mgmt / P4 assets / P5 obs / P6 cicd   │
   │  (each calls Consumer.GetXxx with purpose)    │
   └───────────────────────────────────────────────┘
```

BE module layout:

```
internal/modules/credentials/
  vault/                  # crypto core — master key never leaves this package
    keyloader.go          # env → file fallback resolution
    keyloader_test.go
    cipher.go             # Seal(plaintext) → ciphertext; Open(ciphertext) → plaintext
    cipher_test.go
  sshkey/                 # SSH key feature
    model.go              # GORM model
    repository.go
    service.go            # CRUD + consume
    handler.go
    routes.go
    *_test.go
  kubeconfig/             # symmetrical structure
  cloudkey/               # symmetrical structure
  consume.go              # exported Consumer interface — sole public seam
  module.go               # wiring (DI), permission registration

cmd/vault-keygen/
  main.go                 # prints a fresh base64 32-byte key to stdout
```

`internal/modules/credentials/module.go` is wired into `cmd/server/main.go` alongside the other P0 modules. The `vault` package is constructed once and shared by all three feature sub-packages via `Cipher` (an interface) — feature packages don't import `vault` directly.

FE module layout:

```
src/views/credentials/
  ssh-keys/
    index.vue             # list + create/edit modal
    components/
      SshKeyForm.vue
  kubeconfigs/
    index.vue
    components/
      KubeconfigForm.vue
  cloud-keys/
    index.vue
    components/
      CloudKeyForm.vue
src/api/credentials/
  ssh-key.ts              # hand-written DTOs + http calls (per P0 2a convention)
  kubeconfig.ts
  cloud-key.ts
src/locales/{zh-CN,en-US}.json    # +credentials.* namespace
```

---

## 3. Data model

### 3.1 New tables

All three tables share a base shape (id / name / description / created_by / timestamps) and differ only in type-specific columns + encrypted secret columns. All `*_enc` columns are `BYTEA`, hard-deleted on row delete, no `deleted_at`.

All PKs are `BIGSERIAL` and FK columns are `BIGINT`, matching P0's existing convention (verified in `optimus-be/migrations/00001_create_users.sql` etc. — P0 does NOT use UUIDs).

**credentials_ssh_keys**

```sql
CREATE TABLE credentials_ssh_keys (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(128) NOT NULL UNIQUE,
    description        TEXT         NOT NULL DEFAULT '',
    username           VARCHAR(64)  NOT NULL,
    private_key_enc    BYTEA        NOT NULL,
    passphrase_enc     BYTEA,
    created_by_user_id BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ssh_keys_created_by ON credentials_ssh_keys(created_by_user_id);
```

**credentials_kubeconfigs**

```sql
CREATE TABLE credentials_kubeconfigs (
    id                  BIGSERIAL    PRIMARY KEY,
    name                VARCHAR(128) NOT NULL UNIQUE,
    description         TEXT         NOT NULL DEFAULT '',
    default_namespace   VARCHAR(64)  NOT NULL DEFAULT '',
    kubeconfig_enc      BYTEA        NOT NULL,
    created_by_user_id  BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kubeconfigs_created_by ON credentials_kubeconfigs(created_by_user_id);
```

**credentials_cloud_keys**

```sql
CREATE TABLE credentials_cloud_keys (
    id                     BIGSERIAL    PRIMARY KEY,
    name                   VARCHAR(128) NOT NULL UNIQUE,
    description            TEXT         NOT NULL DEFAULT '',
    provider               VARCHAR(16)  NOT NULL CHECK (provider IN ('aws','gcp','azure')),
    region                 VARCHAR(32)  NOT NULL DEFAULT '',
    access_key_id_enc      BYTEA        NOT NULL,
    secret_access_key_enc  BYTEA        NOT NULL,
    created_by_user_id     BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_cloud_keys_provider ON credentials_cloud_keys(provider);
CREATE INDEX idx_cloud_keys_created_by ON credentials_cloud_keys(created_by_user_id);
```

(Text-column defaults set to `''` rather than nullable matches P0 user/role/menu table style — see `00001_create_users.sql` `display_name` / `avatar_url`.)

Note: `access_key_id_enc` is also encrypted, not just the secret — access-key IDs are themselves sensitive identifiers in AWS/GCP/Azure logs and access-control narratives.

`name` is globally unique **within its table** (no cross-type uniqueness — `ssh_keys.name = "prod"` and `cloud_keys.name = "prod"` may coexist). Migration matches P0's goose embedded migration pattern (`be/migrations/*.sql`).

### 3.2 audit_logs — reuse existing columns (no schema change)

P0's `audit_logs` (verified in `optimus-be/migrations/00008_create_audit_logs.sql`) already has the columns we need for denormalized snapshots — **no migration needed**:

```
audit_logs:
    id           BIGSERIAL PK
    user_id      BIGINT      (FK users.id ON DELETE SET NULL)
    action       VARCHAR(64) NOT NULL               -- event type
    target_type  VARCHAR(64) NOT NULL DEFAULT ''    -- 'credentials.ssh_key' etc.
    target_id    VARCHAR(64) NOT NULL DEFAULT ''    -- the credential id, stringified
    payload      JSONB       NOT NULL DEFAULT '{}'  -- denormalized snapshot + extras
    ip / user_agent / created_at
```

P1 uses these as-is:

- `action` = the event type (e.g., `credentials.create`, `credentials.consume`).
- `target_type` = `credentials.ssh_key` | `credentials.kubeconfig` | `credentials.cloud_key`.
- `target_id` = the credential's BIGSERIAL id rendered as a string.
- `payload` jsonb carries the denormalized snapshot:
  ```json
  {
    "name": "prod-cluster-key",
    "purpose": "k8s.exec.pod",
    "changed_fields": ["description"],
    "secret_rotated": false
  }
  ```
  `name` is always present (so deleted-credential audits still show the name). Other keys appear per event type (see §9).

After a credential is hard-deleted, audit rows carry `target_id` (the now-dangling id, fine — still a stable identifier in the log) and `payload.name` (the snapshot), so the audit UI renders meaningful "(deleted) prod-cluster-key" entries.

---

## 4. Crypto

### 4.1 Algorithm

**AES-256-GCM**, per-ciphertext random 12-byte nonce. Authenticated encryption: GCM tag detects tampering. No additional-data field is used in P1.

**Stored layout** (single `BYTEA` column per secret):

```
[ nonce: 12 bytes ][ ciphertext: N bytes ][ gcm tag: 16 bytes ]
```

`Open` slices off the leading 12 bytes as nonce and passes the remainder to `aesgcm.Open`. No version byte in P1 — algorithm is fixed. A future P1.x can add a version prefix without breaking reads by detecting "looks like raw nonce" vs "looks like version-byte=0x01".

### 4.2 Master-key bootstrap

Called exactly once at server startup, before any feature module that depends on the vault is constructed. Resolution order:

1. `OPTIMUS_VAULT_MASTER_KEY` env var → base64-decode → must be **exactly 32 bytes** after decode → use.
2. Else `OPTIMUS_VAULT_MASTER_KEY_FILE` env var → read file at that path → contents may be base64 OR raw 32 bytes (detected by length / decode-success) → use.
3. Else → **fail-fast**: BE refuses to start with `vault: master key not configured (set OPTIMUS_VAULT_MASTER_KEY or OPTIMUS_VAULT_MASTER_KEY_FILE)`.

No autogen, no zero-key fallback, no warn-and-continue. The vault is useless without a real key; failing fast at boot is the only safe default.

Master key bytes never leave the `vault` package. The `Cipher` interface exposes only `Seal` / `Open` — feature packages cannot read the key material.

### 4.3 Key generation CLI

```
cmd/vault-keygen/main.go
```

Output: a fresh `crypto/rand`-sourced 32 bytes, base64-encoded, single line to stdout.

```
$ optimus-vault-keygen
9pP3xPIcAhCfQ5wKDS2nWf4uVcK+aqL3o7XeR8mTQyk=
```

Used during first deploy:

```
echo "OPTIMUS_VAULT_MASTER_KEY=$(optimus-vault-keygen)" >> deploy/.env
```

Or for file-mode:

```
optimus-vault-keygen > /etc/optimus/vault.key && chmod 0400 /etc/optimus/vault.key
```

The keygen binary is added to the BE multi-stage Dockerfile as a third build target alongside `optimus-server` and `optimus-migrate` (matches P0 Plan 3's pattern).

### 4.4 Rotation (deferred to P1.x — design accommodates)

Master-key rotation in P1 = manual ops procedure, documented but not automated:

1. Stop BE.
2. Run a rotation script (P1.x deliverable in `cmd/vault-rotate/`) that reads with old key, re-encrypts with new key.
3. Update env / file with new key.
4. Restart BE.

The data layout (raw `bytea`, no version byte) doesn't preclude future envelope-encryption (per-row DEK wrapped by master KEK) but doesn't ship it either. If P1.x adds envelope encryption, the version-byte trick from §4.1 lets old and new ciphertexts coexist during migration.

---

## 5. Backend HTTP API surface

Three parallel CRUD surfaces. All use P0's `Envelope<T>` response shape, P0's list-response envelope `{ items, total, page, page_size }`, P0's permission middleware, and P0's audit middleware.

### 5.1 Endpoint catalogue

| Method | Path | Permission | Body / query | Response |
|---|---|---|---|---|
| GET | `/api/v1/credentials/ssh-keys` | `credentials:ssh_key:read` | `?page=&page_size=&q=&username=` | `Envelope<{items: SshKeyDTO[], total, page, page_size}>` |
| POST | `/api/v1/credentials/ssh-keys` | `credentials:ssh_key:write` | `SshKeyCreateReq` | `Envelope<SshKeyDTO>` |
| GET | `/api/v1/credentials/ssh-keys/:id` | `credentials:ssh_key:read` | — | `Envelope<SshKeyDTO>` |
| PUT | `/api/v1/credentials/ssh-keys/:id` | `credentials:ssh_key:write` | `SshKeyUpdateReq` | `Envelope<SshKeyDTO>` |
| DELETE | `/api/v1/credentials/ssh-keys/:id` | `credentials:ssh_key:delete` | — | `Envelope<{}>` |

Identical shape for `kubeconfigs` (filters: `q`, `default_namespace`) and `cloud-keys` (filters: `q`, `provider`).

### 5.2 DTOs

All DTOs **never** include encrypted-column fields nor decrypted secret material.

```go
type SshKeyDTO struct {
    ID            int64     `json:"id"`
    Name          string    `json:"name"`
    Description   string    `json:"description"`
    Username      string    `json:"username"`
    CreatedBy     UserBrief `json:"created_by"`    // {id, email, display_name}
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type SshKeyCreateReq struct {
    Name        string `json:"name"        binding:"required,max=128"`
    Description string `json:"description" binding:"max=4096"`
    Username    string `json:"username"    binding:"required,max=64"`
    PrivateKey  string `json:"private_key" binding:"required"`        // PEM, plaintext-on-wire
    Passphrase  string `json:"passphrase"`                            // optional
}

type SshKeyUpdateReq struct {
    Name        *string `json:"name,omitempty"        binding:"omitempty,max=128"`
    Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
    Username    *string `json:"username,omitempty"    binding:"omitempty,max=64"`
    PrivateKey  *string `json:"private_key,omitempty"`                // empty/absent = unchanged
    Passphrase  *string `json:"passphrase,omitempty"`                 // empty = clear; absent = unchanged
}
```

Pointer-receiver semantics for the Update DTO matches P0 2b's `formDiff` semantics:
- field absent (`nil` pointer) → "don't touch this column"
- field present + empty string → "clear" (for nullable columns) or "rejected" (for required columns)
- field present + non-empty → "set this value"

For secret fields (`PrivateKey`, `Passphrase`, `Kubeconfig`, `AccessKeyID`, `SecretAccessKey`): present + non-empty = rotate the encrypted column. Absent = unchanged. Empty string is rejected for non-nullable secrets and clears the column for nullable ones (`Passphrase` only).

### 5.3 Validation

- `name` matches `^[A-Za-z0-9_.-]{1,128}$` per type.
- SSH `private_key`: validated by parsing with `golang.org/x/crypto/ssh.ParseRawPrivateKey`. Passphrase-protected keys validated via `ParseRawPrivateKeyWithPassphrase` when `passphrase` provided. Both encrypted independently.
- Kubeconfig: validated by parsing with `k8s.io/client-go/tools/clientcmd.Load` — must yield at least one context.
- Cloud key: `provider ∈ {aws, gcp, azure}`. No deep validation of access key ID format (varies by provider; reject only if empty / over 256 chars).

Validation failures return `code=40001` per P0 error code convention, with `message_key=credentials.invalid_key_format` (or more specific keys — see §10).

### 5.4 Error contract

All errors use P0's envelope shape. Top-level new keys:

| code | message_key | meaning |
|---|---|---|
| 40401 | `credentials.not_found` | id not in table |
| 40901 | `credentials.name_taken` | unique constraint on `name` |
| 40001 | `credentials.invalid_key_format` | SSH/kubeconfig parse failure |
| 40002 | `credentials.invalid_provider` | cloud-key provider not in enum |
| 50001 | `credentials.crypto_seal_failed` | encrypt failure (should never happen — defensive) |
| 50002 | `credentials.crypto_open_failed` | decrypt or auth-tag verify failure |
| 50003 | `credentials.master_key_unset` | startup error (won't surface via HTTP — BE refuses to start) |

---

## 6. Internal Go consume seam

This is the API downstream sub-projects (P2/P4/P5/P6) import. It is intentionally **separate from the HTTP surface** so that:

- downstream packages don't pay an HTTP round-trip per consume,
- the seam can be used in background jobs and cron tasks without forging fake HTTP contexts,
- audit attribution is unambiguous (caller passes `purpose` directly).

### 6.1 Public types

```go
// Package credentials, file internal/modules/credentials/consume.go

type SSHKey struct {
    Name       string
    Username   string
    PrivateKey []byte   // PEM bytes, decrypted
    Passphrase []byte   // nil if not set
}

type Kubeconfig struct {
    Name             string
    DefaultNamespace string
    YAML             []byte   // decrypted YAML
}

type CloudKey struct {
    Name            string
    Provider        string   // 'aws' | 'gcp' | 'azure'
    Region          string
    AccessKeyID     string
    SecretAccessKey string   // decrypted
}

type Consumer interface {
    GetSSHKey(ctx context.Context, id int64, purpose string) (*SSHKey, error)
    GetKubeconfig(ctx context.Context, id int64, purpose string) (*Kubeconfig, error)
    GetCloudKey(ctx context.Context, id int64, purpose string) (*CloudKey, error)
}
```

(IDs are `int64` to match P0's `BIGSERIAL` PKs throughout the codebase.)

### 6.2 Behaviour

Every successful `GetXxx` call:

1. Loads the row by id (returns `ErrNotFound` if missing).
2. Decrypts the secret columns via `vault.Cipher.Open`.
3. Emits an audit event `credentials.consume.<type>` with:
   - `actor_user_id` = current user from `ctx` (set by P0 auth middleware) — may be NULL for system callers
   - `credential_id`, `credential_type`, `credential_name` populated from the row
   - `details.purpose` = the supplied `purpose` string

`purpose` rules:

- Must be non-empty.
- For HTTP callers (ctx has a user): any non-empty string; convention is `"<feature>.<action>"` (e.g., `"k8s.exec.pod"`).
- For system callers (ctx has no user): must start with `"system:"` (e.g., `"system:cron.cluster-sync"`). Service returns `ErrSystemPurposeRequired` if violated.

(The Go seam returns Go sentinel errors only; no HTTP code translation happens here because there is no HTTP "consume" endpoint in P1. The `credentials.invalid_purpose` / `credentials.system_purpose_required` message_keys in §10 are pre-registered for future use and for downstream packages that want to surface a consistent label.)

Returned struct's secret fields hold plaintext bytes/strings. The package exposes a `Wipe(k *SSHKey)` / `Wipe(k *Kubeconfig)` / `Wipe(k *CloudKey)` helper that zeroes the secret fields; callers are expected (but not enforced) to defer it after use.

### 6.3 Permission semantics inside the seam

The seam does **not** check `credentials:*:use` permission on each call — it trusts the calling package to enforce the right RBAC at its own boundary. Rationale: downstream packages (P2/P4/...) will already enforce their own feature-specific permissions (e.g., `k8s:exec:write`); double-gating is redundant and forces every internal caller to thread the user's permission set deep into call chains.

The `:use` permission code is registered and surfaced in role management; downstream packages are expected to wire it into their authorization layer when they need it. P1 itself ships the codes but only `:read` and `:write` are gating HTTP routes in P1.

### 6.4 Smoke test

A throwaway `internal/modules/credentials/consume_smoke_test.go` performs one CRUD-then-consume cycle per type against the dockertest harness. Its only purpose is to prove the seam is importable and round-trips correctly — it's a permanent integration test, not a one-shot.

---

## 7. Permission codes

Twelve new codes registered at startup via `internal/infra/permissions/codes.go`. Pattern matches P0's existing `system:user|role|menu` shape (separate `:write` and `:delete` actions), verified against `codes.go`:

```
credentials:ssh_key:read
credentials:ssh_key:write       (create/update)
credentials:ssh_key:delete
credentials:ssh_key:use
credentials:kubeconfig:read
credentials:kubeconfig:write
credentials:kubeconfig:delete
credentials:kubeconfig:use
credentials:cloud_key:read
credentials:cloud_key:write
credentials:cloud_key:delete
credentials:cloud_key:use
```

i18n keys follow P0's actual pattern (verified against `optimus-fe/src/locales/zh-CN.json` and `optimus-be/internal/infra/permissions/codes.go`):

- **One category label:** `perm.category.credentials` (zh-CN: `凭证管理`, en-US: `Credentials`). P0 already has `perm.category.k8s` reserved, so this slot is conventional.
- **One per-code label per permission** (12 total), each registered in `codes.go` with its `Name` field set to the i18n key:

```
perm.credentials.ssh_key.read      → 查看 SSH 凭证 / View SSH credentials
perm.credentials.ssh_key.write     → 新建/修改 SSH 凭证 / Create/update SSH credentials
perm.credentials.ssh_key.delete    → 删除 SSH 凭证 / Delete SSH credentials
perm.credentials.ssh_key.use       → 使用 SSH 凭证 / Use SSH credentials
perm.credentials.kubeconfig.read   → 查看 kubeconfig / View kubeconfigs
perm.credentials.kubeconfig.write  → 新建/修改 kubeconfig / Create/update kubeconfigs
perm.credentials.kubeconfig.delete → 删除 kubeconfig / Delete kubeconfigs
perm.credentials.kubeconfig.use    → 使用 kubeconfig / Use kubeconfigs
perm.credentials.cloud_key.read    → 查看云密钥 / View cloud keys
perm.credentials.cloud_key.write   → 新建/修改云密钥 / Create/update cloud keys
perm.credentials.cloud_key.delete  → 删除云密钥 / Delete cloud keys
perm.credentials.cloud_key.use     → 使用云密钥 / Use cloud keys
```

The default admin role (seeded in P0) automatically gets all 12 codes via the existing "admin = all permissions" seed pattern.

---

## 8. FE pages

Three pages, near-identical structure, all matching P0 Plan 2b's CRUD shape:

### 8.1 Common shape (per page)

- **Header row**: page title + "Create" button (gated on `:write`).
- **Search row**: `name/description` text input + per-type filter dropdown.
- **Table**: name, [type-specific cols], created_by display name, updated_at, actions (Edit button gated on `:write`, Delete button gated on `:delete`).
- **Create modal**: type-specific form with all required fields.
- **Edit modal**: same form, but secret fields are masked with `•••• (unchanged)` placeholder; empty submit = unchanged, non-empty = rotate.
- **Delete confirmation**: standard P0 confirm modal.

Pagination via `?page=&page_size=` (default page_size=20). No URL state sync (per P0 2b decision).

### 8.2 Per-type fields

**SSH keys** (`/credentials/ssh-keys`):

| Field | Create form | Edit form | Table |
|---|---|---|---|
| name | text, required | text | ✓ |
| description | textarea, optional | textarea | — |
| username | text, required | text | ✓ |
| private_key | textarea, required, monospace | textarea, masked placeholder | — |
| passphrase | text, optional | text, masked placeholder | — |

Search filter: `username` text input.

**Kubeconfigs** (`/credentials/kubeconfigs`):

| Field | Create form | Edit form | Table |
|---|---|---|---|
| name | text, required | text | ✓ |
| description | textarea, optional | textarea | — |
| default_namespace | text, optional | text | ✓ |
| kubeconfig | textarea, required, monospace, large | textarea, masked placeholder | — |

Search filter: `default_namespace` text input.

**Cloud keys** (`/credentials/cloud-keys`):

| Field | Create form | Edit form | Table |
|---|---|---|---|
| name | text, required | text | ✓ |
| description | textarea, optional | textarea | — |
| provider | radio: aws/gcp/azure | radio | ✓ |
| region | text, optional | text | ✓ |
| access_key_id | text, required | text, masked placeholder | — |
| secret_access_key | text, required | text, masked placeholder | — |

Search filter: `provider` dropdown (all / aws / gcp / azure).

### 8.3 Permission gating

- Top-level menu entry `凭证管理 / Credentials` is hidden if user has none of `credentials:*:read`.
- Each sub-page is hidden if user lacks that type's `:read`.
- Create button and per-row Edit button gated on `:write`. Per-row Delete button gated on `:delete`. All gating uses P0's `v-permission` directive (e.g., `v-permission="'credentials:ssh_key:write'"`).

### 8.4 i18n

New namespace `credentials.*` added to `src/locales/zh-CN.json` (authority) and `en-US.json`. Includes:

- `credentials.menu.title`, `credentials.menu.ssh_keys`, `credentials.menu.kubeconfigs`, `credentials.menu.cloud_keys`
- `credentials.field.name`, `credentials.field.description`, `credentials.field.username`, `credentials.field.private_key`, `credentials.field.passphrase`, `credentials.field.default_namespace`, `credentials.field.kubeconfig`, `credentials.field.provider`, `credentials.field.region`, `credentials.field.access_key_id`, `credentials.field.secret_access_key`
- `credentials.placeholder.unchanged` (the `•••• (unchanged)` text)
- `credentials.action.create`, `credentials.action.edit`, `credentials.action.delete`, `credentials.action.confirm_delete`
- `credentials.toast.created`, `credentials.toast.updated`, `credentials.toast.deleted`
- All error-side message_keys from §5.4 (e.g., `credentials.not_found`, `credentials.invalid_key_format`)

---

## 9. Audit events

All five events reuse P0's existing `audit_logs` columns (per §3.2 — no schema change). Field mapping:

- `user_id` ← actor user id from ctx (nullable for system callers; FK ON DELETE SET NULL already exists per `00010_foreign_keys.sql`)
- `action` ← the event-type string from the table below
- `target_type` ← one of `credentials.ssh_key` / `credentials.kubeconfig` / `credentials.cloud_key`
- `target_id` ← the credential's BIGSERIAL id, stringified
- `payload` ← jsonb with denormalized `name` + event-specific extras

| action (event type) | trigger | payload jsonb |
|---|---|---|
| `credentials.create` | POST handler success | `{"name": "..."}` |
| `credentials.update` | PUT (metadata-only change) | `{"name": "...", "changed_fields": ["description", "username"]}` |
| `credentials.rotate` | PUT that changed any encrypted column | `{"name": "...", "changed_fields": [...], "secret_rotated": true}` |
| `credentials.delete` | DELETE handler success | `{"name": "..."}` (snapshot before delete; `target_id` survives as a dangling id by design) |
| `credentials.consume` | every successful `Consumer.GetXxx` call | `{"name": "...", "purpose": "k8s.exec.pod"}` |

(`target_type` disambiguates which kind of credential, so we don't suffix the action with the type. This matches P0's existing pattern where `target_type` carries the entity kind and `action` is the verb.)

Service writes audit rows after the main mutation succeeds (matches P0 audit middleware pattern). For `consume`, audit write is best-effort: a failed audit write does **not** fail the consume call (downstream operations would be left half-done). A failed audit emits a warning log.

---

## 10. Errors & i18n

Error catalogue lives in `internal/modules/credentials/errors.go`, registered into P0's error registry at module init. zh-CN is the authority.

| message_key | zh-CN | en-US |
|---|---|---|
| `credentials.not_found` | 凭证不存在 | Credential not found |
| `credentials.name_taken` | 凭证名称已被占用 | Credential name already exists |
| `credentials.invalid_key_format` | 凭证内容格式错误 | Invalid credential format |
| `credentials.invalid_provider` | 云厂商必须是 aws / gcp / azure 之一 | Provider must be one of aws / gcp / azure |
| `credentials.invalid_purpose` | consume 调用必须提供 purpose | A purpose must be supplied for consume |
| `credentials.system_purpose_required` | 系统调用的 purpose 必须以 system: 开头 | System-level consume requires purpose prefix `system:` |
| `credentials.crypto_seal_failed` | 加密失败，请联系管理员 | Encryption failed |
| `credentials.crypto_open_failed` | 解密失败或数据已损坏 | Decryption or integrity check failed |

`credentials.master_key_unset` is a startup error and never surfaces over HTTP (BE refuses to start) — no i18n needed; emitted in English directly.

---

## 11. Testing strategy

Following P0's layered TDD convention:

### 11.1 vault package (unit, no DB)

- Seal / Open round-trip for plaintext sizes {0, 1, 1024, 1 MB}.
- Open with tampered ciphertext (flip one byte) returns `ErrCryptoOpenFailed`.
- Open with wrong key returns `ErrCryptoOpenFailed`.
- Each Seal of the same plaintext produces a different ciphertext (random nonce).
- `keyloader`: env-set → uses env; env-unset + file-set → uses file; both unset → error; env set to non-32-byte → error; file path nonexistent → error.

### 11.2 Repository tests (dockertest)

- Per-type CRUD: create, get-by-id, list with pagination, list with filters, update metadata, update secret, delete.
- `name` unique constraint enforced (returns wrapped `ErrNameTaken`).
- Soft-delete-on-user: deleting a user via P0's flow leaves credential rows intact but `created_by_user_id` = NULL (FK ON DELETE SET NULL).

### 11.3 Service tests (unit, mock repo + mock cipher)

- PUT with absent secret field → repo update call has no secret-column change.
- PUT with non-empty secret field → cipher.Seal called → repo update writes ciphertext.
- Consume calls cipher.Open → returns decrypted struct.
- Consume with empty purpose → returns `ErrInvalidPurpose`.
- Consume with no-user ctx + non-`system:` purpose → returns `ErrSystemPurposeRequired`.
- Consume emits audit event with correct denormalized fields.
- Audit write failure during consume → returns success but logs warning.

### 11.4 Handler tests (dockertest, full HTTP stack with auth middleware)

- Per-type CRUD via HTTP, asserting permission gating (401 / 403 paths).
- Secret fields **never** appear in JSON list / detail responses (integration test scans response body for the original secret bytes).
- Validation: malformed SSH key → 400 with `credentials.invalid_key_format`; missing required field → 400.
- Delete leaves audit rows queryable: `payload.name` snapshot and `target_type` survive after the underlying credential row is gone.

### 11.5 Consume smoke (dockertest)

- One create + one `Consumer.GetXxx` cycle per type. Assert returned decrypted bytes equal what was originally posted. Assert audit row exists.

### 11.6 FE tests (vitest)

- Form components: required-field validation, masked secret placeholder on edit.
- `formDiff` integration: editing only metadata sends a PUT without secret fields; editing secret sends a PUT with the secret.
- Permission gating: create / edit / delete buttons hidden when v-permission is missing.

### 11.7 Coverage gate

Per-module BE coverage ≥ 60% per `credentials/sshkey`, `credentials/kubeconfig`, `credentials/cloudkey`, `credentials/vault`. Matches P0 §12.

---

## 12. Acceptance criteria

P1 is "done" when all of the following hold on `main`:

1. Three CRUD pages exist behind permissions and work end-to-end in the deployed `docker-compose.prod.yml` stack.
2. Secrets never reach the FE list/detail responses, verified by integration test (§11.4).
3. `Consumer` interface is exported from `internal/modules/credentials` and importable; consume smoke test (§11.5) passes.
4. Master-key bootstrap fails-fast with a clear message when no key source is configured.
5. `optimus-vault-keygen` CLI produces a valid base64 32-byte key that the server can load (manual deploy smoke).
6. Every `Consumer.GetXxx` call produces an `audit_logs` row with `action='credentials.consume'`, the right `target_type`, and a non-empty `payload.purpose`.
7. Deleting a credential leaves audit rows readable: `payload.name` snapshot and `target_type` survive after the credential row is gone (no changes to P0's audit page needed — existing columns render correctly).
8. CI green on `main`. Per-module BE coverage ≥ 60% on all four new packages.
9. README "Production deploy" section updated with: how to generate the master key, where to mount it, how to set the env var, ordering relative to migrate / server startup.
10. The `optimus-vault-keygen` binary is built into the BE Dockerfile as a third stage / target alongside `optimus-server` and `optimus-migrate`.

---

## 13. Decisions log

Key choices crystallised during brainstorming (2026-06-10):

- **Scope**: storage + CRUD + Go consume seam. No rotation API, no expiry, no external KMS in P1.
- **Type extensibility**: hard-coded 3 types, 3 separate tables. Adding a 4th type = full new migration + new package. Chosen over single-table-with-payload for stronger DB typing and clearer per-type validation.
- **Master key**: env-var primary, file-path fallback. No KMS in P1.
- **Access model**: flat per-type permission. Per-credential ACLs deferred to P1.x if needed.
- **Consume API**: typed-per-type Go functions, caller-supplied `purpose` string for audit attribution. Generic bytes API rejected to centralise audit obligation.
- **Secret display**: write-only. No reveal, no copy-plaintext from UI. Secrets only leave BE via the Go seam.
- **Delete behaviour**: hard delete + denormalized name snapshot into existing `audit_logs.payload` jsonb (P0 convention preserved; no schema change to `audit_logs` needed; forensics-after-delete preserved via the payload snapshot).

---

## 14. Non-goals (P1.x or later)

- `optimus-vault-rotate` CLI for master-key rotation (data layout accommodates; script deferred).
- Per-credential expiry timestamps + warning UI.
- External KMS (AWS KMS, GCP KMS, HashiCorp Vault) — pluggable master-key backend.
- Per-credential ACLs / role-sharing.
- File-upload UI for credential material (instead of paste-into-textarea).
- Reveal / copy-to-clipboard buttons on detail page.
- Bulk import / export.
- Credential usage analytics / heatmaps.
- Generic credential type registry (only relevant if "kinds of credentials" exceeds ~5).
