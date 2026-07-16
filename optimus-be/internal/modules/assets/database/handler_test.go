package database

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

func TestHandlerListReturnsEnvelope(t *testing.T) {
	port := int32(5432)
	repo := &stubRepository{items: []Summary{{ID: 1, DBInstanceID: "orders", Endpoint: "orders.internal", Port: &port, Tags: map[string]string{"env": "prod"}}}, total: 1}
	router := gin.New()
	router.GET("/databases", NewHandler(NewService(repo)).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/databases?page=1&size=20", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Code int          `json:"code"`
		Data ListResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Endpoint != "orders.internal" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestHandlerListRejectsBindingAndExplicitZeroPagination(t *testing.T) {
	router := gin.New()
	router.GET("/databases", NewHandler(NewService(&stubRepository{})).List)
	for _, path := range []string{
		"/databases?account_id=nope", "/databases?account_id=9223372036854775808",
		"/databases?page=nope", "/databases?page=0", "/databases?size=0", "/databases?size=201",
	} {
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

func TestHandlerListDoesNotLeakRepositoryError(t *testing.T) {
	secret := errors.New("postgres password=do-not-leak")
	router := gin.New()
	router.GET("/databases", NewHandler(NewService(&stubRepository{err: secret})).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/databases", nil))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), secret.Error()) {
		t.Fatalf("unsafe response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerMountUsesResourceReadPermission(t *testing.T) {
	router := gin.New()
	var permission string
	NewHandler(NewService(&stubRepository{})).Mount(router.Group("/api/v1/assets"), func(code string) gin.HandlerFunc {
		permission = code
		return func(c *gin.Context) { c.Next() }
	})
	if permission != "assets:resource:read" {
		t.Fatalf("permission = %q", permission)
	}
	routes := router.Routes()
	if len(routes) != 1 || routes[0].Path != "/api/v1/assets/databases" || routes[0].Method != http.MethodGet {
		t.Fatalf("routes = %#v", routes)
	}
}
