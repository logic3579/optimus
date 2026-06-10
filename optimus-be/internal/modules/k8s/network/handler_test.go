package network_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/network"
)

func init() { gin.SetMode(gin.TestMode) }

func newNetRouter(svc *network.Service) *gin.Engine {
	h := network.NewHandler(svc)
	r := gin.New()
	r.GET("/c/:id/n/:kind", h.List())
	r.GET("/c/:id/n/:kind/:ns/:name", h.Get())
	return r
}

func TestHandler_Network_List_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
	})
	r := newNetRouter(network.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/n/services?namespace=n", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Network_Get_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
	})
	r := newNetRouter(network.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/n/services/n/s", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Network_BadClusterID_400(t *testing.T) {
	r := newNetRouter(network.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/n/services", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestHandler_Network_Get_BadClusterID_400(t *testing.T) {
	r := newNetRouter(network.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/n/services/n/s", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}
