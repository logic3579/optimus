package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/middleware"
)

const mwTestSecret = "test_secret_must_be_at_least_32_bytes_!!"

func TestJWTAuth_AllowsValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	tok, err := signer.Sign(crypto.JWTClaims{UserID: 7, JTI: "j"}, time.Minute)
	require.NoError(t, err)

	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	var captured uint64
	r.GET("/", func(c *gin.Context) { captured = c.GetUint64(middleware.CtxKeyUserID); c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, uint64(7), captured)
}

func TestJWTAuth_RejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	r.GET("/", func(_ *gin.Context) {})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_RejectsExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	tok, _ := signer.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, -1*time.Second)

	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	r.GET("/", func(_ *gin.Context) {})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_RejectsMalformedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := crypto.NewJWTSigner(mwTestSecret)
	r := gin.New()
	r.Use(middleware.JWTAuth(signer))
	r.GET("/", func(_ *gin.Context) {})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "NotBearer xyz")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
