# P1 — credentials-vault Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land an encrypted credential vault on top of P0 — three credential types (SSH key, kubeconfig, cloud access key) with full CRUD over HTTP, AES-256-GCM application-layer encryption, an internal Go `Consumer` seam for downstream P2/P4/P5/P6, and 12 permission codes wired into P0's RBAC.

**Architecture:** New BE module `internal/modules/credentials/` with one crypto sub-package (`vault`) and three feature sub-packages (`sshkey`, `kubeconfig`, `cloudkey`). Each feature package mirrors P0's `role`/`user` shape (model + repo + service + handler + routes). Three new FE pages re-use P0 Plan 2b's CRUD shape. A new `cmd/vault-keygen` CLI mints the master key for first deploy. No schema change to `audit_logs` — denormalized snapshot lives in the existing `payload` jsonb column.

**Tech Stack:** Go 1.25 + Gin + GORM, AES-256-GCM (`crypto/aes` + `crypto/cipher` stdlib), goose v3 (existing P0 migration toolchain), Vue 3 + AntdV (existing P0 FE), dockertest v3 (existing P0 test harness).

**Reference spec:** `docs/superpowers/specs/2026-06-10-p1-credentials-vault-design.md` (commit `26b91d1`).

---

## File map

| Task | New / Modified | Path | Responsibility |
|---|---|---|---|
| 1 | Modify | `optimus-be/internal/infra/config/config.go` | Add `VaultConfig{MasterKey, MasterKeyFile}` struct + field on `Config` |
| 1 | Modify | `optimus-be/internal/infra/config/config_test.go` | Test viper binding for new fields |
| 1 | Modify | `optimus-be/configs/config.yaml` | Add `vault:` section with defaults |
| 2 | New | `optimus-be/internal/modules/credentials/vault/keyloader.go` | env → file resolution; returns 32-byte key or error |
| 2 | New | `optimus-be/internal/modules/credentials/vault/keyloader_test.go` | Unit tests for resolution precedence + length check |
| 2 | New | `optimus-be/internal/modules/credentials/vault/cipher.go` | `Cipher.Seal` / `Cipher.Open` using AES-256-GCM |
| 2 | New | `optimus-be/internal/modules/credentials/vault/cipher_test.go` | Round-trip, tamper detection, wrong-key, varying sizes |
| 3 | New | `optimus-be/cmd/vault-keygen/main.go` | CLI: print base64(crypto/rand 32 bytes) |
| 3 | New | `optimus-be/cmd/vault-keygen/main_test.go` | Smoke: output is valid base64 of exactly 32 bytes |
| 4 | Modify | `optimus-be/internal/infra/permissions/codes.go` | Append 12 new credential permissions |
| 4 | Modify | `optimus-be/internal/infra/permissions/registry_test.go` | Test the 12 new codes are registered with category `credentials` |
| 5 | New | `optimus-be/migrations/00012_create_credentials_ssh_keys.sql` | DDL for `credentials_ssh_keys` |
| 5 | New | `optimus-be/internal/models/credential_ssh_key.go` | GORM model |
| 6 | New | `optimus-be/internal/modules/credentials/sshkey/dto.go` | `Summary`, `Detail`, `CreateRequest`, `UpdateRequest` |
| 6 | New | `optimus-be/internal/modules/credentials/sshkey/repo.go` | CRUD repo |
| 6 | New | `optimus-be/internal/modules/credentials/sshkey/repo_test.go` | dockertest CRUD + uniqueness |
| 7 | New | `optimus-be/internal/modules/credentials/sshkey/service.go` | Validates key, calls cipher, emits audit |
| 7 | New | `optimus-be/internal/modules/credentials/sshkey/service_test.go` | Unit tests with mock cipher + repo |
| 8 | New | `optimus-be/internal/modules/credentials/sshkey/handler.go` | Gin handlers |
| 8 | New | `optimus-be/internal/modules/credentials/sshkey/handler_test.go` | dockertest HTTP path |
| 9 | New | `optimus-be/migrations/00013_create_credentials_kubeconfigs.sql` | DDL |
| 9 | New | `optimus-be/internal/models/credential_kubeconfig.go` | GORM model |
| 9 | New | `optimus-be/internal/modules/credentials/kubeconfig/{dto,repo,repo_test,service,service_test,handler,handler_test}.go` | Symmetric to sshkey |
| 10 | New | `optimus-be/migrations/00014_create_credentials_cloud_keys.sql` | DDL |
| 10 | New | `optimus-be/internal/models/credential_cloud_key.go` | GORM model |
| 10 | New | `optimus-be/internal/modules/credentials/cloudkey/{dto,repo,repo_test,service,service_test,handler,handler_test}.go` | Symmetric to sshkey |
| 11 | New | `optimus-be/internal/modules/credentials/consume.go` | Exported `Consumer` interface + types |
| 11 | New | `optimus-be/internal/modules/credentials/consume_smoke_test.go` | dockertest end-to-end consume |
| 12 | New | `optimus-be/internal/modules/credentials/module.go` | DI wiring + `MountRoutes` helper |
| 12 | Modify | `optimus-be/cmd/server/main.go` | Wire credentials module |
| 13 | Modify | `optimus-be/internal/seed/seed.go` | Seed 1 parent + 3 child menus |
| 13 | Modify | `optimus-be/internal/seed/seed_test.go` | Assert new menus seeded |
| 14 | Modify | `optimus-fe/src/locales/zh-CN.json` | Add `credentials.*`, `perm.credentials.*`, `perm.category.credentials`, `menu.credentials*` |
| 14 | Modify | `optimus-fe/src/locales/en-US.json` | Same |
| 15 | New | `optimus-fe/src/api/credentials/ssh-key.ts` | DTOs + http calls |
| 15 | New | `optimus-fe/src/api/credentials/kubeconfig.ts` | Same shape |
| 15 | New | `optimus-fe/src/api/credentials/cloud-key.ts` | Same shape |
| 16 | New | `optimus-fe/src/views/credentials/ssh-keys/List.vue` | CRUD page |
| 16 | New | `optimus-fe/src/views/credentials/ssh-keys/components/SshKeyForm.vue` | Create/edit form |
| 16 | New | `optimus-fe/src/views/credentials/ssh-keys/__tests__/List.test.ts` | Vitest |
| 17 | New | `optimus-fe/src/views/credentials/kubeconfigs/List.vue` | CRUD page |
| 17 | New | `optimus-fe/src/views/credentials/kubeconfigs/components/KubeconfigForm.vue` | Form |
| 18 | New | `optimus-fe/src/views/credentials/cloud-keys/List.vue` | CRUD page |
| 18 | New | `optimus-fe/src/views/credentials/cloud-keys/components/CloudKeyForm.vue` | Form |
| 19 | Modify | `deploy/be.Dockerfile` | Add `vault-keygen` as 4th build target |
| 19 | Modify | `deploy/.env.example` | Add `OPTIMUS_VAULT_MASTER_KEY` placeholder |
| 19 | Modify | `deploy/docker-compose.prod.yml` | Pass vault env to be service |
| 19 | Modify | `README.md` | "Production deploy" section: master key bootstrap |
| 20 | Verify | (no new files) | Run CI locally + manual smoke + final commit |

---

## Implementation notes (read before starting)

1. **PK type is `uint64` / `BIGSERIAL`, not UUID.** P0 uses integer PKs throughout (verified `optimus-be/internal/models/role.go` and `migrations/00001_create_users.sql`). The spec's §3.1 was corrected during self-review; the data model section uses `BIGSERIAL` accordingly.

2. **audit_logs is NOT altered.** P0's `audit_logs` already has `target_type`/`target_id`/`payload` (jsonb). The denormalized name snapshot lives in `payload.name`. See spec §3.2 and §9. No migration touches `audit_logs`.

3. **Package naming.** Go package names cannot contain underscores or hyphens. Sub-packages are `sshkey`, `kubeconfig`, `cloudkey`. The DB table names use `credentials_ssh_keys` etc. The permission codes use `credentials:ssh_key:read` etc. (snake_case for the resource segment).

4. **Audit recording.** Use `audit.Recorder.Record(ctx, audit.Event{...})` exactly as `role/service.go` does. The recorder lives in `internal/modules/audit/recorder.go`. For `consume` events, IP and UserAgent are empty strings (no HTTP context for internal Go callers); for HTTP-triggered events, the handler extracts them from the gin context (helpers exist in `audit` module).

5. **Per-route RBAC mount style.** Routes are mounted via a `mountXxxRoutes` helper in `cmd/server/main.go`. Each helper does `g.Group("", middleware.RequirePermission(cache, "<code>"))` per action group (read / write / delete). See `mountRoleRoutes` in `cmd/server/main.go:217` for the canonical example. We follow the same shape with three groups per route file: read / write / delete.

6. **The `Cipher` is constructed once.** `vault.NewCipher(cfg.Vault)` in `cmd/server/main.go` returns a `*Cipher` that is shared across all three feature services. The crypto package exposes only `Seal([]byte) ([]byte, error)` and `Open([]byte) ([]byte, error)` — feature packages never see the master key.

7. **FE secret field semantics.** The Edit form pre-fills metadata fields but leaves secret fields empty with a `••••• (unchanged)` placeholder. The `formDiff` helper from P0 2a (`src/utils/form-diff.ts`) already does the "only send changed fields" logic — for secrets we just ensure the form binding is `''` (empty string) initially in edit mode. A non-empty submit ships the new secret; an empty submit omits the field, which the BE treats as "do not change."

8. **DTOs are pointer-receiver for Update.** Following P0's `role.UpdateRequest` pattern, all `XxxUpdateRequest` structs use `*string` for optional fields (`omitempty` + pointer). Service code checks `if req.Field != nil { ... }`.

9. **SSH key validation requires `golang.org/x/crypto/ssh`.** P0 already depends on this transitively. Verify with `go list -m golang.org/x/crypto` from `optimus-be/`. If missing as a direct dep, the task that needs it (Task 7) bumps `go.mod` to make it direct.

10. **kubeconfig validation requires `k8s.io/client-go/tools/clientcmd`.** This is NOT currently in `go.mod`. Task 10's first step is `go get k8s.io/client-go@latest`, which transitively pulls `k8s.io/api`, `k8s.io/apimachinery`, etc. — acceptable since the entire P2 sub-project will need them. Adding now amortizes the cost.

11. **Subprocess audit attribution.** The consume seam reads the actor user_id from `ctx.Value("user_id")` (the existing P0 auth middleware key — verify with `grep -rn 'user_id' optimus-be/internal/infra/middleware/`). If missing, the call is treated as "system" and `purpose` MUST start with `"system:"` (returns `ErrSystemPurposeRequired` otherwise).

12. **Existing menu component path style.** Per `internal/seed/seed.go`, menus reference FE components by path-without-`src/views/` prefix and without the `.vue` extension, e.g. `system/users/List`. The new menus use `credentials/ssh-keys/List` (note: hyphens in URL path are fine in the Vue Router; the FE folder uses the same `ssh-keys` form).

13. **i18n single source of truth.** zh-CN is authority; en-US mirrors. The CI check `check-i18n-keys.ts` already gates that all referenced keys exist in BOTH locales (verify in `optimus-fe/scripts/check-i18n-keys.ts`). All new keys must be added to both files in the same PR.

14. **No new permission gate for `consume`.** The `:use` permission codes ARE registered, but P1 routes do NOT enforce `:use` at the HTTP layer — see spec §6.3. Downstream sub-projects (P2+) gate `:use` at their own feature boundaries. Document this in the consume.go file's package comment.

15. **The smoke test is permanent.** `consume_smoke_test.go` stays in the repo as a permanent integration test (not a throwaway).

16. **Existing dockertest harness pattern.** See `optimus-be/internal/modules/role/repo_test.go` for the canonical dockertest setup. New repo tests follow the same `TestMain` + `setupTestDB(t)` pattern (search for `dockertest.NewPool`).

17. **Permission seeding for admin.** P0's seed automatically grants the admin role all permissions registered via `permissions.All`. So once Task 4 appends the 12 new codes to `permissions.All`, the admin role gets them all on next `cmd/seed` run.

18. **CI coverage gate.** P0 §12 requires per-module BE coverage ≥ 60%. CI runs `go test -coverprofile=coverage.out ./internal/modules/...`. Verify with `make test-cover` or `go test -cover ./internal/modules/credentials/...` locally before commit.

19. **Commits use Conventional Commits prefixes.** Match P0 style: `feat(be/credentials): ...`, `test(be/credentials): ...`, `feat(fe/credentials): ...`, `chore(deploy): ...`. See `git log --oneline | head` for the canonical voice.

20. **No worktree needed.** P1 is a new module that touches no in-flight files except `permissions/codes.go`, `seed.go`, `config.go`, `cmd/server/main.go`, `Dockerfile`, `README.md`, and FE locales. None of these have known concurrent work. Implement directly on `dev`.

---

## Task 1: Config — add Vault section

**Files:**
- Modify: `optimus-be/internal/infra/config/config.go`
- Modify: `optimus-be/internal/infra/config/config_test.go`
- Modify: `optimus-be/configs/config.yaml`

- [ ] **Step 1: Add the `VaultConfig` struct and `Vault` field**

In `optimus-be/internal/infra/config/config.go`, append a new type next to `BootstrapConfig`:

```go
type VaultConfig struct {
	MasterKey     string `mapstructure:"master_key"`
	MasterKeyFile string `mapstructure:"master_key_file"`
}
```

And add the field to `Config`:

```go
type Config struct {
	Server   ServerConfig    `mapstructure:"server"`
	Database DatabaseConfig  `mapstructure:"database"`
	JWT      JWTConfig       `mapstructure:"jwt"`
	Auth     AuthConfig      `mapstructure:"auth"`
	Log      LogConfig       `mapstructure:"log"`
	CORS     CORSConfig      `mapstructure:"cors"`
	I18n     I18nConfig      `mapstructure:"i18n"`
	Boot     BootstrapConfig `mapstructure:"bootstrap"`
	Vault    VaultConfig     `mapstructure:"vault"`
}
```

- [ ] **Step 2: Write a failing test for viper binding**

In `optimus-be/internal/infra/config/config_test.go`, add:

```go
func TestLoad_BindsVaultSection(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte(`
server: {host: "0.0.0.0", port: 8080}
database: {driver: "postgres", dsn: "postgres://x:y@z:5432/db"}
jwt: {secret: "abc"}
vault:
  master_key: "envkey"
  master_key_file: "/tmp/k"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Vault.MasterKey != "envkey" {
		t.Errorf("MasterKey = %q, want envkey", c.Vault.MasterKey)
	}
	if c.Vault.MasterKeyFile != "/tmp/k" {
		t.Errorf("MasterKeyFile = %q, want /tmp/k", c.Vault.MasterKeyFile)
	}
}

