//go:build dbtest

package yaml_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/k8s/yaml"
	"optimus-be/internal/modules/rbac"
)

// fakeCS satisfies yaml.Clientsetter for the in-memory client-go fake.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) Clientset(context.Context, uint64, string) (kubernetes.Interface, error) {
	return f.cs, nil
}

// seedUserWith creates a user, assigns them a role with the given perm codes,
// and returns the user id. Uses the real permissions.Register + DB so the
// rbac.PermissionCache.Get path exercises the canonical join.
func seedUserWith(t *testing.T, perms ...string) (uid uint64, cache *rbac.PermissionCache) {
	t.Helper()
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)

	role := models.Role{Code: "tester", Name: "role.tester", IsBuiltin: false}
	require.NoError(t, gdb.Create(&role).Error)
	for _, code := range perms {
		var p models.Permission
		require.NoError(t, gdb.Where("code = ?", code).First(&p).Error)
		require.NoError(t, gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: p.ID}).Error)
	}

	u := &models.User{Username: "alice", Email: "a@x.io", PasswordHash: "x", Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: u.ID, RoleID: role.ID}).Error)

	return u.ID, rbac.NewPermissionCache(gdb, time.Minute)
}

func TestGet_HappyPath_PodYAML(t *testing.T) {
	uid, cache := seedUserWith(t, "k8s:workload:read")

	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "n"},
	})
	h := yaml.NewHandler(&fakeCS{cs: cs}, cache)

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uid); c.Next() })
	r.GET("/c/:id/yaml", h.Get())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/yaml?kind=pod&namespace=n&name=demo", nil))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Equal(t, "text/yaml", w.Header().Get("Content-Type"))
	body := w.Body.String()
	require.Contains(t, body, "name: demo")
	require.Contains(t, body, "namespace: n")
}

// Secret YAML is the spec's documented :read boundary — make sure a user
// holding only :read (and explicitly NOT :reveal) can pull the YAML.
func TestGet_SecretYAML_ReadIsEnough(t *testing.T) {
	uid, cache := seedUserWith(t, "k8s:secret:read")

	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "n"},
		Data:       map[string][]byte{"k": []byte("v")},
	})
	h := yaml.NewHandler(&fakeCS{cs: cs}, cache)

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uid); c.Next() })
	r.GET("/c/:id/yaml", h.Get())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/yaml?kind=secret&namespace=n&name=creds", nil))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), "name: creds")
}

func TestGet_PermissionDenied_403(t *testing.T) {
	// User only has k8s:cluster:read, but asks for a workload kind.
	uid, cache := seedUserWith(t, "k8s:cluster:read")

	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "n"},
	})
	h := yaml.NewHandler(&fakeCS{cs: cs}, cache)

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, uid); c.Next() })
	r.GET("/c/:id/yaml", h.Get())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/1/yaml?kind=pod&namespace=n&name=demo", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "auth.permission_denied")
}
