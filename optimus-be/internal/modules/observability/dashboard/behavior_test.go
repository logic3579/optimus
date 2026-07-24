package dashboard

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
	"gorm.io/gorm"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

type dashboardServiceStub struct {
	listFn   func(context.Context, ListQuery) (*ListResponse, error)
	getFn    func(context.Context, uint64) (*Detail, error)
	createFn func(context.Context, uint64, string, string, SaveRequest) (*Detail, error)
	updateFn func(context.Context, uint64, string, string, uint64, SaveRequest) (*Detail, error)
	deleteFn func(context.Context, uint64, string, string, uint64) error
}

func (s dashboardServiceStub) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	return s.listFn(ctx, q)
}
func (s dashboardServiceStub) Get(ctx context.Context, id uint64) (*Detail, error) {
	return s.getFn(ctx, id)
}
func (s dashboardServiceStub) Create(ctx context.Context, actor uint64, ip, ua string, req SaveRequest) (*Detail, error) {
	return s.createFn(ctx, actor, ip, ua, req)
}
func (s dashboardServiceStub) Update(ctx context.Context, actor uint64, ip, ua string, id uint64, req SaveRequest) (*Detail, error) {
	return s.updateFn(ctx, actor, ip, ua, id, req)
}
func (s dashboardServiceStub) Delete(ctx context.Context, actor uint64, ip, ua string, id uint64) error {
	return s.deleteFn(ctx, actor, ip, ua, id)
}

func TestHandlerBehaviorContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := SaveRequest{Name: "ops", RefreshIntervalS: 30, TimeRange: "1h", Panels: []PanelInput{{DatasourceID: 2, Title: "CPU", PanelType: "stat", PromQL: "up", Unit: "none", Width: 6}}}
	stub := dashboardServiceStub{
		listFn: func(_ context.Context, q ListQuery) (*ListResponse, error) {
			if q.Page != 2 || q.PageSize != 5 || q.Q != "ops" {
				t.Fatalf("unexpected list query: %#v", q)
			}
			return &ListResponse{Items: []Detail{{ID: 7, Name: "ops"}}, Total: 1, Page: 2, PageSize: 5}, nil
		},
		getFn: func(_ context.Context, id uint64) (*Detail, error) {
			if id != 7 {
				t.Fatalf("get id=%d", id)
			}
			return &Detail{ID: id, Name: "ops"}, nil
		},
		createFn: func(_ context.Context, actor uint64, ip, ua string, req SaveRequest) (*Detail, error) {
			if actor != 42 || ip == "" || ua != "behavior-test" || req.Name != valid.Name {
				t.Fatalf("unexpected create contract: actor=%d ip=%q ua=%q req=%#v", actor, ip, ua, req)
			}
			return &Detail{ID: 8, Name: req.Name}, nil
		},
		updateFn: func(_ context.Context, actor uint64, _, _ string, id uint64, req SaveRequest) (*Detail, error) {
			if actor != 42 || id != 7 || req.Name != valid.Name {
				t.Fatalf("unexpected update contract")
			}
			return &Detail{ID: id, Name: req.Name}, nil
		},
		deleteFn: func(_ context.Context, actor uint64, _, _ string, id uint64) error {
			if actor != 42 || id != 7 {
				t.Fatalf("unexpected delete contract")
			}
			return nil
		},
	}
	h := &Handler{svc: stub}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uint64(42)); c.Next() })
	r.GET("/dashboards", h.List)
	r.GET("/dashboards/:id", h.Get)
	r.POST("/dashboards", h.Create)
	r.PUT("/dashboards/:id", h.Update)
	r.DELETE("/dashboards/:id", h.Delete)

	body, _ := json.Marshal(valid)
	cases := []struct {
		method, path string
		body         []byte
	}{
		{http.MethodGet, "/dashboards?page=2&page_size=5&q=ops", nil},
		{http.MethodGet, "/dashboards/7", nil},
		{http.MethodPost, "/dashboards", body},
		{http.MethodPut, "/dashboards/7", body},
		{http.MethodDelete, "/dashboards/7", nil},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "behavior-test")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/dashboards/0", "/dashboards/nope"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid id status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestHandlerMapsBindingAndServiceErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	boom := errors.New("boom")
	stub := dashboardServiceStub{
		listFn:   func(context.Context, ListQuery) (*ListResponse, error) { return nil, boom },
		getFn:    func(context.Context, uint64) (*Detail, error) { return nil, boom },
		createFn: func(context.Context, uint64, string, string, SaveRequest) (*Detail, error) { return nil, boom },
		updateFn: func(context.Context, uint64, string, string, uint64, SaveRequest) (*Detail, error) { return nil, boom },
		deleteFn: func(context.Context, uint64, string, string, uint64) error { return boom },
	}
	h := &Handler{svc: stub}
	r := gin.New()
	r.GET("/dashboards", h.List)
	r.GET("/dashboards/:id", h.Get)
	r.POST("/dashboards", h.Create)
	r.PUT("/dashboards/:id", h.Update)
	r.DELETE("/dashboards/:id", h.Delete)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/dashboards?page=bad", ""},
		{http.MethodGet, "/dashboards/1", ""},
		{http.MethodPost, "/dashboards", "{"},
		{http.MethodPost, "/dashboards", `{"name":"x","refresh_interval_s":30,"time_range":"1h","panels":[]}`},
		{http.MethodPut, "/dashboards/1", "{"},
		{http.MethodPut, "/dashboards/1", `{"name":"x","refresh_interval_s":30,"time_range":"1h","panels":[]}`},
		{http.MethodDelete, "/dashboards/1", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code < 400 {
			t.Fatalf("%s %s unexpectedly succeeded: %s", tc.method, tc.path, w.Body.String())
		}
	}
}

