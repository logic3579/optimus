package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/models"
)

type datasourceHandlerStub struct {
	fail bool
	seen []string
}

func (s *datasourceHandlerStub) result(name string) error {
	s.seen = append(s.seen, name)
	if s.fail {
		return errors.New("upstream failed")
	}
	return nil
}
func (s *datasourceHandlerStub) List(_ context.Context, q ListQuery) (*ListResponse, error) {
	if q.Page != 2 || q.PageSize != 5 || q.Q != "prod" || q.AuthType != "bearer" || q.ClusterID == nil || *q.ClusterID != 7 {
		return nil, errors.New("unexpected list query")
	}
	return &ListResponse{Items: []Detail{{ID: 3, Name: "prom"}}, Total: 1, Page: 2, PageSize: 5}, s.result("list")
}
func (s *datasourceHandlerStub) Get(_ context.Context, id uint64) (*Detail, error) {
	return &Detail{ID: id, Name: "prom"}, s.result("get")
}
func (s *datasourceHandlerStub) Create(_ context.Context, actor uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	if actor != 42 || ip == "" || ua != "datasource-behavior" || req.Name != "prom" {
		return nil, errors.New("unexpected create contract")
	}
	return &Detail{ID: 3, Name: req.Name}, s.result("create")
}
func (s *datasourceHandlerStub) Update(_ context.Context, actor uint64, _, _ string, id uint64, req UpdateRequest) (*Detail, error) {
	if actor != 42 || id != 3 || req.Name == nil || *req.Name != "prom-new" {
		return nil, errors.New("unexpected update contract")
	}
	return &Detail{ID: id, Name: *req.Name}, s.result("update")
}
func (s *datasourceHandlerStub) Delete(_ context.Context, actor uint64, _, _ string, id uint64) error {
	if actor != 42 || id != 3 {
		return errors.New("unexpected delete contract")
	}
	return s.result("delete")
}
func (s *datasourceHandlerStub) TestConnection(_ context.Context, actor uint64, _, _ string, id uint64) (*TestResponse, error) {
	if actor != 42 || id != 3 {
		return nil, errors.New("unexpected test contract")
	}
	return &TestResponse{Reachable: true, Version: "2.53"}, s.result("test")
}

func TestDatasourceHandlerSuccessContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &datasourceHandlerStub{}
	h := &Handler{svc: stub}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(42)); c.Next() })
	r.GET("/datasources", h.List)
	r.GET("/datasources/:id", h.Get)
	r.POST("/datasources", h.Create)
	r.PUT("/datasources/:id", h.Update)
	r.DELETE("/datasources/:id", h.Delete)
	r.POST("/datasources/:id/test", h.Test)
	clusterID := uint64(7)
	create := CreateRequest{Name: "prom", BaseURL: "https://prom.example", AuthType: "none", ClusterID: &clusterID}
	updateName := "prom-new"
	update := UpdateRequest{Name: &updateName}
	createBody, _ := json.Marshal(create)
	updateBody, _ := json.Marshal(update)
	for _, tc := range []struct {
		method, path string
		body         []byte
	}{
		{http.MethodGet, "/datasources?page=2&page_size=5&q=prod&auth_type=bearer&cluster_id=7", nil},
		{http.MethodGet, "/datasources/3", nil},
		{http.MethodPost, "/datasources", createBody},
		{http.MethodPut, "/datasources/3", updateBody},
		{http.MethodDelete, "/datasources/3", nil},
		{http.MethodPost, "/datasources/3/test", nil},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "datasource-behavior")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	if len(stub.seen) != 6 {
		t.Fatalf("calls=%v", stub.seen)
	}
}

func TestDatasourceHandlerBindingAndServiceErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &datasourceHandlerStub{fail: true}
	h := &Handler{svc: stub}
	r := gin.New()
	r.GET("/datasources", h.List)
	r.GET("/datasources/:id", h.Get)
	r.POST("/datasources", h.Create)
	r.PUT("/datasources/:id", h.Update)
	r.DELETE("/datasources/:id", h.Delete)
	r.POST("/datasources/:id/test", h.Test)
	validCreate := `{"name":"prom","base_url":"https://prom.example","auth_type":"none"}`
	validUpdate := `{"name":"prom-new"}`
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/datasources?page=bad", ""},
		{http.MethodGet, "/datasources/3", ""},
		{http.MethodPost, "/datasources", "{"},
		{http.MethodPost, "/datasources", validCreate},
		{http.MethodPut, "/datasources/3", "{"},
		{http.MethodPut, "/datasources/3", validUpdate},
		{http.MethodDelete, "/datasources/3", ""},
		{http.MethodPost, "/datasources/3/test", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code < 400 {
			t.Fatalf("%s %s unexpectedly succeeded: %s", tc.method, tc.path, w.Body.String())
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPost} {
		path := "/datasources/0"
		if method == http.MethodPost {
			path += "/test"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s invalid id status=%d body=%s", method, w.Code, w.Body.String())
		}
	}
}

func TestDatasourceRepositoryAndDTOContracts(t *testing.T) {
	if NewRepo(nil) == nil {
		t.Fatal("NewRepo returned nil")
	}
	if page, size, offset, err := pageValues(0, 0); err != nil || page != 1 || size != 20 || offset != 0 {
		t.Fatalf("defaults=%d/%d/%d err=%v", page, size, offset, err)
	}
	if page, size, offset, err := pageValues(2, 200); err != nil || page != 2 || size != 100 || offset != 100 {
		t.Fatalf("cap=%d/%d/%d err=%v", page, size, offset, err)
	}
	if _, _, _, err := pageValues(int(^uint(0)>>1), 2); err == nil {
		t.Fatal("expected pagination overflow")
	}
	credentialID, clusterID, actor := uint64(2), uint64(3), uint64(4)
	ca := "public-ca"
	row := &models.ObservabilityDatasource{
		ID: 7, Name: "prom", BaseURL: "https://prom.example", AuthType: "bearer",
		HTTPCredentialID: &credentialID, ClusterID: &clusterID, CustomCAPEM: &ca, CreatedByUserID: &actor,
	}
	got := detailFromModel(row, "token", "prod")
	if got.ID != 7 || got.HTTPCredential == nil || got.HTTPCredential.Name != "token" || got.Cluster == nil || got.Cluster.Name != "prod" || !got.HasCustomCA {
		t.Fatalf("detail=%#v", got)
	}
	copy := got.CustomCAPEMCopy()
	copy[0] = 'X'
	if string(got.CustomCAPEMCopy()) != ca {
		t.Fatal("CA copy aliases internal data")
	}
	if (Detail{}).CustomCAPEMCopy() != nil {
		t.Fatal("empty CA should return nil")
	}
	duplicate := &pgconn.PgError{Code: "23505", ConstraintName: "observability_datasource_name_unique"}
	if errors.Is(mapWriteError(duplicate), duplicate) {
		t.Fatal("duplicate was not mapped")
	}
	other := errors.New("other")
	if !errors.Is(mapWriteError(other), other) || notFound() == nil || nameTaken() == nil {
		t.Fatal("repository error helpers violated contract")
	}
}

func TestDatasourceSmallServiceContracts(t *testing.T) {
	blank := " \n "
	if cleanCA(nil) != nil || cleanCA(&blank) != nil {
		t.Fatal("empty CA was not normalized to nil")
	}
	value := "  certificate  "
	cleaned := cleanCA(&value)
	if cleaned == nil || *cleaned != "certificate" {
		t.Fatalf("cleaned CA=%v", cleaned)
	}
	if actorPtr(0) != nil || actorPtr(12) == nil || *actorPtr(12) != 12 {
		t.Fatal("actor pointer contract failed")
	}
	if err := (&Service{}).record(nil, context.Background(), 1, "action", 2, "", "", nil); err != nil {
		t.Fatalf("nil audit writer should be ignored: %v", err)
	}
}
