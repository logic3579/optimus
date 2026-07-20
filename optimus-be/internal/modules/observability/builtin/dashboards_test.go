package builtin

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDashboardSnapshot(t *testing.T) {
	wantCodes := []string{"kubernetes-cluster", "kubernetes-nodes", "kubernetes-workloads"}
	all := List()
	if len(all) != len(wantCodes) {
		t.Fatalf("codes=%v", all)
	}
	for i, d := range all {
		if d.Code != wantCodes[i] {
			t.Fatalf("code[%d]=%q", i, d.Code)
		}
		refs := map[string]bool{}
		units := map[string]bool{"none": true, "percent": true, "bytes": true, "bytes_per_second": true, "cores": true, "seconds": true, "requests_per_second": true}
		for _, p := range d.Panels {
			if refs[p.RefID] || p.PromQL == "" {
				t.Fatalf("invalid panel %#v", p)
			}
			refs[p.RefID] = true
			if p.PanelType != "time_series" && p.PanelType != "stat" && p.PanelType != "table" {
				t.Fatalf("type=%q", p.PanelType)
			}
			if !units[p.Unit] || (p.Width != 6 && p.Width != 12) {
				t.Fatalf("unit/width=%q/%d", p.Unit, p.Width)
			}
		}
	}
	b, _ := json.Marshal(all)
	if len(b) < 500 {
		t.Fatalf("snapshot unexpectedly small: %s", b)
	}
	if got, want := fmt.Sprintf("%x", sha256.Sum256(b)), "3d0b03fdac148cbeba35c54d6cb482eb34a4ae165222ccaab77757fd7253a653"; got != want {
		t.Fatalf("snapshot changed: got %s want %s\n%s", got, want, b)
	}
	all[0].Panels[0].PromQL = "mutated"
	again, _ := Get("kubernetes-cluster")
	if again.Panels[0].PromQL == "mutated" {
		t.Fatal("definitions are mutable through List")
	}
}

func TestHandlerMountsReadOnlyMetricRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seen := map[string]int{}
	NewHandler().Mount(r.Group("/api/v1/observability"), func(code string) gin.HandlerFunc {
		seen[code]++
		return func(c *gin.Context) { c.Next() }
	})
	if seen["observability:metric:read"] != 1 || len(seen) != 1 {
		t.Fatalf("permissions=%v", seen)
	}
	want := map[string]bool{"GET /api/v1/observability/builtin-dashboards": true, "GET /api/v1/observability/builtin-dashboards/:code": true}
	for _, route := range r.Routes() {
		delete(want, route.Method+" "+route.Path)
		if route.Method != http.MethodGet {
			t.Fatalf("mutation route exposed: %s %s", route.Method, route.Path)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing routes=%v", want)
	}
}

func TestRenderVariablesEscapesAndRejectsUnsafeValues(t *testing.T) {
	d, _ := Get("kubernetes-workloads")
	if _, err := Render(d, map[string]string{"namespace": `prod"} or on() vector(1)`}); err == nil {
		t.Fatal("expected unsafe variable rejection")
	}
	out, err := Render(d, map[string]string{"namespace": "prod-app", "workload": "api_v2"})
	if err != nil || len(out.Panels) == 0 {
		t.Fatalf("render: %v", err)
	}
}

func TestRenderRegexVariablesAreQuotedAsLiterals(t *testing.T) {
	nodes, _ := Get("kubernetes-nodes")
	rendered, err := Render(nodes, map[string]string{"node": "prod.node"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Panels[0].PromQL, `node=~"prod\\.node"`) {
		t.Fatalf("node matcher was not quoted: %s", rendered.Panels[0].PromQL)
	}
	workloads, _ := Get("kubernetes-workloads")
	rendered, err = Render(workloads, map[string]string{"namespace": "prod.ns", "workload": "api.v2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Panels[0].PromQL, `namespace="prod.ns"`) || !strings.Contains(rendered.Panels[0].PromQL, `pod=~"api\\.v2.*"`) {
		t.Fatalf("workload matchers were not rendered literally: %s", rendered.Panels[0].PromQL)
	}
}