func TestLoad_VaultSectionOptional(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte(`
server: {host: "0.0.0.0", port: 8080}
database: {driver: "postgres", dsn: "postgres://x:y@z:5432/db"}
jwt: {secret: "abc"}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Vault.MasterKey != "" || c.Vault.MasterKeyFile != "" {
		t.Errorf("Vault should default to zero, got %+v", c.Vault)
	}
}
```

Add `"os"` and `"path/filepath"` to imports if not already present.

- [ ] **Step 3: Run tests**

```
cd optimus-be && go test ./internal/infra/config/ -run TestLoad_BindsVaultSection -v
```

Expected: PASS (the field is already declared in Step 1).

- [ ] **Step 4: Wire env-var prefix**

If `config.go`'s `Load` already uses `v.SetEnvPrefix("OPTIMUS")` + `v.AutomaticEnv()` (verify by reading the existing `Load` function), confirm that `OPTIMUS_VAULT_MASTER_KEY` env var resolves to `Vault.MasterKey` via viper's default key replacer. P0 already uses `strings.NewReplacer(".", "_")` (verify in the file). No code change needed if so; if the replacer is missing, add it.

If verification shows the env-prefix wiring is missing for nested keys, add this line in `Load`:

```go
v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
```

Add another test to confirm env override works:

```go
func TestLoad_VaultMasterKeyFromEnv(t *testing.T) {
	t.Setenv("OPTIMUS_VAULT_MASTER_KEY", "from-env")
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfg, []byte(`
server: {host: "0.0.0.0", port: 8080}
database: {driver: "postgres", dsn: "postgres://x:y@z:5432/db"}
jwt: {secret: "abc"}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Vault.MasterKey != "from-env" {
		t.Errorf("env override failed, got %q", c.Vault.MasterKey)
	}
}
```

Run:

```
cd optimus-be && go test ./internal/infra/config/ -v
```

Expected: PASS.

- [ ] **Step 5: Add `vault:` section to default config**

In `optimus-be/configs/config.yaml`, append before the final blank line:

```yaml
vault:
  master_key: ""
  master_key_file: ""
```

Empty strings; the actual values come from env vars or `master_key_file` in prod.

- [ ] **Step 6: Commit**

```
cd optimus-be && git add internal/infra/config/config.go internal/infra/config/config_test.go configs/config.yaml
cd .. && git commit -m "feat(be/config): add Vault section for P1 credentials master key"
```

---

## Task 2: Vault crypto package

**Files:**
- New: `optimus-be/internal/modules/credentials/vault/keyloader.go`
- New: `optimus-be/internal/modules/credentials/vault/keyloader_test.go`
- New: `optimus-be/internal/modules/credentials/vault/cipher.go`
- New: `optimus-be/internal/modules/credentials/vault/cipher_test.go`

- [ ] **Step 1: Write failing test for keyloader**

Create `optimus-be/internal/modules/credentials/vault/keyloader_test.go`:

```go
package vault

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKey_FromEnv_Base64(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	enc := base64.StdEncoding.EncodeToString(raw)

	got, err := LoadKey(Source{Env: enc})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("key mismatch: got %x want %x", got, raw)
	}
}

func TestLoadKey_FromFile_Base64(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(0xAA ^ i)
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	if err := os.WriteFile(p, []byte(enc), 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("key mismatch")
	}
}

func TestLoadKey_FromFile_RawBytes(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "key.bin")
	if err := os.WriteFile(p, raw, 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("raw key mismatch")
	}
}

