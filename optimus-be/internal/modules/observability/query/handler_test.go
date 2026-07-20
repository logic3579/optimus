package query

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"optimus-be/internal/infra/middleware"
	"strings"
	"testing"
	"time"
)

type fakeHandlerService struct {
	actor uint64
	label string
}

func (f *fakeHandlerService) Instant(_ context.Context, a uint64, _ InstantRequest) (*BatchResult, error) {
	f.actor = a
	return &BatchResult{}, nil
}
func (f *fakeHandlerService) Range(context.Context, uint64, RangeRequest) (*BatchResult, error) {
	return &BatchResult{}, nil
}
func (f *fakeHandlerService) Labels(context.Context, uint64, uint64) ([]string, error) {
	return []string{"job"}, nil
}
func (f *fakeHandlerService) LabelValues(_ context.Context, _ uint64, _ uint64, l string) ([]string, error) {
	f.label = l
	return []string{"api"}, nil
}
func TestRoutesAndActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &fakeHandlerService{}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(8)); c.Next() })
	var perms []string
	NewHandler(s).Mount(r.Group("/observability"), func(p string) gin.HandlerFunc { perms = append(perms, p); return func(c *gin.Context) { c.Next() } })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/observability/query", strings.NewReader(`{"datasource_id":1,"queries":[{"ref_id":"a","promql":"up"}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Equal(t, uint64(8), s.actor)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/observability/datasources/1/label-values?label=job", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "job", s.label)
	require.Equal(t, []string{"observability:metric:read"}, perms)
}

var _ = time.Second
