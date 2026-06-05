# P0 Plan 1A — Backend Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up `optimus-be`'s foundation: Go module, config/log/errors infrastructure, Postgres schema (8 tables with partial unique indexes & FKs), GORM models, permission-code registration, admin/role/menu seed, and a `/health` endpoint.

**Architecture:** Single Go binary using Gin (HTTP, introduced minimally here for `/health`), GORM (ORM, no `uniqueIndex` tags), goose (SQL migrations), viper (config), slog (logging). Local infra via docker-compose (Postgres 16). All Postgres-specific (partial unique indexes, JSONB, `ON DELETE` rules) intentional.

**Tech Stack:** Go 1.22, Gin, GORM, Postgres 16, goose, viper, slog, testify, dockertest, golangci-lint, air, bcrypt.

**Scope:** This plan stops short of HTTP middleware, JWT, RBAC, and business modules. Those start in Plan 1B. At end of 1A, `make seed && make run` produces a server with `/health` responding and a fully populated DB.

**Spec:** `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md`

---

## File Structure (1A scope)

```
optimus/
├── docker-compose.yml                # local Postgres + Adminer
├── .gitignore
└── optimus-be/
    ├── go.mod
    ├── go.sum
    ├── Makefile
    ├── .air.toml
    ├── configs/config.yaml
    ├── cmd/
    │   ├── server/main.go            # boots HTTP, /health only (1A)
    │   └── seed/main.go              # populates admin/roles/menus/perms
    ├── internal/
    │   ├── infra/
    │   │   ├── config/config.go      # viper wrapper
    │   │   ├── log/log.go            # slog setup
    │   │   ├── errors/errors.go      # BizError + codes
    │   │   ├── errors/codes.go       # numeric code constants
    │   │   ├── response/envelope.go  # {code,data,message,message_key}
    │   │   ├── db/db.go              # GORM connect + tx helper
    │   │   ├── db/dockertest.go      # test helper (under +build dbtest)
    │   │   └── permissions/
    │   │       ├── codes.go          # PermXxx string constants
    │   │       ├── registry.go       # in-process registry + DB upsert
    │   │       └── registry_test.go
    │   └── models/
    │       ├── user.go
    │       ├── role.go
    │       ├── permission.go
    │       ├── user_role.go
    │       ├── role_permission.go
    │       ├── menu.go
    │       ├── refresh_token.go
    │       └── audit_log.go
    ├── migrations/
    │   ├── 00001_create_users.sql
    │   ├── 00002_create_roles.sql
    │   ├── 00003_create_permissions.sql
    │   ├── 00004_create_user_roles.sql
    │   ├── 00005_create_role_permissions.sql
    │   ├── 00006_create_menus.sql
    │   ├── 00007_create_refresh_tokens.sql
    │   ├── 00008_create_audit_logs.sql
    │   ├── 00009_partial_unique_indexes.sql
    │   └── 00010_foreign_keys.sql
    └── tests/integration/
        └── seed_test.go              # dockertest-driven seed verification
```

---

## Phase 1: Repo & Tooling (Tasks 1-3)

### Task 1: Init Go module, directory skeleton, .gitignore

**Files:**
- Create: `optimus-be/go.mod`
- Create: `optimus-be/cmd/server/main.go`
- Modify: `.gitignore`

- [ ] **Step 1: Create directory structure**

```bash
cd optimus-be
mkdir -p cmd/server cmd/seed configs internal/infra/{config,log,errors,response,db,permissions} internal/models migrations tests/integration
```

- [ ] **Step 2: Init Go module**

```bash
cd optimus-be
go mod init optimus-be
```

Expected: creates `go.mod` with `module optimus-be` and Go version line.

- [ ] **Step 3: Add Go version constraint**

Edit `optimus-be/go.mod` to ensure first lines read:

```
module optimus-be

go 1.22
```

- [ ] **Step 4: Add baseline dependencies**

```bash
cd optimus-be
go get github.com/gin-gonic/gin@v1.10.0
go get gorm.io/gorm@v1.25.10
go get gorm.io/driver/postgres@v1.5.7
go get github.com/spf13/viper@v1.18.2
go get github.com/pressly/goose/v3@v3.20.0
go get github.com/stretchr/testify@v1.9.0
go get github.com/ory/dockertest/v3@v3.10.0
go get golang.org/x/crypto/bcrypt
go get golang.org/x/time/rate
```

- [ ] **Step 5: Create stub `cmd/server/main.go`**

```go
package main

func main() {
	// filled in later
}
```

- [ ] **Step 6: Append to root `.gitignore`**

Append to `/Users/logic/Projects/optimus/.gitignore`:

```
# Go
optimus-be/bin/
optimus-be/tmp/
*.test
*.out
.env

# IDE
.vscode/
.idea/

# Docker
postgres-data/
```

- [ ] **Step 7: Verify build**

```bash
cd optimus-be
go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 8: Commit**

```bash
git add optimus-be/ .gitignore
git commit -m "feat(be): init go module and directory skeleton"
```

---

### Task 2: Makefile

**Files:** Create: `optimus-be/Makefile`

- [ ] **Step 1: Write Makefile**

```makefile
.PHONY: run build test test-int lint swag migrate-up migrate-down migrate-new seed perm-check air-install goose-install tools

DSN ?= host=localhost port=5432 user=optimus password=optimus dbname=optimus sslmode=disable

run:
	air

build:
	go build -o bin/optimus-be ./cmd/server

test:
	go test ./... -race -cover

test-int:
	go test ./... -tags=integration -race

lint:
	golangci-lint run

swag:
	swag init -g cmd/server/main.go -o api/docs

migrate-up:
	goose -dir migrations postgres "$(DSN)" up

migrate-down:
	goose -dir migrations postgres "$(DSN)" down

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=<name>"; exit 1)
	goose -dir migrations create $(name) sql

seed:
	go run ./cmd/seed

perm-check:
	go run ./cmd/server -check-permissions

tools:
	go install github.com/cosmtrek/air@v1.52.3
	go install github.com/pressly/goose/v3/cmd/goose@v3.20.0
	go install github.com/swaggo/swag/cmd/swag@v1.16.3
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.1
```

- [ ] **Step 2: Install tools**

```bash
cd optimus-be
make tools
```

Expected: installs `air`, `goose`, `swag`, `golangci-lint` to `$GOPATH/bin`.

- [ ] **Step 3: Create `.air.toml`**

```toml
root = "."
tmp_dir = "tmp"

[build]
cmd = "go build -o ./tmp/optimus-be ./cmd/server"
bin = "./tmp/optimus-be"
include_ext = ["go", "yaml"]
exclude_dir = ["tmp", "bin", "migrations"]
delay = 500
kill_delay = "2s"
```

- [ ] **Step 4: Commit**

```bash
git add optimus-be/Makefile optimus-be/.air.toml
git commit -m "feat(be): add Makefile and air config"
```

---

### Task 3: docker-compose for local Postgres

**Files:** Create: `docker-compose.yml`

- [ ] **Step 1: Write `/Users/logic/Projects/optimus/docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    container_name: optimus-pg
    environment:
      POSTGRES_USER: optimus
      POSTGRES_PASSWORD: optimus
      POSTGRES_DB: optimus
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U optimus -d optimus"]
      interval: 5s
      timeout: 5s
      retries: 10

  adminer:
    image: adminer:latest
    container_name: optimus-adminer
    ports: ["8081:8080"]
    depends_on: [postgres]

