package module

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/delivery/approval"
	"optimus-be/internal/modules/delivery/event"
	"optimus-be/internal/modules/delivery/pipeline"
	"optimus-be/internal/modules/delivery/project"
	"optimus-be/internal/modules/delivery/run"
)

type projectEnvironmentStub struct {
	environments []project.Environment
	err          error
}

func (s projectEnvironmentStub) ListEnvironments(context.Context, uint64) ([]project.Environment, error) {
	return s.environments, s.err
}

type versionListerStub struct {
	items []pipeline.ArtifactVersion
	err   error
}

type artifactResolverStub struct {
	artifact *run.Artifact
	err      error
}

func (s artifactResolverStub) ResolveArtifact(context.Context, uint64, string, string) (*run.Artifact, error) {
	return s.artifact, s.err
}

func (s versionListerStub) ListVersions(context.Context, uint64, string) ([]pipeline.ArtifactVersion, error) {
	return s.items, s.err
}

func TestArtifactCatalogPreservesProjectBusinessError(t *testing.T) {
	want := apperr.New(apperr.CodeDeliveryProjectNotFound, "delivery.project.not_found", "delivery project not found")
	_, err := (artifactCatalog{projects: projectEnvironmentStub{err: want}, versions: versionListerStub{}}).ListArtifacts(context.Background(), 9)
	var got *apperr.BizError
	require.ErrorAs(t, err, &got)
	require.Equal(t, apperr.CodeDeliveryProjectNotFound, got.Code)
	require.Equal(t, http.StatusNotFound, apperr.HTTPStatus(got.Code))
}

func TestArtifactCatalogMapsRawProviderErrorToSafe503(t *testing.T) {
	catalog := artifactCatalog{projects: projectEnvironmentStub{environments: []project.Environment{{ChartRepoID: 4, ChartName: "demo"}}}, versions: versionListerStub{err: errors.New("registry authorization=secret")}}
	h := pipeline.NewHandler(pipeline.NewHTTPService(nil, nil, catalog))
	r := gin.New()
	r.GET("/projects/:id/artifacts", h.ListArtifacts)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/9/artifacts", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "artifact versions unavailable")
	require.NotContains(t, w.Body.String(), "authorization")
	require.NotContains(t, w.Body.String(), "secret")
}