func TestLoadKey_EnvWins(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	if err := os.WriteFile(p, []byte("wrong-content"), 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{Env: enc, File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("env did not win")
	}
}

func TestLoadKey_NoSource_Fails(t *testing.T) {
	_, err := LoadKey(Source{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadKey_WrongLength_Fails(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := LoadKey(Source{Env: short})
	if err == nil {
		t.Fatal("expected length error")
	}
}

func TestLoadKey_FileMissing_Fails(t *testing.T) {
	_, err := LoadKey(Source{File: "/nonexistent/path/xyz"})
	if err == nil {
		t.Fatal("expected file error")
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```
cd optimus-be && go test ./internal/modules/credentials/vault/ -v
```

Expected: FAIL (no package yet).

- [ ] **Step 3: Implement keyloader**

Create `optimus-be/internal/modules/credentials/vault/keyloader.go`:

```go
// Package vault is the credentials-vault crypto core. It owns the master key
// and exposes only Seal/Open via the Cipher type. No other package in the
// service should import crypto/aes or crypto/cipher for credential material.
package vault

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// Source describes where the 32-byte master key comes from. Env wins; if Env
// is empty, File is consulted. If both are empty, LoadKey errors.
type Source struct {
	Env  string // base64-encoded key, length-32 after decode
	File string // path to a file containing either base64 or raw 32 bytes
}

const KeyLen = 32

// LoadKey resolves and returns the master key bytes. Always returns either a
// 32-byte slice or a non-nil error — never both.
func LoadKey(src Source) ([]byte, error) {
	if src.Env != "" {
		return decodeKey([]byte(src.Env), "env OPTIMUS_VAULT_MASTER_KEY")
	}
	if src.File != "" {
		raw, err := os.ReadFile(src.File)
		if err != nil {
			return nil, fmt.Errorf("vault: read master key file %q: %w", src.File, err)
		}
		return decodeKey(raw, fmt.Sprintf("file %q", src.File))
	}
	return nil, errors.New("vault: master key not configured (set OPTIMUS_VAULT_MASTER_KEY or OPTIMUS_VAULT_MASTER_KEY_FILE)")
}

// decodeKey accepts either base64-encoded 32 bytes OR raw 32 bytes.
// Trailing whitespace (common in files written via echo / shell redirect) is trimmed.
func decodeKey(input []byte, sourceLabel string) ([]byte, error) {
	trimmed := trimTrailingWS(input)
	if len(trimmed) == KeyLen {
		out := make([]byte, KeyLen)
		copy(out, trimmed)
		return out, nil
	}
	dec, err := base64.StdEncoding.DecodeString(string(trimmed))
	if err != nil {
		return nil, fmt.Errorf("vault: master key from %s is neither raw 32 bytes nor base64: %w", sourceLabel, err)
	}
	if len(dec) != KeyLen {
		return nil, fmt.Errorf("vault: master key from %s decoded to %d bytes, want %d", sourceLabel, len(dec), KeyLen)
	}
	return dec, nil
}

func trimTrailingWS(b []byte) []byte {
	for len(b) > 0 {
		last := b[len(b)-1]
		if last == '\n' || last == '\r' || last == ' ' || last == '\t' {
			b = b[:len(b)-1]
			continue
		}
		break
	}
	return b
}
```

- [ ] **Step 4: Run keyloader tests — expect pass**

```
cd optimus-be && go test ./internal/modules/credentials/vault/ -run TestLoadKey -v
```

Expected: PASS.

- [ ] **Step 5: Write failing test for cipher**

Create `optimus-be/internal/modules/credentials/vault/cipher_test.go`:

```go
package vault

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	key := make([]byte, KeyLen)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	c, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestCipher_RoundTrip_VariousSizes(t *testing.T) {
	c := newTestCipher(t)
	for _, n := range []int{0, 1, 17, 1024, 1024 * 1024} {
		pt := make([]byte, n)
		if _, err := rand.Read(pt); err != nil {
			t.Fatal(err)
		}
		ct, err := c.Seal(pt)
		if err != nil {
			t.Fatalf("Seal(%d): %v", n, err)
		}
		got, err := c.Open(ct)
		if err != nil {
			t.Fatalf("Open(%d): %v", n, err)
		}
		if !bytes.Equal(pt, got) {
			t.Errorf("size=%d round-trip mismatch", n)
		}
	}
}

func TestCipher_SealProducesDifferentCiphertexts(t *testing.T) {
	c := newTestCipher(t)
	pt := []byte("hello")
	a, _ := c.Seal(pt)
	b, _ := c.Seal(pt)
	if bytes.Equal(a, b) {
		t.Error("two seals of same plaintext are identical — nonce reuse?")
	}
}

func TestCipher_OpenRejectsTampering(t *testing.T) {
	c := newTestCipher(t)
	ct, _ := c.Seal([]byte("secret"))
	ct[len(ct)-1] ^= 0x01 // flip a tag bit
	if _, err := c.Open(ct); err == nil {
		t.Error("expected open to reject tampered ciphertext")
	}
}

func TestCipher_OpenRejectsShortInput(t *testing.T) {
	c := newTestCipher(t)
	if _, err := c.Open(make([]byte, 5)); err == nil {
		t.Error("expected error on short input")
	}
}

func TestCipher_OpenRejectsWrongKey(t *testing.T) {
	a := newTestCipher(t)
	b := newTestCipher(t)
	ct, _ := a.Seal([]byte("data"))
	if _, err := b.Open(ct); err == nil {
		t.Error("expected open with wrong key to fail")
	}
}

func TestNewCipher_RejectsWrongKeyLen(t *testing.T) {
	if _, err := NewCipher(make([]byte, 16)); err == nil {
		t.Error("expected error on 16-byte key")
	}
}
```

- [ ] **Step 6: Implement cipher**

Create `optimus-be/internal/modules/credentials/vault/cipher.go`:

```go
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// Cipher wraps an AES-256-GCM AEAD instance over the master key.
// Stored ciphertext layout: [nonce (12 bytes)][ciphertext][tag (16 bytes)].
type Cipher struct {
	aead      cipher.AEAD
	nonceSize int
}

// NewCipher returns a Cipher using AES-256-GCM. key must be exactly 32 bytes.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("vault: cipher key must be %d bytes, got %d", KeyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vault: NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault: NewGCM: %w", err)
	}
	return &Cipher{aead: aead, nonceSize: aead.NonceSize()}, nil
}

// Seal encrypts plaintext. Returns nonce||ciphertext||tag as a single slice.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("vault: nonce: %w", err)
	}
	// aead.Seal appends to nonce, producing nonce||ciphertext||tag.
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts the layout produced by Seal. Returns ErrInvalidCiphertext on
// length-too-short, and the underlying GCM error on auth failure or tamper.
var ErrInvalidCiphertext = errors.New("vault: ciphertext too short")

func (c *Cipher) Open(data []byte) ([]byte, error) {
	if len(data) < c.nonceSize {
		return nil, ErrInvalidCiphertext
	}
	nonce, ct := data[:c.nonceSize], data[c.nonceSize:]
	return c.aead.Open(nil, nonce, ct, nil)
}
```

- [ ] **Step 7: Run all vault tests — expect pass**

```
cd optimus-be && go test ./internal/modules/credentials/vault/ -v
```

Expected: PASS for both keyloader and cipher.

- [ ] **Step 8: Commit**

```
cd .. && git add optimus-be/internal/modules/credentials/vault/
git commit -m "feat(be/credentials/vault): AES-256-GCM cipher + env→file key loader"
```

---

## Task 3: cmd/vault-keygen CLI

**Files:**
- New: `optimus-be/cmd/vault-keygen/main.go`
- New: `optimus-be/cmd/vault-keygen/main_test.go`

- [ ] **Step 1: Write failing test**

Create `optimus-be/cmd/vault-keygen/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestGenerate_ProducesBase64Of32Bytes(t *testing.T) {
	var buf bytes.Buffer
	if err := generate(&buf); err != nil {
		t.Fatalf("generate: %v", err)
	}
	out := buf.Bytes()
	// Output ends with a newline.
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", out)
	}
	body := bytes.TrimRight(out, "\n")
	raw, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		t.Fatalf("output not valid base64: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("decoded len = %d, want 32", len(raw))
	}
}

func TestGenerate_TwoCallsDiffer(t *testing.T) {
	var a, b bytes.Buffer
	_ = generate(&a)
	_ = generate(&b)
	if a.String() == b.String() {
		t.Error("two generates produced identical output — randomness broken")
	}
}
```

- [ ] **Step 2: Implement keygen main**

Create `optimus-be/cmd/vault-keygen/main.go`:

```go
// vault-keygen prints a fresh 32-byte base64-encoded master key to stdout.
// Usage:
//   $ optimus-vault-keygen > .vault-key
//   $ echo "OPTIMUS_VAULT_MASTER_KEY=$(optimus-vault-keygen)" >> .env
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := generate(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "vault-keygen:", err)
		os.Exit(1)
	}
}

func generate(w io.Writer) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("read random: %w", err)
	}
	enc := base64.StdEncoding.EncodeToString(key)
	_, err := fmt.Fprintln(w, enc)
	return err
}
```

- [ ] **Step 3: Run tests**

```
cd optimus-be && go test ./cmd/vault-keygen/ -v
```

Expected: PASS.

- [ ] **Step 4: Manual smoke build**

```
cd optimus-be && go build -o /tmp/vault-keygen ./cmd/vault-keygen && /tmp/vault-keygen
```

Expected: prints one base64 line, e.g. `aBc...XYZ=`. Pipe through `| base64 -d | wc -c` and confirm 32.

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/cmd/vault-keygen/
git commit -m "feat(be/cmd): vault-keygen CLI for P1 master key bootstrap"
```

---

## Task 4: Register 12 credential permission codes

**Files:**
- Modify: `optimus-be/internal/infra/permissions/codes.go`
- Modify: `optimus-be/internal/infra/permissions/registry_test.go`

- [ ] **Step 1: Write failing test**

Open `optimus-be/internal/infra/permissions/registry_test.go` and append:

```go
func TestAll_IncludesCredentialPermissions(t *testing.T) {
	want := []string{
		"credentials:ssh_key:read", "credentials:ssh_key:write", "credentials:ssh_key:delete", "credentials:ssh_key:use",
		"credentials:kubeconfig:read", "credentials:kubeconfig:write", "credentials:kubeconfig:delete", "credentials:kubeconfig:use",
		"credentials:cloud_key:read", "credentials:cloud_key:write", "credentials:cloud_key:delete", "credentials:cloud_key:use",
	}
	got := map[string]Permission{}
	for _, p := range All {
		got[p.Code] = p
	}
	for _, code := range want {
		p, ok := got[code]
		if !ok {
			t.Errorf("missing permission code: %s", code)
			continue
		}
		if p.Category != "credentials" {
			t.Errorf("%s: Category=%q, want credentials", code, p.Category)
		}
		if p.Name == "" {
			t.Errorf("%s: Name (i18n key) empty", code)
		}
	}
}
```

- [ ] **Step 2: Run — expect fail**

```
cd optimus-be && go test ./internal/infra/permissions/ -run TestAll_IncludesCredentialPermissions -v
```

Expected: FAIL (12 codes missing).

- [ ] **Step 3: Append codes**

Open `optimus-be/internal/infra/permissions/codes.go`, and INSIDE the `All` slice literal, append after the existing `system:audit:read` entry (after line 33):

```go
	// credentials: ssh_key
	{Code: "credentials:ssh_key:read", Name: "perm.credentials.ssh_key.read", Category: "credentials", Description: "Read SSH credentials"},
	{Code: "credentials:ssh_key:write", Name: "perm.credentials.ssh_key.write", Category: "credentials", Description: "Create/update SSH credentials"},
	{Code: "credentials:ssh_key:delete", Name: "perm.credentials.ssh_key.delete", Category: "credentials", Description: "Delete SSH credentials"},
	{Code: "credentials:ssh_key:use", Name: "perm.credentials.ssh_key.use", Category: "credentials", Description: "Use SSH credentials"},

	// credentials: kubeconfig
	{Code: "credentials:kubeconfig:read", Name: "perm.credentials.kubeconfig.read", Category: "credentials", Description: "Read kubeconfigs"},
	{Code: "credentials:kubeconfig:write", Name: "perm.credentials.kubeconfig.write", Category: "credentials", Description: "Create/update kubeconfigs"},
	{Code: "credentials:kubeconfig:delete", Name: "perm.credentials.kubeconfig.delete", Category: "credentials", Description: "Delete kubeconfigs"},
	{Code: "credentials:kubeconfig:use", Name: "perm.credentials.kubeconfig.use", Category: "credentials", Description: "Use kubeconfigs"},

	// credentials: cloud_key
	{Code: "credentials:cloud_key:read", Name: "perm.credentials.cloud_key.read", Category: "credentials", Description: "Read cloud keys"},
	{Code: "credentials:cloud_key:write", Name: "perm.credentials.cloud_key.write", Category: "credentials", Description: "Create/update cloud keys"},
	{Code: "credentials:cloud_key:delete", Name: "perm.credentials.cloud_key.delete", Category: "credentials", Description: "Delete cloud keys"},
	{Code: "credentials:cloud_key:use", Name: "perm.credentials.cloud_key.use", Category: "credentials", Description: "Use cloud keys"},
```

- [ ] **Step 4: Run all permission tests — expect pass**

```
cd optimus-be && go test ./internal/infra/permissions/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/internal/infra/permissions/
git commit -m "feat(be/permissions): register 12 credentials:* codes for P1"
```

---

## Task 5: SSH key — migration + GORM model

**Files:**
- New: `optimus-be/migrations/00012_create_credentials_ssh_keys.sql`
- New: `optimus-be/internal/models/credential_ssh_key.go`

- [ ] **Step 1: Create the migration SQL**

Create `optimus-be/migrations/00012_create_credentials_ssh_keys.sql`:

```sql
-- +goose Up
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
CREATE INDEX idx_credentials_ssh_keys_created_by ON credentials_ssh_keys(created_by_user_id);

-- +goose Down
DROP TABLE credentials_ssh_keys;
```

- [ ] **Step 2: Verify embed picks it up**

The `migrations/embed.go` uses `//go:embed *.sql`. Confirm:

```
cd optimus-be && go test ./migrations/ -v
```

Expected: PASS (the embed test counts SQL files; new file appears automatically).

- [ ] **Step 3: Create GORM model**

Create `optimus-be/internal/models/credential_ssh_key.go`:

```go
package models

import "time"

type CredentialSSHKey struct {
	ID              uint64    `gorm:"primaryKey"`
	Name            string    `gorm:"size:128;not null;uniqueIndex"`
	Description     string    `gorm:"type:text;not null;default:''"`
	Username        string    `gorm:"size:64;not null"`
	PrivateKeyEnc   []byte    `gorm:"column:private_key_enc;not null"`
	PassphraseEnc   []byte    `gorm:"column:passphrase_enc"`
	CreatedByUserID *uint64   `gorm:"column:created_by_user_id"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (CredentialSSHKey) TableName() string { return "credentials_ssh_keys" }
```

- [ ] **Step 4: Verify compilation**

```
cd optimus-be && go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/migrations/00012_create_credentials_ssh_keys.sql optimus-be/internal/models/credential_ssh_key.go
git commit -m "feat(be/credentials): SSH key migration + GORM model"
```

---

## Task 6: SSH key — DTOs + repository + tests

**Files:**
- New: `optimus-be/internal/modules/credentials/sshkey/dto.go`
- New: `optimus-be/internal/modules/credentials/sshkey/repo.go`
- New: `optimus-be/internal/modules/credentials/sshkey/repo_test.go`

- [ ] **Step 1: Write DTOs**

Create `optimus-be/internal/modules/credentials/sshkey/dto.go`:

```go
package sshkey

import "time"

// Summary is the list-row shape. Never contains secret material.
type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Username    string    `json:"username"`
	CreatedBy   *Actor    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Detail is the get-by-id shape. Identical to Summary for SSH keys (no extra
// fields — kubeconfig and cloud_key may differ).
type Detail = Summary

// Actor is the populated creator. ID-only when the user is deleted.
type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type CreateRequest struct {
	Name        string `json:"name"        binding:"required,max=128"`
	Description string `json:"description" binding:"max=4096"`
	Username    string `json:"username"    binding:"required,max=64"`
	PrivateKey  string `json:"private_key" binding:"required"`
	Passphrase  string `json:"passphrase"`
}

type UpdateRequest struct {
	Name        *string `json:"name,omitempty"        binding:"omitempty,max=128"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
	Username    *string `json:"username,omitempty"    binding:"omitempty,max=64"`
	PrivateKey  *string `json:"private_key,omitempty"`
	Passphrase  *string `json:"passphrase,omitempty"`
}

type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Q        string `form:"q"`
	Username string `form:"username"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
```

- [ ] **Step 2: Write the repo**

Create `optimus-be/internal/modules/credentials/sshkey/repo.go`:

```go
package sshkey

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, m *models.CredentialSSHKey) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.CredentialSSHKey, error) {
	var m models.CredentialSSHKey
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByName(ctx context.Context, name string) (*models.CredentialSSHKey, error) {
	var m models.CredentialSSHKey
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]models.CredentialSSHKey, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.CredentialSSHKey{})
	if s := strings.TrimSpace(q.Q); s != "" {
		pat := "%" + s + "%"
		tx = tx.Where("name ILIKE ? OR description ILIKE ?", pat, pat)
	}
	if s := strings.TrimSpace(q.Username); s != "" {
		tx = tx.Where("username = ?", s)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	var rows []models.CredentialSSHKey
	if err := tx.Order("id DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) Update(ctx context.Context, m *models.CredentialSSHKey) error {
	// Use Select(*) so zero-value fields (e.g., cleared passphrase) get written.
	// Caller is responsible for not zeroing fields it didn't intend to clear.
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *Repo) UpdateColumns(ctx context.Context, id uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).
		Model(&models.CredentialSSHKey{}).
		Where("id = ?", id).
		Updates(fields).Error
}

func (r *Repo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.CredentialSSHKey{}, id).Error
}
```

- [ ] **Step 3: Write the dockertest**

Create `optimus-be/internal/modules/credentials/sshkey/repo_test.go`:

```go
//go:build dbtest

package sshkey

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"optimus-be/internal/models"
	"optimus-be/internal/testsupport/dbharness"
)

func newRepo(t *testing.T) (*Repo, *gorm.DB) {
	t.Helper()
	db := dbharness.Open(t)
	return NewRepo(db), db
}

func TestRepo_CreateAndGet(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	m := &models.CredentialSSHKey{
		Name:          "prod-bastion",
		Description:   "bastion host key",
		Username:      "ops",
		PrivateKeyEnc: []byte{0x01, 0x02, 0x03},
	}
	if err := r.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("id not set after create")
	}

	got, err := r.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "prod-bastion" || got.Username != "ops" {
		t.Errorf("unexpected row: %+v", got)
	}
}

func TestRepo_NameUnique(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	a := &models.CredentialSSHKey{Name: "dup", Username: "u", PrivateKeyEnc: []byte{1}}
	if err := r.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := &models.CredentialSSHKey{Name: "dup", Username: "u", PrivateKeyEnc: []byte{2}}
	err := r.Create(ctx, b)
	if err == nil {
		t.Fatal("expected unique constraint violation")
	}
}

func TestRepo_ListPagesAndFilters(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		_ = r.Create(ctx, &models.CredentialSSHKey{
			Name:          "k" + string(rune('a'+i)),
			Username:      "ops",
			PrivateKeyEnc: []byte{byte(i)},
		})
	}
	_ = r.Create(ctx, &models.CredentialSSHKey{Name: "special", Username: "deploy", PrivateKeyEnc: []byte{0xff}})

	rows, total, err := r.List(ctx, ListQuery{Page: 1, PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if total != 8 {
		t.Errorf("total=%d want 8", total)
	}
	if len(rows) != 5 {
		t.Errorf("rows=%d want 5", len(rows))
	}

	rows, total, err = r.List(ctx, ListQuery{Page: 1, PageSize: 10, Username: "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Name != "special" {
		t.Errorf("username filter mismatch: total=%d rows=%+v", total, rows)
	}
}

func TestRepo_FindByName_NotFound(t *testing.T) {
	r, _ := newRepo(t)
	_, err := r.FindByName(context.Background(), "missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("got %v, want ErrRecordNotFound", err)
	}
}

func TestRepo_UpdateColumnsAndDelete(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	m := &models.CredentialSSHKey{Name: "k1", Username: "ops", PrivateKeyEnc: []byte{1}}
	_ = r.Create(ctx, m)

	if err := r.UpdateColumns(ctx, m.ID, map[string]any{"description": "x", "username": "deploy"}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, m.ID)
	if got.Description != "x" || got.Username != "deploy" {
		t.Errorf("update failed: %+v", got)
	}

	if err := r.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	_, err := r.Get(ctx, m.ID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("got %v, want ErrRecordNotFound after delete", err)
	}
}
```

> Note: `optimus-be/internal/testsupport/dbharness` is the existing P0 dockertest harness. Verify path with `find optimus-be -name dbharness -o -name dbtest`. If the existing harness is in a different path or has a different name, swap the import accordingly. Other repo tests in the codebase (e.g., `internal/modules/role/repo_test.go`) show the canonical import.

- [ ] **Step 4: Run dockertest**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./internal/modules/credentials/sshkey/ -v -run TestRepo
```

Expected: PASS for all 5 tests.

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/internal/modules/credentials/sshkey/
git commit -m "feat(be/credentials/sshkey): DTOs + repo + dockertest CRUD"
```

---

## Task 7: SSH key — service + tests

**Files:**
- New: `optimus-be/internal/modules/credentials/sshkey/service.go`
- New: `optimus-be/internal/modules/credentials/sshkey/service_test.go`

- [ ] **Step 1: Implement the service**

Create `optimus-be/internal/modules/credentials/sshkey/service.go`:

```go
package sshkey

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/vault"
)

// Cipher is the subset of vault.Cipher the service depends on. Defined as an
// interface so service_test can mock it without spinning up real AES.
type Cipher interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

type Service struct {
	repo   *Repo
	cipher Cipher
	audit  *audit.Recorder
}

func NewService(repo *Repo, cipher Cipher, rec *audit.Recorder) *Service {
	return &Service{repo: repo, cipher: cipher, audit: rec}
}

func (s *Service) Repo() *Repo { return s.repo }

// --- queries ---------------------------------------------------------------

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSummary(r))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{Items: out, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, err
	}
	d := Detail(toSummary(*m))
	return &d, nil
}

// --- mutations -------------------------------------------------------------

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	if err := validatePrivateKey([]byte(req.PrivateKey), req.Passphrase); err != nil {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_key_format", err.Error())
	}

	if _, err := s.repo.FindByName(ctx, req.Name); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	pkEnc, err := s.cipher.Seal([]byte(req.PrivateKey))
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	var passEnc []byte
	if req.Passphrase != "" {
		passEnc, err = s.cipher.Seal([]byte(req.Passphrase))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
	}

	m := &models.CredentialSSHKey{
		Name:          strings.TrimSpace(req.Name),
		Description:   req.Description,
		Username:      req.Username,
		PrivateKeyEnc: pkEnc,
		PassphraseEnc: passEnc,
	}
	if actorID != 0 {
		v := actorID
		m.CreatedByUserID = &v
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}

	s.writeAudit(ctx, &actorID, "credentials.create", m.ID, m.Name, ip, ua, map[string]any{"name": m.Name})

	d := Detail(toSummary(*m))
	return &d, nil
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, err
	}

	fields := map[string]any{}
	var changed []string
	rotated := false

	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n != m.Name {
			// Check uniqueness only when actually changing.
			if _, err := s.repo.FindByName(ctx, n); err == nil {
				return nil, apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			fields["name"] = n
			changed = append(changed, "name")
		}
	}
	if req.Description != nil && *req.Description != m.Description {
		fields["description"] = *req.Description
		changed = append(changed, "description")
	}
	if req.Username != nil && *req.Username != m.Username {
		fields["username"] = *req.Username
		changed = append(changed, "username")
	}
	if req.PrivateKey != nil && *req.PrivateKey != "" {
		// Validate before sealing.
		var pp string
		if req.Passphrase != nil {
			pp = *req.Passphrase
		}
		if err := validatePrivateKey([]byte(*req.PrivateKey), pp); err != nil {
			return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_key_format", err.Error())
		}
		enc, err := s.cipher.Seal([]byte(*req.PrivateKey))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
		fields["private_key_enc"] = enc
		changed = append(changed, "private_key")
		rotated = true
	}
	if req.Passphrase != nil {
		if *req.Passphrase == "" {
			fields["passphrase_enc"] = nil
			changed = append(changed, "passphrase")
		} else {
			enc, err := s.cipher.Seal([]byte(*req.Passphrase))
			if err != nil {
				return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
			}
			fields["passphrase_enc"] = enc
			changed = append(changed, "passphrase")
			rotated = true
		}
	}

	if len(fields) > 0 {
		if err := s.repo.UpdateColumns(ctx, id, fields); err != nil {
			return nil, err
		}
	}

	action := "credentials.update"
	if rotated {
		action = "credentials.rotate"
	}
	if len(changed) > 0 {
		s.writeAudit(ctx, &actorID, action, id, valueOr(fields["name"], m.Name), ip, ua, map[string]any{
			"name":            valueOr(fields["name"], m.Name),
			"changed_fields":  changed,
			"secret_rotated":  rotated,
		})
	}

	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.writeAudit(ctx, &actorID, "credentials.delete", id, m.Name, ip, ua, map[string]any{"name": m.Name})
	return nil
}

// --- consume ---------------------------------------------------------------

// ConsumeRecord is the decrypted shape returned to the consume seam.
type ConsumeRecord struct {
	Name       string
	Username   string
	PrivateKey []byte
	Passphrase []byte
}

func (s *Service) Consume(ctx context.Context, actorID *uint64, id uint64, purpose string) (*ConsumeRecord, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_purpose", "purpose required")
	}
	if actorID == nil && !strings.HasPrefix(purpose, "system:") {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.system_purpose_required", "system caller purpose must start with system:")
	}

	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, err
	}
	pk, err := s.cipher.Open(m.PrivateKeyEnc)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
	}
	var pp []byte
	if len(m.PassphraseEnc) > 0 {
		pp, err = s.cipher.Open(m.PassphraseEnc)
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
		}
	}

	// Audit is best-effort: never fail a consume on audit-write failure.
	s.writeAudit(ctx, actorID, "credentials.consume", id, m.Name, "", "", map[string]any{
		"name":    m.Name,
		"purpose": purpose,
	})

	return &ConsumeRecord{
		Name:       m.Name,
		Username:   m.Username,
		PrivateKey: pk,
		Passphrase: pp,
	}, nil
}

// --- helpers ---------------------------------------------------------------

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, name, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     actor,
		Action:     action,
		TargetType: "credentials.ssh_key",
		TargetID:   strconv.FormatUint(id, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

func toSummary(m models.CredentialSSHKey) Summary {
	out := Summary{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Username:    m.Username,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.CreatedByUserID != nil {
		out.CreatedBy = &Actor{ID: *m.CreatedByUserID}
	}
	return out
}

func valueOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}

// vault is referenced for its sentinel ErrInvalidCiphertext only — keep import.
var _ = vault.ErrInvalidCiphertext

func validatePrivateKey(pem []byte, passphrase string) error {
	if passphrase == "" {
		_, err := ssh.ParseRawPrivateKey(pem)
		if err != nil {
			return err
		}
		return nil
	}
	_, err := ssh.ParseRawPrivateKeyWithPassphrase(pem, []byte(passphrase))
	return err
}
```

> Note on `apperr.CodeBadRequest` / `CodeConflict` / `CodeNotFound` / `CodeInternal`: verify the exact code constants in `internal/infra/errors/codes.go`. If they're named differently in P0 (e.g., `CodeInvalidArgument`), swap accordingly. The pattern is identical to `role/service.go`.

- [ ] **Step 2: Write service unit tests**

Create `optimus-be/internal/modules/credentials/sshkey/service_test.go`:

```go
//go:build dbtest

package sshkey

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"testing"

	"golang.org/x/crypto/ssh"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/testsupport/dbharness"
)

