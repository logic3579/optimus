package release

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

// newHandlerRouter wires the 6 release endpoints under a no-auth gin engine
// with a middleware that stuffs middleware.CtxKeyUserID = 1. Returns the
// engine, the in-memory recorder, and the stub app service so individual
// tests can assert audit + state.
func newHandlerRouter(_ *testing.T) (*gin.Engine, *Service, *stubAppService, *inMemoryRecorder) {
	s, apps, rec, _ := newTestService()
	h := NewHandler(s)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CtxKeyUserID, uint64(1))
		c.Next()
	})
	g := r.Group("/apps/applications/:id/release")
	g.GET("/status", h.HandleStatus())
	g.GET("/history", h.HandleHistory())
	g.POST("/install", h.HandleInstall())
	g.POST("/upgrade", h.HandleUpgrade())
	g.POST("/rollback", h.HandleRollback())
	g.POST("/uninstall", h.HandleUninstall())
	return r, s, apps, rec
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

func TestHandler_InstallStatusHistoryRollbackUninstall(t *testing.T) {
	r, _, apps, rec := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)

	// install
	w := doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/install",
		InstallRequest{ChartVersion: "1.0.0"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), `"revision":1`)

	// upgrade
	w = doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/upgrade",
		UpgradeRequest{ChartVersion: "1.1.0"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// status
	w = doJSON(t, r, "GET", "/apps/applications/"+idStr+"/release/status", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"deployed"`)
	require.Contains(t, w.Body.String(), `"revision":2`)

	// history
	w = doJSON(t, r, "GET", "/apps/applications/"+idStr+"/release/history", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"items":`)

	// rollback
	w = doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/rollback",
		RollbackRequest{Revision: 1})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// uninstall (empty body OK)
	w = doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/uninstall", nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// audit captured 4 mutating writes
	events := rec.snapshot()
	require.Len(t, events, 4)
}

func TestHandler_BadID(t *testing.T) {
	r, _, _, _ := newHandlerRouter(t)
	w := doJSON(t, r, "GET", "/apps/applications/0/release/status", nil)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestHandler_Install_MissingChartVersion_400(t *testing.T) {
	r, _, apps, _ := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)
	// Empty body fails the chart_version=required binding.
	w := doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/install",
		InstallRequest{})
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":40002`)
}

func TestHandler_Status_NotFound(t *testing.T) {
	r, _, apps, _ := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)
	w := doJSON(t, r, "GET", "/apps/applications/"+idStr+"/release/status", nil)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":42202`)
}

func TestHandler_Rollback_RevisionMissing_42203(t *testing.T) {
	r, _, apps, _ := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)
	// install first
	w := doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/install",
		InstallRequest{ChartVersion: "1.0.0"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	// rollback to a revision that doesn't exist
	w = doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/rollback",
		RollbackRequest{Revision: 999})
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":42203`)
}

func TestHandler_Install_DuplicateRelease_42201(t *testing.T) {
	r, _, apps, _ := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)
	w := doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/install",
		InstallRequest{ChartVersion: "1.0.0"})
	require.Equal(t, http.StatusOK, w.Code)
	w = doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/install",
		InstallRequest{ChartVersion: "1.0.0"})
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":42201`)
}

func TestHandler_AppNotFound(t *testing.T) {
	r, _, _, _ := newHandlerRouter(t)
	w := doJSON(t, r, "POST", "/apps/applications/9999/release/install",
		InstallRequest{ChartVersion: "1.0.0"})
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":40401`)
}
