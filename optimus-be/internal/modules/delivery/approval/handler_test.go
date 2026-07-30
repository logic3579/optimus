package approval

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type handlerStub struct {
	err             error
	approve, reject int
	comment         string
}

func (s *handlerStub) ListPending(context.Context, uint64) ([]PendingApproval, error) {
	return []PendingApproval{}, s.err
}
func (s *handlerStub) Approve(_ context.Context, _ uint64, _, _ string, _ uint64, r DecisionRequest) (*Decision, error) {
	s.approve++
	s.comment = r.Comment
	return &Decision{}, s.err
}
func (s *handlerStub) Reject(_ context.Context, _ uint64, _, _ string, _ uint64, r DecisionRequest) (*Decision, error) {
	s.reject++
	s.comment = r.Comment
	return &Decision{}, s.err
}

func TestApprovalHandlerMountsExactRoutesAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seen := map[string]int{}
	NewHandler(&handlerStub{}).Mount(r.Group("/api/v1/delivery"), func(code string) gin.HandlerFunc { seen[code]++; return func(c *gin.Context) { c.Next() } })
	require.Equal(t, map[string]int{"delivery:approval:read": 1, "delivery:approval:decide": 1}, seen)
	want := map[string]bool{"GET /api/v1/delivery/approvals/pending": true, "POST /api/v1/delivery/run-stages/:id/approve": true, "POST /api/v1/delivery/run-stages/:id/reject": true}
	for _, x := range r.Routes() {
		delete(want, fmt.Sprintf("%s %s", x.Method, x.Path))
	}
	require.Empty(t, want)
	require.Len(t, r.Routes(), 3)
}

func TestApprovalHandlerRejectsBadIDMissingAndOversizedCommentBeforeService(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{{"bad id", "/run-stages/nope/approve", `{"comment":"ok"}`}, {"missing comment", "/run-stages/1/approve", `{}`}, {"over 512 runes", "/run-stages/1/approve", `{"comment":"` + strings.Repeat("界", 513) + `"}`}} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &handlerStub{}
			r := gin.New()
			r.POST("/run-stages/:id/approve", NewHandler(stub).Approve)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Zero(t, stub.approve)
			require.Contains(t, w.Body.String(), `"code"`)
			require.NotContains(t, w.Body.String(), strings.Repeat("界", 20))
		})
	}
}

func TestApprovalHandlerDispatchesDecisionAndHidesRawError(t *testing.T) {
	stub := &handlerStub{}
	h := NewHandler(stub)
	r := gin.New()
	r.POST("/run-stages/:id/reject", h.Reject)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run-stages/3/reject", strings.NewReader(`{"comment":"because"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, stub.reject)
	require.Equal(t, "because", stub.comment)
	stub.err = errors.New("kubeconfig=secret")
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/run-stages/3/reject", strings.NewReader(`{"comment":"because"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, w.Body.String(), "kubeconfig")
	require.NotContains(t, w.Body.String(), "secret")
}