// genTestSSHKey returns an unencrypted ed25519 PEM block valid for ssh.ParseRawPrivateKey.
func genTestSSHKey(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blk, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(blk)
}

type passthroughCipher struct{}

func (passthroughCipher) Seal(b []byte) ([]byte, error) { out := append([]byte("SEAL"), b...); return out, nil }
func (passthroughCipher) Open(b []byte) ([]byte, error) {
	if len(b) < 4 || string(b[:4]) != "SEAL" {
		return nil, errors.New("bad")
	}
	return b[4:], nil
}

func newSvc(t *testing.T) *Service {
	t.Helper()
	db := dbharness.Open(t)
	return NewService(NewRepo(db), passthroughCipher{}, audit.NewRecorder(db))
}

func TestService_Create_RoundTrip(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)

	d, err := s.Create(ctx, 0, "1.2.3.4", "test", CreateRequest{
		Name: "n1", Username: "ops", PrivateKey: string(key),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == 0 || d.Name != "n1" {
		t.Errorf("bad detail: %+v", d)
	}
}

func TestService_Create_InvalidKey(t *testing.T) {
	s := newSvc(t)
	_, err := s.Create(context.Background(), 0, "", "", CreateRequest{
		Name: "n2", Username: "ops", PrivateKey: "not-pem",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.MessageKey != "credentials.invalid_key_format" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestService_Create_NameTaken(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)
	_, _ = s.Create(ctx, 0, "", "", CreateRequest{Name: "dup", Username: "u", PrivateKey: string(key)})
	_, err := s.Create(ctx, 0, "", "", CreateRequest{Name: "dup", Username: "u", PrivateKey: string(key)})
	if err == nil {
		t.Fatal("expected name_taken")
	}
}

func TestService_Update_OnlyChangedFields(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, _ := s.Create(ctx, 0, "", "", CreateRequest{Name: "u1", Username: "ops", PrivateKey: string(key)})

	desc := "new desc"
	_, err := s.Update(ctx, 0, "", "", d.ID, UpdateRequest{Description: &desc})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(ctx, d.ID)
	if got.Description != "new desc" || got.Username != "ops" {
		t.Errorf("update mismatch: %+v", got)
	}
}

func TestService_Update_RotateSecret(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key1 := genTestSSHKey(t)
	d, _ := s.Create(ctx, 0, "", "", CreateRequest{Name: "rot", Username: "ops", PrivateKey: string(key1)})

	key2 := string(genTestSSHKey(t))
	_, err := s.Update(ctx, 0, "", "", d.ID, UpdateRequest{PrivateKey: &key2})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	rec, err := s.Consume(context.Background(), ptrUint64(7), d.ID, "test.rotate")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if string(rec.PrivateKey) != key2 {
		t.Errorf("rotation did not persist new key")
	}
}

func TestService_Delete(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, _ := s.Create(ctx, 0, "", "", CreateRequest{Name: "del", Username: "ops", PrivateKey: string(key)})
	if err := s.Delete(ctx, 0, "", "", d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(ctx, d.ID)
	if err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestService_Consume_EmptyPurpose(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, _ := s.Create(ctx, 0, "", "", CreateRequest{Name: "cp", Username: "ops", PrivateKey: string(key)})
	_, err := s.Consume(ctx, ptrUint64(1), d.ID, "")
	if err == nil {
		t.Fatal("expected invalid_purpose")
	}
}

func TestService_Consume_SystemRequiresPrefix(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, _ := s.Create(ctx, 0, "", "", CreateRequest{Name: "sp", Username: "ops", PrivateKey: string(key)})
	_, err := s.Consume(ctx, nil, d.ID, "not-a-system-purpose")
	if err == nil {
		t.Fatal("expected system_purpose_required")
	}
}

func TestService_Consume_SystemAccepted(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, _ := s.Create(ctx, 0, "", "", CreateRequest{Name: "sa", Username: "ops", PrivateKey: string(key)})
	_, err := s.Consume(ctx, nil, d.ID, "system:cron.sync")
	if err != nil {
		t.Errorf("system caller rejected: %v", err)
	}
}

func ptrUint64(v uint64) *uint64 { return &v }
```

- [ ] **Step 3: Add `golang.org/x/crypto` as direct dep if needed**

```
cd optimus-be && go mod tidy
grep "golang.org/x/crypto" go.mod
```

If not present as a direct require, add it:

```
go get golang.org/x/crypto@latest
```

- [ ] **Step 4: Run service tests**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./internal/modules/credentials/sshkey/ -v -run TestService
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/internal/modules/credentials/sshkey/service.go optimus-be/internal/modules/credentials/sshkey/service_test.go optimus-be/go.mod optimus-be/go.sum
git commit -m "feat(be/credentials/sshkey): service + audit + consume + tests"
```

---

## Task 8: SSH key — handler + routes + tests

**Files:**
- New: `optimus-be/internal/modules/credentials/sshkey/handler.go`
- New: `optimus-be/internal/modules/credentials/sshkey/handler_test.go`

- [ ] **Step 1: Implement the handler**

Create `optimus-be/internal/modules/credentials/sshkey/handler.go`:

```go
package sshkey

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) HandleList() gin.HandlerFunc {
	return func(c *gin.Context) {
		var q ListQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			httpx.Error(c, http.StatusBadRequest, "common.bad_request", err)
			return
		}
		resp, err := h.svc.List(c.Request.Context(), q)
		if err != nil {
			httpx.WriteAppErr(c, err)
			return
		}
		httpx.OK(c, resp)
	}
}

func (h *Handler) HandleGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "common.bad_request", err)
			return
		}
		d, err := h.svc.Get(c.Request.Context(), id)
		if err != nil {
			httpx.WriteAppErr(c, err)
			return
		}
		httpx.OK(c, d)
	}
}

func (h *Handler) HandleCreate() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, http.StatusBadRequest, "common.bad_request", err)
			return
		}
		actor := httpx.ActorID(c)
		d, err := h.svc.Create(c.Request.Context(), actor, c.ClientIP(), c.Request.UserAgent(), req)
		if err != nil {
			httpx.WriteAppErr(c, err)
			return
		}
		c.JSON(http.StatusCreated, httpx.Envelope(d))
	}
}

func (h *Handler) HandleUpdate() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "common.bad_request", err)
			return
		}
		var req UpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Error(c, http.StatusBadRequest, "common.bad_request", err)
			return
		}
		actor := httpx.ActorID(c)
		d, err := h.svc.Update(c.Request.Context(), actor, c.ClientIP(), c.Request.UserAgent(), id, req)
		if err != nil {
			httpx.WriteAppErr(c, err)
			return
		}
		httpx.OK(c, d)
	}
}

func (h *Handler) HandleDelete() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			httpx.Error(c, http.StatusBadRequest, "common.bad_request", err)
			return
		}
		actor := httpx.ActorID(c)
		if err := h.svc.Delete(c.Request.Context(), actor, c.ClientIP(), c.Request.UserAgent(), id); err != nil {
			httpx.WriteAppErr(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
```

> Note: `httpx.Envelope`, `httpx.OK`, `httpx.Error`, `httpx.WriteAppErr`, `httpx.ActorID` are P0 helpers. Verify exact names in `internal/infra/httpx/` (or wherever P0 puts response helpers). Other handlers like `internal/modules/role/handler.go` show the canonical usage — match what's already there if names differ.

- [ ] **Step 2: Write handler dockertest**

Create `optimus-be/internal/modules/credentials/sshkey/handler_test.go`:

```go
//go:build dbtest

package sshkey

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"

	"optimus-be/internal/modules/audit"
	"optimus-be/internal/testsupport/dbharness"
)

func genPEM(t *testing.T) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	blk, _ := ssh.MarshalPrivateKey(priv, "")
	return string(pem.EncodeToMemory(blk))
}

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db := dbharness.Open(t)
	svc := NewService(NewRepo(db), passthroughCipher{}, audit.NewRecorder(db))
	h := NewHandler(svc)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/credentials/ssh-keys")
	g.GET("", h.HandleList())
	g.POST("", h.HandleCreate())
	g.GET("/:id", h.HandleGet())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	return r
}

func doJSON(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_CreateGetListDelete(t *testing.T) {
	r := newRouter(t)
	pem := genPEM(t)

	// Create
	w := doJSON(r, "POST", "/api/v1/credentials/ssh-keys", CreateRequest{
		Name: "h1", Username: "ops", PrivateKey: pem,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create code=%d body=%s", w.Code, w.Body.String())
	}

	// Secret must not be in response.
	if bytes.Contains(w.Body.Bytes(), []byte(pem)) {
		t.Error("response leaks plaintext private key")
	}

	var env struct{ Data Summary }
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	id := env.Data.ID
	if id == 0 {
		t.Fatal("id missing")
	}

	// List
	w = doJSON(r, "GET", "/api/v1/credentials/ssh-keys?page=1&page_size=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list code=%d body=%s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte(pem)) {
		t.Error("list response leaks plaintext")
	}

	// Get
	w = doJSON(r, "GET", "/api/v1/credentials/ssh-keys/"+jsonid(id), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get code=%d", w.Code)
	}

	// Delete
	w = doJSON(r, "DELETE", "/api/v1/credentials/ssh-keys/"+jsonid(id), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete code=%d body=%s", w.Code, w.Body.String())
	}

	// Get after delete → 404
	w = doJSON(r, "GET", "/api/v1/credentials/ssh-keys/"+jsonid(id), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}

	// Audit row survives the delete.
	db := dbharness.Open(t)
	var cnt int64
	db.Raw(`SELECT COUNT(*) FROM audit_logs WHERE action = 'credentials.delete' AND target_type = 'credentials.ssh_key'`).Scan(&cnt)
	if cnt < 1 {
		t.Error("expected at least one delete audit row")
	}
	_ = context.Background()
}

func TestHandler_Validation_RejectsBadKey(t *testing.T) {
	r := newRouter(t)
	w := doJSON(r, "POST", "/api/v1/credentials/ssh-keys", CreateRequest{
		Name: "bad", Username: "u", PrivateKey: "not-pem",
	})
	if w.Code == http.StatusCreated || w.Code == http.StatusOK {
		t.Fatalf("expected error, got %d", w.Code)
	}
}

func jsonid(id uint64) string {
	return strconv.FormatUint(id, 10)
}
```

> Add `"strconv"` to the imports.

- [ ] **Step 3: Run handler tests**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./internal/modules/credentials/sshkey/ -v -run TestHandler
```

Expected: PASS.

- [ ] **Step 4: Commit**

```
cd .. && git add optimus-be/internal/modules/credentials/sshkey/handler.go optimus-be/internal/modules/credentials/sshkey/handler_test.go
git commit -m "feat(be/credentials/sshkey): HTTP handler + dockertest"
```

---

## Task 9: Kubeconfig — full vertical

**Files:**
- New: `optimus-be/migrations/00013_create_credentials_kubeconfigs.sql`
- New: `optimus-be/internal/models/credential_kubeconfig.go`
- New: `optimus-be/internal/modules/credentials/kubeconfig/{dto,repo,repo_test,service,service_test,handler,handler_test}.go`

- [ ] **Step 1: Migration**

Create `optimus-be/migrations/00013_create_credentials_kubeconfigs.sql`:

```sql
-- +goose Up
CREATE TABLE credentials_kubeconfigs (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(128) NOT NULL UNIQUE,
    description        TEXT         NOT NULL DEFAULT '',
    default_namespace  VARCHAR(64)  NOT NULL DEFAULT '',
    kubeconfig_enc     BYTEA        NOT NULL,
    created_by_user_id BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credentials_kubeconfigs_created_by ON credentials_kubeconfigs(created_by_user_id);

-- +goose Down
DROP TABLE credentials_kubeconfigs;
```

- [ ] **Step 2: Model**

Create `optimus-be/internal/models/credential_kubeconfig.go`:

```go
package models

import "time"

type CredentialKubeconfig struct {
	ID               uint64    `gorm:"primaryKey"`
	Name             string    `gorm:"size:128;not null;uniqueIndex"`
	Description      string    `gorm:"type:text;not null;default:''"`
	DefaultNamespace string    `gorm:"column:default_namespace;size:64;not null;default:''"`
	KubeconfigEnc    []byte    `gorm:"column:kubeconfig_enc;not null"`
	CreatedByUserID  *uint64   `gorm:"column:created_by_user_id"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CredentialKubeconfig) TableName() string { return "credentials_kubeconfigs" }
```

- [ ] **Step 3: DTO + repo + service + handler**

Create the seven files under `optimus-be/internal/modules/credentials/kubeconfig/`. They mirror the sshkey package with these deltas:

**dto.go** — same shape as sshkey/dto.go but replace fields:

```go
package kubeconfig

import "time"

type Summary struct {
	ID               uint64    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	DefaultNamespace string    `json:"default_namespace"`
	CreatedBy        *Actor    `json:"created_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Detail = Summary
type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}
type CreateRequest struct {
	Name             string `json:"name"              binding:"required,max=128"`
	Description      string `json:"description"       binding:"max=4096"`
	DefaultNamespace string `json:"default_namespace" binding:"max=64"`
	Kubeconfig       string `json:"kubeconfig"        binding:"required"`
}
type UpdateRequest struct {
	Name             *string `json:"name,omitempty"              binding:"omitempty,max=128"`
	Description      *string `json:"description,omitempty"       binding:"omitempty,max=4096"`
	DefaultNamespace *string `json:"default_namespace,omitempty" binding:"omitempty,max=64"`
	Kubeconfig       *string `json:"kubeconfig,omitempty"`
}
type ListQuery struct {
	Page             int    `form:"page,default=1"`
	PageSize         int    `form:"page_size,default=20"`
	Q                string `form:"q"`
	DefaultNamespace string `form:"default_namespace"`
}
type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
```

**repo.go** — copy `sshkey/repo.go`, replace type `CredentialSSHKey` with `CredentialKubeconfig`, replace `username = ?` filter with `default_namespace = ?`. The Create/Get/FindByName/Update/Delete bodies are identical.

**repo_test.go** — copy `sshkey/repo_test.go`, replace `CredentialSSHKey` with `CredentialKubeconfig`, replace test field assignments accordingly (e.g., `KubeconfigEnc: []byte{0x01}` instead of `PrivateKeyEnc`), and replace the username filter test with a `DefaultNamespace` filter test.

**service.go** — copy `sshkey/service.go` with these changes:

1. Replace package `sshkey` → `kubeconfig`.
2. Replace `CredentialSSHKey` → `CredentialKubeconfig` everywhere.
3. Replace `private_key_enc` column key with `kubeconfig_enc`.
4. Replace `validatePrivateKey` with `validateKubeconfig`:

```go
import "k8s.io/client-go/tools/clientcmd"

func validateKubeconfig(raw []byte) error {
	cfg, err := clientcmd.Load(raw)
	if err != nil {
		return err
	}
	if len(cfg.Contexts) == 0 {
		return errors.New("kubeconfig has no contexts")
	}
	return nil
}
```

5. The Create / Update payloads use `Kubeconfig` (not `PrivateKey`) and a single `kubeconfig_enc` column (no passphrase column).
6. `ConsumeRecord` becomes:

```go
type ConsumeRecord struct {
	Name             string
	DefaultNamespace string
	YAML             []byte
}
```

7. `Consume` body decrypts `KubeconfigEnc` into the `YAML` field.
8. Audit `TargetType` is `"credentials.kubeconfig"`.

**service_test.go** — copy `sshkey/service_test.go`. Replace `genTestSSHKey` with `genTestKubeconfig`:

```go
func genTestKubeconfig(t *testing.T) string {
	return `apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c1
  cluster: {server: https://127.0.0.1:6443, insecure-skip-tls-verify: true}
contexts:
- name: ctx
  context: {cluster: c1, user: u1, namespace: default}
users:
- name: u1
  user: {token: abc}
`
}
```

Update all test calls accordingly.

**handler.go** — copy `sshkey/handler.go`. Only the package name and the bound `Service`/`Repo` types change; the endpoint shapes are identical.

**handler_test.go** — copy `sshkey/handler_test.go`. Update package, type names, and replace `genPEM`/`pem` with `genTestKubeconfig`.

- [ ] **Step 4: Add k8s.io/client-go dependency**

```
cd optimus-be && go get k8s.io/client-go@latest && go mod tidy
```

This is large (pulls k8s.io/api, k8s.io/apimachinery, etc.). Confirm `go build ./...` still succeeds.

- [ ] **Step 5: Run all kubeconfig tests**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./internal/modules/credentials/kubeconfig/ -v
```

Expected: PASS for all repo, service, and handler tests.

- [ ] **Step 6: Commit**

```
cd .. && git add optimus-be/migrations/00013_create_credentials_kubeconfigs.sql optimus-be/internal/models/credential_kubeconfig.go optimus-be/internal/modules/credentials/kubeconfig/ optimus-be/go.mod optimus-be/go.sum
git commit -m "feat(be/credentials/kubeconfig): full CRUD vertical + validation"
```

---

## Task 10: Cloud key — full vertical

**Files:**
- New: `optimus-be/migrations/00014_create_credentials_cloud_keys.sql`
- New: `optimus-be/internal/models/credential_cloud_key.go`
- New: `optimus-be/internal/modules/credentials/cloudkey/{dto,repo,repo_test,service,service_test,handler,handler_test}.go`

- [ ] **Step 1: Migration**

Create `optimus-be/migrations/00014_create_credentials_cloud_keys.sql`:

```sql
-- +goose Up
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
CREATE INDEX idx_credentials_cloud_keys_provider ON credentials_cloud_keys(provider);
CREATE INDEX idx_credentials_cloud_keys_created_by ON credentials_cloud_keys(created_by_user_id);

-- +goose Down
DROP TABLE credentials_cloud_keys;
```

- [ ] **Step 2: Model**

Create `optimus-be/internal/models/credential_cloud_key.go`:

```go
package models

import "time"

type CredentialCloudKey struct {
	ID                  uint64    `gorm:"primaryKey"`
	Name                string    `gorm:"size:128;not null;uniqueIndex"`
	Description         string    `gorm:"type:text;not null;default:''"`
	Provider            string    `gorm:"size:16;not null"`
	Region              string    `gorm:"size:32;not null;default:''"`
	AccessKeyIDEnc      []byte    `gorm:"column:access_key_id_enc;not null"`
	SecretAccessKeyEnc  []byte    `gorm:"column:secret_access_key_enc;not null"`
	CreatedByUserID     *uint64   `gorm:"column:created_by_user_id"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (CredentialCloudKey) TableName() string { return "credentials_cloud_keys" }
```

- [ ] **Step 3: DTO + repo + service + handler**

Apply the same pattern as kubeconfig. Key deltas:

**dto.go**:

```go
package cloudkey

import "time"

type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Provider    string    `json:"provider"`
	Region      string    `json:"region"`
	CreatedBy   *Actor    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Detail = Summary
type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}
type CreateRequest struct {
	Name            string `json:"name"              binding:"required,max=128"`
	Description     string `json:"description"       binding:"max=4096"`
	Provider        string `json:"provider"          binding:"required,oneof=aws gcp azure"`
	Region          string `json:"region"            binding:"max=32"`
	AccessKeyID     string `json:"access_key_id"     binding:"required,max=256"`
	SecretAccessKey string `json:"secret_access_key" binding:"required"`
}
type UpdateRequest struct {
	Name            *string `json:"name,omitempty"              binding:"omitempty,max=128"`
	Description     *string `json:"description,omitempty"       binding:"omitempty,max=4096"`
	Provider        *string `json:"provider,omitempty"          binding:"omitempty,oneof=aws gcp azure"`
	Region          *string `json:"region,omitempty"            binding:"omitempty,max=32"`
	AccessKeyID     *string `json:"access_key_id,omitempty"`
	SecretAccessKey *string `json:"secret_access_key,omitempty"`
}
type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Q        string `form:"q"`
	Provider string `form:"provider"`
}
type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
```

**repo.go** — like sshkey but for `CredentialCloudKey`. The List filter is `provider = ?` (instead of username/default_namespace).

**service.go** — like sshkey, with these changes:

1. Two encrypted fields instead of one: both `AccessKeyID` and `SecretAccessKey` get sealed on Create/Update.
2. No private-key validation — just confirm `Provider` is in the enum (validator tag handles this). Drop the `validatePrivateKey` import.
3. `ConsumeRecord`:

```go
type ConsumeRecord struct {
	Name            string
	Provider        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}
```

4. Audit `TargetType` is `"credentials.cloud_key"`.

**service_test.go** — same shape as sshkey/service_test.go but without PEM generation; use plain strings:

```go
func newCreateReq() CreateRequest {
	return CreateRequest{
		Name: "k", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secretvalue123",
	}
}
```

Drop the SSH-specific "invalid key format" test; replace with "invalid provider" (e.g., `Provider: "ibm"` → 400). Keep all other test cases.

**handler.go / handler_test.go** — same structure as sshkey, with the new DTOs.

- [ ] **Step 4: Run all cloud key tests**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./internal/modules/credentials/cloudkey/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/migrations/00014_create_credentials_cloud_keys.sql optimus-be/internal/models/credential_cloud_key.go optimus-be/internal/modules/credentials/cloudkey/
git commit -m "feat(be/credentials/cloudkey): full CRUD vertical for aws/gcp/azure keys"
```

---

## Task 11: Consumer interface + smoke test

**Files:**
- New: `optimus-be/internal/modules/credentials/consume.go`
- New: `optimus-be/internal/modules/credentials/consume_smoke_test.go`

- [ ] **Step 1: Implement the Consumer**

Create `optimus-be/internal/modules/credentials/consume.go`:

```go
// Package credentials is the entry point for downstream sub-projects (P2/P4/P5/P6)
// that need to read decrypted credential material. The exported Consumer interface
// is the SOLE public API — downstream callers must not import the sshkey /
// kubeconfig / cloudkey sub-packages directly.
//
// Permission semantics: this seam does NOT enforce credentials:*:use. Downstream
// packages enforce their own feature-specific RBAC (e.g., k8s:exec:write); the
// :use code is registered for role-management visibility but P1 itself does not
// gate calls on it.
package credentials

import (
	"context"

	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/credentials/kubeconfig"
	"optimus-be/internal/modules/credentials/sshkey"
)

// SSHKey is the decrypted shape returned by Consumer.GetSSHKey.
type SSHKey struct {
	Name       string
	Username   string
	PrivateKey []byte
	Passphrase []byte // nil if not set
}

// Kubeconfig is the decrypted shape returned by Consumer.GetKubeconfig.
type Kubeconfig struct {
	Name             string
	DefaultNamespace string
	YAML             []byte
}

// CloudKey is the decrypted shape returned by Consumer.GetCloudKey.
type CloudKey struct {
	Name            string
	Provider        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

// Consumer is the seam used by downstream sub-projects. `purpose` is a free-form
// caller-supplied string recorded in audit; for system callers (ctx has no user_id),
// it must start with "system:".
type Consumer interface {
	GetSSHKey(ctx context.Context, id uint64, purpose string) (*SSHKey, error)
	GetKubeconfig(ctx context.Context, id uint64, purpose string) (*Kubeconfig, error)
	GetCloudKey(ctx context.Context, id uint64, purpose string) (*CloudKey, error)
}

// NewConsumer wires a Consumer over the three feature services. Callers obtain
// services from credentials.Module().
func NewConsumer(ssh *sshkey.Service, kc *kubeconfig.Service, ck *cloudkey.Service) Consumer {
	return &consumer{ssh: ssh, kc: kc, ck: ck}
}

type consumer struct {
	ssh *sshkey.Service
	kc  *kubeconfig.Service
	ck  *cloudkey.Service
}

func (c *consumer) GetSSHKey(ctx context.Context, id uint64, purpose string) (*SSHKey, error) {
	actor := actorFromCtx(ctx)
	rec, err := c.ssh.Consume(ctx, actor, id, purpose)
	if err != nil {
		return nil, err
	}
	return &SSHKey{
		Name:       rec.Name,
		Username:   rec.Username,
		PrivateKey: rec.PrivateKey,
		Passphrase: rec.Passphrase,
	}, nil
}

func (c *consumer) GetKubeconfig(ctx context.Context, id uint64, purpose string) (*Kubeconfig, error) {
	actor := actorFromCtx(ctx)
	rec, err := c.kc.Consume(ctx, actor, id, purpose)
	if err != nil {
		return nil, err
	}
	return &Kubeconfig{
		Name:             rec.Name,
		DefaultNamespace: rec.DefaultNamespace,
		YAML:             rec.YAML,
	}, nil
}

func (c *consumer) GetCloudKey(ctx context.Context, id uint64, purpose string) (*CloudKey, error) {
	actor := actorFromCtx(ctx)
	rec, err := c.ck.Consume(ctx, actor, id, purpose)
	if err != nil {
		return nil, err
	}
	return &CloudKey{
		Name:            rec.Name,
		Provider:        rec.Provider,
		Region:          rec.Region,
		AccessKeyID:     rec.AccessKeyID,
		SecretAccessKey: rec.SecretAccessKey,
	}, nil
}

// actorFromCtx extracts the actor user_id from the context if present.
// Returns nil for system callers.
func actorFromCtx(ctx context.Context) *uint64 {
	v := ctx.Value(contextKeyUserID)
	if v == nil {
		return nil
	}
	if id, ok := v.(uint64); ok {
		return &id
	}
	return nil
}

// contextKeyUserID is the key set by P0's auth middleware. Verify the actual
// key constant used in P0 (search: grep -rn 'user_id' internal/infra/middleware/).
// If P0 exports a typed key (e.g., middleware.CtxKeyUserID), reference it directly.
const contextKeyUserID = "user_id"
```

> **Verify before committing:** `grep -rn 'user_id\|UserID\|ActorID' optimus-be/internal/infra/middleware/`. If P0 uses a `middleware.CtxKey` type (e.g., a sentinel struct), replace the string key here with that exported symbol. Other Service.Update / Service.Delete callers (in `cmd/server/main.go` handlers) already read this key — match what they do.

- [ ] **Step 2: Smoke test**

Create `optimus-be/internal/modules/credentials/consume_smoke_test.go`:

```go
//go:build dbtest

package credentials

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"

	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/credentials/kubeconfig"
	"optimus-be/internal/modules/credentials/sshkey"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/testsupport/dbharness"
)

func setupConsumer(t *testing.T) Consumer {
	t.Helper()
	db := dbharness.Open(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := vault.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	rec := audit.NewRecorder(db)
	ssvc := sshkey.NewService(sshkey.NewRepo(db), cipher, rec)
	ksvc := kubeconfig.NewService(kubeconfig.NewRepo(db), cipher, rec)
	csvc := cloudkey.NewService(cloudkey.NewRepo(db), cipher, rec)
	return NewConsumer(ssvc, ksvc, csvc)
}

func TestSmoke_SSHRoundTrip(t *testing.T) {
	c := setupConsumer(t)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	blk, _ := ssh.MarshalPrivateKey(priv, "")
	pemStr := string(pem.EncodeToMemory(blk))

	// Create via direct service (smoke test only — not via HTTP).
	db := dbharness.Open(t)
	cipher, _ := vault.NewCipher(make([]byte, 32))
	ssvc := sshkey.NewService(sshkey.NewRepo(db), cipher, audit.NewRecorder(db))
	d, err := ssvc.Create(context.Background(), 1, "", "", sshkey.CreateRequest{
		Name: "smoke-ssh", Username: "ops", PrivateKey: pemStr,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wire a new consumer that shares the SAME service (and thus the same db).
	c2 := NewConsumer(ssvc,
		kubeconfig.NewService(kubeconfig.NewRepo(db), cipher, audit.NewRecorder(db)),
		cloudkey.NewService(cloudkey.NewRepo(db), cipher, audit.NewRecorder(db)),
	)

	got, err := c2.GetSSHKey(context.Background(), d.ID, "system:smoke.test")
	if err != nil {
		t.Fatalf("GetSSHKey: %v", err)
	}
	if got.Name != "smoke-ssh" || got.Username != "ops" {
		t.Errorf("bad consume: %+v", got)
	}
	if string(got.PrivateKey) != pemStr {
		t.Error("decrypted plaintext mismatch")
	}

	_ = c // satisfy unused
}
```

> The smoke test's two-NewConsumer dance avoids cross-test state and proves the wire-up. Trim the duplication if the dbharness allows shared instances per-test-process.

- [ ] **Step 3: Run smoke**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./internal/modules/credentials/ -v -run TestSmoke
```

Expected: PASS.

- [ ] **Step 4: Commit**

```
cd .. && git add optimus-be/internal/modules/credentials/consume.go optimus-be/internal/modules/credentials/consume_smoke_test.go
git commit -m "feat(be/credentials): Consumer seam + cross-package smoke test"
```

---

## Task 12: Module wiring + server main integration

**Files:**
- New: `optimus-be/internal/modules/credentials/module.go`
- Modify: `optimus-be/cmd/server/main.go`

- [ ] **Step 1: Implement module.go**

Create `optimus-be/internal/modules/credentials/module.go`:

```go
package credentials

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/credentials/kubeconfig"
	"optimus-be/internal/modules/credentials/sshkey"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/rbac"
)

// Module bundles the three feature services and exposes a Consumer for
// downstream sub-projects.
type Module struct {
	SSH         *sshkey.Service
	Kubeconfig  *kubeconfig.Service
	CloudKey    *cloudkey.Service
	Consumer    Consumer
	sshHandler  *sshkey.Handler
	kcHandler   *kubeconfig.Handler
	ckHandler   *cloudkey.Handler
}

// New constructs the module. cipher must be a real *vault.Cipher (or test fake).
func New(db *gorm.DB, cipher *vault.Cipher, rec *audit.Recorder) *Module {
	ssvc := sshkey.NewService(sshkey.NewRepo(db), cipher, rec)
	ksvc := kubeconfig.NewService(kubeconfig.NewRepo(db), cipher, rec)
	csvc := cloudkey.NewService(cloudkey.NewRepo(db), cipher, rec)
	return &Module{
		SSH:        ssvc,
		Kubeconfig: ksvc,
		CloudKey:   csvc,
		Consumer:   NewConsumer(ssvc, ksvc, csvc),
		sshHandler: sshkey.NewHandler(ssvc),
		kcHandler:  kubeconfig.NewHandler(ksvc),
		ckHandler:  cloudkey.NewHandler(csvc),
	}
}

// MountRoutes wires all three CRUD surfaces under /credentials, with per-route
// RBAC gates per spec §5.1. Call from cmd/server/main.go inside the protected
// router group.
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	mountOne := func(path, resource string, h interface {
		HandleList() gin.HandlerFunc
		HandleGet() gin.HandlerFunc
		HandleCreate() gin.HandlerFunc
		HandleUpdate() gin.HandlerFunc
		HandleDelete() gin.HandlerFunc
	}) {
		g := protected.Group("/credentials/" + path)
		rd := g.Group("", middleware.RequirePermission(cache, "credentials:"+resource+":read"))
		rd.GET("", h.HandleList())
		rd.GET("/:id", h.HandleGet())

		wr := g.Group("", middleware.RequirePermission(cache, "credentials:"+resource+":write"))
		wr.POST("", h.HandleCreate())
		wr.PUT("/:id", h.HandleUpdate())

		del := g.Group("", middleware.RequirePermission(cache, "credentials:"+resource+":delete"))
		del.DELETE("/:id", h.HandleDelete())
	}
	mountOne("ssh-keys", "ssh_key", m.sshHandler)
	mountOne("kubeconfigs", "kubeconfig", m.kcHandler)
	mountOne("cloud-keys", "cloud_key", m.ckHandler)
}
```

- [ ] **Step 2: Modify cmd/server/main.go**

In `optimus-be/cmd/server/main.go`, add this import:

```go
"optimus-be/internal/modules/credentials"
"optimus-be/internal/modules/credentials/vault"
```

After the existing audit-recorder construction (`auditRec := audit.NewRecorder(gdb)` around line 106), add cipher construction:

```go
masterKey, err := vault.LoadKey(vault.Source{
	Env:  cfg.Vault.MasterKey,
	File: cfg.Vault.MasterKeyFile,
})
if err != nil {
	fail("load vault master key", err)
}
cipher, err := vault.NewCipher(masterKey)
if err != nil {
	fail("build vault cipher", err)
}
credsModule := credentials.New(gdb, cipher, auditRec)
```

Then, before the `srv := &http.Server{...}` block (after the existing `mountAuditRoutes` call), add:

```go
credsModule.MountRoutes(protected, permCache)
```

- [ ] **Step 3: Verify the server builds**

```
cd optimus-be && go build ./cmd/server
```

Expected: no errors.

- [ ] **Step 4: Manual smoke**

Start postgres separately (or via existing dev compose), set the env var, run:

```
cd optimus-be && OPTIMUS_VAULT_MASTER_KEY=$(go run ./cmd/vault-keygen) go run ./cmd/server -config configs/config.yaml
```

Expected: server starts without "vault: master key not configured" error. Visit `http://localhost:8080/api/v1/credentials/ssh-keys` — should return 401 (unauthenticated).

Kill it.

Also test fail-fast:

```
cd optimus-be && unset OPTIMUS_VAULT_MASTER_KEY && go run ./cmd/server -config configs/config.yaml
```

Expected: exits with "vault: master key not configured (set OPTIMUS_VAULT_MASTER_KEY or OPTIMUS_VAULT_MASTER_KEY_FILE)".

- [ ] **Step 5: Commit**

```
cd .. && git add optimus-be/internal/modules/credentials/module.go optimus-be/cmd/server/main.go
git commit -m "feat(be): wire credentials module into server with master-key bootstrap"
```

---

## Task 13: Seed credentials menus

**Files:**
- Modify: `optimus-be/internal/seed/seed.go`
- Modify: `optimus-be/internal/seed/seed_test.go`

- [ ] **Step 1: Append the menu tree**

In `optimus-be/internal/seed/seed.go`, find the `tree` variable in `ensureInitialMenus` and append a new top-level entry after the `system` block:

```go
{Code: "credentials", Name: "menu.credentials_group", Path: "/credentials", Component: "", Icon: "key", Children: []spec{
	{Code: "credentials.ssh_keys", Name: "menu.credentials.ssh_keys", Path: "/credentials/ssh-keys", Component: "credentials/ssh-keys/List", PermissionCode: sp("credentials:ssh_key:read")},
	{Code: "credentials.kubeconfigs", Name: "menu.credentials.kubeconfigs", Path: "/credentials/kubeconfigs", Component: "credentials/kubeconfigs/List", PermissionCode: sp("credentials:kubeconfig:read")},
	{Code: "credentials.cloud_keys", Name: "menu.credentials.cloud_keys", Path: "/credentials/cloud-keys", Component: "credentials/cloud-keys/List", PermissionCode: sp("credentials:cloud_key:read")},
}},
```

- [ ] **Step 2: Update seed_test.go**

In `optimus-be/internal/seed/seed_test.go`, find the existing menu-seeding assertion. If it just checks for a known menu count, update the expected count to include the 4 new menus (parent + 3 children = 4). If it spot-checks specific codes, add assertions for the new codes:

```go
expectedCodes := []string{
	"dashboard", "system", "system.users", "system.roles",
	"system.permissions", "system.menus", "system.audit_logs",
	"credentials", "credentials.ssh_keys", "credentials.kubeconfigs", "credentials.cloud_keys",
}
```

Adapt to whatever the existing test does.

- [ ] **Step 3: Run seed test**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test ./internal/seed/ -v -tags=dbtest
```

(If the existing seed test isn't `dbtest`-tagged, drop the `-tags=dbtest`.)

Expected: PASS.

- [ ] **Step 4: Commit**

```
cd .. && git add optimus-be/internal/seed/seed.go optimus-be/internal/seed/seed_test.go
git commit -m "feat(be/seed): seed credentials menu tree (1 parent + 3 children)"
```

---

## Task 14: FE locale additions

**Files:**
- Modify: `optimus-fe/src/locales/zh-CN.json`
- Modify: `optimus-fe/src/locales/en-US.json`

- [ ] **Step 1: Add zh-CN keys**

In `optimus-fe/src/locales/zh-CN.json`, locate the `menu.system_group` line and add after the `system` sub-object inside `menu`:

```json
"credentials_group": "凭证管理",
"credentials": {
  "ssh_keys": "SSH 密钥",
  "kubeconfigs": "Kubeconfig",
  "cloud_keys": "云密钥"
}
```

In the `perm` object, locate the `system` sub-object and add a sibling:

```json
"credentials": {
  "ssh_key":    { "read": "查看 SSH 凭证", "write": "新建/修改 SSH 凭证", "delete": "删除 SSH 凭证", "use": "使用 SSH 凭证" },
  "kubeconfig": { "read": "查看 Kubeconfig", "write": "新建/修改 Kubeconfig", "delete": "删除 Kubeconfig", "use": "使用 Kubeconfig" },
  "cloud_key":  { "read": "查看云密钥", "write": "新建/修改云密钥", "delete": "删除云密钥", "use": "使用云密钥" }
},
```

And inside `perm.category`, add:

```json
"credentials": "凭证管理",
```

Finally, at the top level of the JSON (sibling to `menu`, `perm`, etc.), add:

```json
"credentials": {
  "search_placeholder": "搜索名称或描述",
  "filter_username": "用户名",
  "filter_namespace": "默认命名空间",
  "filter_provider": "云厂商",
  "filter_provider_all": "全部",
  "field": {
    "name": "名称",
    "description": "描述",
    "username": "用户名",
    "private_key": "私钥",
    "passphrase": "口令",
    "default_namespace": "默认命名空间",
    "kubeconfig": "Kubeconfig YAML",
    "provider": "云厂商",
    "region": "区域",
    "access_key_id": "Access Key ID",
    "secret_access_key": "Secret Access Key"
  },
  "placeholder": {
    "unchanged": "•••••（保持不变）"
  },
  "action": {
    "create": "新建",
    "edit": "编辑",
    "delete": "删除",
    "confirm_delete": "确认删除该凭证？该操作不可撤销。"
  },
  "toast": {
    "created": "凭证已创建",
    "updated": "凭证已更新",
    "deleted": "凭证已删除"
  },
  "ssh_keys": {
    "title": "SSH 密钥",
    "col_username": "用户名"
  },
  "kubeconfigs": {
    "title": "Kubeconfig",
    "col_default_namespace": "默认命名空间"
  },
  "cloud_keys": {
    "title": "云密钥",
    "col_provider": "云厂商",
    "col_region": "区域"
  },
  "not_found": "凭证不存在",
  "name_taken": "凭证名称已被占用",
  "invalid_key_format": "凭证内容格式错误",
  "invalid_provider": "云厂商必须是 aws / gcp / azure 之一",
  "invalid_purpose": "consume 调用必须提供 purpose",
  "system_purpose_required": "系统调用的 purpose 必须以 system: 开头",
  "crypto_seal_failed": "加密失败，请联系管理员",
  "crypto_open_failed": "解密失败或数据已损坏"
}
```

- [ ] **Step 2: Mirror to en-US**

Add the same structure to `optimus-fe/src/locales/en-US.json` with English values:

```json
"credentials_group": "Credentials",
"credentials": {
  "ssh_keys": "SSH keys",
  "kubeconfigs": "Kubeconfigs",
  "cloud_keys": "Cloud keys"
},
```

```json
"credentials": {
  "ssh_key":    { "read": "View SSH credentials", "write": "Create/update SSH credentials", "delete": "Delete SSH credentials", "use": "Use SSH credentials" },
  "kubeconfig": { "read": "View kubeconfigs", "write": "Create/update kubeconfigs", "delete": "Delete kubeconfigs", "use": "Use kubeconfigs" },
  "cloud_key":  { "read": "View cloud keys", "write": "Create/update cloud keys", "delete": "Delete cloud keys", "use": "Use cloud keys" }
},
```

```json
"credentials": "Credentials",
```

```json
"credentials": {
  "search_placeholder": "Search by name or description",
  "filter_username": "Username",
  "filter_namespace": "Default namespace",
  "filter_provider": "Provider",
  "filter_provider_all": "All",
  "field": {
    "name": "Name",
    "description": "Description",
    "username": "Username",
    "private_key": "Private key",
    "passphrase": "Passphrase",
    "default_namespace": "Default namespace",
    "kubeconfig": "Kubeconfig YAML",
    "provider": "Provider",
    "region": "Region",
    "access_key_id": "Access Key ID",
    "secret_access_key": "Secret Access Key"
  },
  "placeholder": { "unchanged": "••••• (unchanged)" },
  "action": {
    "create": "Create",
    "edit": "Edit",
    "delete": "Delete",
    "confirm_delete": "Delete this credential? This cannot be undone."
  },
  "toast": {
    "created": "Credential created",
    "updated": "Credential updated",
    "deleted": "Credential deleted"
  },
  "ssh_keys": { "title": "SSH keys", "col_username": "Username" },
  "kubeconfigs": { "title": "Kubeconfigs", "col_default_namespace": "Default namespace" },
  "cloud_keys": { "title": "Cloud keys", "col_provider": "Provider", "col_region": "Region" },
  "not_found": "Credential not found",
  "name_taken": "Credential name already exists",
  "invalid_key_format": "Invalid credential format",
  "invalid_provider": "Provider must be aws / gcp / azure",
  "invalid_purpose": "A purpose must be supplied for consume",
  "system_purpose_required": "System consume requires purpose prefix system:",
  "crypto_seal_failed": "Encryption failed",
  "crypto_open_failed": "Decryption or integrity check failed"
}
```

- [ ] **Step 3: Run the i18n CI check locally**

```
cd optimus-fe && bun run check-i18n-keys
```

(Verify the actual script name with `cat package.json | grep -A1 scripts`. The check enforces zh-CN ⇄ en-US parity and that referenced keys exist.)

Expected: PASS, or it lists missing keys. Add any missing ones.

- [ ] **Step 4: Commit**

```
cd .. && git add optimus-fe/src/locales/zh-CN.json optimus-fe/src/locales/en-US.json
git commit -m "feat(fe/i18n): credentials.* + perm.credentials.* + perm.category.credentials"
```

---

## Task 15: FE api/credentials/{ssh-key,kubeconfig,cloud-key}.ts

**Files:**
- New: `optimus-fe/src/api/credentials/ssh-key.ts`
- New: `optimus-fe/src/api/credentials/kubeconfig.ts`
- New: `optimus-fe/src/api/credentials/cloud-key.ts`

- [ ] **Step 1: Inspect the existing api/role.ts as a template**

```
cat optimus-fe/src/api/role.ts
```

Adopt its shape (DTO interface + `listRoles`, `getRole`, `createRole`, `updateRole`, `deleteRole` wrappers around the shared `http` client from `./client`).

- [ ] **Step 2: Create ssh-key.ts**

Create `optimus-fe/src/api/credentials/ssh-key.ts`:

```ts
import { http } from '../client'
import type { Envelope, Paginated } from '../client'

export interface SshKeySummary {
  id: number
  name: string
  description: string
  username: string
  created_by?: { id: number; username?: string; display_name?: string }
  created_at: string
  updated_at: string
}

export interface SshKeyCreateRequest {
  name: string
  description: string
  username: string
  private_key: string
  passphrase?: string
}

export interface SshKeyUpdateRequest {
  name?: string
  description?: string
  username?: string
  private_key?: string
  passphrase?: string
}

export interface SshKeyListQuery {
  page?: number
  page_size?: number
  q?: string
  username?: string
}

const base = '/credentials/ssh-keys'

export function listSshKeys(q: SshKeyListQuery) {
  return http.get<Envelope<Paginated<SshKeySummary>>>(base, { params: q })
}

export function getSshKey(id: number) {
  return http.get<Envelope<SshKeySummary>>(`${base}/${id}`)
}

export function createSshKey(body: SshKeyCreateRequest) {
  return http.post<Envelope<SshKeySummary>>(base, body)
}

export function updateSshKey(id: number, body: SshKeyUpdateRequest) {
  return http.put<Envelope<SshKeySummary>>(`${base}/${id}`, body)
}

export function deleteSshKey(id: number) {
  return http.delete<Envelope<unknown>>(`${base}/${id}`)
}
```

> Verify `Envelope` and `Paginated` are exported from `src/api/client.ts`. If P0 named them differently (e.g. `ApiResponse<T>` and `Page<T>`), adopt those names. Other api modules in P0 are the source of truth.

- [ ] **Step 3: Create kubeconfig.ts**

```ts
import { http } from '../client'
import type { Envelope, Paginated } from '../client'

export interface KubeconfigSummary {
  id: number
  name: string
  description: string
  default_namespace: string
  created_by?: { id: number; username?: string; display_name?: string }
  created_at: string
  updated_at: string
}

export interface KubeconfigCreateRequest {
  name: string
  description: string
  default_namespace: string
  kubeconfig: string
}

export interface KubeconfigUpdateRequest {
  name?: string
  description?: string
  default_namespace?: string
  kubeconfig?: string
}

export interface KubeconfigListQuery {
  page?: number
  page_size?: number
  q?: string
  default_namespace?: string
}

const base = '/credentials/kubeconfigs'

export function listKubeconfigs(q: KubeconfigListQuery) {
  return http.get<Envelope<Paginated<KubeconfigSummary>>>(base, { params: q })
}
export function getKubeconfig(id: number) {
  return http.get<Envelope<KubeconfigSummary>>(`${base}/${id}`)
}
export function createKubeconfig(body: KubeconfigCreateRequest) {
  return http.post<Envelope<KubeconfigSummary>>(base, body)
}
export function updateKubeconfig(id: number, body: KubeconfigUpdateRequest) {
  return http.put<Envelope<KubeconfigSummary>>(`${base}/${id}`, body)
}
export function deleteKubeconfig(id: number) {
  return http.delete<Envelope<unknown>>(`${base}/${id}`)
}
```

- [ ] **Step 4: Create cloud-key.ts**

```ts
import { http } from '../client'
import type { Envelope, Paginated } from '../client'

export type CloudProvider = 'aws' | 'gcp' | 'azure'

export interface CloudKeySummary {
  id: number
  name: string
  description: string
  provider: CloudProvider
  region: string
  created_by?: { id: number; username?: string; display_name?: string }
  created_at: string
  updated_at: string
}

export interface CloudKeyCreateRequest {
  name: string
  description: string
  provider: CloudProvider
  region: string
  access_key_id: string
  secret_access_key: string
}

export interface CloudKeyUpdateRequest {
  name?: string
  description?: string
  provider?: CloudProvider
  region?: string
  access_key_id?: string
  secret_access_key?: string
}

export interface CloudKeyListQuery {
  page?: number
  page_size?: number
  q?: string
  provider?: CloudProvider
}

const base = '/credentials/cloud-keys'

export function listCloudKeys(q: CloudKeyListQuery) {
  return http.get<Envelope<Paginated<CloudKeySummary>>>(base, { params: q })
}
export function getCloudKey(id: number) {
  return http.get<Envelope<CloudKeySummary>>(`${base}/${id}`)
}
export function createCloudKey(body: CloudKeyCreateRequest) {
  return http.post<Envelope<CloudKeySummary>>(base, body)
}
export function updateCloudKey(id: number, body: CloudKeyUpdateRequest) {
  return http.put<Envelope<CloudKeySummary>>(`${base}/${id}`, body)
}
export function deleteCloudKey(id: number) {
  return http.delete<Envelope<unknown>>(`${base}/${id}`)
}
```

- [ ] **Step 5: Type check**

```
cd optimus-fe && bun run type-check
```

(Or `bun run tsc --noEmit` — match whatever P0's package.json scripts.type-check is.)

Expected: PASS.

- [ ] **Step 6: Commit**

```
cd .. && git add optimus-fe/src/api/credentials/
git commit -m "feat(fe/api): credentials/ssh-key + kubeconfig + cloud-key DTOs and http wrappers"
```

---

## Task 16: FE SSH keys page

**Files:**
- New: `optimus-fe/src/views/credentials/ssh-keys/List.vue`
- New: `optimus-fe/src/views/credentials/ssh-keys/components/SshKeyForm.vue`
- New: `optimus-fe/src/views/credentials/ssh-keys/__tests__/List.test.ts`

- [ ] **Step 1: Inspect P0's reference page**

```
cat optimus-fe/src/views/system/users/List.vue
```

Adopt its structure: a single SFC with `<script setup lang="ts">` + `<template>` + `<style scoped lang="scss">`. The script imports the API module, defines reactive state (`list`, `query`, `loading`), wires search/pagination, and opens a modal with the Form component for create/edit.

- [ ] **Step 2: Create the form component**

Create `optimus-fe/src/views/credentials/ssh-keys/components/SshKeyForm.vue`:

```vue
<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { SshKeySummary } from '@/api/credentials/ssh-key'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  modelValue: boolean         // dialog visibility (v-model)
  mode: 'create' | 'edit'
  initial?: SshKeySummary | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'submit', body: Record<string, any>): void
}>()

