package vpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/models"
)

func TestHandlerMountRoutesWithResourceReadPermission(t *testing.T) {
	router := gin.New()
	var permissions []string
	NewHandler(NewService(&stubRepo{})).Mount(router.Group("/api/v1/assets"), func(code string) gin.HandlerFunc {
		permissions = append(permissions, code)
		return func(c *gin.Context) { c.Next() }
	})
	if len(permissions) != 1 || permissions[0] != "assets:resource:read" {
		t.Fatalf("permissions = %#v", permissions)
	}
	routes := router.Routes()
	if len(routes) != 2 || routes[0].Path != "/api/v1/assets/vpcs" || routes[1].Path != "/api/v1/assets/vpcs/:id/subnets" {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestHandlerListSuccessAndQueryBinding(t *testing.T) {
	repo := &stubRepo{vpcs: []Summary{}, vpcTotal: 0}
	router := gin.New()
	router.GET("/vpcs", NewHandler(NewService(repo)).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/vpcs?account_id=3&region=us-east-1&q=prod&include_deleted=true&page=2&size=5", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	want := ListFilter{AccountID: 3, Region: "us-east-1", Q: "prod", IncludeDeleted: true, Page: 2, Size: 5}
	if repo.listFilter != want {
		t.Fatalf("filter=%#v want=%#v", repo.listFilter, want)
	}
	assertEmptyItemsEnvelope(t, recorder)
}

func TestHandlerListSubnetsSuccessAndQueryBinding(t *testing.T) {
	vpc := &models.AWSVPC{ID: 8, CloudAccountID: 3, Region: "us-east-1", VPCID: "vpc-a"}
	repo := &stubRepo{vpc: vpc}
	router := gin.New()
	router.GET("/vpcs/:id/subnets", NewHandler(NewService(repo)).ListSubnets)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/vpcs/8/subnets?q=private&include_deleted=true&page=2&size=5", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	want := SubnetListFilter{CloudAccountID: 3, Region: "us-east-1", VPCID: "vpc-a", Q: "private", IncludeDeleted: true, Page: 2, Size: 5}
	if repo.subFilter != want {
		t.Fatalf("filter=%#v want=%#v", repo.subFilter, want)
	}
	assertEmptyItemsEnvelope(t, recorder)
}

func TestHandlerRejectsInvalidQueriesAndIDsSafely(t *testing.T) {
	router := gin.New()
	handler := NewHandler(NewService(&stubRepo{}))
	router.GET("/vpcs", handler.List)
	router.GET("/vpcs/:id/subnets", handler.ListSubnets)
	paths := []string{
		"/vpcs?page=0", "/vpcs?size=0", "/vpcs?include_deleted=nope",
		"/vpcs/0/subnets", "/vpcs/-1/subnets", "/vpcs/9223372036854775808/subnets",
		"/vpcs/18446744073709551616/subnets",
		"/vpcs/1/subnets?page=0", "/vpcs/1/subnets?size=0",
	}
	for _, path := range paths {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
		var envelope response.Envelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Code != int(apperr.CodeValidation) || envelope.MessageKey != "common.validation" {
			t.Fatalf("%s envelope=%#v", path, envelope)
		}
	}
}

func TestHandlerDatabaseErrorDoesNotLeak(t *testing.T) {
	router := gin.New()
	router.GET("/vpcs", NewHandler(NewService(&stubRepo{listErr: errSecretDB})).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/vpcs", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), errSecretDB.Error()) {
		t.Fatalf("database details leaked: %s", recorder.Body.String())
	}
}

func TestHandlerUnknownVPCUsesNotFoundEnvelope(t *testing.T) {
	router := gin.New()
	router.GET("/vpcs/:id/subnets", NewHandler(NewService(&stubRepo{findErr: gorm.ErrRecordNotFound})).ListSubnets)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/vpcs/42/subnets", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope response.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != int(apperr.CodeAssetsVPCNotFound) || envelope.MessageKey != "assets.vpc.not_found" {
		t.Fatalf("envelope=%#v", envelope)
	}
}

func assertEmptyItemsEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	if items, ok := data["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("items=%#v", data["items"])
	}
}
