//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func TestMeService_ListPermissions_AdminGetsAll(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	admin := models.Role{Code: "admin", Name: "role.admin", IsBuiltin: true}
	gdb.Create(&admin)
	var perms []models.Permission
	gdb.Find(&perms)
	for _, p := range perms {
		gdb.Create(&models.RolePermission{RoleID: admin.ID, PermissionID: p.ID})
	}
	u := &models.User{Username: "alice2", Email: "a2@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(u)
	gdb.Create(&models.UserRole{UserID: u.ID, RoleID: admin.ID})

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute), nil)
	codes, err := svc.ListPermissions(context.Background(), u.ID)
	require.NoError(t, err)
	require.Equal(t, len(permissions.All), len(codes))
}

func TestMeService_ListPermissions_ViewerGetsOnlyReads(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, _ = permissions.Register(context.Background(), gdb, permissions.All)
	viewer := models.Role{Code: "viewer", Name: "role.viewer", IsBuiltin: true}
	gdb.Create(&viewer)
	var perms []models.Permission
	gdb.Where("code LIKE ?", "%:read").Find(&perms)
	for _, p := range perms {
		gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: p.ID})
	}
	u := &models.User{Username: "bob2", Email: "b2@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(u)
	gdb.Create(&models.UserRole{UserID: u.ID, RoleID: viewer.ID})

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute), nil)
	codes, err := svc.ListPermissions(context.Background(), u.ID)
	require.NoError(t, err)
	require.NotEmpty(t, codes)
	for _, c := range codes {
		require.True(t, strings.HasSuffix(c, ":read"))
	}
}
