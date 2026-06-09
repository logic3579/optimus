//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func seedUserWithRole(t *testing.T, gdb *gorm.DB, username, roleCode string) uint64 {
	t.Helper()
	u := &models.User{Username: username, Email: username + "@x.io", PasswordHash: "x", Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	var role models.Role
	require.NoError(t, gdb.Where("code = ?", roleCode).First(&role).Error)
	require.NoError(t, gdb.Create(&models.UserRole{UserID: u.ID, RoleID: role.ID}).Error)
	return u.ID
}

func setupSeed(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	admin := models.Role{Code: "admin", Name: "role.admin", IsBuiltin: true}
	viewer := models.Role{Code: "viewer", Name: "role.viewer", IsBuiltin: true}
	gdb.Create(&admin)
	gdb.Create(&viewer)
	var allPerms []models.Permission
	gdb.Find(&allPerms)
	for _, p := range allPerms {
		gdb.Create(&models.RolePermission{RoleID: admin.ID, PermissionID: p.ID})
		if strings.HasSuffix(p.Code, ":read") {
			gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: p.ID})
		}
	}
}

func TestCache_LoadsPermissionsFromDB(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uid := seedUserWithRole(t, gdb, "alice", "admin")

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	codes, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, len(permissions.All), len(codes))
}

func TestCache_RespectsTTL(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uid := seedUserWithRole(t, gdb, "alice", "viewer")

	cache := rbac.NewPermissionCache(gdb, 50*time.Millisecond)
	first, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)

	var role models.Role
	gdb.Where("code = ?", "viewer").First(&role)
	var newPerm models.Permission
	gdb.Where("code = ?", "system:user:write").First(&newPerm)
	gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: newPerm.ID})

	cached, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, first, cached)

	time.Sleep(80 * time.Millisecond)
	fresh, err := cache.Get(context.Background(), uid)
	require.NoError(t, err)
	require.Contains(t, fresh, "system:user:write")
}

func TestCache_InvalidateAll(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uidA := seedUserWithRole(t, gdb, "alice", "viewer")
	uidB := seedUserWithRole(t, gdb, "bob", "viewer")

	cache := rbac.NewPermissionCache(gdb, time.Hour)
	_, err := cache.Get(context.Background(), uidA)
	require.NoError(t, err)
	_, err = cache.Get(context.Background(), uidB)
	require.NoError(t, err)

	// Grant viewer a new permission after both users are cached, then nuke the whole cache.
	var role models.Role
	gdb.Where("code = ?", "viewer").First(&role)
	var newPerm models.Permission
	gdb.Where("code = ?", "system:user:write").First(&newPerm)
	gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: newPerm.ID})

	cache.InvalidateAll()

	freshA, _ := cache.Get(context.Background(), uidA)
	freshB, _ := cache.Get(context.Background(), uidB)
	require.Contains(t, freshA, "system:user:write")
	require.Contains(t, freshB, "system:user:write")
}

func TestCache_InvalidateForUser(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	setupSeed(t, gdb)
	uid := seedUserWithRole(t, gdb, "alice", "viewer")

	cache := rbac.NewPermissionCache(gdb, time.Hour)
	first, _ := cache.Get(context.Background(), uid)

	var role models.Role
	gdb.Where("code = ?", "viewer").First(&role)
	var newPerm models.Permission
	gdb.Where("code = ?", "system:user:write").First(&newPerm)
	gdb.Create(&models.RolePermission{RoleID: role.ID, PermissionID: newPerm.ID})

	cache.InvalidateUser(uid)
	fresh, _ := cache.Get(context.Background(), uid)
	require.NotEqual(t, len(first), len(fresh))
	require.Contains(t, fresh, "system:user:write")
}