const { t } = useI18n()

const form = reactive({
  name: '',
  description: '',
  username: '',
  private_key: '',
  passphrase: '',
})

const rules = {
  name:       [{ required: true, max: 128, message: t('credentials.field.name') }],
  username:   [{ required: true, max: 64, message: t('credentials.field.username') }],
  private_key:[{ required: props.mode === 'create', message: t('credentials.field.private_key') }],
}

watch(() => props.modelValue, (open) => {
  if (!open) return
  if (props.mode === 'edit' && props.initial) {
    form.name = props.initial.name
    form.description = props.initial.description
    form.username = props.initial.username
    form.private_key = ''
    form.passphrase = ''
  } else {
    form.name = ''
    form.description = ''
    form.username = ''
    form.private_key = ''
    form.passphrase = ''
  }
}, { immediate: true })

function onOk() {
  if (props.mode === 'create') {
    emit('submit', { ...form })
  } else {
    const body: Record<string, any> = {}
    if (props.initial) {
      if (form.name !== props.initial.name) body.name = form.name
      if (form.description !== props.initial.description) body.description = form.description
      if (form.username !== props.initial.username) body.username = form.username
    }
    if (form.private_key) body.private_key = form.private_key
    if (form.passphrase) body.passphrase = form.passphrase
    emit('submit', body)
  }
}

