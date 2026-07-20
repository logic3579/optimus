package module

import (
	"context"
	"errors"
	"net/url"
	"reflect"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/assets"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/observability/builtin"
	"optimus-be/internal/modules/observability/dashboard"
	"optimus-be/internal/modules/observability/datasource"
	dsinuse "optimus-be/internal/modules/observability/datasource/inuse"
	"optimus-be/internal/modules/observability/prometheus"
	"optimus-be/internal/modules/observability/query"
	"optimus-be/internal/modules/rbac"
)

type Input struct {
	DB          *gorm.DB
	Credentials credentials.Consumer
	Audit       *audit.Recorder
	Config      config.ObservabilityConfig
	Assets      assets.Consumer
}
type Module struct {
	CredentialUsage *dsinuse.Counter
	ClusterUsage    *dsinuse.Counter
	datasource      *datasource.Handler
	query           *query.Handler
	dashboard       *dashboard.Handler
	builtin         *builtin.Handler
}

type clientFactory struct {
	transport *prometheus.TransportFactory
	maxBody   int64
	maxSeries int
}

func (f clientFactory) build(baseURL, authType string, tlsSkip bool, ca []byte, secret *credentials.HTTPCredential) (*prometheus.Client, func(), error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, err
	}
	auth := prometheus.Auth{Type: authType}
	if auth.Type == "none" {
		auth.Type = ""
	}
	if secret != nil {
		auth.Username = secret.Username
		auth.Secret = secret.Secret
	}
	hc, err := f.transport.New(base, prometheus.TLSOptions{SkipVerify: tlsSkip, CustomCAPEM: ca}, auth)
	if err != nil {
		return nil, nil, err
	}
	return prometheus.NewClient(hc, base, f.maxBody, f.maxSeries), hc.CloseIdleConnections, nil
}
func (f clientFactory) Build(_ context.Context, d query.Datasource, c *credentials.HTTPCredential) (query.Runner, func(), error) {
	return f.build(d.BaseURL, d.AuthType, d.TLSSkipVerify, d.CustomCAPEM, c)
}
func (f clientFactory) Test(ctx context.Context, d datasource.Detail, c *credentials.HTTPCredential) (map[string]string, error) {
	client, closeFn, err := f.build(d.BaseURL, d.AuthType, d.TLSSkipVerify, d.CustomCAPEMCopy(), c)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return client.BuildInfo(ctx)
}

func Wire(in Input) (*Module, error) {
	if isNil(in.DB) || isNil(in.Credentials) || isNil(in.Audit) {
		return nil, errors.New("observability database, credentials, and audit are required")
	}
	if isNil(in.Assets) {
		in.Assets = nil
	}
	o := in.Config
	if o.QueryTimeout <= 0 || o.MaxBatchQueries <= 0 || o.MaxConcurrent <= 0 || o.MaxRange <= 0 || o.MinStep <= 0 || o.MaxPointsPerSeries <= 0 || o.MaxSeries <= 0 || o.MaxResponseBytes <= 0 || o.MaxEnrichmentIPs <= 0 || o.MaxConcurrent > o.MaxBatchQueries {
		return nil, errors.New("invalid observability limits")
	}
	policy, err := prometheus.NewPolicy(o.AllowedPrivateCIDRs, nil)
	if err != nil {
		return nil, err
	}
	transport := prometheus.NewTransportFactory(policy, nil)
	factory := clientFactory{transport: transport, maxBody: o.MaxResponseBytes, maxSeries: o.MaxSeries}
	dsRepo := datasource.NewRepo(in.DB)
	dashRepo := dashboard.NewRepo(in.DB)
	counter := dsinuse.New(in.DB)
	dsSvc := datasource.NewService(dsRepo, dsRepo, dsRepo, dashRepo, in.Credentials, factory, in.Audit)
	loader := query.DatasourceLoaderFunc(func(ctx context.Context, id uint64) (query.Datasource, error) {
		d, err := dsRepo.GetByID(ctx, id)
		if err != nil {
			return query.Datasource{}, err
		}
		var cid *uint64
		if d.HTTPCredential != nil {
			x := d.HTTPCredential.ID
			cid = &x
		}
		return query.Datasource{ID: d.ID, BaseURL: d.BaseURL, AuthType: d.AuthType, CredentialID: cid, TLSSkipVerify: d.TLSSkipVerify, CustomCAPEM: d.CustomCAPEMCopy()}, nil
	})
	qSvc := query.NewService(loader, in.Credentials, factory, in.Assets, query.Limits{MaxBatch: o.MaxBatchQueries, MaxConcurrent: o.MaxConcurrent, MaxPromQLBytes: 8192, MaxRange: o.MaxRange, MinStep: o.MinStep, Timeout: o.QueryTimeout, MaxPoints: o.MaxPointsPerSeries, MaxEnrichmentIPs: o.MaxEnrichmentIPs})
	dashSvc := dashboard.NewService(dashRepo, in.Audit)
	return &Module{CredentialUsage: counter, ClusterUsage: counter, datasource: datasource.NewHandler(dsSvc), query: query.NewHandler(qSvc), dashboard: dashboard.NewHandler(dashSvc), builtin: builtin.NewHandler()}, nil
}
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
		return rv.IsNil()
	}
	return false
}
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	permission := func(code string) gin.HandlerFunc { return middleware.RequirePermission(cache, code) }
	m.mountRoutesWithPermission(protected, permission)
}
func (m *Module) mountRoutesWithPermission(protected *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	g := protected.Group("/observability")
	m.datasource.Mount(g, permission)
	m.query.Mount(g, permission)
	m.dashboard.Mount(g, permission)
	m.builtin.Mount(g, permission)
}
