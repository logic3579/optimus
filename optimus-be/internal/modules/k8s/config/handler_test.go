package config_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/config"
)

func init() { gin.SetMode(gin.TestMode) }

func newCfgRouter(svc *config.Service) *gin.Engine {
	h := config.NewHandler(svc)
	r := gin.New()
	r.GET("/c/:id/cm", h.List())
	r.GET("/c/:id/cm/:ns/:name", h.Get())
	return r
}

func TestHandler_Config_List_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "n"},
	})
	r := newCfgRouter(config.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/cm?namespace=n", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Config_Get_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "n"},
	})
	r := newCfgRouter(config.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/cm/n/demo", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Config_BadClusterID_400(t *testing.T) {
	r := newCfgRouter(config.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/cm", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestHandler_Config_Get_BadClusterID_400(t *testing.T) {
	r := newCfgRouter(config.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/cm/n/demo", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHandler_Config_Get_NotFound exercises the error envelope path through
// the handler — Service returns a NotFound BizError which is rendered by
// response.Error.
func TestHandler_Config_Get_NotFound(t *testing.T) {
	r := newCfgRouter(config.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/cm/n/missing", nil))
	require.NotEqual(t, http.StatusOK, w.Code)
}
