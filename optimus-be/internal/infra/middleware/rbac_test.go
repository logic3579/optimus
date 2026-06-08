//go:build dbtest

package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func setupRBACSeed(t *testing.T, gdb *gorm.DB) (adminUID, viewerUID uint64) {
	t.Helper()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	admin := models.Role{Code: "admin", Name: "role.admin", IsBuiltin: true}
	viewer := models.Role{Code: "viewer", Name: "role.viewer", IsBuiltin: true}
	gdb.Create(&admin)
	gdb.Create(&viewer)
	var perms []models.Permission
	gdb.Find(&perms)
	for _, p := range perms {
		gdb.Create(&models.RolePermission{RoleID: admin.ID, PermissionID: p.ID})
		if strings.HasSuffix(p.Code, ":read") {
			gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: p.ID})
		}
	}
	a := &models.User{Username: "adminx", Email: "a@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(a)
	gdb.Create(&models.UserRole{UserID: a.ID, RoleID: admin.ID})

	v := &models.User{Username: "viewerx", Email: "v@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(v)
	gdb.Create(&models.UserRole{UserID: v.ID, RoleID: viewer.ID})
	return a.ID, v.ID
}

func TestRBAC_AllowsUserWithPermission(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	adminUID, _ := setupRBACSeed(t, gdb)

	cache := rbac.NewPermissionCache(gdb, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, adminUID); c.Next() })
	r.GET("/u", middleware.RequirePermission(cache, "system:user:write"), func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/u", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRBAC_RejectsUserWithoutPermission(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, viewerUID := setupRBACSeed(t, gdb)

	cache := rbac.NewPermissionCache(gdb, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, viewerUID); c.Next() })
	r.GET("/u", middleware.RequirePermission(cache, "system:user:write"), func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/u", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRBAC_RejectsAnonymous(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	cache := rbac.NewPermissionCache(gdb, 0)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/u", middleware.RequirePermission(cache, "system:user:write"), func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/u", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
