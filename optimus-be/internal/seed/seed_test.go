//go:build dbtest

package seed_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/seed"
)

func TestRun_IsIdempotent(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)

	r1, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)
	require.NotEmpty(t, r1.AdminInitialPassword)

	r2, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)
	require.Empty(t, r2.AdminInitialPassword, "second seed must not print a password")

	var users int64
	gdb.Model(&models.User{}).Where("username = ?", "admin").Count(&users)
	require.Equal(t, int64(1), users)

	var roles int64
	gdb.Model(&models.Role{}).Where("is_builtin").Count(&roles)
	require.Equal(t, int64(2), roles, "expected admin + viewer builtin roles")
}

func TestRun_AdminRoleHasAllPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var adminRole models.Role
	require.NoError(t, gdb.Where("code = ?", "admin").First(&adminRole).Error)
	var bound int64
	gdb.Model(&models.RolePermission{}).Where("role_id = ?", adminRole.ID).Count(&bound)
	require.Equal(t, int64(len(permissions.All)), bound)
}

func TestRun_FailsLoudlyWhenNoPermissionsRegistered(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	// Note: we deliberately do NOT call permissions.Register
	_, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "permissions")
}

func TestRun_ViewerRoleHasOnlyReadPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	var perms []models.Permission
	gdb.Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", viewer.ID).
		Find(&perms)
	require.NotEmpty(t, perms)
	for _, p := range perms {
		require.Contains(t, p.Code, ":read")
	}
}

func TestRun_SeedsInitialMenuTree(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	wantCodes := []string{
		"dashboard",
		"system", "system.users", "system.roles", "system.permissions", "system.menus", "system.audit_logs",
		"credentials", "credentials.ssh_keys", "credentials.kubeconfigs", "credentials.cloud_keys",
		"k8s", "k8s.clusters", "k8s.workloads", "k8s.network", "k8s.config", "k8s.cluster_resources",
		"apps", "apps.applications", "apps.chart_repos",
	}
	for _, code := range wantCodes {
		var m models.Menu
		err := gdb.Where("code = ?", code).First(&m).Error
		require.NoError(t, err, "missing menu code %q", code)
	}

	// Parent linkage: credentials.* children must have parent_id = credentials.id.
	var parent models.Menu
	require.NoError(t, gdb.Where("code = ?", "credentials").First(&parent).Error)
	var childrenCount int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", parent.ID).Count(&childrenCount)
	require.Equal(t, int64(3), childrenCount)

	// Parent linkage: k8s.* children must have parent_id = k8s.id.
	var k8sParent models.Menu
	require.NoError(t, gdb.Where("code = ?", "k8s").First(&k8sParent).Error)
	var k8sChildren int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", k8sParent.ID).Count(&k8sChildren)
	require.Equal(t, int64(5), k8sChildren)

	// Parent linkage: apps.* children must have parent_id = apps.id (P3).
	var appsParent models.Menu
	require.NoError(t, gdb.Where("code = ?", "apps").First(&appsParent).Error)
	var appsChildren int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", appsParent.ID).Count(&appsChildren)
	require.Equal(t, int64(2), appsChildren)
}

// TestRun_AdminRoleIncludesAppsPermissions covers the implicit P3 grant:
// bindAdminPermissions binds every permission row, so the 10 apps:* codes
// flow to admin automatically without explicit per-role wiring in seed.
func TestRun_AdminRoleIncludesAppsPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var adminRole models.Role
	require.NoError(t, gdb.Where("code = ?", "admin").First(&adminRole).Error)

	wantCodes := []string{
		"apps:repo:read", "apps:repo:write", "apps:repo:delete",
		"apps:application:read", "apps:application:write", "apps:application:delete",
		"apps:release:install", "apps:release:upgrade",
		"apps:release:rollback", "apps:release:uninstall",
	}
	for _, code := range wantCodes {
		var n int64
		gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Where("role_permissions.role_id = ? AND permissions.code = ?", adminRole.ID, code).
			Count(&n)
		require.Equal(t, int64(1), n, "admin role missing %q", code)
	}
}

// TestRun_ViewerRoleIncludesAppsReadPermissions covers the implicit P3 viewer
// grant: bindViewerPermissions binds every "%:read" code, so the apps read
// permissions flow to viewer automatically.
func TestRun_ViewerRoleIncludesAppsReadPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)

	wantCodes := []string{"apps:repo:read", "apps:application:read"}
	for _, code := range wantCodes {
		var n int64
		gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Where("role_permissions.role_id = ? AND permissions.code = ?", viewer.ID, code).
			Count(&n)
		require.Equal(t, int64(1), n, "viewer role missing %q", code)
	}
}