function onCancel() {
  emit('update:modelValue', false)
}
</script>

<template>
  <a-modal
    :open="modelValue"
    :title="mode === 'create' ? t('credentials.action.create') : t('credentials.action.edit')"
    @update:open="(v) => emit('update:modelValue', v)"
    @ok="onOk"
    @cancel="onCancel"
    :destroy-on-close="true"
    width="640px"
  >
    <a-form :model="form" layout="vertical">
      <a-form-item :label="t('credentials.field.name')" :rules="rules.name">
        <a-input v-model:value="form.name" :max-length="128" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.description')">
        <a-textarea v-model:value="form.description" :rows="2" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.username')" :rules="rules.username">
        <a-input v-model:value="form.username" :max-length="64" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.private_key')" :rules="rules.private_key">
        <a-textarea
          v-model:value="form.private_key"
          :rows="8"
          :placeholder="mode === 'edit' ? t('credentials.placeholder.unchanged') : ''"
          style="font-family: monospace; font-size: 12px"
        />
      </a-form-item>
      <a-form-item :label="t('credentials.field.passphrase')">
        <a-input-password
          v-model:value="form.passphrase"
          :placeholder="mode === 'edit' ? t('credentials.placeholder.unchanged') : ''"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>
```

- [ ] **Step 3: Create the list page**

Create `optimus-fe/src/views/credentials/ssh-keys/List.vue`:

```vue
<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { listSshKeys, createSshKey, updateSshKey, deleteSshKey, type SshKeySummary } from '@/api/credentials/ssh-key'
import SshKeyForm from './components/SshKeyForm.vue'