func TestArtifactCatalogMapsMalformedProviderOutputToSafe503(t *testing.T) {
	for _, tc := range []struct{ name, version string }{
		{"empty", ""},
		{"whitespace", "  \t  "},
		{"too long", strings.Repeat("sensitive-version-", 9)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := artifactCatalog{
				projects: projectEnvironmentStub{environments: []project.Environment{{ChartRepoID: 4, ChartName: "demo"}}},
				versions: versionListerStub{items: []pipeline.ArtifactVersion{{ChartRepoID: 999, ChartName: "untrusted", Version: tc.version}}},
			}
			h := pipeline.NewHandler(pipeline.NewHTTPService(nil, nil, catalog))
			r := gin.New()
			r.GET("/projects/:id/artifacts", h.ListArtifacts)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/9/artifacts", nil))
			require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"message_key":"delivery.execution.unavailable"`)
			require.Contains(t, w.Body.String(), "artifact versions unavailable")
			require.NotContains(t, w.Body.String(), "sensitive-version")
			require.NotContains(t, w.Body.String(), "untrusted")
			require.NotContains(t, w.Body.String(), "999")
		})
	}
}

func TestArtifactCatalogResolvesOnlyAuthoritativeListedArtifact(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	catalog := artifactCatalog{
		projects: projectEnvironmentStub{environments: []project.Environment{{ChartRepoID: 4, ChartName: "demo"}}},
		versions: versionListerStub{items: []pipeline.ArtifactVersion{{Version: "1.2.3"}}},
		resolver: artifactResolverStub{artifact: &run.Artifact{RepoID: 4, ChartName: "demo", Version: "1.2.3", Digest: digest}},
	}
	got, err := catalog.ResolveArtifact(context.Background(), 9, pipeline.ResolveArtifactRequest{ChartRepoID: 4, ChartName: "demo", ChartVersion: "1.2.3"})
	require.NoError(t, err)
	require.Equal(t, digest, got.Digest)
	_, err = catalog.ResolveArtifact(context.Background(), 9, pipeline.ResolveArtifactRequest{ChartRepoID: 99, ChartName: "other", ChartVersion: "1.2.3"})
	var business *apperr.BizError
	require.ErrorAs(t, err, &business)
	require.Equal(t, apperr.CodeDeliveryChartIdentityMismatch, business.Code)
}

func TestArtifactCatalogHidesResolverErrorsAndMalformedDigest(t *testing.T) {
	base := artifactCatalog{projects: projectEnvironmentStub{environments: []project.Environment{{ChartRepoID: 4, ChartName: "demo"}}}, versions: versionListerStub{items: []pipeline.ArtifactVersion{{Version: "1.2.3"}}}}
	for _, resolver := range []artifactResolverStub{{err: errors.New("registry token=secret")}, {artifact: &run.Artifact{RepoID: 4, ChartName: "demo", Version: "1.2.3", Digest: "sha256:bad"}}} {
		base.resolver = resolver
		h := pipeline.NewHandler(pipeline.NewHTTPService(nil, nil, base))
		r := gin.New()
		r.POST("/projects/:id/artifacts/resolve", h.ResolveArtifact)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/projects/9/artifacts/resolve", strings.NewReader(`{"chart_repo_id":4,"chart_name":"demo","chart_version":"1.2.3"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"message_key":"delivery.execution.unavailable"`)
		require.NotContains(t, w.Body.String(), "secret")
		require.NotContains(t, w.Body.String(), "sha256:bad")
	}
}

func TestWireRejectsMissingRequiredSeams(t *testing.T) {
	module, err := Wire(Input{})
	require.Error(t, err)
	require.Nil(t, module)
}

func TestRoutesExposeOnlyApprovedSurfaceWithExactPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := &Module{
		project:  project.NewHandler(nil),
		pipeline: pipeline.NewHandler(nil),
		run:      run.NewHandler(nil),
		approval: approval.NewHandler(nil),
		event:    event.NewHandler(event.NewService(nil), 0, 1),
	}
	router := gin.New()
	permissions := make([]string, 0, 12)
	m.mountRoutesWithPermission(router.Group("/api/v1"), func(code string) gin.HandlerFunc {
		permissions = append(permissions, code)
		return func(c *gin.Context) { c.Next() }
	})

	wantPermissions := []string{
		"delivery:project:read", "delivery:project:write", "delivery:project:delete",
		"delivery:pipeline:read", "delivery:pipeline:write", "delivery:run:create",
		"delivery:run:read", "delivery:run:create", "delivery:run:cancel",
		"delivery:approval:read", "delivery:approval:decide", "delivery:run:read",
	}
	require.Equal(t, wantPermissions, permissions)
	routes := router.Routes()
	require.Len(t, routes, 23)
	approved := map[string]struct{}{
		http.MethodGet + " /api/v1/delivery/projects": {}, http.MethodPost + " /api/v1/delivery/projects": {},
		http.MethodGet + " /api/v1/delivery/projects/:id": {}, http.MethodPut + " /api/v1/delivery/projects/:id": {}, http.MethodDelete + " /api/v1/delivery/projects/:id": {},
		http.MethodGet + " /api/v1/delivery/projects/:id/environments": {}, http.MethodPost + " /api/v1/delivery/projects/:id/environments": {},
		http.MethodPut + " /api/v1/delivery/projects/:id/environments/:environmentId": {}, http.MethodDelete + " /api/v1/delivery/projects/:id/environments/:environmentId": {},
		http.MethodGet + " /api/v1/delivery/projects/:id/pipeline": {}, http.MethodPut + " /api/v1/delivery/projects/:id/pipeline": {},
		http.MethodGet + " /api/v1/delivery/projects/:id/artifacts": {}, http.MethodPost + " /api/v1/delivery/projects/:id/artifacts/resolve": {}, http.MethodGet + " /api/v1/delivery/projects/:id/runs": {}, http.MethodPost + " /api/v1/delivery/projects/:id/runs": {},
		http.MethodGet + " /api/v1/delivery/runs/:id": {}, http.MethodPost + " /api/v1/delivery/runs/:id/cancel": {}, http.MethodPost + " /api/v1/delivery/runs/:id/reconcile": {}, http.MethodPost + " /api/v1/delivery/runs/:id/retry": {},
		http.MethodGet + " /api/v1/delivery/runs/:id/events": {}, http.MethodGet + " /api/v1/delivery/approvals/pending": {},
		http.MethodPost + " /api/v1/delivery/run-stages/:id/approve": {}, http.MethodPost + " /api/v1/delivery/run-stages/:id/reject": {},
	}
	for _, route := range routes {
		_, ok := approved[route.Method+" "+route.Path]
		require.Truef(t, ok, "unexpected delivery route %s %s", route.Method, route.Path)
		require.NotContains(t, route.Path, "install")
		require.NotContains(t, route.Path, "uninstall")
		require.NotContains(t, route.Path, "script")
	}
}