type dashboardRepoStub struct {
	detail       Detail
	model        models.ObservabilityDashboard
	panels       []models.ObservabilityPanel
	auditEvents  []audit.Event
	deletedRows  int64
	panelCount   int64
	nameConflict bool
}

func (r *dashboardRepoStub) List(context.Context, ListQuery) ([]Detail, int64, error) {
	return []Detail{r.detail}, 1, nil
}
func (r *dashboardRepoStub) GetByID(_ context.Context, id uint64) (*Detail, error) {
	out := r.detail
	out.ID = id
	return &out, nil
}
func (r *dashboardRepoStub) Transaction(_ context.Context, fn func(*gorm.DB) error) error {
	return fn(&gorm.DB{})
}
func (r *dashboardRepoStub) GetModelForUpdate(context.Context, *gorm.DB, uint64) (*models.ObservabilityDashboard, error) {
	out := r.model
	return &out, nil
}
func (r *dashboardRepoStub) FindNameAliveTx(context.Context, *gorm.DB, string, uint64) (bool, error) {
	return r.nameConflict, nil
}
func (r *dashboardRepoStub) LockActiveDatasourcesTx(context.Context, *gorm.DB, []uint64) error {
	return nil
}
func (r *dashboardRepoStub) CreateTx(_ context.Context, _ *gorm.DB, row *models.ObservabilityDashboard) error {
	row.ID = 9
	return nil
}
func (r *dashboardRepoStub) UpdateTx(context.Context, *gorm.DB, uint64, map[string]any) (int64, error) {
	return 1, nil
}
func (r *dashboardRepoStub) ReplacePanelsTx(_ context.Context, _ *gorm.DB, _ uint64, panels []models.ObservabilityPanel) error {
	r.panels = append([]models.ObservabilityPanel(nil), panels...)
	return nil
}
func (r *dashboardRepoStub) CountPanelsTx(context.Context, *gorm.DB, uint64) (int64, error) {
	return r.panelCount, nil
}
func (r *dashboardRepoStub) DeleteTx(context.Context, *gorm.DB, uint64) (int64, error) {
	return r.deletedRows, nil
}

type dashboardAuditStub struct{ repo *dashboardRepoStub }

func (a dashboardAuditStub) Record(_ context.Context, event audit.Event) error {
	a.repo.auditEvents = append(a.repo.auditEvents, event)
	return nil
}