volumes:
  pgdata:
```

- [ ] **Step 2: Start and verify**

```bash
cd /Users/logic/Projects/optimus
docker compose up -d
docker compose ps
```

Expected: both containers running, postgres `(healthy)` after ~10s.

- [ ] **Step 3: Verify connection**

```bash
docker exec optimus-pg psql -U optimus -d optimus -c '\dt'
```

Expected: `Did not find any relations.` (empty DB).

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "chore: add docker-compose for local postgres"
```

---

## Phase 2: Infrastructure Packages (Tasks 4-7)

### Task 4: Config loading (viper + env override)

**Files:**
- Create: `optimus-be/configs/config.yaml`
- Create: `optimus-be/internal/infra/config/config.go`
- Create: `optimus-be/internal/infra/config/config_test.go`

- [ ] **Step 1: Write `configs/config.yaml`**

```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 15s
  write_timeout: 15s
  shutdown_timeout: 20s

database:
  driver: postgres
  dsn: "host=localhost port=5432 user=optimus password=optimus dbname=optimus sslmode=disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 1h

jwt:
  secret: ""
  access_ttl: 15m
  refresh_ttl: 168h

auth:
  bcrypt_cost: 10
  login_rate_limit:
    per_ip: 5
    per_username: 5
    window: 1m
    block: 1m

log:
  level: info
  format: json
  output: stdout

cors:
  allowed_origins: ["http://localhost:5173"]
  allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
  allow_credentials: false

i18n:
  default_lang: zh-CN
  supported: ["zh-CN", "en-US"]

bootstrap:
  admin_username: admin
  admin_email: admin@example.com
```

- [ ] **Step 2: Write failing test `config_test.go`**

```go
package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
)

func TestLoad_DefaultsFromYAML(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 8080, cfg.Server.Port)
	require.Equal(t, 15*time.Second, cfg.Server.ReadTimeout)
	require.Equal(t, "info", cfg.Log.Level)
	require.Equal(t, []string{"zh-CN", "en-US"}, cfg.I18n.Supported)
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("OPTIMUS_SERVER_PORT", "9090")
	t.Setenv("OPTIMUS_JWT_SECRET", "x_very_long_jwt_secret_for_testing_only_32+")
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 9090, cfg.Server.Port)
	require.Equal(t, "x_very_long_jwt_secret_for_testing_only_32+", cfg.JWT.Secret)
}

func TestLoad_RejectsShortJWTSecretWhenProvided(t *testing.T) {
	t.Setenv("OPTIMUS_JWT_SECRET", "tooshort")
	_, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.Error(t, err)
}

func TestValidate_RequiresJWTSecretWhenStrict(t *testing.T) {
	cfg := &config.Config{}
	require.Error(t, cfg.ValidateStrict())
}
```

- [ ] **Step 3: Run, verify fail**

```bash
cd optimus-be
go test ./internal/infra/config/...
```

Expected: build error / `package config not found`.

- [ ] **Step 4: Implement `config.go`**

```go
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Log      LogConfig      `mapstructure:"log"`
	CORS     CORSConfig     `mapstructure:"cors"`
	I18n     I18nConfig     `mapstructure:"i18n"`
	Boot     BootstrapConfig `mapstructure:"bootstrap"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type AuthConfig struct {
	BcryptCost      int                  `mapstructure:"bcrypt_cost"`
	LoginRateLimit  LoginRateLimitConfig `mapstructure:"login_rate_limit"`
}

type LoginRateLimitConfig struct {
	PerIP       int           `mapstructure:"per_ip"`
	PerUsername int           `mapstructure:"per_username"`
	Window      time.Duration `mapstructure:"window"`
	Block       time.Duration `mapstructure:"block"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

type I18nConfig struct {
	DefaultLang string   `mapstructure:"default_lang"`
	Supported   []string `mapstructure:"supported"`
}

type BootstrapConfig struct {
	AdminUsername string `mapstructure:"admin_username"`
	AdminEmail    string `mapstructure:"admin_email"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("OPTIMUS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.JWT.Secret != "" && len(cfg.JWT.Secret) < 32 {
		return nil, fmt.Errorf("jwt.secret too short: must be >= 32 bytes, got %d", len(cfg.JWT.Secret))
	}
	return cfg, nil
}

// ValidateStrict enforces that all sensitive fields are populated.
// Called at server startup but skipped in tests.
func (c *Config) ValidateStrict() error {
	if c.JWT.Secret == "" {
		return errors.New("jwt.secret is required (set OPTIMUS_JWT_SECRET)")
	}
	if len(c.JWT.Secret) < 32 {
		return errors.New("jwt.secret must be >= 32 bytes")
	}
	if c.Database.DSN == "" {
		return errors.New("database.dsn is required")
	}
	return nil
}
```

- [ ] **Step 5: Run, verify pass**

```bash
cd optimus-be
go test ./internal/infra/config/... -v
```

Expected: 4 tests pass.

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/infra/config/ optimus-be/configs/
git commit -m "feat(be): config loading (viper + env override + validation)"
```

---

### Task 5: Logger (slog)

**Files:**
- Create: `optimus-be/internal/infra/log/log.go`
- Create: `optimus-be/internal/infra/log/log_test.go`

- [ ] **Step 1: Write failing test**

```go
package log_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/log"
)

func TestNew_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(log.Options{Level: "info", Format: "json", Writer: buf})
	logger.Info("hello", "k", "v")
	require.Contains(t, buf.String(), `"msg":"hello"`)
	require.Contains(t, buf.String(), `"k":"v"`)
}

func TestNew_RespectsLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(log.Options{Level: "warn", Format: "json", Writer: buf})
	logger.Debug("debug-msg")
	logger.Info("info-msg")
	logger.Warn("warn-msg")
	out := buf.String()
	require.False(t, strings.Contains(out, "debug-msg"))
	require.False(t, strings.Contains(out, "info-msg"))
	require.True(t, strings.Contains(out, "warn-msg"))
}

func TestNew_DefaultsToInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(log.Options{Format: "text", Writer: buf})
	logger.Info("v")
	require.Contains(t, buf.String(), "v")
}
```

- [ ] **Step 2: Run, fail**

```bash
go test ./internal/infra/log/...
```

Expected: package missing.

- [ ] **Step 3: Implement `log.go`**

```go
package log

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Level  string
	Format string
	Writer io.Writer
}

func New(opts Options) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	level := parseLevel(opts.Level)
	var handler slog.Handler
	hOpts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(opts.Format, "text") {
		handler = slog.NewTextHandler(w, hOpts)
	} else {
		handler = slog.NewJSONHandler(w, hOpts)
	}
	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
go test ./internal/infra/log/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/log/
git commit -m "feat(be): structured slog wrapper"
```

---

### Task 6: Errors package

**Files:**
- Create: `optimus-be/internal/infra/errors/codes.go`
- Create: `optimus-be/internal/infra/errors/errors.go`
- Create: `optimus-be/internal/infra/errors/errors_test.go`

- [ ] **Step 1: Write `codes.go` (single source of all numeric codes)**

