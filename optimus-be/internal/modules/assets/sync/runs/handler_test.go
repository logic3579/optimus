package runs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

func TestHandlerListSuccessEnvelopeAndJSONContract(t *testing.T) {
	repo := &stubListRepo{items: []Summary{{
		ID: 1, CloudAccountID: 2, CloudAccountName: "prod", Region: "us-east-1",
		ResourceType: "instance", StartedAt: time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC),
		Status: "success", ItemsSeen: 3, ItemsSoftDeleted: 2, Trigger: "cron",
	}}, total: 1}
	router := gin.New()
	router.GET("/sync-runs", NewHandler(NewService(repo)).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sync-runs?page=1&size=20", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	item := data["items"].([]any)[0].(map[string]any)
	if item["items_softdeleted"] != float64(2) {
		t.Fatalf("items_softdeleted contract missing: %v", item)
	}
	if _, wrong := item["items_soft_deleted"]; wrong {
		t.Fatalf("unexpected items_soft_deleted key: %v", item)
	}
}

func TestHandlerListInvalidQueryUsesValidationEnvelope(t *testing.T) {
	router := gin.New()
	router.GET("/sync-runs", NewHandler(NewService(&stubListRepo{})).List)
	for _, path := range []string{
		"/sync-runs?page=not-a-number",
		"/sync-runs?page=0",
		"/sync-runs?size=0",
		"/sync-runs?account_id=9223372036854775808",
		"/sync-runs?resource_type=bucket",
		"/sync-runs?started_after=not-a-date",
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

func TestHandlerMountRegistersSyncReadPermission(t *testing.T) {
	handler := NewHandler(NewService(&stubListRepo{}))
	router := gin.New()
	var permission string
	handler.Mount(router.Group("/api/v1/assets"), func(code string) gin.HandlerFunc {
		permission = code
		return func(c *gin.Context) { c.Next() }
	})
	if permission != "assets:sync:read" {
		t.Fatalf("permission = %q", permission)
	}
	routes := router.Routes()
	if len(routes) != 1 || routes[0].Method != http.MethodGet || routes[0].Path != "/api/v1/assets/sync-runs" {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestHandlerListDatabaseErrorDoesNotLeak(t *testing.T) {
	router := gin.New()
	router.GET("/sync-runs", NewHandler(NewService(&stubListRepo{err: errSecretDB})).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sync-runs", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() == "" || contains(recorder.Body.String(), errSecretDB.Error()) {
		t.Fatalf("unsafe response: %s", recorder.Body.String())
	}
}

var errSecretDB = &secretError{"postgres password=do-not-leak"}

type secretError struct{ text string }

func (e *secretError) Error() string { return e.text }

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
