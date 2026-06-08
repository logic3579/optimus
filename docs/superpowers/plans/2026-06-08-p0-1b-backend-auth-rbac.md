# P0 Plan 1B — Backend Auth + RBAC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up `optimus-be`'s HTTP middleware chain, JWT-based authentication (login / refresh with rotation + replay detection / logout), RBAC middleware + permission cache, and the `/me` family of endpoints. End state: curl can drive a complete login → protected request → refresh → replay-detection cycle.

**Architecture:** Gin middleware stack: RequestID → Logger → Recover → CORS → I18n → (JWT for protected routes) → (RBAC per-route). Bcrypt for passwords, HS256 JWT for access tokens, sha256-hashed random 256-bit refresh tokens stored in DB. Permission cache uses `sync.Map` with 60s TTL — single-process only (P0). Login is rate-limited per-IP and per-username using `golang.org/x/time/rate` token buckets in-memory.

**Tech Stack:** Go 1.22+, Gin 1.10, GORM 1.25, `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, `golang.org/x/time/rate`, `crypto/rand`, `crypto/sha256`. Tests use testify + dockertest (re-uses 1A helper).

**Scope:** No user/role/menu/audit CRUD yet — those land in Plan 1C. /me/menus and /me/permissions DO ship here because they're how the frontend bootstraps after login. Audit emission inside auth (login success/fail/refresh replay) lands here as direct inserts; the audit module's read API is Plan 1C.

**Spec:** `docs/superpowers/specs/2026-06-05-p0-platform-skeleton-design.md` §7 (API), §7.3 (token), §7.4 (RBAC), §7.5 (middleware), §7.6 (security).

**Prerequisites:** Plan 1A complete (config, log, errors, response, db, models, permissions registry, seed, /health all in place and tests green).

---

## File Structure (1B scope)

```
optimus-be/
├── internal/
│   ├── infra/
│   │   ├── crypto/
│   │   │   ├── bcrypt.go            # Hash / Compare
│   │   │   ├── jwt.go               # Sign / Verify (HS256)
│   │   │   ├── bcrypt_test.go
│   │   │   └── jwt_test.go
│   │   ├── middleware/
│   │   │   ├── context_keys.go      # context key constants
│   │   │   ├── request_id.go
│   │   │   ├── logger.go
│   │   │   ├── recover.go
│   │   │   ├── cors.go
│   │   │   ├── i18n.go
│   │   │   ├── jwt_auth.go
│   │   │   ├── rbac.go
│   │   │   └── *_test.go
│   │   └── ratelimit/
│   │       ├── login_limiter.go
│   │       └── login_limiter_test.go
│   └── modules/
│       ├── auth/
│       │   ├── repo.go              # refresh_tokens CRUD
│       │   ├── service.go           # Login / Refresh / Logout
│       │   ├── handler.go
│       │   ├── dto.go
│       │   └── *_test.go
│       └── rbac/
│           ├── cache.go             # permission cache (sync.Map + TTL)
│           ├── service.go           # MeService: GetUser, ListMenus, ListPermissions
│           ├── handler.go           # /me, /me/menus, /me/permissions
│           ├── dto.go
│           └── *_test.go
├── cmd/server/main.go               # Modified: wire full middleware chain + auth + me routes
└── tests/integration/
    └── auth_e2e_test.go             # full flow smoke test
```

---

## Phase 1: HTTP Middleware Chain (Tasks 1-5)

### Task 1: Context keys + RequestID middleware

**Files:**
- Create: `optimus-be/internal/infra/middleware/context_keys.go`
- Create: `optimus-be/internal/infra/middleware/request_id.go`
- Create: `optimus-be/internal/infra/middleware/request_id_test.go`

- [ ] **Step 1: Write context_keys.go**

```go
package middleware

// Context key strings used across middleware to set/read request-scoped values.
// Strings (not custom types) because gin's c.Set/c.Get takes string keys.
const (
	CtxKeyRequestID = "x-request-id"
	CtxKeyUserID    = "x-user-id"     // populated by JWT middleware
	CtxKeyLang      = "x-lang"        // populated by I18n middleware
)
```

- [ ] **Step 2: Write failing test request_id_test.go**

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	var captured string
	r.GET("/", func(c *gin.Context) {
		captured = c.GetString(middleware.CtxKeyRequestID)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	require.Len(t, captured, 32) // hex of 16 bytes
	require.Equal(t, captured, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_PreservesInbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	var captured string
	r.GET("/", func(c *gin.Context) {
		captured = c.GetString(middleware.CtxKeyRequestID)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "client-supplied-id")
	r.ServeHTTP(rec, req)

	require.Equal(t, "client-supplied-id", captured)
	require.Equal(t, "client-supplied-id", rec.Header().Get("X-Request-ID"))
}
```

- [ ] **Step 3: Run, verify fail**

```bash
cd optimus-be && go test ./internal/infra/middleware/...
```

Expected: build error.

- [ ] **Step 4: Write request_id.go**

```go
package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const headerRequestID = "X-Request-ID"

// RequestID sets an X-Request-ID on the request context (and echoes it in the response).
// Preserves inbound value if the client sent one; otherwise generates a 16-byte hex.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(headerRequestID)
		if rid == "" {
			rid = newRequestID()
		}
		c.Set(CtxKeyRequestID, rid)
		c.Writer.Header().Set(headerRequestID, rid)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand cannot fail in practice on supported platforms
	return hex.EncodeToString(b)
}
```

- [ ] **Step 5: Run, verify pass**

```bash
cd optimus-be && go test ./internal/infra/middleware/... -v
```

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): RequestID middleware + context key constants

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Logger middleware (slog structured)

**Files:**
- Create: `optimus-be/internal/infra/middleware/logger.go`
- Create: `optimus-be/internal/infra/middleware/logger_test.go`

- [ ] **Step 1: Write logger_test.go**

```go
package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

func TestLogger_EmitsStructuredEntry(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.GET("/things/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/things/42", nil)
	req.Header.Set("X-Request-ID", "abc")
	r.ServeHTTP(rec, req)

	out := buf.String()
	require.Contains(t, out, `"method":"GET"`)
	require.Contains(t, out, `"path":"/things/42"`)
	require.Contains(t, out, `"status":200`)
	require.Contains(t, out, `"request_id":"abc"`)
	require.Contains(t, out, `"latency_ms"`)
}

func TestLogger_LogsErrorForStatus5xx(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	require.Contains(t, buf.String(), `"level":"ERROR"`)
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/middleware/...
```

- [ ] **Step 3: Write logger.go**

```go
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a middleware that emits one structured log line per request.
// 4xx is logged at Info, 5xx at Error, otherwise Info.
func Logger(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latencyMs := time.Since(start).Milliseconds()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", latencyMs,
			"request_id", c.GetString(CtxKeyRequestID),
			"remote_ip", c.ClientIP(),
		}
		if uid := c.GetUint64(CtxKeyUserID); uid != 0 {
			attrs = append(attrs, "user_id", uid)
		}
		switch {
		case c.Writer.Status() >= 500:
			l.Error("http.request", attrs...)
		default:
			l.Info("http.request", attrs...)
		}
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/middleware/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): Logger middleware (structured slog)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Recover middleware

**Files:**
- Create: `optimus-be/internal/infra/middleware/recover.go`
- Create: `optimus-be/internal/infra/middleware/recover_test.go`

- [ ] **Step 1: Write recover_test.go**

```go
package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
)

func TestRecover_ConvertsPanicToInternalEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recover(logger))
	r.GET("/boom", func(c *gin.Context) { panic("oops") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(apperr.CodeInternal), body["code"])
	require.Equal(t, "internal server error", body["message"])

	// Panic detail is in the log, NOT the response.
	require.Contains(t, buf.String(), `"panic":"oops"`)
	require.NotContains(t, rec.Body.String(), "oops")
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/middleware/...
```

- [ ] **Step 3: Write recover.go**

```go
package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

// Recover catches panics, logs them with stack, and returns a generic 500 envelope.
// The panic value is NEVER written to the response body.
func Recover(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				l.Error("panic recovered",
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
					"request_id", c.GetString(CtxKeyRequestID),
					"path", c.Request.URL.Path,
				)
				if !c.Writer.Written() {
					response.Error(c, apperr.New(apperr.CodeInternal, "common.internal", "internal server error"))
				}
				c.Abort()
			}
		}()
		_ = http.StatusOK // silence unused if other refactors trim this
		c.Next()
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/middleware/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): Recover middleware (panic → 500 envelope + slog)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: CORS middleware

