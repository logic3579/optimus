package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"optimus-be/internal/models"
)

type handlerStub struct {
	err     error
	key     string
	creates int
	run     *Run
	detail  *RunView
	list    *ListResponse
}

func TestRunHTTPViewOmitsFingerprintAndIncludesSafeRecoveryFields(t *testing.T) {
	code, key, correlation := 45302, "delivery.execution.artifact_drift", "corr-safe"
	row := &models.DeliveryRun{ID: 1, RequestFingerprint: "internal-fingerprint", ErrorCode: &code, ErrorMessageKey: &key, CorrelationID: &correlation}
	stages := []models.DeliveryRunStage{{ID: 2, ErrorCode: &code, ErrorMessageKey: &key, CorrelationID: &correlation}}
	body, err := json.Marshal(runViewFromRows(row, stages))
	require.NoError(t, err)
	require.NotContains(t, string(body), "fingerprint")
	require.NotContains(t, string(body), "internal-fingerprint")
	require.Contains(t, string(body), `"error_code":45302`)
	require.Contains(t, string(body), `"error_message_key":"delivery.execution.artifact_drift"`)
	require.Contains(t, string(body), `"correlation_id":"corr-safe"`)
}

func (s *handlerStub) List(context.Context, uint64, ListQuery) (*ListResponse, error) {
	if s.list != nil {
		return s.list, s.err
	}
	return &ListResponse{}, s.err
}
func (s *handlerStub) Get(context.Context, uint64) (*RunView, error) {
	if s.detail != nil {
		return s.detail, s.err
	}
	return &RunView{}, s.err
}
func (s *handlerStub) Create(_ context.Context, _ uint64, _, _ string, _ uint64, k string, _ CreateRequest) (*Run, error) {
	s.key = k
	s.creates++
	if s.run != nil {
		return s.run, s.err
	}
	return &Run{}, s.err
}
func (s *handlerStub) Cancel(context.Context, uint64, string, string, uint64) (*Run, error) {
	return &Run{}, s.err
}

func TestRunHandlerResponseContractsUseStringTimeoutAndOmitInternals(t *testing.T) {
	stage := Stage{ID: 2, EnvironmentID: 3, EnvironmentKey: "prod", EnvironmentName: "Production", ApplicationID: 4,
		ClusterID: 5, Namespace: "apps", ReleaseName: "demo", Order: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease,
		ApprovalRequired: true, Timeout: 10 * time.Minute, State: models.DeliveryStageRunning, OperationID: "internal-operation"}
	base := &Run{ID: 1, RequestFingerprint: "internal-fingerprint", Stages: []Stage{stage}}
	detail := runViewFromRun(base)
	stub := &handlerStub{run: base, detail: detail, list: &ListResponse{Items: []RunView{*detail}, Page: 1, PageSize: 20}}
	for _, tc := range []struct {
		name, method, path, body string
		setup                    func(*http.Request)
	}{
		{"create", http.MethodPost, "/projects/1/runs", `{"chart_repo_id":1,"chart_name":"demo","chart_version":"1.0.0"}`, func(r *http.Request) {
			r.Header.Set("Idempotency-Key", "key")
			r.Header.Set("Content-Type", "application/json")
		}},
		{"get", http.MethodGet, "/runs/1", "", nil},
		{"list", http.MethodGet, "/projects/1/runs", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			h := NewHandler(stub)
			r.POST("/projects/:id/runs", h.Create)
			r.GET("/runs/:id", h.Get)
			r.GET("/projects/:id/runs", h.List)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.setup != nil {
				tc.setup(req)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var envelope map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
			encoded := w.Body.String()
			require.Contains(t, encoded, `"timeout":"10m0s"`)
			require.NotContains(t, encoded, "request_fingerprint")
			require.NotContains(t, encoded, "internal-fingerprint")
			require.NotContains(t, encoded, "operation_id")
			require.NotContains(t, encoded, "internal-operation")
			require.NotContains(t, encoded, "lease_owner")
			require.NotContains(t, encoded, "lease_expires_at")
		})
	}
}
func (s *handlerStub) RequestReconcile(context.Context, uint64, string, string, uint64) (*Run, error) {
	return &Run{}, s.err
}
func (s *handlerStub) Retry(_ context.Context, _ uint64, _, _ string, _ uint64, k string) (*Run, error) {
	s.key = k
	s.creates++
	return &Run{}, s.err
}

func TestRunHandlerMountsExactRoutesAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seen := map[string]int{}
	NewHandler(&handlerStub{}).Mount(r.Group("/api/v1/delivery"), func(code string) gin.HandlerFunc { seen[code]++; return func(c *gin.Context) { c.Next() } })
	require.Equal(t, map[string]int{"delivery:run:read": 1, "delivery:run:create": 1, "delivery:run:cancel": 1}, seen)
	want := map[string]bool{}
	for _, x := range []string{"GET /api/v1/delivery/projects/:id/runs", "POST /api/v1/delivery/projects/:id/runs", "GET /api/v1/delivery/runs/:id", "POST /api/v1/delivery/runs/:id/cancel", "POST /api/v1/delivery/runs/:id/reconcile", "POST /api/v1/delivery/runs/:id/retry"} {
		want[x] = true
	}
	for _, x := range r.Routes() {
		delete(want, fmt.Sprintf("%s %s", x.Method, x.Path))
	}
	require.Empty(t, want)
	require.Len(t, r.Routes(), 6)
}

func TestRunHandlerRequiresAndBoundsIdempotencyKey(t *testing.T) {
	for _, tc := range []struct{ name, path, key, body string }{{"create missing", "/projects/1/runs", "", `{"chart_repo_id":1,"chart_name":"app","chart_version":"1.0.0"}`}, {"retry missing", "/runs/1/retry", "", ""}, {"too long", "/runs/1/retry", strings.Repeat("k", 129), ""}} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &handlerStub{}
			h := NewHandler(stub)
			r := gin.New()
			r.POST("/projects/:id/runs", h.Create)
			r.POST("/runs/:id/retry", h.Retry)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", tc.key)
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Zero(t, stub.creates)
			require.Contains(t, w.Body.String(), `"code"`)
		})
	}
}

func TestRunHandlerTrimsAndPassesIdempotencyKey(t *testing.T) {
	stub := &handlerStub{}
	r := gin.New()
	r.POST("/runs/:id/retry", NewHandler(stub).Retry)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/runs/9/retry", nil)
	req.Header.Set("Idempotency-Key", "  stable-key  ")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "stable-key", stub.key)
	require.Equal(t, 1, stub.creates)
}

func TestRunHandlerRejectsPaginationCapAndHidesRawError(t *testing.T) {
	stub := &handlerStub{}
	r := gin.New()
	h := NewHandler(stub)
	r.GET("/projects/:id/runs", h.List)
	r.GET("/runs/:id", h.Get)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/1/runs?page_size=101", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	stub.err = errors.New("authorization=secret")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/1", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, w.Body.String(), "authorization")
	require.NotContains(t, w.Body.String(), "secret")
}
