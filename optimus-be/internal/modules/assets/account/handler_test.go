//go:build dbtest

package account

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
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

func testActor(userID uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.CtxKeyUserID, userID)
		c.Next()
	}
}