**Files:**
- Create: `optimus-be/internal/infra/middleware/cors.go`
- Create: `optimus-be/internal/infra/middleware/cors_test.go`

- [ ] **Step 1: Write cors_test.go**

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/middleware"
)

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(config.CORSConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST"},
	}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(rec, req)

	require.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_BlocksUnknownOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(config.CORSConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET"},
	}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	r.ServeHTTP(rec, req)

	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_HandlesPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.CORS(config.CORSConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}))
	r.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/middleware/...
```

- [ ] **Step 3: Write cors.go**

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
)

// CORS implements a minimal CORS middleware. Origin is whitelisted exactly
// (no wildcards). Preflight (OPTIONS) requests get an empty 204 response.
func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	allowed := map[string]struct{}{}
	for _, o := range cfg.AllowedOrigins {
		allowed[o] = struct{}{}
	}
	methods := strings.Join(cfg.AllowedMethods, ", ")
	credentials := "false"
	if cfg.AllowCredentials {
		credentials = "true"
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Methods", methods)
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language, X-Request-ID")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", credentials)
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/middleware/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): CORS middleware (origin whitelist + preflight)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: I18n middleware

**Files:**
- Create: `optimus-be/internal/infra/middleware/i18n.go`
- Create: `optimus-be/internal/infra/middleware/i18n_test.go`

- [ ] **Step 1: Write i18n_test.go**

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/middleware"
)

func mkI18n() gin.HandlerFunc {
	return middleware.I18n(config.I18nConfig{
		DefaultLang: "zh-CN",
		Supported:   []string{"zh-CN", "en-US"},
	})
}

func TestI18n_UsesAcceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mkI18n())
	var lang string
	r.GET("/", func(c *gin.Context) { lang = c.GetString(middleware.CtxKeyLang); c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.ServeHTTP(rec, req)
	require.Equal(t, "en-US", lang)
}

func TestI18n_FallsBackToDefaultForUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mkI18n())
	var lang string
	r.GET("/", func(c *gin.Context) { lang = c.GetString(middleware.CtxKeyLang); c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "fr-FR")
	r.ServeHTTP(rec, req)
	require.Equal(t, "zh-CN", lang)
}

func TestI18n_NoHeaderUsesDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mkI18n())
	var lang string
	r.GET("/", func(c *gin.Context) { lang = c.GetString(middleware.CtxKeyLang); c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, "zh-CN", lang)
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/middleware/...
```

- [ ] **Step 3: Write i18n.go**

```go
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
)

// I18n parses Accept-Language and picks the first supported tag; falls back to default.
// Simple greedy parser — no q-value math, no BCP47 negotiation.
func I18n(cfg config.I18nConfig) gin.HandlerFunc {
	supported := map[string]struct{}{}
	for _, s := range cfg.Supported {
		supported[s] = struct{}{}
	}
	return func(c *gin.Context) {
		lang := cfg.DefaultLang
		raw := c.GetHeader("Accept-Language")
		if raw != "" {
			for _, part := range strings.Split(raw, ",") {
				tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
				if _, ok := supported[tag]; ok {
					lang = tag
					break
				}
			}
		}
		c.Set(CtxKeyLang, lang)
		c.Next()
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/middleware/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): I18n middleware (Accept-Language parsing)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 2: Crypto Primitives (Tasks 6-7)

### Task 6: Bcrypt helper

**Files:**
- Create: `optimus-be/internal/infra/crypto/bcrypt.go`
- Create: `optimus-be/internal/infra/crypto/bcrypt_test.go`

- [ ] **Step 1: Write bcrypt_test.go**

```go
package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
)

func TestHashPassword_VerifiesWithCompare(t *testing.T) {
	h, err := crypto.HashPassword("s3cret", 4) // cost 4 for fast tests
	require.NoError(t, err)
	require.NotEqual(t, "s3cret", h)
	require.NoError(t, crypto.ComparePassword(h, "s3cret"))
}

func TestComparePassword_RejectsWrong(t *testing.T) {
	h, _ := crypto.HashPassword("s3cret", 4)
	require.Error(t, crypto.ComparePassword(h, "wrong"))
}

func TestHashPassword_DistinctHashesForSameInput(t *testing.T) {
	h1, _ := crypto.HashPassword("s3cret", 4)
	h2, _ := crypto.HashPassword("s3cret", 4)
	require.NotEqual(t, h1, h2, "bcrypt must produce per-call salt")
}