```go
package errors

// Code is the business-level numeric error code returned in response envelope.
type Code int

const (
	CodeOK Code = 0

	// 1xxxx system-level
	CodeInternal       Code = 10001
	CodeDBError        Code = 10002
	CodeTimeout        Code = 10003
	CodeUnauthorized   Code = 10004 // generic auth failure (used internally)

	// 4xxxx client errors (mirror HTTP 4xx)
	CodeBadRequest             Code = 40001
	CodeValidation             Code = 40002
	CodeInvalidCredentials     Code = 40101
	CodeTokenInvalid           Code = 40102
	CodeTokenExpired           Code = 40103
	CodeRefreshTokenReplay     Code = 40104
	CodeForbidden              Code = 40301
	CodePermissionDenied       Code = 40302
	CodeNotFound               Code = 40401
	CodeConflict               Code = 40901
	CodeUserAlreadyExists      Code = 40902
	CodeRoleAlreadyExists      Code = 40903
	CodeMenuAlreadyExists      Code = 40904
	CodeBuiltinRoleImmutable   Code = 40905
	CodeCannotDeleteSelf       Code = 40906
	CodeCannotDeleteAdmin      Code = 40907
	CodeRateLimited            Code = 42901

	// 5xxxx server business errors
	CodeSeedFailed       Code = 50001
	CodePermRegistryErr  Code = 50002
)
```

- [ ] **Step 2: Write test `errors_test.go`**

```go
package errors_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
)

func TestNew_HasCodeAndMessageKey(t *testing.T) {
	e := apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	require.Equal(t, apperr.CodeInvalidCredentials, e.Code)
	require.Equal(t, "auth.invalid_credentials", e.MessageKey)
	require.Equal(t, "invalid username or password", e.Error())
}

func TestWrap_PreservesCause(t *testing.T) {
	cause := errors.New("db dead")
	e := apperr.Wrap(cause, apperr.CodeDBError, "db.error", "database failure")
	require.ErrorIs(t, e, cause)
	require.Equal(t, apperr.CodeDBError, e.Code)
}

func TestAsBizError(t *testing.T) {
	e := apperr.New(apperr.CodeNotFound, "common.not_found", "not found")
	var be *apperr.BizError
	require.True(t, errors.As(e, &be))
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestHTTPStatus_DerivedFromCode(t *testing.T) {
	require.Equal(t, 400, apperr.HTTPStatus(apperr.CodeBadRequest))
	require.Equal(t, 401, apperr.HTTPStatus(apperr.CodeInvalidCredentials))
	require.Equal(t, 403, apperr.HTTPStatus(apperr.CodeForbidden))
	require.Equal(t, 404, apperr.HTTPStatus(apperr.CodeNotFound))
	require.Equal(t, 409, apperr.HTTPStatus(apperr.CodeConflict))
	require.Equal(t, 429, apperr.HTTPStatus(apperr.CodeRateLimited))
	require.Equal(t, 500, apperr.HTTPStatus(apperr.CodeInternal))
}
```

- [ ] **Step 3: Run, fail**

```bash
go test ./internal/infra/errors/...
```

- [ ] **Step 4: Implement `errors.go`**

```go
package errors

import (
	"errors"
	"fmt"
)

type BizError struct {
	Code       Code
	MessageKey string
	Message    string
	Cause      error
}

func New(code Code, messageKey, message string) *BizError {
	return &BizError{Code: code, MessageKey: messageKey, Message: message}
}

func Wrap(cause error, code Code, messageKey, message string) *BizError {
	return &BizError{Code: code, MessageKey: messageKey, Message: message, Cause: cause}
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *BizError) Unwrap() error { return e.Cause }

// AsBiz pulls a BizError out of a wrapped error chain.
func AsBiz(err error) (*BizError, bool) {
	var be *BizError
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// HTTPStatus maps a business Code to an HTTP status code.
// Strategy: the leading digit of the code (after dropping the trailing 3) maps to HTTP.
// Implemented as a switch for clarity & to keep mappings explicit.
func HTTPStatus(c Code) int {
	switch {
	case c == CodeOK:
		return 200
	case c >= 10000 && c < 20000:
		return 500
	case c == CodeRateLimited:
		return 429
	case c >= 40400 && c < 40500:
		return 404
	case c >= 40300 && c < 40400:
		return 403
	case c >= 40100 && c < 40200:
		return 401
	case c >= 40900 && c < 41000:
		return 409
	case c >= 40000 && c < 41000:
		return 400
	case c >= 50000 && c < 60000:
		return 500
	default:
		return 500
	}
}
```

- [ ] **Step 5: Run, pass**

```bash
go test ./internal/infra/errors/... -v
```

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/infra/errors/
git commit -m "feat(be): BizError type + numeric code registry + HTTP mapping"
```

---

### Task 7: Response envelope

**Files:**
- Create: `optimus-be/internal/infra/response/envelope.go`
- Create: `optimus-be/internal/infra/response/envelope_test.go`

- [ ] **Step 1: Write test**

```go
package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

func TestSuccess_WritesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	response.Success(c, gin.H{"hello": "world"})

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Code    int               `json:"code"`
		Data    map[string]string `json:"data"`
		Message string            `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "world", body.Data["hello"])
}

func TestError_WritesEnvelopeWithHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	response.Error(c, apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "bad"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var body struct {
		Code       int    `json:"code"`
		Message    string `json:"message"`
		MessageKey string `json:"message_key"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, int(apperr.CodeInvalidCredentials), body.Code)
	require.Equal(t, "auth.invalid_credentials", body.MessageKey)
}

func TestError_FallsBackToInternalForNonBiz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	response.Error(c, gin.Error{}.Err) // nil err
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
```

- [ ] **Step 2: Run, fail**

```bash
go test ./internal/infra/response/...
```

- [ ] **Step 3: Implement `envelope.go`**

```go
package response

import (
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
)

type Envelope struct {
	Code       int    `json:"code"`
	Data       any    `json:"data"`
	Message    string `json:"message"`
	MessageKey string `json:"message_key,omitempty"`
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:    int(apperr.CodeOK),
		Data:    data,
		Message: "",
	})
}

