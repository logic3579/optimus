//go:build dbtest

package account

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

func TestHandler_Create_Returns200(t *testing.T) {
	svc, _, _, cloudKey := setupSvc(t)
	handler := NewHandler(svc)
	router := gin.New()
	router.POST("/api/v1/assets/cloud-accounts", testActor(7), handler.Create)

	body, err := json.Marshal(CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assets/cloud-accounts", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code int    `json:"code"`
		Data Detail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Zero(t, response.Code)
	require.Equal(t, "prod", response.Data.Name)
}

func TestHandler_TriggerSync_501WhenUnwired(t *testing.T) {
	handler := NewHandler(nil)
	router := gin.New()
	router.POST("/api/v1/assets/cloud-accounts/:id/sync", testActor(7), handler.TriggerSync)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assets/cloud-accounts/1/sync", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotImplemented, recorder.Code)
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotZero(t, response.Code)
	require.Equal(t, "not implemented", response.Message)
}

func TestHandler_MountRegistersPermissionRoutes(t *testing.T) {
	handler := NewHandler(nil)
	router := gin.New()
	group := router.Group("/api/v1/assets")
	var permissions []string
	handler.Mount(group, func(code string) gin.HandlerFunc {
		permissions = append(permissions, code)
		return func(c *gin.Context) { c.Next() }
	})
	require.Equal(t, []string{"assets:account:read", "assets:account:write", "assets:account:delete"}, permissions)
	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/assets/cloud-accounts",
		"GET /api/v1/assets/cloud-accounts/:id",
		"POST /api/v1/assets/cloud-accounts",
		"PUT /api/v1/assets/cloud-accounts/:id",
		"POST /api/v1/assets/cloud-accounts/:id/sync",
		"DELETE /api/v1/assets/cloud-accounts/:id",
	} {
		require.True(t, routes[route], route)
	}
}

func TestHandler_ListGetUpdateDelete(t *testing.T) {
	svc, _, _, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "before", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	handler := NewHandler(svc)
	id := strconv.FormatUint(detail.ID, 10)
	router := gin.New()
	router.GET("/accounts", handler.List)
	router.GET("/accounts/:id", handler.Get)
	router.PUT("/accounts/:id", testActor(7), handler.Update)
	router.DELETE("/accounts/:id", testActor(7), handler.Delete)

	assertEnvelopeOK(t, serve(router, http.MethodGet, "/accounts?page=1&size=20", nil))
	assertEnvelopeOK(t, serve(router, http.MethodGet, "/accounts/"+id, nil))
	name := "after"
	assertEnvelopeOK(t, serve(router, http.MethodPut, "/accounts/"+id, UpdateRequest{Name: &name}))
	assertEnvelopeOK(t, serve(router, http.MethodDelete, "/accounts/"+id, nil))
}

func TestHandler_InvalidIDsReturnEnvelope(t *testing.T) {
	handler := NewHandler(nil)
	router := gin.New()
	router.GET("/accounts/:id", handler.Get)
	router.PUT("/accounts/:id", handler.Update)
	router.DELETE("/accounts/:id", handler.Delete)
	router.POST("/accounts/:id/sync", handler.TriggerSync)
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/accounts/not-a-number"},
		{http.MethodPut, "/accounts/0"},
		{http.MethodDelete, "/accounts/-1"},
		{http.MethodPost, "/accounts/nope/sync"},
	} {
		recorder := serve(router, request.method, request.path, nil)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		var envelope response.Envelope
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
		require.NotZero(t, envelope.Code)
		require.Equal(t, "common.bad_request", envelope.MessageKey)
	}
}

func TestHandler_ValidationErrorsUseEnvelope(t *testing.T) {
	handler := NewHandler(nil)
	router := gin.New()
	router.GET("/accounts", handler.List)
	router.POST("/accounts", handler.Create)
	recorder := serve(router, http.MethodGet, "/accounts?page=0", nil)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assertErrorEnvelope(t, recorder)
	recorder = serveRaw(router, http.MethodPost, "/accounts", []byte(`{"name":`))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assertErrorEnvelope(t, recorder)
}

func TestHandler_TriggerSyncSetterInvoked(t *testing.T) {
	handler := NewHandler(nil)
	var gotID uint64
	handler.SetTriggerSync(func(c *gin.Context, id uint64) {
		gotID = id
		response.Success(c, gin.H{"queued": true})
	})
	router := gin.New()
	router.POST("/accounts/:id/sync", handler.TriggerSync)
	recorder := serve(router, http.MethodPost, "/accounts/42/sync", nil)
	assertEnvelopeOK(t, recorder)
	require.Equal(t, uint64(42), gotID)
}

func serve(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	if body == nil {
		return serveRaw(router, method, path, nil)
	}
	payload, _ := json.Marshal(body)
	return serveRaw(router, method, path, payload)
}

func serveRaw(router *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertEnvelopeOK(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var envelope response.Envelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)
}

func assertErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var envelope response.Envelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.NotZero(t, envelope.Code)
	require.Equal(t, "common.validation", envelope.MessageKey)
}

func testActor(userID uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.CtxKeyUserID, userID)
		c.Next()
	}
}
