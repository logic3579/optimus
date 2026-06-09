//go:build dbtest

package menu_test

import (
	"bytes"
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
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/menu"
)

func newHandlerRouter(t *testing.T) *gin.Engine {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	t.Cleanup(td)
	h := menu.NewHandler(menu.NewService(menu.NewRepo(gdb), audit.NewRecorder(gdb)))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(1)); c.Next() })
	api := r.Group("/api/v1")
	h.Register(api.Group("/menus"))
	return r
}

func createMenu(t *testing.T, r *gin.Engine, req menu.CreateRequest) uint64 {
	t.Helper()
	body, _ := json.Marshal(req)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodPost, "/api/v1/menus", body))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return uint64(resp["data"].(map[string]any)["id"].(float64))
}

func mustReq(method, path string, body []byte) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestHandler_CreateAndList(t *testing.T) {
	r := newHandlerRouter(t)
	createMenu(t, r, menu.CreateRequest{Code: "dash", Name: "menu.dash", Path: "/dash"})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodGet, "/api/v1/menus", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"dash"`)
}

func TestHandler_UpdateAndDelete(t *testing.T) {
	r := newHandlerRouter(t)
	id := createMenu(t, r, menu.CreateRequest{Code: "dash", Name: "menu.dash", Path: "/dash"})

	newName := "menu.dashboard"
	hidden := true
	body, _ := json.Marshal(menu.UpdateRequest{Name: &newName, Hidden: &hidden})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodPut, "/api/v1/menus/"+strconv.FormatUint(id, 10), body))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"name":"menu.dashboard"`)
	require.Contains(t, rec.Body.String(), `"hidden":true`)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodDelete, "/api/v1/menus/"+strconv.FormatUint(id, 10), nil))
	require.Equal(t, http.StatusOK, rec.Code)

	// Confirm gone from tree.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodGet, "/api/v1/menus", nil))
	require.NotContains(t, rec.Body.String(), `"code":"dash"`)
}

func TestHandler_DeleteRejectsParentWithChildren(t *testing.T) {
	r := newHandlerRouter(t)
	parent := createMenu(t, r, menu.CreateRequest{Code: "pp", Name: "p"})
	_ = createMenu(t, r, menu.CreateRequest{Code: "cc", Name: "c", ParentID: &parent})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodDelete, "/api/v1/menus/"+strconv.FormatUint(parent, 10), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":40901`)
}

func TestHandler_BadID(t *testing.T) {
	r := newHandlerRouter(t)
	body, _ := json.Marshal(menu.UpdateRequest{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, mustReq(http.MethodPut, "/api/v1/menus/abc", body))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