const { t } = useI18n()

const query = reactive({
  page: 1,
  page_size: 20,
  q: '',
  username: '',
})

const list = ref<SshKeySummary[]>([])
const total = ref(0)
const loading = ref(false)

const dialogOpen = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const editing = ref<SshKeySummary | null>(null)

async function fetchList() {
  loading.value = true
  try {
    const res = await listSshKeys(query)
    list.value = res.data.data.items
    total.value = res.data.data.total
  } finally {
    loading.value = false
  }
}

function openCreate() {
  dialogMode.value = 'create'
  editing.value = null
  dialogOpen.value = true
}

function openEdit(row: SshKeySummary) {
  dialogMode.value = 'edit'
  editing.value = row
  dialogOpen.value = true
}

async function onSubmit(body: Record<string, any>) {
  try {
    if (dialogMode.value === 'create') {
      await createSshKey(body as any)
      message.success(t('credentials.toast.created'))
    } else if (editing.value) {
      await updateSshKey(editing.value.id, body)
      message.success(t('credentials.toast.updated'))
    }
    dialogOpen.value = false
    fetchList()
  } catch (e: any) {
    // The global http interceptor surfaces the message_key as a toast.
  }
}

function onDelete(row: SshKeySummary) {
  Modal.confirm({
    title: t('credentials.action.confirm_delete'),
    okType: 'danger',
    onOk: async () => {
      await deleteSshKey(row.id)
      message.success(t('credentials.toast.deleted'))
      fetchList()
    },
  })
}

const columns = [
  { title: t('credentials.field.name'), dataIndex: 'name' },
  { title: t('credentials.field.description'), dataIndex: 'description' },
  { title: t('credentials.ssh_keys.col_username'), dataIndex: 'username' },
  { title: 'updated_at', dataIndex: 'updated_at' },
  { title: t('common.actions'), key: 'actions' },
]

onMounted(fetchList)
</script>

<template>
  <div class="page">
    <div class="header">
      <h2>{{ t('credentials.ssh_keys.title') }}</h2>
      <a-button
        v-permission="'credentials:ssh_key:write'"
        type="primary"
        @click="openCreate"
      >{{ t('credentials.action.create') }}</a-button>
    </div>

    <div class="filters">
      <a-input
        v-model:value="query.q"
        :placeholder="t('credentials.search_placeholder')"
        style="width: 240px"
        @press-enter="() => { query.page = 1; fetchList() }"
        allow-clear
      />
      <a-input
        v-model:value="query.username"
        :placeholder="t('credentials.filter_username')"
        style="width: 180px"
        @press-enter="() => { query.page = 1; fetchList() }"
        allow-clear
      />
      <a-button @click="() => { query.page = 1; fetchList() }">Search</a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="list"
      :loading="loading"
      :pagination="{ current: query.page, pageSize: query.page_size, total, onChange: (p: number) => { query.page = p; fetchList() } }"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'actions'">
          <a-space>
            <a v-permission="'credentials:ssh_key:write'" @click="() => openEdit(record)">
              {{ t('credentials.action.edit') }}
            </a>
            <a v-permission="'credentials:ssh_key:delete'" class="danger" @click="() => onDelete(record)">
              {{ t('credentials.action.delete') }}
            </a>
          </a-space>
        </template>
      </template>
    </a-table>

    <SshKeyForm
      v-model="dialogOpen"
      :mode="dialogMode"
      :initial="editing"
      @submit="onSubmit"
    />
  </div>
</template>

<style scoped lang="scss">
.page { padding: 16px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.filters { display: flex; gap: 8px; margin-bottom: 16px; }
.danger { color: var(--ant-color-error); }
</style>
```

- [ ] **Step 4: Vitest**

Create `optimus-fe/src/views/credentials/ssh-keys/__tests__/List.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import List from '../List.vue'

vi.mock('@/api/credentials/ssh-key', () => ({
  listSshKeys: vi.fn().mockResolvedValue({ data: { data: { items: [], total: 0, page: 1, page_size: 20 } } }),
  createSshKey: vi.fn(),
  updateSshKey: vi.fn(),
  deleteSshKey: vi.fn(),
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))

describe('SSH keys List', () => {
  it('renders the page title', async () => {
    const wrapper = mount(List, {
      global: {
        directives: { permission: () => {} },
        stubs: { 'a-button': true, 'a-input': true, 'a-table': true, 'a-space': true, SshKeyForm: true },
      },
    })
    expect(wrapper.text()).toContain('credentials.ssh_keys.title')
  })
})
```

- [ ] **Step 5: Run tests + type check**

```
cd optimus-fe && bun run test src/views/credentials/ssh-keys/ && bun run type-check
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
cd .. && git add optimus-fe/src/views/credentials/ssh-keys/
git commit -m "feat(fe/credentials): SSH keys CRUD page + write-only secret form"
```

---

## Task 17: FE Kubeconfigs page

**Files:**
- New: `optimus-fe/src/views/credentials/kubeconfigs/List.vue`
- New: `optimus-fe/src/views/credentials/kubeconfigs/components/KubeconfigForm.vue`

- [ ] **Step 1: Form component**

Create `optimus-fe/src/views/credentials/kubeconfigs/components/KubeconfigForm.vue` by adapting `SshKeyForm.vue`:

```vue
<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { KubeconfigSummary } from '@/api/credentials/kubeconfig'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  modelValue: boolean
  mode: 'create' | 'edit'
  initial?: KubeconfigSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'submit', body: Record<string, any>): void
}>()
const { t } = useI18n()

const form = reactive({ name: '', description: '', default_namespace: '', kubeconfig: '' })

watch(() => props.modelValue, (open) => {
  if (!open) return
  if (props.mode === 'edit' && props.initial) {
    form.name = props.initial.name
    form.description = props.initial.description
    form.default_namespace = props.initial.default_namespace
    form.kubeconfig = ''
  } else {
    form.name = ''; form.description = ''; form.default_namespace = ''; form.kubeconfig = ''
  }
}, { immediate: true })

function onOk() {
  if (props.mode === 'create') {
    emit('submit', { ...form })
  } else {
    const body: Record<string, any> = {}
    if (props.initial) {
      if (form.name !== props.initial.name) body.name = form.name
      if (form.description !== props.initial.description) body.description = form.description
      if (form.default_namespace !== props.initial.default_namespace) body.default_namespace = form.default_namespace
    }
    if (form.kubeconfig) body.kubeconfig = form.kubeconfig
    emit('submit', body)
  }
}
function onCancel() { emit('update:modelValue', false) }
</script>

<template>
  <a-modal
    :open="modelValue"
    :title="mode === 'create' ? t('credentials.action.create') : t('credentials.action.edit')"
    @update:open="(v) => emit('update:modelValue', v)"
    @ok="onOk" @cancel="onCancel" :destroy-on-close="true" width="720px"
  >
    <a-form :model="form" layout="vertical">
      <a-form-item :label="t('credentials.field.name')">
        <a-input v-model:value="form.name" :max-length="128" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.description')">
        <a-textarea v-model:value="form.description" :rows="2" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.default_namespace')">
        <a-input v-model:value="form.default_namespace" :max-length="64" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.kubeconfig')">
        <a-textarea
          v-model:value="form.kubeconfig"
          :rows="12"
          :placeholder="mode === 'edit' ? t('credentials.placeholder.unchanged') : ''"
          style="font-family: monospace; font-size: 12px"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>
```

- [ ] **Step 2: List page**

Create `optimus-fe/src/views/credentials/kubeconfigs/List.vue` by copying SSH keys' `List.vue` and applying these deltas:

1. Imports: replace `ssh-key` module with `kubeconfig`; replace `SshKeyForm` with `KubeconfigForm`.
2. `query`: replace `username` field with `default_namespace`.
3. Permission codes: `credentials:kubeconfig:write` / `credentials:kubeconfig:delete`.
4. Columns: replace `col_username` with `col_default_namespace` (uses `t('credentials.kubeconfigs.col_default_namespace')` → "默认命名空间" / "Default namespace").
5. Title: `t('credentials.kubeconfigs.title')`.
6. Filter input: bind to `query.default_namespace`, placeholder `t('credentials.filter_namespace')`.

- [ ] **Step 3: Type check + commit**

```
cd optimus-fe && bun run type-check
cd .. && git add optimus-fe/src/views/credentials/kubeconfigs/
git commit -m "feat(fe/credentials): kubeconfigs CRUD page"
```

---

## Task 18: FE Cloud keys page

**Files:**
- New: `optimus-fe/src/views/credentials/cloud-keys/List.vue`
- New: `optimus-fe/src/views/credentials/cloud-keys/components/CloudKeyForm.vue`

- [ ] **Step 1: Form component**

Create `optimus-fe/src/views/credentials/cloud-keys/components/CloudKeyForm.vue`:

```vue
<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { CloudKeySummary, CloudProvider } from '@/api/credentials/cloud-key'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  modelValue: boolean
  mode: 'create' | 'edit'
  initial?: CloudKeySummary | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'submit', body: Record<string, any>): void
}>()
const { t } = useI18n()

