package secret_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/secret"
)

func init() { gin.SetMode(gin.TestMode) }

func newSecRouter(svc *secret.Service) *gin.Engine {
	h := secret.NewHandler(svc)
	r := gin.New()
	r.GET("/c/:id/s", h.List())
	r.GET("/c/:id/s/:ns/:name", h.Get())
	r.GET("/c/:id/s/:ns/:name/data", h.Data())
	return r
}

func TestHandler_Secret_List_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Data:       map[string][]byte{"k": []byte("v")},
	})
	r := newSecRouter(secret.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/s?namespace=n", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "v", "value must not leak via List handler")
}

func TestHandler_Secret_Get_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Data:       map[string][]byte{"k": []byte("v")},
	})
	r := newSecRouter(secret.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/s/n/s", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "\"v\"", "value must not leak via Get handler")
}

func TestHandler_Secret_Data_OK(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Data:       map[string][]byte{"k": []byte("v")},
	})
	r := newSecRouter(secret.NewService(&fakeCS{cs: cs}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/s/n/s/data", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestHandler_Secret_BadClusterID_400(t *testing.T) {
	r := newSecRouter(secret.NewService(&fakeCS{cs: fake.NewSimpleClientset()}))
	for _, path := range []string{"/c/0/s", "/c/0/s/n/x", "/c/0/s/n/x/data"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, w.Code, "path %s", path)
	}
}
