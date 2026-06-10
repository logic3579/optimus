//go:build dbtest

package cluster_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/k8s/cluster"
	"optimus-be/internal/seed"
)

func newHandlerRouter(t *testing.T) (*gin.Engine, *cluster.Service, uint64) {
	t.Helper()
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)

	// Seed admin so the audit_logs FK (user_id → users.id) is satisfied.
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	kc := &models.CredentialKubeconfig{Name: "kc", KubeconfigEnc: []byte{1}}
	require.NoError(t, gdb.Create(kc).Error)

	repo := cluster.NewRepo(gdb)
	rec := audit.NewRecorder(gdb)
	svc := cluster.NewService(repo, &fakeConsumer{yaml: []byte(goodYAML)}, nil, rec)
	h := cluster.NewHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	g := r.Group("/api/v1/k8s/clusters")
	g.GET("", h.HandleList())
	g.POST("", h.HandleCreate())
	g.GET("/:id", h.HandleGet())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	g.POST("/:id/ping", h.HandlePing())
	return r, svc, kc.ID
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

func TestHandler_FullCRUD(t *testing.T) {
	r, _, kcID := newHandlerRouter(t)

	// create
	w := doJSON(t, r, "POST", "/api/v1/k8s/clusters", cluster.CreateRequest{
		Name: "c1", KubeconfigID: kcID, Context: "ctx", Tags: []string{"prod"},
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, 0, env.Code)
	id := uint64(env.Data["id"].(float64))

	// list
	w = doJSON(t, r, "GET", "/api/v1/k8s/clusters?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"c1"`)
	require.Contains(t, w.Body.String(), `"kubeconfig_name":"kc"`)

	// list with tag filter
	w = doJSON(t, r, "GET", "/api/v1/k8s/clusters?tag=prod", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"c1"`)

	// get
	w = doJSON(t, r, "GET", "/api/v1/k8s/clusters/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// update description
	newDesc := "edited"
	w = doJSON(t, r, "PUT", "/api/v1/k8s/clusters/"+strconv.FormatUint(id, 10),
		cluster.UpdateRequest{Description: &newDesc})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), `"description":"edited"`)

	// ping (nil prober → ok=false with message)
	w = doJSON(t, r, "POST", "/api/v1/k8s/clusters/"+strconv.FormatUint(id, 10)+"/ping", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"ok":false`)
	require.Contains(t, w.Body.String(), "prober not configured")

	// delete
	w = doJSON(t, r, "DELETE", "/api/v1/k8s/clusters/"+strconv.FormatUint(id, 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_RejectsTagInjection(t *testing.T) {
	r, _, _ := newHandlerRouter(t)
	// URL-encoded `foo"]'); --` — chars outside [a-zA-Z0-9_.-] must be rejected.
	w := doJSON(t, r, "GET", `/api/v1/k8s/clusters?tag=foo%22%5D%27%29%3B`, nil)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "k8s.cluster.tag_filter_charset")
}

func TestHandler_BadIDOnGet(t *testing.T) {
	r, _, _ := newHandlerRouter(t)
	w := doJSON(t, r, "GET", "/api/v1/k8s/clusters/0", nil)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}