const form = reactive({
  name: '', description: '', provider: 'aws' as CloudProvider, region: '',
  access_key_id: '', secret_access_key: '',
})

watch(() => props.modelValue, (open) => {
  if (!open) return
  if (props.mode === 'edit' && props.initial) {
    form.name = props.initial.name
    form.description = props.initial.description
    form.provider = props.initial.provider
    form.region = props.initial.region
    form.access_key_id = ''
    form.secret_access_key = ''
  } else {
    form.name = ''; form.description = ''; form.provider = 'aws'; form.region = ''
    form.access_key_id = ''; form.secret_access_key = ''
  }
}, { immediate: true })

function onOk() {
  if (props.mode === 'create') {
    emit('submit', { ...form })
  } else {
    const body: Record<string, any> = {}
    if (props.initial) {
      if (form.name !== props.initial.name) body.name = form.name
      if (form.description !== props.initial.description) body.description = form.description
      if (form.provider !== props.initial.provider) body.provider = form.provider
      if (form.region !== props.initial.region) body.region = form.region
    }
    if (form.access_key_id) body.access_key_id = form.access_key_id
    if (form.secret_access_key) body.secret_access_key = form.secret_access_key
    emit('submit', body)
  }
}
function onCancel() { emit('update:modelValue', false) }
</script>

<template>
  <a-modal
    :open="modelValue"
    :title="mode === 'create' ? t('credentials.action.create') : t('credentials.action.edit')"
    @update:open="(v) => emit('update:modelValue', v)"
    @ok="onOk" @cancel="onCancel" :destroy-on-close="true" width="640px"
  >
    <a-form :model="form" layout="vertical">
      <a-form-item :label="t('credentials.field.name')">
        <a-input v-model:value="form.name" :max-length="128" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.description')">
        <a-textarea v-model:value="form.description" :rows="2" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.provider')">
        <a-radio-group v-model:value="form.provider">
          <a-radio-button value="aws">AWS</a-radio-button>
          <a-radio-button value="gcp">GCP</a-radio-button>
          <a-radio-button value="azure">Azure</a-radio-button>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="t('credentials.field.region')">
        <a-input v-model:value="form.region" :max-length="32" />
      </a-form-item>
      <a-form-item :label="t('credentials.field.access_key_id')">
        <a-input
          v-model:value="form.access_key_id"
          :max-length="256"
          :placeholder="mode === 'edit' ? t('credentials.placeholder.unchanged') : ''"
        />
      </a-form-item>
      <a-form-item :label="t('credentials.field.secret_access_key')">
        <a-input-password
          v-model:value="form.secret_access_key"
          :placeholder="mode === 'edit' ? t('credentials.placeholder.unchanged') : ''"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>
```

- [ ] **Step 2: List page**

Create `optimus-fe/src/views/credentials/cloud-keys/List.vue` by adapting SSH keys' List.vue:

1. Replace imports with cloud-key module + `CloudKeyForm`.
2. Replace `query.username` with `query.provider: CloudProvider | undefined` (filter dropdown).
3. Permission codes: `credentials:cloud_key:write` / `credentials:cloud_key:delete`.
4. Columns: `provider` (use `col_provider` label) + `region` (`col_region` label) instead of `username`.
5. Title: `t('credentials.cloud_keys.title')`.
6. Filter: replace the username `a-input` with an `a-select` bound to `query.provider`:

```vue
<a-select
  v-model:value="query.provider"
  :placeholder="t('credentials.filter_provider')"
  :options="[
    { value: undefined, label: t('credentials.filter_provider_all') },
    { value: 'aws', label: 'AWS' },
    { value: 'gcp', label: 'GCP' },
    { value: 'azure', label: 'Azure' },
  ]"
  style="width: 160px"
  allow-clear
/>
```

- [ ] **Step 3: Type check + commit**

```
cd optimus-fe && bun run type-check
cd .. && git add optimus-fe/src/views/credentials/cloud-keys/
git commit -m "feat(fe/credentials): cloud keys CRUD page with provider radio"
```

---

## Task 19: Deploy artifacts + README

**Files:**
- Modify: `deploy/be.Dockerfile`
- Modify: `deploy/.env.example`
- Modify: `deploy/docker-compose.prod.yml`
- Modify: `README.md`

- [ ] **Step 1: Add vault-keygen build target to BE Dockerfile**

In `deploy/be.Dockerfile`, after the existing `migrate` target, add (verify exact format with the existing migrate target):

```dockerfile
# ---- target: vault-keygen (small CLI for master-key bootstrap) ----
FROM build AS vault-keygen-build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w" \
      -o /out/optimus-vault-keygen ./cmd/vault-keygen

FROM gcr.io/distroless/static-debian12:nonroot AS vault-keygen
COPY --from=vault-keygen-build /out/optimus-vault-keygen /optimus-vault-keygen
ENTRYPOINT ["/optimus-vault-keygen"]
```

(If P0's existing Dockerfile uses a different distroless variant, match it. Read the migrate stage first.)

- [ ] **Step 2: Add env to .env.example**

In `deploy/.env.example`, append:

```
# --- vault (P1) ---
# Generate with: docker run --rm $(docker build -q -f deploy/be.Dockerfile --target vault-keygen ..)
# Or with the host Go toolchain: cd optimus-be && go run ./cmd/vault-keygen
OPTIMUS_VAULT_MASTER_KEY=
# Alternative to OPTIMUS_VAULT_MASTER_KEY: file path inside the container.
# Mount the file via docker-compose volumes if you use this.
OPTIMUS_VAULT_MASTER_KEY_FILE=
```

- [ ] **Step 3: Pass env to be service in compose**

In `deploy/docker-compose.prod.yml`, locate the `be:` service's `environment:` block and append:

```yaml
- OPTIMUS_VAULT_MASTER_KEY=${OPTIMUS_VAULT_MASTER_KEY:-}
- OPTIMUS_VAULT_MASTER_KEY_FILE=${OPTIMUS_VAULT_MASTER_KEY_FILE:-}
```

- [ ] **Step 4: README update**

In `README.md`, find the "Production deploy" section and add a subsection before "Bring up the stack":

```markdown
### Step 0: mint the credentials-vault master key

P1 (credentials-vault) requires a 32-byte AES-256 master key. Generate one ONCE and store it safely — losing it makes all encrypted credentials unrecoverable.

Option A — env var (simplest, dev / single-machine):

```
cd optimus-be && go run ./cmd/vault-keygen
# Copy the output line into deploy/.env:
echo "OPTIMUS_VAULT_MASTER_KEY=<paste here>" >> deploy/.env
```

Or all in one:

```
cd optimus-be && \
  KEY=$(go run ./cmd/vault-keygen) && \
  echo "OPTIMUS_VAULT_MASTER_KEY=$KEY" >> ../deploy/.env
```

Option B — file (better hygiene; chmod the file to 0400):

```
cd optimus-be && go run ./cmd/vault-keygen > /etc/optimus/vault.key
chmod 0400 /etc/optimus/vault.key
# In deploy/.env:
OPTIMUS_VAULT_MASTER_KEY_FILE=/etc/optimus/vault.key
# Mount the file into the BE container — uncomment the volumes: block under be:
```

The BE refuses to start if neither variable is set.
```

- [ ] **Step 5: Build the Dockerfile to verify**

```
cd /Users/logic/Projects/optimus && docker build -f deploy/be.Dockerfile --target vault-keygen optimus-be -t optimus-vault-keygen:smoke
docker run --rm optimus-vault-keygen:smoke
```

Expected: prints one base64 line.

- [ ] **Step 6: Commit**

```
git add deploy/be.Dockerfile deploy/.env.example deploy/docker-compose.prod.yml README.md
git commit -m "chore(deploy): vault-keygen Dockerfile target + master-key env wiring + README"
```

---

## Task 20: Verification + final commit

- [ ] **Step 1: Full backend test suite**

```
cd optimus-be && DOCKER_HOST=unix:///Users/logic/.colima/docker.sock go test -tags=dbtest ./... -cover
```

Expected: all packages PASS, and the four new packages each report ≥ 60% coverage:

- `internal/modules/credentials/vault`
- `internal/modules/credentials/sshkey`
- `internal/modules/credentials/kubeconfig`
- `internal/modules/credentials/cloudkey`

If any package is below 60%, add tests for the lowest-covered branches before moving on. (Use `go test -coverprofile=cov.out ./internal/modules/credentials/sshkey/ && go tool cover -func=cov.out` to find them.)

- [ ] **Step 2: Frontend tests + type check**

```
cd optimus-fe && bun run test && bun run type-check && bun run check-i18n-keys
```

Expected: all PASS.

- [ ] **Step 3: Build the full prod stack**

```
cd deploy && \
  KEY=$(cd ../optimus-be && go run ./cmd/vault-keygen) && \
  OPTIMUS_VAULT_MASTER_KEY=$KEY docker compose -f docker-compose.prod.yml --env-file .env up -d --build
```

Wait for the `be` service to report healthy:

```
docker compose -f docker-compose.prod.yml ps
```

- [ ] **Step 4: Manual smoke — create one of each type**

Get admin token (assume admin/admin per P0 seed, or whatever the seed printed):

```
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<the-printed-pw>"}' | jq -r .data.access_token)
```

Create one of each:

```
# SSH key
PEM=$(ssh-keygen -t ed25519 -N "" -f /tmp/k && cat /tmp/k)
curl -X POST http://localhost:8080/api/v1/credentials/ssh-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg pk "$PEM" '{name:"smoke-ssh", username:"ops", private_key:$pk}')"

# Kubeconfig (minimal valid)
KC='apiVersion: v1
kind: Config
clusters: [{name: c1, cluster: {server: https://127.0.0.1:6443, insecure-skip-tls-verify: true}}]
contexts: [{name: ctx, context: {cluster: c1, user: u1}}]
users:    [{name: u1, user: {token: abc}}]
current-context: ctx'
curl -X POST http://localhost:8080/api/v1/credentials/kubeconfigs \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg kc "$KC" '{name:"smoke-kc", kubeconfig:$kc}')"

# Cloud key
curl -X POST http://localhost:8080/api/v1/credentials/cloud-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"smoke-aws","provider":"aws","region":"us-east-1","access_key_id":"AKIAEXAMPLE","secret_access_key":"abcd"}'
```

Verify by listing each, and confirm no secret material appears in the responses (`grep -i 'BEGIN OPENSSH' <response>` should return nothing for the SSH list response).

- [ ] **Step 5: Verify encrypted-at-rest**

```
docker compose -f docker-compose.prod.yml exec postgres psql -U optimus -d optimus -c \
  "SELECT name, encode(private_key_enc, 'hex') FROM credentials_ssh_keys LIMIT 1;"
```

Expected: `private_key_enc` is a hex blob that starts with 24 random hex chars (the 12-byte nonce), NOT a PEM header.

- [ ] **Step 6: Verify audit rows survive delete**

```
curl -X DELETE http://localhost:8080/api/v1/credentials/ssh-keys/1 \
  -H "Authorization: Bearer $TOKEN"

docker compose -f docker-compose.prod.yml exec postgres psql -U optimus -d optimus -c \
  "SELECT action, target_type, target_id, payload FROM audit_logs WHERE target_type LIKE 'credentials.%' ORDER BY id DESC LIMIT 5;"
```

Expected: at least one row with `action='credentials.delete'` and `payload->>'name' = 'smoke-ssh'`.

- [ ] **Step 7: Tear down the smoke stack**

```
cd deploy && docker compose -f docker-compose.prod.yml down -v
```

- [ ] **Step 8: Push & open PR**

```
git push -u origin dev
gh pr create --base main --head dev --title "feat: P1 credentials-vault" --body "$(cat <<'EOF'
## Summary

Implements P1 (credentials-vault) — encrypted storage for SSH keys, kubeconfigs, and cloud access keys with full CRUD over HTTP, AES-256-GCM application-layer encryption, and an internal Go `Consumer` seam for downstream P2/P4/P5/P6.

Design: `docs/superpowers/specs/2026-06-10-p1-credentials-vault-design.md`
Plan:   `docs/superpowers/plans/2026-06-10-p1-credentials-vault.md`

- 3 new BE modules + crypto package (`internal/modules/credentials/{vault,sshkey,kubeconfig,cloudkey}`)
- 12 new permission codes
- 3 new FE CRUD pages (re-using P0 Plan 2b shape)
- New `optimus-vault-keygen` CLI + Dockerfile target
- No schema changes to `audit_logs` (denormalized snapshot in `payload` jsonb)

## Test plan

- [x] BE unit + dockertest passes for all 4 new packages
- [x] BE coverage ≥60% per package
- [x] FE vitest + type-check + i18n-key check passes
- [x] Prod compose stack builds and starts with master key
- [x] Manual smoke: create + list + delete one of each type via HTTP
- [x] Encrypted-at-rest verified (BYTEA blobs, no plaintext in DB)
- [x] Audit rows survive delete with denormalized name snapshot

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review checklist (run after writing this plan, not during execution)

1. **Spec coverage:**
   - §1 goal — Tasks 5-12 + 14-18 cover all P1 deliverables.
   - §2 architecture — Task 2 (vault), Tasks 5-10 (3 verticals), Task 11 (consume seam), Task 12 (wiring).
   - §3 data model — Tasks 5, 9, 10 create the three tables; Task 4 + Task 13 use existing `audit_logs.payload`.
   - §4 crypto — Task 2 implements AES-GCM + key bootstrap; Task 3 implements keygen CLI.
   - §5 HTTP API — Tasks 8, 9, 10 (handlers), Task 12 (RBAC mount).
   - §6 Consumer seam — Task 11.
   - §7 permissions — Task 4 (12 codes), Task 14 (i18n).
   - §8 FE pages — Tasks 16-18 (3 pages), Task 13 (menus), Task 15 (api modules), Task 14 (locales).
   - §9 audit — embedded in service code (Task 7) using existing recorder API; Task 20 verifies persistence.
   - §10 errors + i18n — service-side codes in Task 7, locale keys in Task 14.
   - §11 testing — every BE task has its test step; Task 16 has FE vitest; Task 20 runs coverage gate.
   - §12 acceptance — Task 20 verifies all 10 items.
2. **No placeholders.** Every code block is complete. Symmetric files (kubeconfig/cloudkey) cite the SSH-key template + explicit deltas.
3. **Type consistency.** `Service` / `Repo` / `Handler` names, `Summary` / `Detail` / `CreateRequest` / `UpdateRequest` / `ListQuery` / `ListResponse` are identical across all 3 feature packages. `ConsumeRecord` differs per type as intended.
4. **Order of tasks.** Foundation → BE features → consume seam → wiring → seed → FE → deploy → verification. No task depends on a later task.
