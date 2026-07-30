package pipeline

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type handlerStub struct {
	err       error
	published PublishRequest
}

func (s *handlerStub) GetCurrent(context.Context, uint64) (*Pipeline, error) {
	return &Pipeline{}, s.err
}
func (s *handlerStub) Publish(_ context.Context, _ uint64, _, _ string, _ uint64, r PublishRequest) (*Pipeline, error) {
	s.published = r
	return &Pipeline{}, s.err
}
func (s *handlerStub) ListArtifacts(context.Context, uint64) ([]ArtifactVersion, error) {
	return []ArtifactVersion{}, s.err
}
func (s *handlerStub) ResolveArtifact(context.Context, uint64, ResolveArtifactRequest) (*ResolvedArtifact, error) {
	return &ResolvedArtifact{Digest: "sha256:" + strings.Repeat("a", 64)}, s.err
}

func TestPipelineHandlerMountsExactRoutesAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seen := map[string]int{}
	NewHandler(&handlerStub{}).Mount(r.Group("/api/v1/delivery"), func(code string) gin.HandlerFunc { seen[code]++; return func(c *gin.Context) { c.Next() } })
	require.Equal(t, map[string]int{"delivery:pipeline:read": 1, "delivery:pipeline:write": 1, "delivery:run:create": 1}, seen)
	want := map[string]bool{"GET /api/v1/delivery/projects/:id/pipeline": true, "PUT /api/v1/delivery/projects/:id/pipeline": true, "GET /api/v1/delivery/projects/:id/artifacts": true, "POST /api/v1/delivery/projects/:id/artifacts/resolve": true}
	for _, route := range r.Routes() {
		delete(want, fmt.Sprintf("%s %s", route.Method, route.Path))
	}
	require.Empty(t, want)
	require.Len(t, r.Routes(), 4)
}

func TestResolveArtifactRejectsInvalidBodyAndReturnsDigest(t *testing.T) {
	r := gin.New()
	r.POST("/projects/:id/artifacts/resolve", NewHandler(&handlerStub{}).ResolveArtifact)
	bad := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects/1/artifacts/resolve", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(bad, req)
	require.Equal(t, http.StatusBadRequest, bad.Code)
	good := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/projects/1/artifacts/resolve", strings.NewReader(`{"chart_repo_id":4,"chart_name":"demo","chart_version":"1.2.3"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(good, req)
	require.Equal(t, http.StatusOK, good.Code)
	require.Contains(t, good.Body.String(), `"digest":"sha256:`)
}

func TestPipelineHandlerRejectsInvalidIDAndBinding(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{{"bad id", "/projects/nope/pipeline", `{"stages":[{"environment_id":1,"timeout":"10m"}]}`}, {"empty stages", "/projects/1/pipeline", `{"stages":[]}`}, {"numeric timeout", "/projects/1/pipeline", `{"stages":[{"environment_id":1,"timeout":600}]}`}} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.PUT("/projects/:id/pipeline", NewHandler(&handlerStub{}).Publish)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"code"`)
			require.NotContains(t, w.Body.String(), tc.body)
		})
	}
}

func TestPipelineHandlerHidesRawServiceError(t *testing.T) {
	r := gin.New()
	r.GET("/projects/:id/pipeline", NewHandler(&handlerStub{err: errors.New("registry_token=secret")}).Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/1/pipeline", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, w.Body.String(), "registry_token")
	require.NotContains(t, w.Body.String(), "secret")
}
