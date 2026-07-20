package dashboard

import (
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMountUsesExactPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seen := map[string]int{}
	permission := func(code string) gin.HandlerFunc { seen[code]++; return func(c *gin.Context) { c.Next() } }
	NewHandler(nil).Mount(r.Group("/api/v1/observability"), permission)
	for _, code := range []string{"observability:dashboard:read", "observability:dashboard:write", "observability:dashboard:delete"} {
		if seen[code] != 1 {
			t.Fatalf("permission %s count=%d", code, seen[code])
		}
	}
	want := map[string]bool{
		"GET /api/v1/observability/dashboards": true, "POST /api/v1/observability/dashboards": true,
		"GET /api/v1/observability/dashboards/:id": true, "PUT /api/v1/observability/dashboards/:id": true,
		"DELETE /api/v1/observability/dashboards/:id": true,
	}
	for _, route := range r.Routes() {
		delete(want, fmt.Sprintf("%s %s", route.Method, route.Path))
	}
	if len(want) != 0 {
		t.Fatalf("missing routes=%v", want)
	}
}
