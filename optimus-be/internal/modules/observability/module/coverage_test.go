package module

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/modules/observability/datasource"
	"optimus-be/internal/modules/observability/prometheus"
	"optimus-be/internal/modules/observability/query"
)

func TestClientFactoryRejectsMalformedBaseURLAcrossAdapters(t *testing.T) {
	f := clientFactory{}
	_, _, err := f.build("://bad", "none", false, nil, nil)
	require.Error(t, err)
	_, _, err = f.Build(t.Context(), query.Datasource{BaseURL: "://bad"}, nil)
	require.Error(t, err)
	_, err = f.Test(t.Context(), datasource.Detail{BaseURL: "://bad"}, nil)
	require.Error(t, err)
}

func TestClientFactoryBuildsRequestScopedClientWithoutNetwork(t *testing.T) {
	policy, err := prometheus.NewPolicy(nil, nil)
	require.NoError(t, err)
	f := clientFactory{transport: prometheus.NewTransportFactory(policy, nil), maxBody: 1024, maxSeries: 10}
	runner, closeFn, err := f.Build(t.Context(), query.Datasource{BaseURL: "https://prom.example", AuthType: "none"}, nil)
	require.NoError(t, err)
	require.NotNil(t, runner)
	require.NotNil(t, closeFn)
	closeFn()
}

func TestMountRoutesPublicEntryPointAndInvalidLimitVariants(t *testing.T) {
	m, err := Wire(validInput())
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	m.MountRoutes(gin.New().Group("/api/v1"), nil)

	for name, mutate := range map[string]func(*Input){
		"timeout":     func(in *Input) { in.Config.QueryTimeout = 0 },
		"batch":       func(in *Input) { in.Config.MaxBatchQueries = 0 },
		"concurrency": func(in *Input) { in.Config.MaxConcurrent = in.Config.MaxBatchQueries + 1 },
		"range":       func(in *Input) { in.Config.MaxRange = 0 },
		"step":        func(in *Input) { in.Config.MinStep = 0 },
		"points":      func(in *Input) { in.Config.MaxPointsPerSeries = 0 },
		"series":      func(in *Input) { in.Config.MaxSeries = 0 },
		"bytes":       func(in *Input) { in.Config.MaxResponseBytes = 0 },
		"enrichment":  func(in *Input) { in.Config.MaxEnrichmentIPs = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mutate(&in)
			_, err := Wire(in)
			require.Error(t, err)
		})
	}
}
