package module

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"optimus-be/internal/infra/config"
	"optimus-be/internal/modules/assets"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/httpcredential"
)

type fakeCredentials struct{}

func (*fakeCredentials) GetSSHKey(context.Context, uint64, string) (*credentials.SSHKey, error) {
	return nil, nil
}
func (*fakeCredentials) GetKubeconfig(context.Context, uint64, string) (*credentials.Kubeconfig, error) {
	return nil, nil
}
func (*fakeCredentials) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	return nil, nil
}
func (*fakeCredentials) GetHTTPCredential(context.Context, uint64, string) (*credentials.HTTPCredential, error) {
	return nil, nil
}

type fakeAssets struct{}

func (*fakeAssets) LookupInstanceByPrivateIP(context.Context, netip.Addr) (*assets.Instance, error) {
	return nil, nil
}
func (*fakeAssets) LookupInstanceByID(context.Context, int64, string, string) (*assets.Instance, error) {
	return nil, nil
}
func (*fakeAssets) ListInstancesByVPC(context.Context, int64, string, string) ([]assets.Instance, error) {
	return nil, nil
}

func TestWireRejectsInvalidCIDR(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedPrivateCIDRs = []string{"bad-cidr"}
	in := validInput()
	in.Config = cfg
	if _, err := Wire(in); err == nil {
		t.Fatal("expected invalid CIDR")
	}
}
func TestWireRejectsRequiredTypedNilAndAcceptsTypedNilAssets(t *testing.T) {
	base := validInput()
	var db *gorm.DB
	v := base
	v.DB = db
	if _, e := Wire(v); e == nil {
		t.Fatal("typed nil db")
	}
	var c *fakeCredentials
	v = base
	v.Credentials = c
	if _, e := Wire(v); e == nil {
		t.Fatal("typed nil credentials")
	}
	var a *audit.Recorder
	v = base
	v.Audit = a
	if _, e := Wire(v); e == nil {
		t.Fatal("typed nil audit")
	}
	var optional *fakeAssets
	v = base
	v.Assets = optional
	if _, e := Wire(v); e != nil {
		t.Fatalf("typed nil assets: %v", e)
	}
}

func TestExactRoutePermissionSnapshotContainsNoAlert(t *testing.T) {
	m, err := Wire(validInput())
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	actual := map[string]string{}
	permission := func(code string) gin.HandlerFunc {
		return func(c *gin.Context) { actual[c.Request.Method+" "+c.FullPath()] = code; c.AbortWithStatus(204) }
	}
	m.mountRoutesWithPermission(r.Group("/api/v1"), permission)
	httpcredential.NewHandler(nil).Mount(r.Group("/api/v1/credentials/http-credentials"), permission)
	want := map[string]string{
		"GET /api/v1/observability/datasources": "observability:datasource:read", "POST /api/v1/observability/datasources": "observability:datasource:write", "GET /api/v1/observability/datasources/:id": "observability:datasource:read", "PUT /api/v1/observability/datasources/:id": "observability:datasource:write", "DELETE /api/v1/observability/datasources/:id": "observability:datasource:delete", "POST /api/v1/observability/datasources/:id/test": "observability:datasource:write",
		"GET /api/v1/observability/datasources/:id/labels": "observability:metric:read", "GET /api/v1/observability/datasources/:id/label-values": "observability:metric:read", "POST /api/v1/observability/query": "observability:metric:read", "POST /api/v1/observability/query-range": "observability:metric:read",
		"GET /api/v1/observability/dashboards": "observability:dashboard:read", "POST /api/v1/observability/dashboards": "observability:dashboard:write", "GET /api/v1/observability/dashboards/:id": "observability:dashboard:read", "PUT /api/v1/observability/dashboards/:id": "observability:dashboard:write", "DELETE /api/v1/observability/dashboards/:id": "observability:dashboard:delete", "GET /api/v1/observability/builtin-dashboards": "observability:metric:read", "GET /api/v1/observability/builtin-dashboards/:code": "observability:metric:read",
		"GET /api/v1/credentials/http-credentials": "credentials:http:read", "POST /api/v1/credentials/http-credentials": "credentials:http:write", "GET /api/v1/credentials/http-credentials/:id": "credentials:http:read", "PUT /api/v1/credentials/http-credentials/:id": "credentials:http:write", "DELETE /api/v1/credentials/http-credentials/:id": "credentials:http:delete",
	}
	if len(r.Routes()) != len(want) {
		t.Fatalf("unexpected route count got=%d want=%d: %#v", len(r.Routes()), len(want), r.Routes())
	}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		if strings.Contains(strings.ToLower(key), "alert") {
			t.Fatalf("alert route %s", key)
		}
		path := strings.ReplaceAll(strings.ReplaceAll(route.Path, ":id", "1"), ":code", "kubernetes-cluster")
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(route.Method, path, nil))
	}
	if len(actual) != len(want) {
		t.Fatalf("observed=%v", actual)
	}
	for route, permission := range want {
		if actual[route] != permission {
			t.Fatalf("%s permission=%q want=%q", route, actual[route], permission)
		}
	}
}

func testConfig() config.ObservabilityConfig {
	return config.ObservabilityConfig{QueryTimeout: 15 * time.Second, MaxBatchQueries: 12, MaxConcurrent: 4, MaxRange: 7 * 24 * time.Hour, MinStep: 15 * time.Second, MaxPointsPerSeries: 11000, MaxSeries: 1000, MaxResponseBytes: 16 << 20, MaxEnrichmentIPs: 100}
}
func validInput() Input {
	return Input{DB: &gorm.DB{}, Credentials: credentials.NewConsumer(nil, nil, nil, nil), Audit: audit.NewRecorder(&gorm.DB{}), Config: testConfig()}
}
