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
	step  time.Duration
}
type fakeSourceLister struct{}

func (fakeSourceLister) ListQuerySources(context.Context) ([]QuerySource, error) {
	clusterID := uint64(7)
	return []QuerySource{{ID: 3, Name: "prom", ClusterID: &clusterID}}, nil
}

func (f *fakeHandlerService) Instant(_ context.Context, a uint64, _ InstantRequest) (*BatchResult, error) {
	f.actor = a
	return &BatchResult{}, nil
}
func (f *fakeHandlerService) Range(_ context.Context, _ uint64, req RangeRequest) (*BatchResult, error) {
	f.step = req.Step
	if req.Step < 15*time.Second {
		return nil, invalid()
	}
	return &BatchResult{}, nil
}
func (f *fakeHandlerService) Labels(context.Context, uint64, uint64) ([]string, error) {
	return []string{"job"}, nil
}

func TestRangeStepAcceptsDurationStringsAndRejectsNumbers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, step string
		status     int
		want       time.Duration
	}{
		{"fifteen seconds", `"15s"`, 200, 15 * time.Second},
		{"minute", `"1m"`, 200, time.Minute},
		{"malformed", `"soon"`, 400, 0},
		{"number", `60000000000`, 400, 0},
		{"too small", `"1s"`, 400, time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeHandlerService{}
			r := gin.New()
			NewHandler(s).Mount(r.Group("/observability"), func(string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } })
			body := `{"datasource_id":1,"start":"2026-07-20T00:00:00Z","end":"2026-07-20T01:00:00Z","step":` + tc.step + `,"queries":[{"ref_id":"a","promql":"up"}]}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/observability/query-range", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.want, s.step)
			if tc.status == http.StatusBadRequest {
				require.Contains(t, w.Body.String(), `"code":44107`)
			}
		})
	}
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
	NewHandler(s, fakeSourceLister{}).Mount(r.Group("/observability"), func(p string) gin.HandlerFunc { perms = append(perms, p); return func(c *gin.Context) { c.Next() } })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/observability/query-sources", nil))
	require.Equal(t, 200, w.Code)
	require.JSONEq(t, `{"code":0,"message":"","data":[{"id":3,"name":"prom","cluster_id":7}]}`, w.Body.String())
	require.NotContains(t, w.Body.String(), "base_url")
	w = httptest.NewRecorder()
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
