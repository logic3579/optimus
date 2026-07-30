package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCreateRejectsOversizedBodyWithDashboardError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/dashboards", NewHandler(nil).Create)
	body := bytes.Repeat([]byte("x"), int(maxRequestBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPost, "/dashboards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var envelope struct {
		Code       int    `json:"code"`
		MessageKey string `json:"message_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	if envelope.Code != 44203 || envelope.MessageKey != "observability.dashboard.invalid_panel" || strings.Contains(w.Body.String(), "request body too large") {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}
