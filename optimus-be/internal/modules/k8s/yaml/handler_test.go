package yaml_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	yamlmod "optimus-be/internal/modules/k8s/yaml"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestKindPerm_Complete(t *testing.T) {
	expected := map[string]string{
		"deployment":  "k8s:workload:read",
		"statefulset": "k8s:workload:read",
		"daemonset":   "k8s:workload:read",
		"pod":         "k8s:workload:read",
		"job":         "k8s:workload:read",
		"cronjob":     "k8s:workload:read",
		"replicaset":  "k8s:workload:read",
		"service":     "k8s:network:read",
		"ingress":     "k8s:network:read",
		"configmap":   "k8s:config:read",
		"secret":      "k8s:secret:read",
		"namespace":   "k8s:cluster_resource:read",
		"node":        "k8s:cluster_resource:read",
		"event":       "k8s:cluster_resource:read",
	}
	for k, want := range expected {
		got, ok := yamlmod.KindPerm[k]
		require.Truef(t, ok, "missing kind %s", k)
		require.Equalf(t, want, got, "kind %s wired to wrong perm", k)
	}
	require.Equal(t, len(expected), len(yamlmod.KindPerm), "unexpected extra kinds")
}

// Documented boundary: spec §5.6/§11.5. Secret YAML is gated on the same
// :read perm as List/Get, not the stronger :reveal. A regression here
// would either over-restrict legitimate ops (everyone needs :reveal to
// view YAML) or under-restrict (no perm check). Pin it explicitly.
func TestKindPerm_SecretUsesReadNotReveal(t *testing.T) {
	require.Equal(t, "k8s:secret:read", yamlmod.KindPerm["secret"])
	require.NotEqual(t, "k8s:secret:reveal", yamlmod.KindPerm["secret"])
}

func TestGet_BadClusterID_400(t *testing.T) {
	h := yamlmod.NewHandler(nil, nil)
	r := gin.New()
	r.GET("/c/:id/yaml", h.Get())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/yaml?kind=pod&name=p", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestGet_MissingKind_400(t *testing.T) {
	h := yamlmod.NewHandler(nil, nil)
	r := gin.New()
	r.GET("/c/:id/yaml", h.Get())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/yaml", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestGet_MissingName_400(t *testing.T) {
	h := yamlmod.NewHandler(nil, nil)
	r := gin.New()
	r.GET("/c/:id/yaml", h.Get())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/yaml?kind=pod", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "common.bad_request")
}

func TestGet_UnsupportedKind_400(t *testing.T) {
	h := yamlmod.NewHandler(nil, nil)
	r := gin.New()
	r.GET("/c/:id/yaml", h.Get())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/yaml?kind=widget&name=foo", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "k8s.yaml.unsupported_kind")
}
