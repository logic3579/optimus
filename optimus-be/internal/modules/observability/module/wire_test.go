package module

import (
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"optimus-be/internal/infra/config"
)

func TestWireRejectsInvalidCIDR(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedPrivateCIDRs = []string{"bad-cidr"}
	_, err := Wire(Input{DB: &gorm.DB{}, Config: cfg})
	if err == nil {
		t.Fatal("expected invalid CIDR")
	}
}

func TestRouteSnapshotContainsNoAlert(t *testing.T) {
	m, err := Wire(Input{DB: &gorm.DB{}, Config: testConfig()})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	m.MountRoutes(r.Group("/api/v1"), nil)
	want := map[string]bool{
		"GET /api/v1/observability/datasources": true, "POST /api/v1/observability/datasources": true, "GET /api/v1/observability/datasources/:id": true, "PUT /api/v1/observability/datasources/:id": true, "DELETE /api/v1/observability/datasources/:id": true, "POST /api/v1/observability/datasources/:id/test": true,
		"GET /api/v1/observability/datasources/:id/labels": true, "GET /api/v1/observability/datasources/:id/label-values": true, "POST /api/v1/observability/query": true, "POST /api/v1/observability/query-range": true,
		"GET /api/v1/observability/dashboards": true, "POST /api/v1/observability/dashboards": true, "GET /api/v1/observability/dashboards/:id": true, "PUT /api/v1/observability/dashboards/:id": true, "DELETE /api/v1/observability/dashboards/:id": true, "GET /api/v1/observability/builtin-dashboards": true, "GET /api/v1/observability/builtin-dashboards/:code": true,
	}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		if strings.Contains(strings.ToLower(key), "alert") {
			t.Fatalf("alert route %s", key)
		}
		delete(want, key)
	}
	if len(want) > 0 {
		t.Fatalf("missing routes %v", want)
	}
}

func testConfig() config.ObservabilityConfig {
	return config.ObservabilityConfig{QueryTimeout: 15 * time.Second, MaxBatchQueries: 12, MaxConcurrent: 4, MaxRange: 7 * 24 * time.Hour, MinStep: 15 * time.Second, MaxPointsPerSeries: 11000, MaxSeries: 1000, MaxResponseBytes: 16 << 20, MaxEnrichmentIPs: 100}
}
