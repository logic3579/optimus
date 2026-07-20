package instance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

func TestHandlerListSuccessEnvelope(t *testing.T) {
	repo := &stubListRepo{items: []Summary{{
		ID: 1, CloudAccountID: 2, CloudAccountName: "prod", Region: "us-east-1",
		InstanceID: "i-a", PrivateIP: "10.0.0.5", Tags: map[string]string{"role": "web"},
	}}, total: 1}
	router := gin.New()
	router.GET("/instances", NewHandler(NewService(repo)).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/instances?page=1&size=20", nil))
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
	if envelope.Code != 0 || envelope.Data.Total != 1 || len(envelope.Data.Items) != 1 {
		t.Fatalf("envelope=%#v", envelope)
	}
	item := envelope.Data.Items[0]
	if item.PrivateIP != "10.0.0.5" || item.Tags["role"] != "web" {
		t.Fatalf("item=%#v", item)
	}
}

func TestHandlerListRejectsInvalidBindingAndExplicitZeroPagination(t *testing.T) {
	router := gin.New()
	router.GET("/instances", NewHandler(NewService(&stubListRepo{})).List)
	paths := []string{
		"/instances?page=not-a-number",
		"/instances?page=0",
		"/instances?size=0",
		"/instances?size=201",
		"/instances?include_deleted=not-a-bool",
		"/instances?account_id=9223372036854775808",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != int(apperr.CodeValidation) || envelope.MessageKey != "common.validation" {
				t.Fatalf("envelope=%#v", envelope)
			}
		})
	}
}

func TestHandlerMountRegistersResourceReadPermission(t *testing.T) {
	handler := NewHandler(NewService(&stubListRepo{}))
	router := gin.New()
	var permission string
	handler.Mount(router.Group("/api/v1/assets"), func(code string) gin.HandlerFunc {
		permission = code
		return func(c *gin.Context) { c.Next() }
	})
	if permission != "assets:resource:read" {
		t.Fatalf("permission=%q", permission)
	}
	routes := router.Routes()
	if len(routes) != 1 || routes[0].Method != http.MethodGet || routes[0].Path != "/api/v1/assets/instances" {
		t.Fatalf("routes=%#v", routes)
	}
}

func TestHandlerListDatabaseErrorDoesNotLeak(t *testing.T) {
	secret := "postgres password=do-not-leak"
	router := gin.New()
	router.GET("/instances", NewHandler(NewService(&stubListRepo{err: &secretError{secret}})).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/instances", nil))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("status=%d unsafe body=%s", recorder.Code, recorder.Body.String())
	}
}

type secretError struct{ text string }

func (e *secretError) Error() string { return e.text }