func TestHashPassword_RejectsEmpty(t *testing.T) {
	_, err := crypto.HashPassword("", 4)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/crypto/...
```

- [ ] **Step 3: Write bcrypt.go**

```go
package crypto

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrEmptyPassword is returned when an empty password is supplied.
var ErrEmptyPassword = errors.New("password is empty")

// HashPassword returns a bcrypt hash for the given plaintext at the given cost.
// Cost <= 0 is treated as bcrypt.DefaultCost.
func HashPassword(plain string, cost int) (string, error) {
	if plain == "" {
		return "", ErrEmptyPassword
	}
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ComparePassword returns nil if plain matches hash, or a non-nil error otherwise.
func ComparePassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/crypto/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/crypto/
git commit -m "feat(be): bcrypt password helpers

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: JWT helper (HS256 sign/verify)

**Files:**
- Create: `optimus-be/internal/infra/crypto/jwt.go`
- Create: `optimus-be/internal/infra/crypto/jwt_test.go`

- [ ] **Step 1: Add dependency**

```bash
cd optimus-be
go get github.com/golang-jwt/jwt/v5@v5.2.1
```

- [ ] **Step 2: Write jwt_test.go**

```go
package crypto_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
)

const testSecret = "test_secret_must_be_at_least_32_bytes_!!"

func TestJWT_SignAndVerifyRoundTrip(t *testing.T) {
	signer := crypto.NewJWTSigner(testSecret)
	tok, err := signer.Sign(crypto.JWTClaims{UserID: 42, JTI: "j1"}, 5*time.Minute)
	require.NoError(t, err)
	require.Contains(t, tok, ".") // header.payload.signature

	claims, err := signer.Verify(tok)
	require.NoError(t, err)
	require.Equal(t, uint64(42), claims.UserID)
	require.Equal(t, "j1", claims.JTI)
}

func TestJWT_RejectsTamperedToken(t *testing.T) {
	signer := crypto.NewJWTSigner(testSecret)
	tok, _ := signer.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, 5*time.Minute)
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	tampered := parts[0] + "." + parts[1] + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err := signer.Verify(tampered)
	require.Error(t, err)
}

func TestJWT_RejectsExpiredToken(t *testing.T) {
	signer := crypto.NewJWTSigner(testSecret)
	tok, _ := signer.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, -1*time.Second)
	_, err := signer.Verify(tok)
	require.Error(t, err)
}

func TestJWT_RejectsDifferentSecret(t *testing.T) {
	a := crypto.NewJWTSigner(testSecret)
	b := crypto.NewJWTSigner("different_secret_at_least_32_bytes_wxyz!")
	tok, _ := a.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, 5*time.Minute)
	_, err := b.Verify(tok)
	require.Error(t, err)
}
```

- [ ] **Step 3: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/crypto/...
```

- [ ] **Step 4: Write jwt.go**

```go
package crypto

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims is the application-specific claims set.
type JWTClaims struct {
	UserID uint64 `json:"uid"`
	JTI    string `json:"jti"`
}

type fullClaims struct {
	JWTClaims
	jwt.RegisteredClaims
}

// JWTSigner signs and verifies HS256 JWTs with a fixed secret.
type JWTSigner struct {
	secret []byte
}

// NewJWTSigner returns a signer; secret should be >= 32 bytes (validated upstream).
func NewJWTSigner(secret string) *JWTSigner {
	return &JWTSigner{secret: []byte(secret)}
}

// Sign issues a token valid for `ttl`. Negative ttl is allowed for tests.
func (s *JWTSigner) Sign(c JWTClaims, ttl time.Duration) (string, error) {
	now := time.Now()
	fc := fullClaims{
		JWTClaims: c,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, fc)
	return tok.SignedString(s.secret)
}

// Verify parses and validates the token. Returns the application claims.
func (s *JWTSigner) Verify(raw string) (*JWTClaims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &fullClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := parsed.Claims.(*fullClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return &c.JWTClaims, nil
}
```

- [ ] **Step 5: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/crypto/... -v
```

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/infra/crypto/ optimus-be/go.mod optimus-be/go.sum
git commit -m "feat(be): JWT signer/verifier (HS256)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 3: Login Rate Limiter (Task 8)

### Task 8: Login rate limiter

**Files:**
- Create: `optimus-be/internal/infra/ratelimit/login_limiter.go`
- Create: `optimus-be/internal/infra/ratelimit/login_limiter_test.go`

- [ ] **Step 1: Write login_limiter_test.go**

```go
package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/ratelimit"
)

func TestLoginLimiter_AllowsUnderQuota(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		require.True(t, lim.Allow("1.2.3.4", "alice"), "attempt %d should be allowed", i+1)
	}
}

func TestLoginLimiter_BlocksAfterIPQuota(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		lim.Allow("1.2.3.4", "alice")
	}
	// 4th attempt from same IP, even with different username, blocked
	require.False(t, lim.Allow("1.2.3.4", "bob"))
}

func TestLoginLimiter_BlocksAfterUsernameQuota(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(3, time.Minute, time.Minute)
	for i := 0; i < 3; i++ {
		lim.Allow("1.2.3.4", "alice")
		lim.Allow("9.9.9.9", "alice") // bump per-username from each IP
	}
	require.False(t, lim.Allow("5.5.5.5", "alice"), "should be blocked by username quota")
}

func TestLoginLimiter_IsolatesDifferentKeys(t *testing.T) {
	lim := ratelimit.NewLoginLimiter(2, time.Minute, time.Minute)
	require.True(t, lim.Allow("1.1.1.1", "alice"))
	require.True(t, lim.Allow("1.1.1.1", "alice"))
	require.False(t, lim.Allow("1.1.1.1", "alice"))
	// Different IP + username pair is independent
	require.True(t, lim.Allow("2.2.2.2", "bob"))
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/ratelimit/...
```

- [ ] **Step 3: Write login_limiter.go**

```go
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LoginLimiter enforces per-IP AND per-username quotas. Allow returns false if either is exhausted.
// In-memory only — single process, no Redis. Sufficient for P0 internal tool.
type LoginLimiter struct {
	quota  int
	window time.Duration
	mu     sync.Mutex
	ip     map[string]*rate.Limiter
	user   map[string]*rate.Limiter
}

// NewLoginLimiter creates a limiter where each (ip OR username) gets `quota` attempts per `window`.
// `block` is currently informational (we don't track explicit block durations — token bucket suffices).
func NewLoginLimiter(quota int, window, block time.Duration) *LoginLimiter {
	_ = block // reserved for future explicit-block enforcement
	return &LoginLimiter{
		quota:  quota,
		window: window,
		ip:     map[string]*rate.Limiter{},
		user:   map[string]*rate.Limiter{},
	}
}

func (l *LoginLimiter) getOrCreate(m map[string]*rate.Limiter, key string) *rate.Limiter {
	if r, ok := m[key]; ok {
		return r
	}
	// rate.Limit = events per second; we want `quota` per `window`, so the rate is quota/window seconds.
	r := rate.NewLimiter(rate.Limit(float64(l.quota)/l.window.Seconds()), l.quota)
	m[key] = r
	return r
}

// Allow returns true if the attempt is within both per-IP and per-username quotas.
// Both buckets are debited only if BOTH have capacity (atomic — avoids one-sided spend).
func (l *LoginLimiter) Allow(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	ipLim := l.getOrCreate(l.ip, ip)
	userLim := l.getOrCreate(l.user, username)

	// Peek both before consuming to keep them aligned.
	if ipLim.Tokens() < 1 || userLim.Tokens() < 1 {
		return false
	}
	if !ipLim.Allow() {
		return false
	}
	if !userLim.Allow() {
		// Restore the ipLim token we just spent — best-effort by waiting briefly is wrong; the
		// simplest correct approach is to consume both only when both pass. Use AllowN at time t.
		// For simplicity we re-add by resetting Tokens via consuming-from-zero is not possible;
		// instead we accept one-token mis-accounting on the rare boundary. Document and move on.
		return false
	}
	return true
}
```

(Note: a more precise atomic two-bucket spend is possible but the boundary case is extremely rare for a login limiter and the test set doesn't exercise it. If you want strict atomicity, replace with a single counter map keyed by "ip|user" — but per-IP and per-username need separate keys per the spec. Accept the rare drift for P0.)

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/ratelimit/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/ratelimit/
git commit -m "feat(be): login rate limiter (per-IP and per-username, in-memory)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 4: Auth Module — Refresh Token Repo (Task 9)

### Task 9: Auth repo

**Files:**
- Create: `optimus-be/internal/modules/auth/repo.go`
- Create: `optimus-be/internal/modules/auth/repo_test.go`

- [ ] **Step 1: Write failing test repo_test.go**

```go
//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func seedUser(t *testing.T, gdb *gorm.DB, username string) uint64 {
	t.Helper()
	u := &models.User{Username: username, Email: username + "@x.io", PasswordHash: "x", Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	return u.ID
}

func TestRepo_CreateAndFindByHash(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUser(t, gdb, "alice")

	repo := auth.NewRepo(gdb)
	exp := time.Now().Add(7 * 24 * time.Hour)
	rt, err := repo.CreateRefreshToken(context.Background(), uid, "hashhash", exp, "ua", "1.1.1.1")
	require.NoError(t, err)
	require.NotZero(t, rt.ID)

	got, err := repo.FindRefreshTokenByHash(context.Background(), "hashhash")
	require.NoError(t, err)
	require.Equal(t, rt.ID, got.ID)
	require.Equal(t, uid, got.UserID)
}

func TestRepo_FindMissingReturnsError(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	repo := auth.NewRepo(gdb)
	_, err := repo.FindRefreshTokenByHash(context.Background(), "nope")
	require.Error(t, err)
}

func TestRepo_RevokeMarksRevokedAt(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUser(t, gdb, "alice")
	repo := auth.NewRepo(gdb)
	rt, _ := repo.CreateRefreshToken(context.Background(), uid, "h1", time.Now().Add(time.Hour), "", "")

	require.NoError(t, repo.RevokeRefreshToken(context.Background(), rt.ID))

	var got models.RefreshToken
	gdb.First(&got, rt.ID)
	require.NotNil(t, got.RevokedAt)
}

func TestRepo_RevokeAllForUserMarksAll(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUser(t, gdb, "alice")
	other := seedUser(t, gdb, "bob")
	repo := auth.NewRepo(gdb)
	_, _ = repo.CreateRefreshToken(context.Background(), uid, "h1", time.Now().Add(time.Hour), "", "")
	_, _ = repo.CreateRefreshToken(context.Background(), uid, "h2", time.Now().Add(time.Hour), "", "")
	_, _ = repo.CreateRefreshToken(context.Background(), other, "h3", time.Now().Add(time.Hour), "", "")

	require.NoError(t, repo.RevokeAllRefreshTokensForUser(context.Background(), uid))

	var revoked int64
	gdb.Model(&models.RefreshToken{}).Where("user_id = ? AND revoked_at IS NOT NULL", uid).Count(&revoked)
	require.Equal(t, int64(2), revoked)

	var otherActive int64
	gdb.Model(&models.RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", other).Count(&otherActive)
	require.Equal(t, int64(1), otherActive)
}
```

Also at the top of the test file, add the gorm import. The full imports block is:

```go
import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/auth/...
```

- [ ] **Step 3: Write repo.go**

```go
package auth

import (
	"context"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) CreateRefreshToken(ctx context.Context, userID uint64, hash string, expiresAt time.Time, ua, ip string) (*models.RefreshToken, error) {
	rt := &models.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		UserAgent: ua,
		IP:        ip,
	}
	if err := r.db.WithContext(ctx).Create(rt).Error; err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *Repo) FindRefreshTokenByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	if err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *Repo) RevokeRefreshToken(ctx context.Context, id uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("id = ?", id).
		Update("revoked_at", &now).Error
}

func (r *Repo) RevokeAllRefreshTokensForUser(ctx context.Context, userID uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", &now).Error
}

// FindUserByUsername loads the user by username (only non-deleted, status='enabled').
func (r *Repo) FindUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("username = ? AND status = ?", username, "enabled").First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateLastLogin updates last_login_at to the given time.
func (r *Repo) UpdateLastLogin(ctx context.Context, userID uint64, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		Update("last_login_at", &at).Error
}

// InsertAuditLog writes an audit row inline (no separate audit module yet).
func (r *Repo) InsertAuditLog(ctx context.Context, userID *uint64, action, ip, ua string, payload []byte) error {
	if payload == nil {
		payload = []byte("{}")
	}
	return r.db.WithContext(ctx).Create(&models.AuditLog{
		UserID:    userID,
		Action:    action,
		IP:        ip,
		UserAgent: ua,
		Payload:   payload,
	}).Error
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/auth/... -v -timeout 180s
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/modules/auth/
git commit -m "feat(be): auth repo — refresh token CRUD + user lookup + audit insert

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 5: Auth Service (Tasks 10-12)

### Task 10: Auth service — Login

**Files:**
- Create: `optimus-be/internal/modules/auth/dto.go`
- Create: `optimus-be/internal/modules/auth/service.go`
- Create: `optimus-be/internal/modules/auth/service_login_test.go`

- [ ] **Step 1: Write dto.go**

```go
package auth

import "time"

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"` // access token exp
}
```

- [ ] **Step 2: Write failing test service_login_test.go**

```go
//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func mkSvc(t *testing.T, gdb *gorm.DB) *auth.Service {
	t.Helper()
	return auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: 7 * 24 * time.Hour, BcryptCost: 4},
	)
}

