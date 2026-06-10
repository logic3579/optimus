//go:build dbtest

package sshkey_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/sshkey"
	"optimus-be/internal/seed"
)

func newHandlerRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	rec := audit.NewRecorder(gdb)
	h := sshkey.NewHandler(sshkey.NewService(sshkey.NewRepo(gdb), passthroughCipher{}, rec))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	g := r.Group("/api/v1/credentials/ssh-keys")
	g.GET("", h.HandleList())
	g.POST("", h.HandleCreate())
	g.GET("/:id", h.HandleGet())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	return r, gdb
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

type envelope struct {
	Code int            `json:"code"`
	Data map[string]any `json:"data"`
}

func TestHandler_CreateGetListDelete(t *testing.T) {
	r, gdb := newHandlerRouter(t)
	pem := genTestSSHKey(t)

	// Create
	w := doJSON(t, r, "POST", "/api/v1/credentials/ssh-keys", sshkey.CreateRequest{
		Name: "h1", Username: "ops", PrivateKey: pem,
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte(strings.TrimSpace(pem))),
		"response leaks plaintext private key")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, 0, env.Code)
	id := uint64(env.Data["id"].(float64))
	require.NotZero(t, id)

	// List
	w = doJSON(t, r, "GET", "/api/v1/credentials/ssh-keys?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, bytes.Contains(w.Body.Bytes(), []byte(strings.TrimSpace(pem))),
		"list response leaks plaintext")

	// Get
	w = doJSON(t, r, "GET", "/api/v1/credentials/ssh-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Delete
	w = doJSON(t, r, "DELETE", "/api/v1/credentials/ssh-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// Get after delete → 404
	w = doJSON(t, r, "GET", "/api/v1/credentials/ssh-keys/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Audit row survives the delete (denormalized payload.name).
	var cnt int64
	require.NoError(t, gdb.Raw(`SELECT COUNT(*) FROM audit_logs
	    WHERE action = 'credentials.delete' AND target_type = 'credentials.ssh_key'`).Scan(&cnt).Error)
	require.GreaterOrEqual(t, cnt, int64(1))
}

func TestHandler_Validation_RejectsBadKey(t *testing.T) {
	r, _ := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/ssh-keys", sshkey.CreateRequest{
		Name: "bad", Username: "u", PrivateKey: "not-pem",
	})
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "credentials.invalid_key_format")
}

func TestHandler_Validation_RejectsMissingRequired(t *testing.T) {
	r, _ := newHandlerRouter(t)
	// Missing name + private_key
	w := doJSON(t, r, "POST", "/api/v1/credentials/ssh-keys", map[string]any{"username": "u"})
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), "common.validation")
}

func TestHandler_BadIDReturns400(t *testing.T) {
	r, _ := newHandlerRouter(t)
	w := doJSON(t, r, "GET", "/api/v1/credentials/ssh-keys/abc", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Update_PartialEdit(t *testing.T) {
	r, _ := newHandlerRouter(t)
	pem := genTestSSHKey(t)
	w := doJSON(t, r, "POST", "/api/v1/credentials/ssh-keys", sshkey.CreateRequest{
		Name: "edit-me", Username: "ops", PrivateKey: pem,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	id := uint64(env.Data["id"].(float64))

	newDesc := "edited"
	w = doJSON(t, r, "PUT", "/api/v1/credentials/ssh-keys/"+strconv.FormatUint(id, 10),
		sshkey.UpdateRequest{Description: &newDesc})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), `"description":"edited"`)
	require.Contains(t, w.Body.String(), `"username":"ops"`) // unchanged
}
