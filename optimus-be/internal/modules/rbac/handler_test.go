//go:build dbtest

package rbac_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/user"
	"optimus-be/internal/seed"
)

const meHandlerTestSecret = "test_secret_must_be_at_least_32_bytes_!!"

// meHandlerHarness wires a real Postgres schema + user.Service + MeService +
// Handler + JWTAuth middleware so the /me PUT routes can be exercised
// end-to-end. JWT is synthesised directly (no /auth/login round-trip) to keep
// tests focused on handler behaviour — the JWTAuth middleware itself is
// covered separately under internal/infra/middleware.
type meHandlerHarness struct {
	ctx     context.Context
	gdb     *gorm.DB
	signer  *crypto.JWTSigner
	router  *gin.Engine
	userSvc *user.Service
	cleanup func()
}

func buildMeHandlerHarness(t *testing.T) *meHandlerHarness {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	rec := audit.NewRecorder(gdb)
	userSvc := user.NewService(user.NewRepo(gdb), cache, rec, user.ServiceOptions{BcryptCost: 4, AdminUsername: "admin"})
	meSvc := rbac.NewMeService(gdb, cache, userSvc)
	h := rbac.NewHandler(meSvc)

	signer := crypto.NewJWTSigner(meHandlerTestSecret)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(signer))
	h.RegisterMe(protected)

	return &meHandlerHarness{
		ctx:     ctx,
		gdb:     gdb,
		signer:  signer,
		router:  r,
		userSvc: userSvc,
		cleanup: teardown,
	}
}

// loginUser provisions a user via user.Service.Create (so password hashing +
// audit live in the same place as production) and synthesises a JWT directly
// using the harness signer. Returns (user_id, access_token).
func (h *meHandlerHarness) loginUser(t *testing.T, username, email, password string) (uint64, string) {
	t.Helper()
	out, err := h.userSvc.Create(h.ctx, 1, "127.0.0.1", "go-test", user.CreateRequest{
		Username: username, Email: email, Password: password,
	})
	require.NoError(t, err)
	require.NotZero(t, out.ID)
	tok, err := h.signer.Sign(crypto.JWTClaims{UserID: out.ID, JTI: "j"}, time.Minute)
	require.NoError(t, err)
	return out.ID, tok
}

func (h *meHandlerHarness) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if body == "" {
		bodyReader = strings.NewReader("")
	} else {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func TestHandler_UpdateMe_OK(t *testing.T) {
	h := buildMeHandlerHarness(t)
	defer h.cleanup()

	_, token := h.loginUser(t, "alice", "alice@example.com", "oldpass1234")
	body := `{"display_name":"Alice C","email":"alice2@example.com"}`
	w := h.do(t, http.MethodPut, "/api/v1/me", body, token)

	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"display_name":"Alice C"`)
}

func TestHandler_ChangeMyPassword_WrongOld(t *testing.T) {
	h := buildMeHandlerHarness(t)
	defer h.cleanup()

	_, token := h.loginUser(t, "bob", "bob@example.com", "rightpass00")
	w := h.do(t, http.MethodPut, "/api/v1/me/password", `{"old_password":"wrong","new_password":"newpass5678"}`, token)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