func seedUserWithPassword(t *testing.T, gdb *gorm.DB, username, password string) uint64 {
	t.Helper()
	h, err := crypto.HashPassword(password, 4)
	require.NoError(t, err)
	u := &models.User{Username: username, Email: username + "@x.io", PasswordHash: h, Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	return u.ID
}

func TestLogin_SuccessReturnsTokens(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := mkSvc(t, gdb)
	pair, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), pair.ExpiresAt, 5*time.Second)

	// last_login_at updated
	var u models.User
	gdb.First(&u, uid)
	require.NotNil(t, u.LastLoginAt)
}

func TestLogin_WrongPasswordReturnsInvalidCredentials(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := mkSvc(t, gdb)
	_, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "WRONG"}, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeInvalidCredentials, be.Code)
}

func TestLogin_UnknownUserReturnsInvalidCredentials(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := mkSvc(t, gdb)
	_, err := svc.Login(context.Background(), auth.LoginRequest{Username: "nobody", Password: "x"}, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeInvalidCredentials, be.Code)
}

func TestLogin_RateLimitsExcessAttempts(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(2, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)

	// 2 wrong attempts allowed, 3rd is rate-limited (even though credentials are correct)
	for i := 0; i < 2; i++ {
		_, _ = svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "WRONG"}, "1.1.1.1", "ua")
	}
	_, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeRateLimited, be.Code)
}
```

(Imports block of service_login_test.go also needs `"gorm.io/gorm"` — add it.)

- [ ] **Step 3: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/auth/...
```

- [ ] **Step 4: Write service.go**

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/infra/crypto"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/ratelimit"
)

type ServiceOptions struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	BcryptCost int
}

type Service struct {
	repo    *Repo
	signer  *crypto.JWTSigner
	limiter *ratelimit.LoginLimiter
	opts    ServiceOptions
}

func NewService(repo *Repo, signer *crypto.JWTSigner, limiter *ratelimit.LoginLimiter, opts ServiceOptions) *Service {
	return &Service{repo: repo, signer: signer, limiter: limiter, opts: opts}
}

// Login validates credentials, applies rate limit, issues an access+refresh pair.
func (s *Service) Login(ctx context.Context, req LoginRequest, ip, ua string) (*TokenPair, error) {
	if !s.limiter.Allow(ip, req.Username) {
		_ = s.repo.InsertAuditLog(ctx, nil, "auth.login.rate_limited", ip, ua, mustJSON(map[string]any{"username": req.Username}))
		return nil, apperr.New(apperr.CodeRateLimited, "auth.rate_limited", "too many login attempts")
	}

	u, err := s.repo.FindUserByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = s.repo.InsertAuditLog(ctx, nil, "auth.login.failed", ip, ua, mustJSON(map[string]any{"username": req.Username, "reason": "not_found"}))
			return nil, apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
		}
		return nil, err
	}
	if err := crypto.ComparePassword(u.PasswordHash, req.Password); err != nil {
		uid := u.ID
		_ = s.repo.InsertAuditLog(ctx, &uid, "auth.login.failed", ip, ua, mustJSON(map[string]any{"reason": "bad_password"}))
		return nil, apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}

	pair, err := s.issuePair(ctx, u.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	_ = s.repo.UpdateLastLogin(ctx, u.ID, time.Now())
	uid := u.ID
	_ = s.repo.InsertAuditLog(ctx, &uid, "auth.login.success", ip, ua, nil)
	return pair, nil
}

func (s *Service) issuePair(ctx context.Context, userID uint64, ip, ua string) (*TokenPair, error) {
	jti, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	access, err := s.signer.Sign(crypto.JWTClaims{UserID: userID, JTI: jti}, s.opts.AccessTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := randomBase64(32)
	if err != nil {
		return nil, err
	}
	hash := sha256Hex(refresh)
	expiresAt := time.Now().Add(s.opts.RefreshTTL)
	if _, err := s.repo.CreateRefreshToken(ctx, userID, hash, expiresAt, ua, ip); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    time.Now().Add(s.opts.AccessTTL),
	}, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
```

- [ ] **Step 5: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/auth/... -v -run TestLogin -timeout 180s
```

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/modules/auth/
git commit -m "feat(be): auth service — Login (creds + rate limit + token issue + audit)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Auth service — Refresh with rotation + replay detection

**Files:**
- Modify: `optimus-be/internal/modules/auth/service.go` (add Refresh method)
- Create: `optimus-be/internal/modules/auth/service_refresh_test.go`

- [ ] **Step 1: Write failing test service_refresh_test.go**

```go
//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func mkSvcStd(t *testing.T, gdb *gorm.DB) *auth.Service {
	return auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)
}

func loginAlice(t *testing.T, svc *auth.Service) *auth.TokenPair {
	pair, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.NoError(t, err)
	return pair
}

func TestRefresh_RotatesTokens(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")
	svc := mkSvcStd(t, gdb)

	pair := loginAlice(t, svc)
	newPair, err := svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.NoError(t, err)

	require.NotEqual(t, pair.RefreshToken, newPair.RefreshToken, "refresh must rotate")
	require.NotEqual(t, pair.AccessToken, newPair.AccessToken)

	// Old refresh now revoked
	var old models.RefreshToken
	gdb.Where("token_hash = ?", sha256HexHelper(pair.RefreshToken)).First(&old)
	require.NotNil(t, old.RevokedAt)
}

func TestRefresh_ReplayRevokesAllTokensAndAudits(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")
	svc := mkSvcStd(t, gdb)

	pair := loginAlice(t, svc)
	_, err := svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.NoError(t, err)

	// Reuse old refresh — replay
	_, err = svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeRefreshTokenReplay, be.Code)

	// All refresh tokens for alice now revoked
	var active int64
	gdb.Model(&models.RefreshToken{}).Where("revoked_at IS NULL").Count(&active)
	require.Equal(t, int64(0), active)

	// Audit entry recorded
	var auditCount int64
	gdb.Model(&models.AuditLog{}).Where("action = ?", "auth.refresh.replay").Count(&auditCount)
	require.GreaterOrEqual(t, auditCount, int64(1))
}

func TestRefresh_RejectsExpiredToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: -1 * time.Second, BcryptCost: 4}, // already expired
	)
	pair, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeTokenExpired, be.Code)
	_ = uid
}

func TestRefresh_RejectsUnknownToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := mkSvcStd(t, gdb)
	_, err := svc.Refresh(context.Background(), "not-a-real-token", "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeTokenInvalid, be.Code)
}

// helper exported across test files within the package
func sha256HexHelper(s string) string { return auth.Sha256HexForTest(s) }
```

- [ ] **Step 2: Add `Sha256HexForTest` to service.go (test helper)**

Append to `optimus-be/internal/modules/auth/service.go`:

```go
// Sha256HexForTest is exported for test files in this package to compute the same
// hash the service stores for refresh tokens. Not for production use elsewhere.
func Sha256HexForTest(s string) string { return sha256Hex(s) }
```

- [ ] **Step 3: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/auth/...
```

(Will fail because Refresh isn't implemented yet.)

- [ ] **Step 4: Add Refresh method to service.go**

Append to `optimus-be/internal/modules/auth/service.go`:

```go
// Refresh validates the refresh token, rotates the pair, and detects replay.
// If the supplied refresh token is already revoked, ALL of the user's refresh
// tokens are revoked and an audit row is written.
func (s *Service) Refresh(ctx context.Context, refresh, ip, ua string) (*TokenPair, error) {
	hash := sha256Hex(refresh)
	row, err := s.repo.FindRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeTokenInvalid, "auth.token_invalid", "refresh token not recognized")
		}
		return nil, err
	}
	if row.RevokedAt != nil {
		// Replay! Burn everything.
		_ = s.repo.RevokeAllRefreshTokensForUser(ctx, row.UserID)
		uid := row.UserID
		_ = s.repo.InsertAuditLog(ctx, &uid, "auth.refresh.replay", ip, ua, nil)
		return nil, apperr.New(apperr.CodeRefreshTokenReplay, "auth.refresh_replay", "refresh token replay detected")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, apperr.New(apperr.CodeTokenExpired, "auth.token_expired", "refresh token expired")
	}

	// Rotate: revoke old, issue new.
	if err := s.repo.RevokeRefreshToken(ctx, row.ID); err != nil {
		return nil, err
	}
	return s.issuePair(ctx, row.UserID, ip, ua)
}
```

- [ ] **Step 5: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/auth/... -v -run TestRefresh -timeout 180s
```

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/modules/auth/
git commit -m "feat(be): auth service — Refresh with rotation and replay detection

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Auth service — Logout

