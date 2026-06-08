package response_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

func TestSuccess_WritesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	response.Success(c, gin.H{"hello": "world"})

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Code    int               `json:"code"`
		Data    map[string]string `json:"data"`
		Message string            `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "world", body.Data["hello"])
}

func TestError_WritesEnvelopeWithHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	response.Error(c, apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "bad"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var body struct {
		Code       int    `json:"code"`
		Message    string `json:"message"`
		MessageKey string `json:"message_key"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, int(apperr.CodeInvalidCredentials), body.Code)
	require.Equal(t, "auth.invalid_credentials", body.MessageKey)
}

func TestError_FallsBackToInternalForNilErr(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	response.Error(c, nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestError_DoesNotLeakRawErrorMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	leaky := errors.New("postgres: connection refused on /var/run/postgresql/.s.PGSQL.5432")
	response.Error(c, leaky)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	body := rec.Body.String()
	require.NotContains(t, body, "postgres")
	require.NotContains(t, body, "PGSQL")
	require.Contains(t, body, "internal server error")
}
