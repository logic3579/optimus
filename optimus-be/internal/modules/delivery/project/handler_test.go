package project

import (
	"bytes"
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
	listQuery ListQuery
	createReq CreateProjectRequest
	bindReq   BindEnvironmentRequest
	updateIDs [2]uint64
	err       error
}

func (s *handlerStub) List(_ context.Context, q ListQuery) (*ListResponse, error) {
	s.listQuery = q
	return &ListResponse{Page: q.Page, PageSize: q.PageSize}, s.err
}
func (s *handlerStub) Get(context.Context, uint64) (*ProjectDetail, error) {
	return &ProjectDetail{}, s.err
}
func (s *handlerStub) ListEnvironments(context.Context, uint64) ([]Environment, error) {
	return []Environment{}, s.err
}
func (s *handlerStub) CreateProject(_ context.Context, _ uint64, _, _ string, r CreateProjectRequest) (*ProjectDetail, error) {
	s.createReq = r
	return &ProjectDetail{}, s.err
}
func (s *handlerStub) UpdateProject(context.Context, uint64, string, string, uint64, UpdateProjectRequest) (*ProjectDetail, error) {
	return &ProjectDetail{}, s.err
}
func (s *handlerStub) DeleteProject(context.Context, uint64, string, string, uint64) error {
	return s.err
}
func (s *handlerStub) BindEnvironment(_ context.Context, _ uint64, _, _ string, _ uint64, r BindEnvironmentRequest) (*Environment, error) {
	s.bindReq = r
	return &Environment{}, s.err
}
func (s *handlerStub) UpdateEnvironment(_ context.Context, _ uint64, _, _ string, pid, eid uint64, _ UpdateEnvironmentRequest) (*Environment, error) {
	s.updateIDs = [2]uint64{pid, eid}
	return &Environment{}, s.err
}
func (s *handlerStub) UnbindEnvironment(context.Context, uint64, string, string, uint64, uint64) error {
	return s.err
}

func TestProjectHandlerMountsExactRoutesAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seen := map[string]int{}
	NewHandler(&handlerStub{}).Mount(r.Group("/api/v1/delivery"), func(code string) gin.HandlerFunc { seen[code]++; return func(c *gin.Context) { c.Next() } })
	require.Equal(t, map[string]int{"delivery:project:read": 1, "delivery:project:write": 1, "delivery:project:delete": 1}, seen)
	want := map[string]bool{}
	for _, route := range []string{
		"GET /api/v1/delivery/projects", "POST /api/v1/delivery/projects", "GET /api/v1/delivery/projects/:id", "PUT /api/v1/delivery/projects/:id", "DELETE /api/v1/delivery/projects/:id",
		"GET /api/v1/delivery/projects/:id/environments", "POST /api/v1/delivery/projects/:id/environments", "PUT /api/v1/delivery/projects/:id/environments/:environmentId", "DELETE /api/v1/delivery/projects/:id/environments/:environmentId",
	} {
		want[route] = true
	}
	for _, route := range r.Routes() {
		delete(want, fmt.Sprintf("%s %s", route.Method, route.Path))
	}
	require.Empty(t, want)
	require.Len(t, r.Routes(), 9)
}

func TestProjectHandlerRejectsInvalidIDsQueriesAndBodies(t *testing.T) {
	tests := []struct{ name, method, path, body string }{
		{"zero project id", http.MethodGet, "/projects/0", ""},
		{"signed project id", http.MethodGet, "/projects/-1", ""},
		{"bad environment id", http.MethodPut, "/projects/2/environments/nope", `{}`},
		{"page size cap", http.MethodGet, "/projects?page_size=101", ""},
		{"malformed create", http.MethodPost, "/projects", `{"name":`},
		{"invalid binding", http.MethodPost, "/projects/2/environments", `{"environment_key":"prod","display_name":"Production"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			h := NewHandler(&handlerStub{})
			r.GET("/projects", h.List)
			r.GET("/projects/:id", h.Get)
			r.POST("/projects", h.Create)
			r.POST("/projects/:id/environments", h.BindEnvironment)
			r.PUT("/projects/:id/environments/:environmentId", h.UpdateEnvironment)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			requireEnvelope(t, w.Body.String())
			if tt.body != "" {
				require.NotContains(t, w.Body.String(), tt.body)
			}
		})
	}
}

func TestProjectHandlerRejectsOversizedBodyWithoutRawLeakage(t *testing.T) {
	r := gin.New()
	r.POST("/projects", NewHandler(&handlerStub{}).Create)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewReader(bytes.Repeat([]byte("s"), int(maxRequestBodyBytes)+1)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	requireEnvelope(t, w.Body.String())
	require.NotContains(t, strings.ToLower(w.Body.String()), "request body too large")
}

func TestProjectHandlerHidesRawServiceErrors(t *testing.T) {
	r := gin.New()
	r.GET("/projects", NewHandler(&handlerStub{err: errors.New("kubeconfig=super-secret")}).List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	requireEnvelope(t, w.Body.String())
	require.NotContains(t, w.Body.String(), "kubeconfig")
	require.NotContains(t, w.Body.String(), "super-secret")
}

func requireEnvelope(t *testing.T, body string) {
	t.Helper()
	require.Contains(t, body, `"code"`)
	require.Contains(t, body, `"data"`)
	require.Contains(t, body, `"message"`)
}
