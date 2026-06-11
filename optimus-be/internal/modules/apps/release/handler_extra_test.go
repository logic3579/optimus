package release

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHandler_BadJSONBodies asserts that an unparseable JSON body on the
// mutating endpoints surfaces CodeValidation (40002) instead of crashing
// or returning a 500. These cover the c.ShouldBindJSON error branch on
// install, upgrade, rollback, and uninstall — branches that the happy-path
// tests cannot reach.
func TestHandler_BadJSONBodies(t *testing.T) {
	r, _, apps, _ := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)

	type kase struct {
		method, path string
	}
	cases := []kase{
		{"POST", "/apps/applications/" + idStr + "/release/install"},
		{"POST", "/apps/applications/" + idStr + "/release/upgrade"},
		{"POST", "/apps/applications/" + idStr + "/release/rollback"},
		{"POST", "/apps/applications/" + idStr + "/release/uninstall"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(c.method, c.path, bytes.NewReader([]byte("not-json{")))
			req.Header.Set("Content-Type", "application/json")
			// Force a non-zero ContentLength so the uninstall handler bothers
			// to bind (it short-circuits on ContentLength == 0).
			req.ContentLength = int64(len("not-json{"))
			r.ServeHTTP(w, req)
			require.NotEqual(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
			require.Contains(t, w.Body.String(), `"code":40002`,
				"expected CodeValidation, body=%s", w.Body.String())
		})
	}
}

// TestHandler_UninstallEmptyBodyIsOK exercises the explicit ContentLength==0
// branch in handler.uninstall — it must accept an empty body and default
// keep_history to false. Originally not covered because the lifecycle test
// passes nil (which httptest still sends as an empty body but with
// ContentLength==0).
func TestHandler_UninstallEmptyBodyIsOK(t *testing.T) {
	r, _, apps, _ := newHandlerRouter(t)
	idStr := strconv.FormatUint(apps.app.ID, 10)
	// install first so uninstall has a release to target.
	w := doJSON(t, r, "POST", "/apps/applications/"+idStr+"/release/install",
		InstallRequest{ChartVersion: "1.0.0"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// Send an empty body explicitly.
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/apps/applications/"+idStr+"/release/uninstall", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
}