**Files:**
- Modify: `optimus-be/internal/modules/auth/service.go`
- Create: `optimus-be/internal/modules/auth/service_logout_test.go`

- [ ] **Step 1: Write failing test service_logout_test.go**

```go
//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func TestLogout_RevokesGivenToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")
	svc := mkSvcStd(t, gdb)
	pair := loginAlice(t, svc)

	require.NoError(t, svc.Logout(context.Background(), pair.RefreshToken))

	var row models.RefreshToken
	gdb.Where("token_hash = ?", auth.Sha256HexForTest(pair.RefreshToken)).First(&row)
	require.NotNil(t, row.RevokedAt)
}

func TestLogout_IsIdempotentForUnknownToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := mkSvcStd(t, gdb)
	require.NoError(t, svc.Logout(context.Background(), "nonexistent"))
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/auth/...
```

- [ ] **Step 3: Add Logout method to service.go**

Append to `optimus-be/internal/modules/auth/service.go`:

```go
// Logout revokes the given refresh token. Idempotent: unknown / already-revoked tokens
// are not errors (logout should never block the client from clearing local state).
func (s *Service) Logout(ctx context.Context, refresh string) error {
	if refresh == "" {
		return nil
	}
	row, err := s.repo.FindRefreshTokenByHash(ctx, sha256Hex(refresh))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if row.RevokedAt != nil {
		return nil
	}
	return s.repo.RevokeRefreshToken(ctx, row.ID)
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/auth/... -v -run TestLogout -timeout 180s
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/modules/auth/
git commit -m "feat(be): auth service — Logout (idempotent refresh revocation)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 6: Auth Handler + Routes (Task 13)

### Task 13: Auth HTTP handler + route registration

**Files:**
- Create: `optimus-be/internal/modules/auth/handler.go`

- [ ] **Step 1: Write handler.go**

```go
package auth

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register attaches routes to the supplied auth group (typically /api/v1/auth).
func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", err.Error()))
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, pair)
}

func (h *Handler) refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", err.Error()))
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, pair)
}

func (h *Handler) logout(c *gin.Context) {
	var req LogoutRequest
	_ = c.ShouldBindJSON(&req) // body optional; absence is fine
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
```

- [ ] **Step 2: Build verification**

```bash
cd optimus-be && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add optimus-be/internal/modules/auth/handler.go
git commit -m "feat(be): auth HTTP handler — login/refresh/logout endpoints

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 7: JWT Middleware (Task 14)

### Task 14: JWT auth middleware

**Files:**
- Create: `optimus-be/internal/infra/middleware/jwt_auth.go`
- Create: `optimus-be/internal/infra/middleware/jwt_auth_test.go`

- [ ] **Step 1: Write jwt_auth_test.go**

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/middleware"
)

const mwTestSecret = "test_secret_must_be_at_least_32_bytes_!!"

func TestJWTAuth_AllowsValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	tok, err := signer.Sign(crypto.JWTClaims{UserID: 7, JTI: "j"}, time.Minute)
	require.NoError(t, err)

	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	var captured uint64
	r.GET("/", func(c *gin.Context) { captured = c.GetUint64(middleware.CtxKeyUserID); c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, uint64(7), captured)
}

func TestJWTAuth_RejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_RejectsExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	tok, _ := signer.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, -1*time.Second)

	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_RejectsMalformedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "NotBearer xyz")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test ./internal/infra/middleware/...
```

- [ ] **Step 3: Write jwt_auth.go**

```go
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/crypto"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

// JWTAuth validates the Authorization: Bearer <token> header. On success, sets
// the user_id in the context. On failure, aborts with 401 + envelope.
func JWTAuth(signer *crypto.JWTSigner) gin.HandlerFunc {
	return func(c *gin.Context) {
		hdr := c.GetHeader("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.token_invalid", "missing or malformed Authorization header"))
			c.Abort()
			return
		}
		tok := strings.TrimPrefix(hdr, "Bearer ")
		claims, err := signer.Verify(tok)
		if err != nil {
			// We can't distinguish "expired" from "invalid signature" without inspecting the err
			// (jwt/v5 returns sentinel errors). Map both to TokenInvalid for safety; client retries
			// via /auth/refresh either way.
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.token_invalid", "invalid or expired token"))
			c.Abort()
			return
		}
		c.Set(CtxKeyUserID, claims.UserID)
		c.Next()
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be && go test ./internal/infra/middleware/... -v
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): JWT auth middleware (Bearer token → ctx.user_id)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 8: Permission Cache + RBAC Middleware (Tasks 15-16)

### Task 15: Permission cache (sync.Map + TTL)

**Files:**
- Create: `optimus-be/internal/modules/rbac/cache.go`
- Create: `optimus-be/internal/modules/rbac/cache_test.go`

- [ ] **Step 1: Write cache_test.go**

```go
//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func seedUserWithRole(t *testing.T, gdb *gorm.DB, username, roleCode string) uint64 {
	t.Helper()
	u := &models.User{Username: username, Email: username + "@x.io", PasswordHash: "x", Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	var role models.Role
	require.NoError(t, gdb.Where("code = ?", roleCode).First(&role).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: u.ID, RoleID: role.ID}).Error)
	return u.ID
}

func setupSeed(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	// Recreate builtin admin & viewer roles + bindings — replicate seed minimally.
	admin := models.Role{Code: "admin", Name: "role.admin", IsBuiltin: true}
	viewer := models.Role{Code: "viewer", Name: "role.viewer", IsBuiltin: true}
	gdb.Create(&admin)
	gdb.Create(&viewer)
	var allPerms []models.Permission
	gdb.Find(&allPerms)
	for _, p := range allPerms {
		gdb.Create(&models.RolePermission{RoleID: admin.ID, PermissionID: p.ID})
		if strings.HasSuffix(p.Code, ":read") {
			gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: p.ID})
		}
	}
}

func TestCache_LoadsPermissionsFromDB(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uid := seedUserWithRole(t, gdb, "alice", "admin")

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	codes, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, len(permissions.All), len(codes))
}

func TestCache_RespectsTTL(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uid := seedUserWithRole(t, gdb, "alice", "viewer")

	cache := rbac.NewPermissionCache(gdb, 50*time.Millisecond)
	first, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)

	// Add a permission to viewer that wasn't there before by bumping the role's bindings directly
	var role models.Role
	gdb.Where("code = ?", "viewer").First(&role)
	var newPerm models.Permission
	gdb.Where("code = ?", "system:user:write").First(&newPerm)
	gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: newPerm.ID})

	// Within TTL — cached
	cached, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, first, cached)

	time.Sleep(80 * time.Millisecond) // past TTL
	fresh, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Contains(t, fresh, "system:user:write")
}

func TestCache_InvalidateForUser(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uid := seedUserWithRole(t, gdb, "alice", "viewer")

	cache := rbac.NewPermissionCache(gdb, time.Hour)
	first, _ := cache.Get(context.Background(), uid)

	var role models.Role
	gdb.Where("code = ?", "viewer").First(&role)
	var newPerm models.Permission
	gdb.Where("code = ?", "system:user:write").First(&newPerm)
	gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: newPerm.ID})

	cache.InvalidateUser(uid)
	fresh, _ := cache.Get(context.Background(), uid)
	require.NotEqual(t, len(first), len(fresh))
	require.Contains(t, fresh, "system:user:write")
}
```

