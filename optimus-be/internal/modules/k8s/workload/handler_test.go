package workload_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/workload"
)

func init() { gin.SetMode(gin.TestMode) }

func newWlRouter(svc *workload.Service) *gin.Engine {
	h := workload.NewHandler(svc)
	r := gin.New()
	r.GET("/c/:id/w/:kind", h.List())
	r.GET("/c/:id/w/:kind/:ns/:name", h.Get())
	return r
}

func TestHandler_Workload_List_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "n"},
	})
	r := newWlRouter(workload.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/w/deployments?namespace=n", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Workload_Get_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "n"},
	})
	r := newWlRouter(workload.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/w/deployments/n/d", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Workload_BadClusterID_400(t *testing.T) {
	r := newWlRouter(workload.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	for _, path := range []string{"/c/0/w/deployments", "/c/0/w/deployments/n/d"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, w.Code, "path %s", path)
	}
}
