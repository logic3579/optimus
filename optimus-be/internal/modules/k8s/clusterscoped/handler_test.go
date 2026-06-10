package clusterscoped_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/clusterscoped"
)

func init() { gin.SetMode(gin.TestMode) }

// newRouter wires the four routes the production module mounts so handler
// tests can drive them via httptest without depending on the composition root.
func newRouter(svc *clusterscoped.Service) *gin.Engine {
	h := clusterscoped.NewHandler(svc)
	r := gin.New()
	r.GET("/c/:id/namespaces", h.ListNamespaces())
	r.GET("/c/:id/nodes", h.ListNodes())
	r.GET("/c/:id/nodes/:name", h.GetNode())
	r.GET("/c/:id/events", h.ListEvents())
	return r
}

func TestHandler_ListNamespaces_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	r := newRouter(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/namespaces", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_BadClusterID_400(t *testing.T) {
	r := newRouter(clusterscoped.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/namespaces", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestHandler_ListNodes_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}})
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	r := newRouter(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/nodes", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_GetNode_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}})
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	r := newRouter(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/nodes/n1", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_GetNode_NotFound(t *testing.T) {
	r := newRouter(clusterscoped.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/nodes/missing", nil))
	require.NotEqual(t, http.StatusOK, w.Code)
}

func TestHandler_ListEvents_OK(t *testing.T) {
	cs := fake.NewSimpleClientset()
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	r := newRouter(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/events?namespace=default", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