(Imports also need `"gorm.io/gorm"` and `"strings"`.)

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/rbac/...
```

- [ ] **Step 3: Write cache.go**

```go
package rbac

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type cacheEntry struct {
	codes  []string
	stored time.Time
}

// PermissionCache caches user → permission code list with a TTL.
// Single-process; no cross-instance invalidation. Sufficient for P0.
type PermissionCache struct {
	db  *gorm.DB
	ttl time.Duration
	m   sync.Map // map[uint64]cacheEntry
}

func NewPermissionCache(db *gorm.DB, ttl time.Duration) *PermissionCache {
	return &PermissionCache{db: db, ttl: ttl}
}

// Get returns permission codes for the user. Hits cache if within TTL, else queries DB.
func (p *PermissionCache) Get(ctx context.Context, userID uint64) ([]string, error) {
	if v, ok := p.m.Load(userID); ok {
		e := v.(cacheEntry)
		if time.Since(e.stored) < p.ttl {
			return e.codes, nil
		}
	}
	codes, err := p.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	p.m.Store(userID, cacheEntry{codes: codes, stored: time.Now()})
	return codes, nil
}

// InvalidateUser drops a user from the cache. Call after a role/perm change for that user.
func (p *PermissionCache) InvalidateUser(userID uint64) { p.m.Delete(userID) }

// InvalidateAll clears the entire cache.
func (p *PermissionCache) InvalidateAll() {
	p.m.Range(func(k, _ any) bool { p.m.Delete(k); return true })
}

func (p *PermissionCache) load(ctx context.Context, userID uint64) ([]string, error) {
	var codes []string
	err := p.db.WithContext(ctx).
		Table("permissions p").
		Select("DISTINCT p.code").
		Joins("JOIN role_permissions rp ON rp.permission_id = p.id").
		Joins("JOIN user_roles ur ON ur.role_id = rp.role_id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where("u.id = ? AND u.deleted_at IS NULL", userID).
		Where("EXISTS (SELECT 1 FROM roles r WHERE r.id = ur.role_id AND r.deleted_at IS NULL)").
		Pluck("p.code", &codes).Error
	if err != nil {
		return nil, err
	}
	_ = models.Permission{} // silence unused import in some build envs
	return codes, nil
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/rbac/... -v -timeout 180s
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/modules/rbac/
git commit -m "feat(be): permission cache (sync.Map + TTL + invalidation)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 16: RBAC middleware

**Files:**
- Create: `optimus-be/internal/infra/middleware/rbac.go`
- Create: `optimus-be/internal/infra/middleware/rbac_test.go`

- [ ] **Step 1: Write rbac_test.go**

```go
//go:build dbtest

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func setupRBACSeed(t *testing.T, gdb *gorm.DB) (adminUID, viewerUID uint64) {
	t.Helper()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	admin := models.Role{Code: "admin", Name: "role.admin", IsBuiltin: true}
	viewer := models.Role{Code: "viewer", Name: "role.viewer", IsBuiltin: true}
	gdb.Create(&admin)
	gdb.Create(&viewer)
	var perms []models.Permission
	gdb.Find(&perms)
	for _, p := range perms {
		gdb.Create(&models.RolePermission{RoleID: admin.ID, PermissionID: p.ID})
		if strings.HasSuffix(p.Code, ":read") {
			gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: p.ID})
		}
	}
	a := &models.User{Username: "adminx", Email: "a@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(a)
	gdb.Create(&models.UserRole{UserID: a.ID, RoleID: admin.ID})

	v := &models.User{Username: "viewerx", Email: "v@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(v)
	gdb.Create(&models.UserRole{UserID: v.ID, RoleID: viewer.ID})
	return a.ID, v.ID
}

func TestRBAC_AllowsUserWithPermission(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	adminUID, _ := setupRBACSeed(t, gdb)

	cache := rbac.NewPermissionCache(gdb, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, adminUID); c.Next() })
	r.GET("/u", middleware.RequirePermission(cache, "system:user:write"), func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/u", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRBAC_RejectsUserWithoutPermission(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, viewerUID := setupRBACSeed(t, gdb)

	cache := rbac.NewPermissionCache(gdb, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, viewerUID); c.Next() })
	r.GET("/u", middleware.RequirePermission(cache, "system:user:write"), func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/u", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRBAC_RejectsAnonymous(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	cache := rbac.NewPermissionCache(gdb, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/u", middleware.RequirePermission(cache, "system:user:write"), func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/u", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/infra/middleware/...
```

- [ ] **Step 3: Write rbac.go**

```go
package middleware

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/modules/rbac"
)

// RequirePermission rejects requests whose authenticated user lacks the given permission code.
// Must come AFTER JWTAuth in the middleware chain.
func RequirePermission(cache *rbac.PermissionCache, code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetUint64(CtxKeyUserID)
		if uid == 0 {
			response.Error(c, apperr.New(apperr.CodeUnauthorized, "auth.unauthenticated", "authentication required"))
			c.Abort()
			return
		}
		codes, err := cache.Get(c.Request.Context(), uid)
		if err != nil {
			response.Error(c, apperr.Wrap(err, apperr.CodeInternal, "common.internal", "permission lookup failed"))
			c.Abort()
			return
		}
		for _, p := range codes {
			if p == code {
				c.Next()
				return
			}
		}
		response.Error(c, apperr.New(apperr.CodePermissionDenied, "auth.permission_denied", "missing permission: "+code))
		c.Abort()
	}
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/infra/middleware/... -v -timeout 180s
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/infra/middleware/
git commit -m "feat(be): RBAC middleware (per-route permission check)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 9: /me Endpoints (Tasks 17-19)

### Task 17: /me service skeleton + GetUser

**Files:**
- Create: `optimus-be/internal/modules/rbac/dto.go`
- Create: `optimus-be/internal/modules/rbac/service.go`
- Create: `optimus-be/internal/modules/rbac/service_test.go`

- [ ] **Step 1: Write dto.go**

```go
package rbac

import "time"

type MeUserDTO struct {
	ID          uint64     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type MeMenuNode struct {
	ID             uint64        `json:"id"`
	Code           string        `json:"code"`
	Name           string        `json:"name"`
	Path           string        `json:"path"`
	Component      string        `json:"component"`
	Icon           string        `json:"icon"`
	PermissionCode *string       `json:"permission_code,omitempty"`
	SortOrder      int           `json:"sort_order"`
	Hidden         bool          `json:"hidden"`
	Children       []MeMenuNode  `json:"children,omitempty"`
}
```

- [ ] **Step 2: Write failing test service_test.go**

```go
//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func TestMeService_GetUser(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	u := &models.User{Username: "alice", Email: "a@x", PasswordHash: "x", Status: "enabled", DisplayName: "Alice"}
	require.NoError(t, gdb.Create(u).Error)

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute))
	dto, err := svc.GetUser(context.Background(), u.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, dto.ID)
	require.Equal(t, "alice", dto.Username)
	require.Equal(t, "Alice", dto.DisplayName)
}

func TestMeService_GetUser_NotFound(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, 0))
	_, err := svc.GetUser(context.Background(), 999999)
	require.Error(t, err)
}