func TestServiceAggregateHappyPaths(t *testing.T) {
	repo := &dashboardRepoStub{
		detail:      Detail{Name: "ops"},
		model:       models.ObservabilityDashboard{Name: "old", Description: "old", RefreshIntervalS: 30, TimeRange: "1h"},
		deletedRows: 1,
		panelCount:  2,
	}
	svc := &Service{repo: repo, auditTx: func(*gorm.DB) auditWriter { return dashboardAuditStub{repo} }}
	req := SaveRequest{Name: " ops ", Description: " desc ", RefreshIntervalS: 60, TimeRange: "6h", Panels: []PanelInput{
		{DatasourceID: 5, Title: " CPU ", PanelType: "time_series", PromQL: " rate(cpu[5m]) ", Unit: "cores", Legend: " pod ", SortOrder: 0, Width: 6},
		{DatasourceID: 3, Title: "Memory", PanelType: "stat", PromQL: "memory", Unit: "bytes", SortOrder: 1, Width: 12},
		{DatasourceID: 5, Title: "Again", PanelType: "table", PromQL: "up", Unit: "none", SortOrder: 2, Width: 12},
	}}
	list, err := svc.List(context.Background(), ListQuery{Page: 2, PageSize: 10})
	if err != nil || list.Total != 1 || list.Page != 2 || list.PageSize != 10 {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	if got, err := svc.Get(context.Background(), 4); err != nil || got.ID != 4 {
		t.Fatalf("get=%#v err=%v", got, err)
	}
	created, err := svc.Create(context.Background(), 11, "127.0.0.1", "ua", req)
	if err != nil || created.ID != 9 || len(repo.panels) != 3 || repo.panels[0].Title != "CPU" || repo.panels[0].PromQL != "rate(cpu[5m])" {
		t.Fatalf("create=%#v panels=%#v err=%v", created, repo.panels, err)
	}
	updated, err := svc.Update(context.Background(), 11, "127.0.0.1", "ua", 9, req)
	if err != nil || updated.ID != 9 {
		t.Fatalf("update=%#v err=%v", updated, err)
	}
	if err := svc.Delete(context.Background(), 11, "127.0.0.1", "ua", 9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(repo.auditEvents) != 3 {
		t.Fatalf("audit events=%d", len(repo.auditEvents))
	}
	for _, event := range repo.auditEvents {
		if event.UserID == nil || event.TargetType != "observability_dashboard" || event.TargetID != "9" {
			t.Fatalf("bad audit event: %#v", event)
		}
	}
}

func TestServiceRejectsNameConflictsAndMissingDelete(t *testing.T) {
	repo := &dashboardRepoStub{nameConflict: true, deletedRows: 0}
	svc := &Service{repo: repo, auditTx: func(*gorm.DB) auditWriter { return dashboardAuditStub{repo} }}
	req := SaveRequest{Name: "ops", RefreshIntervalS: 30, TimeRange: "1h"}
	if _, err := svc.Create(context.Background(), 0, "", "", req); err == nil {
		t.Fatal("expected create name conflict")
	}
	repo.nameConflict = false
	if err := svc.Delete(context.Background(), 0, "", "", 1); err == nil {
		t.Fatal("expected missing delete")
	}
}

func TestDashboardRepositoryPureContracts(t *testing.T) {
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
	actor := uint64(8)
	model := &models.ObservabilityDashboard{ID: 4, Name: "ops", CreatedByUserID: &actor}
	panel := models.ObservabilityPanel{ID: 2, DashboardID: 4, DatasourceID: 9, Title: "cpu", PanelType: "stat", PromQL: "up", Unit: "none", Width: 6}
	got := detailFromModel(model, []models.ObservabilityPanel{panel})
	if got.ID != 4 || got.CreatedByUserID == nil || len(got.Panels) != 1 || got.Panels[0].DatasourceID != 9 {
		t.Fatalf("detail=%#v", got)
	}
	duplicate := &pgconn.PgError{Code: "23505", ConstraintName: "observability_dashboard_name_unique"}
	if errors.Is(mapWriteError(duplicate), duplicate) {
		t.Fatal("duplicate was not mapped")
	}
	other := errors.New("other")
	if !errors.Is(mapWriteError(other), other) {
		t.Fatal("non-duplicate error changed")
	}
	if notFound() == nil || nameTaken() == nil || actorPtr(0) != nil || actorPtr(3) == nil {
		t.Fatal("error/actor helpers violated contract")
	}
}