func Error(c *gin.Context, err error) {
	if err == nil {
		c.JSON(http.StatusInternalServerError, Envelope{
			Code:    int(apperr.CodeInternal),
			Data:    nil,
			Message: "internal server error",
		})
		return
	}
	var be *apperr.BizError
	if stderrors.As(err, &be) {
		c.JSON(apperr.HTTPStatus(be.Code), Envelope{
			Code:       int(be.Code),
			Data:       nil,
			Message:    be.Message,
			MessageKey: be.MessageKey,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, Envelope{
		Code:    int(apperr.CodeInternal),
		Data:    nil,
		Message: err.Error(),
	})
}
```

- [ ] **Step 4: Run, pass**

```bash
go test ./internal/infra/response/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/response/
git commit -m "feat(be): unified response envelope helpers"
```

---

## Phase 3: Database & Migrations (Tasks 8-12)

### Task 8: First migration — users

**Files:** Create: `optimus-be/migrations/00001_create_users.sql`

- [ ] **Step 1: Generate migration file**

```bash
cd optimus-be
goose -dir migrations create create_users sql
```

This creates `migrations/<timestamp>_create_users.sql`. Rename it to `00001_create_users.sql`.

- [ ] **Step 2: Replace content with**

```sql
-- +goose Up
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(64)  NOT NULL,
    email           VARCHAR(128) NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    avatar_url      VARCHAR(512) NOT NULL DEFAULT '',
    status          VARCHAR(16)  NOT NULL DEFAULT 'enabled',
    last_login_at   TIMESTAMPTZ,
    created_by      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_users_deleted_at ON users (deleted_at);

-- +goose Down
DROP TABLE users;
```

- [ ] **Step 3: Apply and verify**

```bash
cd optimus-be
make migrate-up
docker exec optimus-pg psql -U optimus -d optimus -c '\d users'
```

Expected: table listed with all columns.

- [ ] **Step 4: Verify down**

```bash
make migrate-down
make migrate-up
```

Expected: both succeed.

- [ ] **Step 5: Commit**

```bash
git add optimus-be/migrations/00001_create_users.sql
git commit -m "feat(be): migration — create users table"
```

---

### Task 9: Migrations — roles, permissions

**Files:**
- Create: `optimus-be/migrations/00002_create_roles.sql`
- Create: `optimus-be/migrations/00003_create_permissions.sql`

- [ ] **Step 1: Write `00002_create_roles.sql`**

```sql
-- +goose Up
CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(64)  NOT NULL,
    name        VARCHAR(128) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    is_builtin  BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_roles_deleted_at ON roles (deleted_at);

-- +goose Down
DROP TABLE roles;
```

- [ ] **Step 2: Write `00003_create_permissions.sql`**

```sql
-- +goose Up
CREATE TABLE permissions (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(128) NOT NULL UNIQUE,
    name        VARCHAR(128) NOT NULL,
    category    VARCHAR(64)  NOT NULL DEFAULT '',
    description VARCHAR(512) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE permissions;
```

(Note: `permissions` has full UNIQUE since no soft delete.)

- [ ] **Step 3: Apply**

```bash
make migrate-up
docker exec optimus-pg psql -U optimus -d optimus -c '\dt'
```

Expected: tables `goose_db_version`, `permissions`, `roles`, `users`.

- [ ] **Step 4: Commit**

```bash
git add optimus-be/migrations/00002_create_roles.sql optimus-be/migrations/00003_create_permissions.sql
git commit -m "feat(be): migrations — roles, permissions tables"
```

---

### Task 10: Migrations — junction tables

**Files:**
- Create: `optimus-be/migrations/00004_create_user_roles.sql`
- Create: `optimus-be/migrations/00005_create_role_permissions.sql`

- [ ] **Step 1: Write `00004_create_user_roles.sql`**

```sql
-- +goose Up
CREATE TABLE user_roles (
    user_id    BIGINT      NOT NULL,
    role_id    BIGINT      NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);

-- +goose Down
DROP TABLE user_roles;
```

- [ ] **Step 2: Write `00005_create_role_permissions.sql`**

```sql
-- +goose Up
CREATE TABLE role_permissions (
    role_id       BIGINT      NOT NULL,
    permission_id BIGINT      NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);

-- +goose Down
DROP TABLE role_permissions;
```

- [ ] **Step 3: Apply**

```bash
make migrate-up
```

- [ ] **Step 4: Commit**

```bash
git add optimus-be/migrations/00004_create_user_roles.sql optimus-be/migrations/00005_create_role_permissions.sql
git commit -m "feat(be): migrations — user_roles, role_permissions junction tables"
```

---

### Task 11: Migrations — menus, refresh_tokens, audit_logs

**Files:**
- Create: `optimus-be/migrations/00006_create_menus.sql`
- Create: `optimus-be/migrations/00007_create_refresh_tokens.sql`
- Create: `optimus-be/migrations/00008_create_audit_logs.sql`

- [ ] **Step 1: Write `00006_create_menus.sql`**

```sql
-- +goose Up
CREATE TABLE menus (
    id              BIGSERIAL PRIMARY KEY,
    parent_id       BIGINT,
    code            VARCHAR(64)  NOT NULL,
    name            VARCHAR(128) NOT NULL,
    path            VARCHAR(255) NOT NULL DEFAULT '',
    component       VARCHAR(255) NOT NULL DEFAULT '',
    icon            VARCHAR(64)  NOT NULL DEFAULT '',
    permission_code VARCHAR(128),
    sort_order      INT          NOT NULL DEFAULT 0,
    hidden          BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_menus_parent_id  ON menus (parent_id);
CREATE INDEX idx_menus_deleted_at ON menus (deleted_at);

-- +goose Down
DROP TABLE menus;
```

- [ ] **Step 2: Write `00007_create_refresh_tokens.sql`**

```sql
-- +goose Up
CREATE TABLE refresh_tokens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    token_hash  VARCHAR(64) NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    user_agent  VARCHAR(512) NOT NULL DEFAULT '',
    ip          VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user_id   ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_expires   ON refresh_tokens (expires_at);

-- +goose Down
DROP TABLE refresh_tokens;
```

- [ ] **Step 3: Write `00008_create_audit_logs.sql`**

```sql
-- +goose Up
CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT,
    action       VARCHAR(64)  NOT NULL,
    target_type  VARCHAR(64)  NOT NULL DEFAULT '',
    target_id    VARCHAR(64)  NOT NULL DEFAULT '',
    payload      JSONB        NOT NULL DEFAULT '{}'::JSONB,
    ip           VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent   VARCHAR(512) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user_id     ON audit_logs (user_id);
CREATE INDEX idx_audit_logs_action      ON audit_logs (action);
CREATE INDEX idx_audit_logs_created_at  ON audit_logs (created_at);

-- +goose Down
DROP TABLE audit_logs;
```

- [ ] **Step 4: Apply**

```bash
make migrate-up
docker exec optimus-pg psql -U optimus -d optimus -c '\dt'
```

Expected: 8 tables present.

- [ ] **Step 5: Commit**

```bash
git add optimus-be/migrations/00006_create_menus.sql optimus-be/migrations/00007_create_refresh_tokens.sql optimus-be/migrations/00008_create_audit_logs.sql
git commit -m "feat(be): migrations — menus, refresh_tokens, audit_logs tables"
```

---

### Task 12: Migrations — partial unique indexes & foreign keys

**Files:**
- Create: `optimus-be/migrations/00009_partial_unique_indexes.sql`
- Create: `optimus-be/migrations/00010_foreign_keys.sql`

- [ ] **Step 1: Write `00009_partial_unique_indexes.sql`**

```sql
-- +goose Up
CREATE UNIQUE INDEX users_username_uniq ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_email_uniq    ON users (email)    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX roles_code_uniq     ON roles (code)     WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX menus_code_uniq     ON menus (code)     WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX users_username_uniq;
DROP INDEX users_email_uniq;
DROP INDEX roles_code_uniq;
DROP INDEX menus_code_uniq;
```

- [ ] **Step 2: Write `00010_foreign_keys.sql`**

```sql
-- +goose Up
ALTER TABLE audit_logs
    ADD CONSTRAINT fk_audit_logs_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE user_roles
    ADD CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id)
    REFERENCES roles(id) ON DELETE CASCADE;

ALTER TABLE role_permissions
    ADD CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id)
    REFERENCES roles(id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id)
    REFERENCES permissions(id) ON DELETE CASCADE;

ALTER TABLE refresh_tokens
    ADD CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE refresh_tokens   DROP CONSTRAINT fk_refresh_tokens_user;
ALTER TABLE role_permissions DROP CONSTRAINT fk_role_permissions_role, DROP CONSTRAINT fk_role_permissions_permission;
ALTER TABLE user_roles       DROP CONSTRAINT fk_user_roles_user, DROP CONSTRAINT fk_user_roles_role;
ALTER TABLE audit_logs       DROP CONSTRAINT fk_audit_logs_user;
```

- [ ] **Step 3: Apply**

```bash
make migrate-up
docker exec optimus-pg psql -U optimus -d optimus -c '\d users'
```

Expected: shows `users_username_uniq` and `users_email_uniq` as partial indexes (Postgres prints `WHERE` predicate).

- [ ] **Step 4: Verify partial-unique behavior**

```bash
docker exec optimus-pg psql -U optimus -d optimus -c "
INSERT INTO users (username,email,password_hash) VALUES ('foo','f@x','h');
UPDATE users SET deleted_at = NOW() WHERE username='foo';
INSERT INTO users (username,email,password_hash) VALUES ('foo','f@x','h');
SELECT id, username, deleted_at FROM users WHERE username='foo';
DELETE FROM users WHERE username='foo';
"
```

Expected: both inserts succeed, second insert NOT rejected on partial unique constraint, then both rows deleted (hard delete).

- [ ] **Step 5: Commit**

```bash
git add optimus-be/migrations/00009_partial_unique_indexes.sql optimus-be/migrations/00010_foreign_keys.sql
git commit -m "feat(be): migrations — partial unique indexes and foreign keys"
```

---

### Task 13: DB connection + dockertest helper

**Files:**
- Create: `optimus-be/internal/infra/db/db.go`
- Create: `optimus-be/internal/infra/db/dockertest.go`
- Create: `optimus-be/internal/infra/db/db_test.go`

- [ ] **Step 1: Write `db.go`**

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"optimus-be/internal/infra/config"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: false,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("gorm sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	return gdb, nil
}

func Ping(ctx context.Context, gdb *gorm.DB) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

// WithTx runs fn inside a transaction.
func WithTx(gdb *gorm.DB, fn func(tx *gorm.DB) error) error {
	return gdb.Transaction(fn)
}

// AsSQL returns the underlying *sql.DB (for goose).
func AsSQL(gdb *gorm.DB) (*sql.DB, error) { return gdb.DB() }
```

- [ ] **Step 2: Write `dockertest.go` (test helper)**

```go
//go:build dbtest

package db

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// StartTestPostgres boots an ephemeral Postgres in a container,
// runs migrations under <repoRoot>/optimus-be/migrations, returns the GORM DB
// and a teardown function. Caller must defer teardown().
func StartTestPostgres(t *testing.T, migrationsDir string) (*gorm.DB, func()) {
	t.Helper()
	pool, err := dockertest.NewPool("")
	if err != nil { t.Fatalf("dockertest pool: %v", err) }

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil { t.Fatalf("start postgres: %v", err) }

	dsn := fmt.Sprintf(
		"host=localhost port=%s user=test password=test dbname=test sslmode=disable",
		res.GetPort("5432/tcp"),
	)

	pool.MaxWait = 60 * time.Second
	var gdb *gorm.DB
	if err := pool.Retry(func() error {
		var openErr error
		gdb, openErr = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if openErr != nil { return openErr }
		return Ping(context.Background(), gdb)
	}); err != nil {
		_ = pool.Purge(res)
		t.Fatalf("connect postgres: %v", err)
	}

	sqlDB, _ := gdb.DB()
	migrationsAbs, _ := filepath.Abs(migrationsDir)
	if err := goose.SetDialect("postgres"); err != nil { t.Fatal(err) }
	if err := goose.Up(sqlDB, migrationsAbs); err != nil {
		log.Printf("migration up failed: %v", err)
		_ = pool.Purge(res)
		t.Fatal(err)
	}

	teardown := func() { _ = pool.Purge(res) }
	return gdb, teardown
}
```

- [ ] **Step 3: Write `db_test.go`**

```go
//go:build dbtest

package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
)

func TestStartTestPostgres_RunsMigrations(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	require.NoError(t, db.Ping(context.Background(), gdb))

	var count int
	require.NoError(t, gdb.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'").Row().Scan(&count))
	// 8 business tables + goose_db_version = 9
	require.GreaterOrEqual(t, count, 9)
}
```

- [ ] **Step 4: Run integration test**

```bash
cd optimus-be
go test -tags=dbtest ./internal/infra/db/... -v
```

Expected: PASS within ~10-20s (container start dominates).

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/db/
git commit -m "feat(be): GORM connection + dockertest helper"
```

---

## Phase 4: GORM Models (Task 14)

### Task 14: Define all 8 models

**Files:** Create: `optimus-be/internal/models/*.go`

- [ ] **Step 1: Write `models/user.go`**

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           uint64         `gorm:"primaryKey"`
	Username     string         `gorm:"size:64;not null"`
	Email        string         `gorm:"size:128;not null"`
	PasswordHash string         `gorm:"size:255;not null"`
	DisplayName  string         `gorm:"size:128;not null;default:''"`
	AvatarURL    string         `gorm:"size:512;not null;default:''"`
	Status       string         `gorm:"size:16;not null;default:'enabled'"`
	LastLoginAt  *time.Time
	CreatedBy    *uint64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string { return "users" }
```

- [ ] **Step 2: Write `models/role.go`**

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

type Role struct {
	ID          uint64         `gorm:"primaryKey"`
	Code        string         `gorm:"size:64;not null"`
	Name        string         `gorm:"size:128;not null"`
	Description string         `gorm:"size:512;not null;default:''"`
	IsBuiltin   bool           `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Role) TableName() string { return "roles" }
```

- [ ] **Step 3: Write `models/permission.go`**

```go
package models

import "time"

type Permission struct {
	ID          uint64    `gorm:"primaryKey"`
	Code        string    `gorm:"size:128;not null;uniqueIndex"`
	Name        string    `gorm:"size:128;not null"`
	Category    string    `gorm:"size:64;not null;default:''"`
	Description string    `gorm:"size:512;not null;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Permission) TableName() string { return "permissions" }
```

(Permissions are hard-delete + globally unique, so `uniqueIndex` is fine here.)

- [ ] **Step 4: Write `models/user_role.go` and `models/role_permission.go`**

```go
// models/user_role.go
package models

import "time"

type UserRole struct {
	UserID    uint64    `gorm:"primaryKey"`
	RoleID    uint64    `gorm:"primaryKey"`
	CreatedAt time.Time
}

func (UserRole) TableName() string { return "user_roles" }
```

```go
// models/role_permission.go
package models

import "time"

type RolePermission struct {
	RoleID       uint64    `gorm:"primaryKey"`
	PermissionID uint64    `gorm:"primaryKey;column:permission_id"`
	CreatedAt    time.Time
}

func (RolePermission) TableName() string { return "role_permissions" }
```

- [ ] **Step 5: Write `models/menu.go`**

```go
package models

import (
	"time"

	"gorm.io/gorm"
)

type Menu struct {
	ID             uint64         `gorm:"primaryKey"`
	ParentID       *uint64
	Code           string         `gorm:"size:64;not null"`
	Name           string         `gorm:"size:128;not null"`
	Path           string         `gorm:"size:255;not null;default:''"`
	Component      string         `gorm:"size:255;not null;default:''"`
	Icon           string         `gorm:"size:64;not null;default:''"`
	PermissionCode *string        `gorm:"size:128"`
	SortOrder      int            `gorm:"not null;default:0"`
	Hidden         bool           `gorm:"not null;default:false"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (Menu) TableName() string { return "menus" }
```

- [ ] **Step 6: Write `models/refresh_token.go`**

```go
package models

import "time"

type RefreshToken struct {
	ID         uint64     `gorm:"primaryKey"`
	UserID     uint64     `gorm:"not null;index"`
	TokenHash  string     `gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt  time.Time  `gorm:"not null;index"`
	RevokedAt  *time.Time
	UserAgent  string     `gorm:"size:512;not null;default:''"`
	IP         string     `gorm:"size:64;not null;default:''"`
	CreatedAt  time.Time
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
```

- [ ] **Step 7: Write `models/audit_log.go`**

```go
package models

import (
	"time"

	"gorm.io/datatypes"
)

type AuditLog struct {
	ID         uint64         `gorm:"primaryKey"`
	UserID     *uint64        `gorm:"index"`
	Action     string         `gorm:"size:64;not null;index"`
	TargetType string         `gorm:"size:64;not null;default:''"`
	TargetID   string         `gorm:"size:64;not null;default:''"`
	Payload    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	IP         string         `gorm:"size:64;not null;default:''"`
	UserAgent  string         `gorm:"size:512;not null;default:''"`
	CreatedAt  time.Time      `gorm:"index"`
}

func (AuditLog) TableName() string { return "audit_logs" }
```

- [ ] **Step 8: Pull GORM datatypes**

```bash
cd optimus-be
go get gorm.io/datatypes@v1.2.1
go build ./...
```

Expected: clean build.

- [ ] **Step 9: Commit**

```bash
git add optimus-be/internal/models/ optimus-be/go.mod optimus-be/go.sum
git commit -m "feat(be): GORM models for all 8 entities (no uniqueIndex tags on soft-deleted)"
```

---

## Phase 5: Permission Registry (Tasks 15-16)

### Task 15: Permission code constants

**Files:** Create: `optimus-be/internal/infra/permissions/codes.go`

- [ ] **Step 1: Write `codes.go`**

```go
package permissions

// Permission describes a permission code registered at startup.
type Permission struct {
	Code        string
	Name        string // i18n key
	Category    string
	Description string
}

// All P0 permission codes. Future modules append to this list.
var All = []Permission{
	// system: user
	{Code: "system:user:read",       Name: "perm.system.user.read",       Category: "system", Description: "Read users"},
	{Code: "system:user:write",      Name: "perm.system.user.write",      Category: "system", Description: "Create/update users"},
	{Code: "system:user:delete",     Name: "perm.system.user.delete",     Category: "system", Description: "Delete users"},
	{Code: "system:user:reset_pass", Name: "perm.system.user.reset_pass", Category: "system", Description: "Reset user password as admin"},

	// system: role
	{Code: "system:role:read",   Name: "perm.system.role.read",   Category: "system", Description: "Read roles"},
	{Code: "system:role:write",  Name: "perm.system.role.write",  Category: "system", Description: "Create/update roles and bind permissions"},
	{Code: "system:role:delete", Name: "perm.system.role.delete", Category: "system", Description: "Delete roles"},

	// system: permission
	{Code: "system:permission:read", Name: "perm.system.permission.read", Category: "system", Description: "Read permission registry"},

	// system: menu
	{Code: "system:menu:read",   Name: "perm.system.menu.read",   Category: "system", Description: "Read menus"},
	{Code: "system:menu:write",  Name: "perm.system.menu.write",  Category: "system", Description: "Create/update menus"},
	{Code: "system:menu:delete", Name: "perm.system.menu.delete", Category: "system", Description: "Delete menus"},

	// system: audit
	{Code: "system:audit:read", Name: "perm.system.audit.read", Category: "system", Description: "Read audit logs"},
}

// CodeSet returns a set for O(1) membership testing.
func CodeSet() map[string]struct{} {
	out := make(map[string]struct{}, len(All))
	for _, p := range All {
		out[p.Code] = struct{}{}
	}
	return out
}
```

- [ ] **Step 2: Commit**

```bash
git add optimus-be/internal/infra/permissions/codes.go
git commit -m "feat(be): permission code constants for P0"
```

---

### Task 16: Permission registry upsert + drift detection

**Files:**
- Create: `optimus-be/internal/infra/permissions/registry.go`
- Create: `optimus-be/internal/infra/permissions/registry_test.go`

- [ ] **Step 1: Write failing test**

```go
//go:build dbtest

package permissions_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
)

func TestRegister_InsertsAllCodes(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	result, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	require.Equal(t, len(permissions.All), result.Inserted)
	require.Equal(t, 0, result.Updated)
	require.Empty(t, result.Stale)

	var count int64
	gdb.Model(&models.Permission{}).Count(&count)
	require.Equal(t, int64(len(permissions.All)), count)
}

func TestRegister_UpdatesChangedRows(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)

	modified := append([]permissions.Permission{}, permissions.All...)
	modified[0].Description = "NEW DESCRIPTION"

	result, err := permissions.Register(context.Background(), gdb, modified)
	require.NoError(t, err)
	require.Equal(t, 0, result.Inserted)
	require.Equal(t, 1, result.Updated)

	var got models.Permission
	gdb.Where("code = ?", modified[0].Code).First(&got)
	require.Equal(t, "NEW DESCRIPTION", got.Description)
}

func TestRegister_DetectsStaleRows(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	// Seed an extra permission row that's not in our registry.
	gdb.Create(&models.Permission{Code: "obsolete:thing:read", Name: "obsolete", Category: "obsolete"})

	result, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	require.Contains(t, result.Stale, "obsolete:thing:read")
}
```

- [ ] **Step 2: Run, fail**

```bash
go test -tags=dbtest ./internal/infra/permissions/...
```

- [ ] **Step 3: Implement `registry.go`**

```go
package permissions

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type RegisterResult struct {
	Inserted int
	Updated  int
	Stale    []string // codes present in DB but not in registry
}

// Register upserts all permissions in `defs` into the DB and reports rows in DB
// that are no longer declared (stale). It does NOT delete stale rows — that
// must be a deliberate decision (e.g. a separate housekeeping command).
func Register(ctx context.Context, gdb *gorm.DB, defs []Permission) (*RegisterResult, error) {
	result := &RegisterResult{}

	existing := map[string]models.Permission{}
	var rows []models.Permission
	if err := gdb.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load permissions: %w", err)
	}
	for _, r := range rows {
		existing[r.Code] = r
	}

	declared := map[string]struct{}{}
	for _, d := range defs {
		declared[d.Code] = struct{}{}
		cur, ok := existing[d.Code]
		if !ok {
			row := models.Permission{
				Code: d.Code, Name: d.Name, Category: d.Category, Description: d.Description,
			}
			if err := gdb.WithContext(ctx).Create(&row).Error; err != nil {
				return nil, fmt.Errorf("insert %s: %w", d.Code, err)
			}
			result.Inserted++
			continue
		}
		if cur.Name != d.Name || cur.Category != d.Category || cur.Description != d.Description {
			if err := gdb.WithContext(ctx).Model(&models.Permission{}).
				Where("id = ?", cur.ID).
				Updates(map[string]any{
					"name":        d.Name,
					"category":    d.Category,
					"description": d.Description,
				}).Error; err != nil {
				return nil, fmt.Errorf("update %s: %w", d.Code, err)
			}
			result.Updated++
		}
	}

	for code := range existing {
		if _, ok := declared[code]; !ok {
			result.Stale = append(result.Stale, code)
		}
	}
	return result, nil
}
```

- [ ] **Step 4: Run, pass**

```bash
go test -tags=dbtest ./internal/infra/permissions/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/permissions/registry.go optimus-be/internal/infra/permissions/registry_test.go
git commit -m "feat(be): permission registry upsert + stale detection"
```

---

## Phase 6: HTTP Server Bootstrap + Seed (Tasks 17-19)

### Task 17: Minimal HTTP server with /health

**Files:**
- Modify: `optimus-be/cmd/server/main.go`
- Create: `optimus-be/internal/modules/health/handler.go`

- [ ] **Step 1: Write `internal/modules/health/handler.go`**

```go
package health

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
)

type Handler struct {
	DB      *gorm.DB
	Version string
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.health)
}

func (h *Handler) health(c *gin.Context) {
	status := "ok"
	if err := db.Ping(context.Background(), h.DB); err != nil {
		status = "degraded"
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"db":      "down",
			"version": h.Version,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"db": status, "version": h.Version})
}
```

- [ ] **Step 2: Rewrite `cmd/server/main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/health"
)

var (
	Version = "dev" // set via -ldflags at build time
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	checkPerms := flag.Bool("check-permissions", false, "register permission codes and exit")
	flag.Parse()

	abs, _ := filepath.Abs(*cfgPath)
	cfg, err := config.Load(abs)
	if err != nil { fail("load config", err) }
	if err := cfg.ValidateStrict(); err != nil { fail("validate config", err) }

	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})
	logger.Info("optimus-be starting", "version", Version)

	gdb, err := db.Open(cfg.Database)
	if err != nil { fail("open db", err) }

	if _, err := permissions.Register(context.Background(), gdb, permissions.All); err != nil {
		fail("register permissions", err)
	}
	if *checkPerms {
		logger.Info("permissions registered, exiting due to -check-permissions")
		return
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api/v1")
	(&health.Handler{DB: gdb, Version: Version}).Register(api)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "err", err)
	}

	if sqlDB, _ := gdb.DB(); sqlDB != nil {
		_ = sqlDB.Close()
	}
	logger.Info("stopped")
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", stage, err)
	os.Exit(1)
}
```

- [ ] **Step 3: Run server**

```bash
cd optimus-be
make migrate-up
OPTIMUS_JWT_SECRET="dev_secret_must_be_at_least_32_bytes_long_xx" go run ./cmd/server &
SERVER_PID=$!
sleep 2
curl -s http://localhost:8080/api/v1/health
kill $SERVER_PID
```

Expected: JSON `{"db":"ok","version":"dev"}`.

- [ ] **Step 4: Commit**

```bash
git add optimus-be/cmd/server/main.go optimus-be/internal/modules/health/handler.go
git commit -m "feat(be): minimal HTTP server with /health and graceful shutdown"
```

---

### Task 18: Seed command — admin user, builtin roles, initial menus

**Files:**
- Create: `optimus-be/cmd/seed/main.go`
- Create: `optimus-be/internal/seed/seed.go`
- Create: `optimus-be/internal/seed/seed_test.go`

- [ ] **Step 1: Write failing test**

```go
//go:build dbtest

package seed_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/seed"
)

func TestRun_IsIdempotent(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)

	r1, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)
	require.NotEmpty(t, r1.AdminInitialPassword)

	r2, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)
	require.Empty(t, r2.AdminInitialPassword, "second seed must not print a password")

	var users int64
	gdb.Model(&models.User{}).Where("username = ?", "admin").Count(&users)
	require.Equal(t, int64(1), users)

	var roles int64
	gdb.Model(&models.Role{}).Where("is_builtin").Count(&roles)
	require.Equal(t, int64(2), roles, "expected admin + viewer builtin roles")
}

func TestRun_AdminRoleHasAllPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var adminRole models.Role
	require.NoError(t, gdb.Where("code = ?", "admin").First(&adminRole).Error)
	var bound int64
	gdb.Model(&models.RolePermission{}).Where("role_id = ?", adminRole.ID).Count(&bound)
	require.Equal(t, int64(len(permissions.All)), bound)
}

func TestRun_ViewerRoleHasOnlyReadPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	var perms []models.Permission
	gdb.Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", viewer.ID).
		Find(&perms)
	require.NotEmpty(t, perms)
	for _, p := range perms {
		require.Contains(t, p.Code, ":read")
	}
}
```

- [ ] **Step 2: Run, fail**

```bash
go test -tags=dbtest ./internal/seed/...
```

- [ ] **Step 3: Implement `internal/seed/seed.go`**

```go
package seed

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
)

type Options struct {
	AdminUsername string
	AdminEmail    string
	BcryptCost    int // 0 → bcrypt.DefaultCost
}

type Result struct {
	AdminInitialPassword string // populated only on first creation
}

func Run(ctx context.Context, gdb *gorm.DB, opts Options) (*Result, error) {
	if opts.BcryptCost == 0 {
		opts.BcryptCost = bcrypt.DefaultCost
	}
	result := &Result{}

	if err := gdb.Transaction(func(tx *gorm.DB) error {
		if err := ensureBuiltinRoles(ctx, tx); err != nil { return err }
		if err := bindAdminPermissions(ctx, tx); err != nil { return err }
		if err := bindViewerPermissions(ctx, tx); err != nil { return err }
		if err := ensureInitialMenus(ctx, tx); err != nil { return err }
		pw, err := ensureAdminUser(ctx, tx, opts)
		if err != nil { return err }
		result.AdminInitialPassword = pw
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func ensureBuiltinRoles(ctx context.Context, tx *gorm.DB) error {
	roles := []models.Role{
		{Code: "admin",  Name: "role.admin",  Description: "Full access", IsBuiltin: true},
		{Code: "viewer", Name: "role.viewer", Description: "Read-only",    IsBuiltin: true},
	}
	for i := range roles {
		var existing models.Role
		err := tx.WithContext(ctx).Where("code = ?", roles[i].Code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&roles[i]).Error; err != nil { return err }
			continue
		}
		if err != nil { return err }
	}
	return nil
}

func bindAdminPermissions(ctx context.Context, tx *gorm.DB) error {
	var role models.Role
	if err := tx.WithContext(ctx).Where("code = ?", "admin").First(&role).Error; err != nil {
		return err
	}
	var perms []models.Permission
	if err := tx.WithContext(ctx).Find(&perms).Error; err != nil { return err }
	return bindPermsToRole(ctx, tx, role.ID, perms)
}

func bindViewerPermissions(ctx context.Context, tx *gorm.DB) error {
	var role models.Role
	if err := tx.WithContext(ctx).Where("code = ?", "viewer").First(&role).Error; err != nil {
		return err
	}
	var perms []models.Permission
	if err := tx.WithContext(ctx).Where("code LIKE ?", "%:read").Find(&perms).Error; err != nil {
		return err
	}
	return bindPermsToRole(ctx, tx, role.ID, perms)
}

func bindPermsToRole(ctx context.Context, tx *gorm.DB, roleID uint64, perms []models.Permission) error {
	for _, p := range perms {
		rp := models.RolePermission{RoleID: roleID, PermissionID: p.ID}
		// composite PK upsert: ignore on conflict
		if err := tx.WithContext(ctx).Where("role_id = ? AND permission_id = ?", roleID, p.ID).
			FirstOrCreate(&rp).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureInitialMenus(ctx context.Context, tx *gorm.DB) error {
	type spec struct {
		Code, Name, Path, Component, Icon string
		PermissionCode                    *string
		Children                          []spec
	}
	sp := func(s string) *string { return &s }
	tree := []spec{
		{Code: "dashboard", Name: "menu.dashboard", Path: "/dashboard", Component: "dashboard/Index", Icon: "dashboard"},
		{Code: "system", Name: "menu.system", Path: "/system", Component: "", Icon: "setting", Children: []spec{
			{Code: "system.users",       Name: "menu.system.users",       Path: "/system/users",       Component: "system/users/List",      PermissionCode: sp("system:user:read")},
			{Code: "system.roles",       Name: "menu.system.roles",       Path: "/system/roles",       Component: "system/roles/List",      PermissionCode: sp("system:role:read")},
			{Code: "system.permissions", Name: "menu.system.permissions", Path: "/system/permissions", Component: "system/permissions/List", PermissionCode: sp("system:permission:read")},
			{Code: "system.menus",       Name: "menu.system.menus",       Path: "/system/menus",       Component: "system/menus/List",      PermissionCode: sp("system:menu:read")},
			{Code: "system.audit_logs",  Name: "menu.system.audit_logs",  Path: "/system/audit-logs",  Component: "system/audit-logs/List", PermissionCode: sp("system:audit:read")},
		}},
	}
	var insert func(parentID *uint64, nodes []spec) error
	insert = func(parentID *uint64, nodes []spec) error {
		for i, n := range nodes {
			var existing models.Menu
			err := tx.WithContext(ctx).Where("code = ?", n.Code).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				m := models.Menu{
					ParentID: parentID, Code: n.Code, Name: n.Name, Path: n.Path,
					Component: n.Component, Icon: n.Icon,
					PermissionCode: n.PermissionCode,
					SortOrder:      i,
				}
				if err := tx.Create(&m).Error; err != nil { return err }
				existing = m
			} else if err != nil {
				return err
			}
			if len(n.Children) > 0 {
				id := existing.ID
				if err := insert(&id, n.Children); err != nil { return err }
			}
		}
		return nil
	}
	return insert(nil, tree)
}

func ensureAdminUser(ctx context.Context, tx *gorm.DB, opts Options) (string, error) {
	var existing models.User
	err := tx.WithContext(ctx).Where("username = ?", opts.AdminUsername).First(&existing).Error
	if err == nil {
		return "", nil // already exists; do not print a password
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	pw, err := randomPassword(24)
	if err != nil { return "", err }
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), opts.BcryptCost)
	if err != nil { return "", err }
	u := models.User{
		Username:     opts.AdminUsername,
		Email:        opts.AdminEmail,
		PasswordHash: string(hash),
		DisplayName:  "Administrator",
		Status:       "enabled",
	}
	if err := tx.Create(&u).Error; err != nil { return "", err }

	var adminRole models.Role
	if err := tx.WithContext(ctx).Where("code = ?", "admin").First(&adminRole).Error; err != nil {
		return "", err
	}
	if err := tx.Create(&models.UserRole{UserID: u.ID, RoleID: adminRole.ID}).Error; err != nil {
		return "", err
	}
	return pw, nil
}

func randomPassword(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil { return "", err }
	s := base64.RawURLEncoding.EncodeToString(buf)
	return strings.TrimRight(s, "="), nil
}
```

- [ ] **Step 4: Implement `cmd/seed/main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/seed"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	flag.Parse()

	abs, _ := filepath.Abs(*cfgPath)
	cfg, err := config.Load(abs)
	if err != nil { die("load config", err) }
	if err := cfg.ValidateStrict(); err != nil { die("validate config", err) }
	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})

	gdb, err := db.Open(cfg.Database)
	if err != nil { die("open db", err) }

	if r, err := permissions.Register(context.Background(), gdb, permissions.All); err != nil {
		die("register permissions", err)
	} else {
		logger.Info("permissions registered", "inserted", r.Inserted, "updated", r.Updated, "stale", r.Stale)
	}

	res, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: cfg.Boot.AdminUsername,
		AdminEmail:    cfg.Boot.AdminEmail,
		BcryptCost:    cfg.Auth.BcryptCost,
	})
	if err != nil { die("seed", err) }

	if res.AdminInitialPassword != "" {
		logger.Warn(
			"INITIAL ADMIN CREDENTIALS — RECORD THESE NOW (printed only once)",
			"username", cfg.Boot.AdminUsername,
			"password", res.AdminInitialPassword,
		)
	} else {
		logger.Info("admin user already exists; no password generated")
	}
}

func die(stage string, err error) {
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", stage, err)
	os.Exit(1)
}
```

- [ ] **Step 5: Run unit test**

```bash
go test -tags=dbtest ./internal/seed/... -v
```

Expected: 3 tests pass.

- [ ] **Step 6: Run end-to-end**

```bash
cd optimus-be
make migrate-down 2>/dev/null || true
make migrate-up
OPTIMUS_JWT_SECRET="dev_secret_must_be_at_least_32_bytes_long_xx" make seed
```

Expected: log line contains `INITIAL ADMIN CREDENTIALS` with a base64 password.

- [ ] **Step 7: Rerun to verify idempotence**

```bash
OPTIMUS_JWT_SECRET="dev_secret_must_be_at_least_32_bytes_long_xx" make seed
docker exec optimus-pg psql -U optimus -d optimus -c "SELECT username FROM users; SELECT code FROM roles; SELECT COUNT(*) FROM menus;"
```

Expected: only one `admin` user, no new password printed; roles `admin`/`viewer`; menus count = 7 (2 top-level + 5 children).

- [ ] **Step 8: Commit**

```bash
git add optimus-be/cmd/seed/ optimus-be/internal/seed/
git commit -m "feat(be): seed command — admin user, builtin roles, initial menus, role-permission bindings"
```

---

### Task 19: Lint config + initial CI sanity

**Files:** Create: `optimus-be/.golangci.yml`

- [ ] **Step 1: Write `.golangci.yml`**

```yaml
run:
  timeout: 5m

linters:
  enable:
    - govet
    - errcheck
    - staticcheck
    - unused
    - gocritic
    - revive
    - gofmt
    - goimports

issues:
  exclude-rules:
    - path: _test\.go
      linters: [errcheck]
```

- [ ] **Step 2: Run**

```bash
cd optimus-be
golangci-lint run
```

Expected: 0 issues (or minor goimports nits — fix them).

- [ ] **Step 3: Run all unit tests**

```bash
cd optimus-be
go test ./...
```

Expected: all green.

- [ ] **Step 4: Run all integration tests**

```bash
cd optimus-be
go test -tags=dbtest ./...
```

Expected: all green (within 60s total).

- [ ] **Step 5: Commit**

```bash
git add optimus-be/.golangci.yml
git commit -m "chore(be): golangci-lint config"
```

---

## Acceptance Checklist (end of Plan 1A)

- [ ] `docker compose up -d` brings up Postgres healthy
- [ ] `cd optimus-be && make migrate-up` runs all 10 migrations cleanly
- [ ] `make migrate-down` reverses all 10 migrations cleanly, `make migrate-up` re-runs successfully
- [ ] `OPTIMUS_JWT_SECRET=<32+ bytes> make seed` creates admin user (first time prints credentials, second time is a no-op)
- [ ] DB after seed: 1 admin user, 2 builtin roles, 7 menus, permissions registry matches `permissions.All`
- [ ] `OPTIMUS_JWT_SECRET=<32+ bytes> make run` starts server; `curl localhost:8080/api/v1/health` returns `{"db":"ok","version":"dev"}`
- [ ] Hard-delete + reinsert with same username works (verified via partial unique index in Task 12 Step 4)
- [ ] `go test ./...` green; `go test -tags=dbtest ./...` green; `golangci-lint run` clean

When all boxes ticked, ping me to start **Plan 1B (Backend Auth + RBAC)**.