var _ = gorm.DB{} // keep import
```

- [ ] **Step 3: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/rbac/...
```

- [ ] **Step 4: Write service.go**

```go
package rbac

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type MeService struct {
	db    *gorm.DB
	cache *PermissionCache
}

func NewMeService(db *gorm.DB, cache *PermissionCache) *MeService {
	return &MeService{db: db, cache: cache}
}

func (s *MeService) GetUser(ctx context.Context, userID uint64) (*MeUserDTO, error) {
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, userID).Error; err != nil {
		return nil, err
	}
	return &MeUserDTO{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		Status:      u.Status,
		LastLoginAt: u.LastLoginAt,
	}, nil
}
```

- [ ] **Step 5: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/rbac/... -v -run TestMeService -timeout 180s
```

- [ ] **Step 6: Commit**

```bash
git add optimus-be/internal/modules/rbac/
git commit -m "feat(be): MeService skeleton — GetUser by ID

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 18: /me/permissions service method

**Files:**
- Modify: `optimus-be/internal/modules/rbac/service.go`
- Create: `optimus-be/internal/modules/rbac/service_permissions_test.go`

- [ ] **Step 1: Write failing test service_permissions_test.go**

```go
//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func TestMeService_ListPermissions_AdminGetsAll(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	admin := models.Role{Code: "admin", Name: "role.admin", IsBuiltin: true}
	gdb.Create(&admin)
	var perms []models.Permission
	gdb.Find(&perms)
	for _, p := range perms {
		gdb.Create(&models.RolePermission{RoleID: admin.ID, PermissionID: p.ID})
	}
	u := &models.User{Username: "alice", Email: "a@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(u)
	gdb.Create(&models.UserRole{UserID: u.ID, RoleID: admin.ID})

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute))
	codes, err := svc.ListPermissions(context.Background(), u.ID)
	require.NoError(t, err)
	require.Equal(t, len(permissions.All), len(codes))
}

func TestMeService_ListPermissions_ViewerGetsOnlyReads(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, _ = permissions.Register(context.Background(), gdb, permissions.All)
	viewer := models.Role{Code: "viewer", Name: "role.viewer", IsBuiltin: true}
	gdb.Create(&viewer)
	var perms []models.Permission
	gdb.Where("code LIKE ?", "%:read").Find(&perms)
	for _, p := range perms {
		gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: p.ID})
	}
	u := &models.User{Username: "bob", Email: "b@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(u)
	gdb.Create(&models.UserRole{UserID: u.ID, RoleID: viewer.ID})

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute))
	codes, err := svc.ListPermissions(context.Background(), u.ID)
	require.NoError(t, err)
	require.NotEmpty(t, codes)
	for _, c := range codes {
		require.True(t, strings.HasSuffix(c, ":read"))
	}
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/rbac/...
```

- [ ] **Step 3: Add ListPermissions to service.go**

Append:

```go
// ListPermissions returns the permission codes the user has (via cache).
func (s *MeService) ListPermissions(ctx context.Context, userID uint64) ([]string, error) {
	return s.cache.Get(ctx, userID)
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/rbac/... -v -run TestMeService_ListPermissions -timeout 180s
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/modules/rbac/
git commit -m "feat(be): MeService.ListPermissions via cache

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 19: /me/menus service method + handler + route registration

**Files:**
- Modify: `optimus-be/internal/modules/rbac/service.go`
- Create: `optimus-be/internal/modules/rbac/service_menus_test.go`
- Create: `optimus-be/internal/modules/rbac/handler.go`

- [ ] **Step 1: Write failing test service_menus_test.go**

```go
//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/seed"
)

func TestMeService_ListMenus_ViewerSeesOnlyPermittedNodes(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x"})
	require.NoError(t, err)

	// Create a viewer user
	u := &models.User{Username: "viewer1", Email: "v@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(u)
	var viewer models.Role
	gdb.Where("code = ?", "viewer").First(&viewer)
	gdb.Create(&models.UserRole{UserID: u.ID, RoleID: viewer.ID})

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute))
	tree, err := svc.ListMenus(context.Background(), u.ID)
	require.NoError(t, err)

	// Tree must contain dashboard (no perm required) and the system parent
	var foundDashboard, foundSystem bool
	for _, top := range tree {
		if top.Code == "dashboard" {
			foundDashboard = true
		}
		if top.Code == "system" {
			foundSystem = true
			// system.users requires system:user:read which viewer DOES have (it's a :read perm)
			require.Contains(t, codes(top.Children), "system.users")
		}
	}
	require.True(t, foundDashboard)
	require.True(t, foundSystem)
}

func codes(nodes []rbac.MeMenuNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Code)
	}
	return out
}
```

- [ ] **Step 2: Run, fail**

```bash
cd optimus-be && go test -tags=dbtest ./internal/modules/rbac/...
```

- [ ] **Step 3: Add ListMenus + handler.go**

Append to `service.go`:

```go
// ListMenus returns the menu tree filtered by the user's permissions.
// A node is included iff (permission_code is empty OR user has the code).
// Empty branches (parent that loses all children due to filtering AND has no path)
// are still returned because the frontend may want to render the empty parent header.
func (s *MeService) ListMenus(ctx context.Context, userID uint64) ([]MeMenuNode, error) {
	codes, err := s.cache.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, c := range codes {
		set[c] = struct{}{}
	}

	var rows []models.Menu
	if err := s.db.WithContext(ctx).
		Where("hidden = ?", false).
		Order("sort_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	byParent := map[uint64][]models.Menu{}
	const rootKey uint64 = 0
	for _, m := range rows {
		key := rootKey
		if m.ParentID != nil {
			key = *m.ParentID
		}
		byParent[key] = append(byParent[key], m)
	}

	var build func(parentID uint64) []MeMenuNode
	build = func(parentID uint64) []MeMenuNode {
		kids := byParent[parentID]
		out := make([]MeMenuNode, 0, len(kids))
		for _, m := range kids {
			if m.PermissionCode != nil {
				if _, ok := set[*m.PermissionCode]; !ok {
					continue
				}
			}
			node := MeMenuNode{
				ID:             m.ID,
				Code:           m.Code,
				Name:           m.Name,
				Path:           m.Path,
				Component:      m.Component,
				Icon:           m.Icon,
				PermissionCode: m.PermissionCode,
				SortOrder:      m.SortOrder,
				Hidden:         m.Hidden,
				Children:       build(m.ID),
			}
			out = append(out, node)
		}
		return out
	}
	return build(rootKey), nil
}
```

Create `handler.go`:

```go
package rbac

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc *MeService
}

func NewHandler(svc *MeService) *Handler { return &Handler{svc: svc} }

// RegisterMe attaches the /me family of routes.
// The supplied group should already have JWTAuth middleware applied.
func (h *Handler) RegisterMe(g *gin.RouterGroup) {
	g.GET("/me", h.getMe)
	g.GET("/me/menus", h.getMyMenus)
	g.GET("/me/permissions", h.getMyPermissions)
}

func (h *Handler) getMe(c *gin.Context) {
	uid := c.GetUint64(middleware.CtxKeyUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeUnauthorized, "auth.unauthenticated", "authentication required"))
		return
	}
	dto, err := h.svc.GetUser(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, apperr.Wrap(err, apperr.CodeNotFound, "common.not_found", "user not found"))
		return
	}
	response.Success(c, dto)
}

func (h *Handler) getMyMenus(c *gin.Context) {
	uid := c.GetUint64(middleware.CtxKeyUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeUnauthorized, "auth.unauthenticated", "authentication required"))
		return
	}
	tree, err := h.svc.ListMenus(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, tree)
}

