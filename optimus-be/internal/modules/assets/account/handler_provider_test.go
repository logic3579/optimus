package account

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/modules/assets/errs"
)

func TestHandler_Create_UnsupportedProviderUsesDomainError(t *testing.T) {
	handler := NewHandler(NewService(nil, nil, nil))
	router := gin.New()
	router.POST("/accounts", handler.Create)
	body, err := json.Marshal(CreateRequest{
		Name: "prod", Provider: "gcp", CloudKeyID: 1, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	var envelope struct {
		Code       int    `json:"code"`
		MessageKey string `json:"message_key"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int(errs.CodeAssetsProviderUnsupported), envelope.Code)
	require.Equal(t, errs.KeyProviderUnsupported, envelope.MessageKey)
}