func (h *Handler) getMyPermissions(c *gin.Context) {
	uid := c.GetUint64(middleware.CtxKeyUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeUnauthorized, "auth.unauthenticated", "authentication required"))
		return
	}
	codes, err := h.svc.ListPermissions(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, codes)
}
```

- [ ] **Step 4: Run, pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./internal/modules/rbac/... -v -timeout 240s
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add optimus-be/internal/modules/rbac/
git commit -m "feat(be): MeService.ListMenus + /me HTTP handler

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 10: Wire-up + E2E Smoke (Tasks 20-21)

### Task 20: Wire middleware chain + auth + /me routes into cmd/server

**Files:**
- Modify: `optimus-be/cmd/server/main.go`

- [ ] **Step 1: Replace cmd/server/main.go**

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
	"time"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/rbac"
)

var Version = "dev"

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	checkPerms := flag.Bool("check-permissions", false, "register permission codes and exit")
	flag.Parse()

	abs, err := filepath.Abs(*cfgPath)
	if err != nil {
		fail("resolve config path", err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		fail("load config", err)
	}
	if err := cfg.ValidateStrict(); err != nil {
		fail("validate config", err)
	}

	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})
	logger.Info("optimus-be starting", "version", Version)

	gdb, err := db.Open(cfg.Database)
	if err != nil {
		fail("open db", err)
	}

	if _, err := permissions.Register(context.Background(), gdb, permissions.All); err != nil {
		fail("register permissions", err)
	}
	if *checkPerms {
		logger.Info("permissions registered, exiting due to -check-permissions")
		return
	}

	signer := crypto.NewJWTSigner(cfg.JWT.Secret)
	limiter := ratelimit.NewLoginLimiter(
		cfg.Auth.LoginRateLimit.PerIP, // per-IP quota also used as per-username for simplicity
		cfg.Auth.LoginRateLimit.Window,
		cfg.Auth.LoginRateLimit.Block,
	)
	authRepo := auth.NewRepo(gdb)
	authSvc := auth.NewService(authRepo, signer, limiter, auth.ServiceOptions{
		AccessTTL:  cfg.JWT.AccessTTL,
		RefreshTTL: cfg.JWT.RefreshTTL,
		BcryptCost: cfg.Auth.BcryptCost,
	})
	authHandler := auth.NewHandler(authSvc)

	// Permission cache TTL: 60s per spec §7.4. A dedicated config key (rbac.cache_ttl)
	// can be added in Plan 1C — for P0 a fixed constant is fine.
	permCache := rbac.NewPermissionCache(gdb, 60*time.Second)

	meSvc := rbac.NewMeService(gdb, permCache)
	meHandler := rbac.NewHandler(meSvc)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recover(logger))
	r.Use(middleware.CORS(cfg.CORS))
	r.Use(middleware.I18n(cfg.I18n))

	api := r.Group("/api/v1")

	// public
	(&health.Handler{DB: gdb, Version: Version}).Register(api)
	authHandler.Register(api.Group("/auth"))

	// authenticated
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(signer))
	meHandler.RegisterMe(protected)

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

- [ ] **Step 2: Verify build**

```bash
cd optimus-be && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add optimus-be/cmd/server/main.go
git commit -m "feat(be): wire middleware chain + auth routes + /me routes into server

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 21: End-to-end auth smoke test

**Files:**
- Create: `optimus-be/tests/integration/auth_e2e_test.go`

- [ ] **Step 1: Write the e2e test**

```go
//go:build dbtest

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/seed"
)

const e2eSecret = "test_secret_must_be_at_least_32_bytes_!!"

func mustJSONBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func bodyMap(t *testing.T, r *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &out))
	return out
}

func setupServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	t.Cleanup(teardown)
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	// Set admin password to something known
	hash, _ := crypto.HashPassword("S3cret-Pass!", 4)
	require.NoError(t, gdb.Model(&models.User{}).Where("username = ?", "admin").Update("password_hash", hash).Error)

	signer := crypto.NewJWTSigner(e2eSecret)
	authSvc := auth.NewService(
		auth.NewRepo(gdb), signer,
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)
	authHandler := auth.NewHandler(authSvc)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	meHandler := rbac.NewHandler(rbac.NewMeService(gdb, cache))

	logger := log.New(log.Options{Level: "warn", Format: "json"})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recover(logger))
	r.Use(middleware.CORS(config.CORSConfig{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST"}}))
	r.Use(middleware.I18n(config.I18nConfig{DefaultLang: "zh-CN", Supported: []string{"zh-CN", "en-US"}}))
	api := r.Group("/api/v1")
	(&health.Handler{DB: gdb, Version: "test"}).Register(api)
	authHandler.Register(api.Group("/auth"))
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(signer))
	meHandler.RegisterMe(protected)
	return r, gdb
}

func TestE2E_LoginRefreshReplayLogout(t *testing.T) {
	r, _ := setupServer(t)

	// 1) Login
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		mustJSONBody(t, map[string]string{"username": "admin", "password": "S3cret-Pass!"}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := bodyMap(t, rec)
	data := body["data"].(map[string]any)
	access1 := data["access_token"].(string)
	refresh1 := data["refresh_token"].(string)
	require.NotEmpty(t, access1)
	require.NotEmpty(t, refresh1)

	// 2) Hit /me with access1
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+access1)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"username":"admin"`)

	// 3) Refresh
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		mustJSONBody(t, map[string]string{"refresh_token": refresh1}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	data2 := bodyMap(t, rec)["data"].(map[string]any)
	access2 := data2["access_token"].(string)
	refresh2 := data2["refresh_token"].(string)
	require.NotEqual(t, access1, access2)
	require.NotEqual(t, refresh1, refresh2)

	// 4) Replay refresh1 — must 401 with refresh_replay code
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		mustJSONBody(t, map[string]string{"refresh_token": refresh1}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.True(t, strings.Contains(rec.Body.String(), "refresh_replay") || strings.Contains(rec.Body.String(), "40104"))

	// 5) After replay, refresh2 also dead (was revoked en masse). Confirm.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		mustJSONBody(t, map[string]string{"refresh_token": refresh2}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusOK, rec.Code)
}

func TestE2E_MeMenusFiltersByPermission(t *testing.T) {
	r, gdb := setupServer(t)

	// Add a viewer user with known password
	hash, _ := crypto.HashPassword("viewer-pw-1", 4)
	v := &models.User{Username: "viewer1", Email: "v@x", PasswordHash: hash, Status: "enabled"}
	require.NoError(t, gdb.Create(v).Error)
	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: v.ID, RoleID: viewer.ID}).Error)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		mustJSONBody(t, map[string]string{"username": "viewer1", "password": "viewer-pw-1"}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	access := bodyMap(t, rec)["data"].(map[string]any)["access_token"].(string)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me/menus", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// viewer has *:read perms, so they should see all :read-gated menu items.
	require.Contains(t, rec.Body.String(), "system.users")
	require.Contains(t, rec.Body.String(), "dashboard")
}
```

(Imports also need `"gorm.io/gorm"` — add it.)

- [ ] **Step 2: Run, expecting pass**

```bash
cd optimus-be
export DOCKER_HOST=unix:///Users/logic/.colima/docker.sock
go test -tags=dbtest ./tests/integration/... -v -timeout 300s
```

Expected: both e2e tests PASS.

- [ ] **Step 3: Run full suite + lint as final sanity**

```bash
cd optimus-be
go test ./...
go test -tags=dbtest ./... -timeout 300s
golangci-lint run
```

All green.

- [ ] **Step 4: Commit**

```bash
git add optimus-be/tests/integration/
git commit -m "test(be): end-to-end auth + /me smoke tests (login/refresh/replay/menu filter)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Acceptance Checklist (end of Plan 1B)

- [ ] All middleware unit tests pass (RequestID, Logger, Recover, CORS, I18n, JWTAuth, RBAC).
- [ ] All crypto unit tests pass (bcrypt, JWT sign/verify).
- [ ] Login rate limiter unit tests pass.
- [ ] Auth repo integration tests pass.
- [ ] Auth service integration tests pass (login + refresh + rotation + replay + logout + rate limit).
- [ ] Permission cache integration tests pass (load, TTL, invalidate).
- [ ] MeService integration tests pass (GetUser, ListPermissions, ListMenus).
- [ ] `cmd/server/main.go` wires full chain: RequestID → Logger → Recover → CORS → I18n; protected group has JWTAuth.
- [ ] `tests/integration/auth_e2e_test.go` exercises full login → /me → refresh → replay flow against an HTTP test server with seed data.
- [ ] `golangci-lint run` clean.
- [ ] `go test ./...` and `go test -tags=dbtest ./...` both green.
- [ ] Audit logs written for: `auth.login.success`, `auth.login.failed`, `auth.login.rate_limited`, `auth.refresh.replay`.

When done, ping me to start **Plan 1C** (user/role/permission/menu/audit modules + swagger + CI).
