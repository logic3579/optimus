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
	require.Equal(t, int64(3), roles, "expected admin + viewer + editor builtin roles")
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
	var operatorBeforeSeed int64
	require.NoError(t, gdb.Model(&models.Role{}).Where("code = ?", "k8s_operator").Count(&operatorBeforeSeed).Error)
	require.Zero(t, operatorBeforeSeed, "delivery must not require a k8s_operator role")
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
		require.NotRegexp(t, `^(system|credentials):`, p.Code)
	}
}

func TestRun_EditorRoleHasAllNonSystemPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"})
	require.NoError(t, err)

	var editor models.Role
	require.NoError(t, gdb.Where("code = ?", "editor").First(&editor).Error)
	var got []string
	require.NoError(t, gdb.Table("permissions").
		Select("permissions.code").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", editor.ID).
		Order("permissions.code").
		Scan(&got).Error)

	var want []string
	require.NoError(t, gdb.Model(&models.Permission{}).
		Where("code NOT LIKE ?", "system:%").Order("code").Pluck("code", &want).Error)
	require.Equal(t, want, got)
	for _, code := range got {
		require.NotRegexp(t, `^system:`, code)
	}
}

func TestRun_PromotesExistingEditorToBuiltinRole(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	require.NoError(t, gdb.Create(&models.Role{
		Code: "editor", Name: "Legacy editor", Description: "custom", IsBuiltin: false,
	}).Error)

	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"})
	require.NoError(t, err)
	var editor models.Role
	require.NoError(t, gdb.Where("code = ?", "editor").First(&editor).Error)
	require.True(t, editor.IsBuiltin)
	require.Equal(t, "role.editor", editor.Name)
	require.Equal(t, "Full access except system administration", editor.Description)
}

func TestRun_ResynchronizesBuiltinRolePermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	opts := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"}
	_, err = seed.Run(context.Background(), gdb, opts)
	require.NoError(t, err)

	var viewer models.Role
	require.NoError(t, gdb.Where("code = ?", "viewer").First(&viewer).Error)
	var systemRead models.Permission
	require.NoError(t, gdb.Where("code = ?", "system:user:read").First(&systemRead).Error)
	require.NoError(t, gdb.Create(&models.RolePermission{RoleID: viewer.ID, PermissionID: systemRead.ID}).Error)

	_, err = seed.Run(context.Background(), gdb, opts)
	require.NoError(t, err)
	var stale int64
	require.NoError(t, gdb.Model(&models.RolePermission{}).
		Where("role_id = ? AND permission_id = ?", viewer.ID, systemRead.ID).Count(&stale).Error)
	require.Zero(t, stale)
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
		"credentials", "credentials.ssh_keys", "credentials.kubeconfigs", "credentials.cloud_keys", "credentials.http_credentials",
		"k8s", "k8s.clusters", "k8s.workloads", "k8s.network", "k8s.config", "k8s.cluster_resources",
		"apps", "apps.applications", "apps.chart_repos",
		"observability", "observability.kubernetes", "observability.dashboards", "observability.datasources",
		"delivery", "delivery.projects", "delivery.approvals",
	}
	for _, code := range wantCodes {
		var m models.Menu
		err := gdb.Where("code = ?", code).First(&m).Error
		require.NoError(t, err, "missing menu code %q", code)
	}
	var topLevel []models.Menu
	require.NoError(t, gdb.Where("parent_id IS NULL").Order("sort_order, id").Find(&topLevel).Error)
	require.Equal(t,
		[]string{"dashboard", "assets", "k8s", "apps", "delivery", "observability", "credentials", "system"},
		[]string{topLevel[0].Code, topLevel[1].Code, topLevel[2].Code, topLevel[3].Code, topLevel[4].Code, topLevel[5].Code, topLevel[6].Code, topLevel[7].Code},
	)

	// Parent linkage: credentials.* children must have parent_id = credentials.id.
	var parent models.Menu
	require.NoError(t, gdb.Where("code = ?", "credentials").First(&parent).Error)
	var childrenCount int64
	gdb.Model(&models.Menu{}).Where("parent_id = ?", parent.ID).Count(&childrenCount)
	require.Equal(t, int64(4), childrenCount)

	for code, contract := range map[string][2]string{
		"credentials.http_credentials": {"credentials/http-credentials/List", "credentials:http:read"},
		"observability.kubernetes":     {"observability/kubernetes/Index", "observability:metric:read"},
		"observability.dashboards":     {"observability/dashboards/List", "observability:dashboard:read"},
		"observability.datasources":    {"observability/datasources/List", "observability:datasource:read"},
	} {
		var menu models.Menu
		require.NoError(t, gdb.Where("code = ?", code).First(&menu).Error)
		require.Equal(t, contract[0], menu.Component)
		require.NotNil(t, menu.PermissionCode)
		require.Equal(t, contract[1], *menu.PermissionCode)
	}
	var observabilityParent models.Menu
	require.NoError(t, gdb.Where("code = ?", "observability").First(&observabilityParent).Error)
	var observabilityChildren []models.Menu
	require.NoError(t, gdb.Where("parent_id = ?", observabilityParent.ID).Order("sort_order").Find(&observabilityChildren).Error)
	require.Equal(t, []string{"observability.kubernetes", "observability.dashboards", "observability.datasources"}, []string{observabilityChildren[0].Code, observabilityChildren[1].Code, observabilityChildren[2].Code})

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

func TestRun_DeliveryPermissionsAndMenus(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var builtinRoles int64
	require.NoError(t, gdb.Model(&models.Role{}).Where("is_builtin").Count(&builtinRoles).Error)
	require.Equal(t, int64(3), builtinRoles, "admin, viewer, and editor are seeded builtin roles")
	var operatorRoles int64
	require.NoError(t, gdb.Model(&models.Role{}).Where("code = ?", "k8s_operator").Count(&operatorRoles).Error)
	require.Zero(t, operatorRoles, "delivery must not create a k8s_operator role or grant")

	roleHasPermission := func(roleCode, permissionCode string) bool {
		var count int64
		require.NoError(t, gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Joins("JOIN roles ON roles.id = role_permissions.role_id").
			Where("roles.code = ? AND permissions.code = ?", roleCode, permissionCode).
			Count(&count).Error)
		return count == 1
	}
	for _, code := range []string{
		"delivery:project:read", "delivery:project:write", "delivery:project:delete",
		"delivery:pipeline:read", "delivery:pipeline:write",
		"delivery:run:read", "delivery:run:create", "delivery:run:cancel",
		"delivery:approval:read", "delivery:approval:decide",
	} {
		require.Truef(t, roleHasPermission("admin", code), "admin role missing %q", code)
	}
	for _, code := range []string{"delivery:project:read", "delivery:pipeline:read", "delivery:run:read", "delivery:approval:read"} {
		require.Truef(t, roleHasPermission("viewer", code), "viewer role missing %q", code)
	}

	projectRead := "delivery:project:read"
	approvalRead := "delivery:approval:read"
	wantMenus := []struct {
		code, name, path, component, icon string
		permission                        *string
		sortOrder                         int
	}{
		{"delivery", "menu.delivery_group", "/delivery", "", "deployment-unit", nil, 4},
		{"delivery.projects", "menu.delivery.projects", "/delivery/projects", "delivery/projects/List", "", &projectRead, 0},
		{"delivery.approvals", "menu.delivery.approvals", "/delivery/approvals", "delivery/approvals/List", "", &approvalRead, 1},
	}
	var parent models.Menu
	require.NoError(t, gdb.Where("code = ?", "delivery").First(&parent).Error)
	for i, expected := range wantMenus {
		var menu models.Menu
		require.NoErrorf(t, gdb.Where("code = ?", expected.code).First(&menu).Error, "missing menu %q", expected.code)
		require.Equal(t, expected.name, menu.Name)
		require.Equal(t, expected.path, menu.Path)
		require.Equal(t, expected.component, menu.Component)
		require.Equal(t, expected.icon, menu.Icon)
		require.Equal(t, expected.permission, menu.PermissionCode)
		require.Equal(t, expected.sortOrder, menu.SortOrder)
		if i == 0 {
			require.Nil(t, menu.ParentID)
		} else {
			require.Equal(t, &parent.ID, menu.ParentID)
		}
	}
	var children []models.Menu
	require.NoError(t, gdb.Where("parent_id = ?", parent.ID).Order("sort_order").Find(&children).Error)
	require.Len(t, children, 2)
	require.Equal(t, []string{"delivery.projects", "delivery.approvals"}, []string{children[0].Code, children[1].Code})
	var deliveryMenus int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code = ? OR code LIKE ?", "delivery", "delivery.%").Count(&deliveryMenus).Error)
	require.Equal(t, int64(3), deliveryMenus)
}

func TestRun_DeliveryDoesNotGrantExistingK8sOperator(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	operator := models.Role{Code: "k8s_operator", Name: "role.k8s_operator", Description: "Pre-existing Kubernetes operator", IsBuiltin: true}
	require.NoError(t, gdb.Create(&operator).Error)

	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	var deliveryGrants int64
	require.NoError(t, gdb.Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ? AND permissions.code LIKE ?", operator.ID, "delivery:%").
		Count(&deliveryGrants).Error)
	require.Zero(t, deliveryGrants, "seed must not grant delivery permissions to an existing k8s_operator role")
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

func TestRun_SeedsAssetsMenuTree(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	resourceRead := "assets:resource:read"
	accountRead := "assets:account:read"
	syncRead := "assets:sync:read"
	want := []struct {
		code, name, path, component string
		permission                  *string
		sortOrder                   int
	}{
		{"assets", "menu.assets_group", "/assets", "", &resourceRead, 1},
		{"assets.cloud_accounts", "menu.assets.cloud_accounts", "/assets/cloud-accounts", "assets/cloud-accounts/List", &accountRead, 1},
		{"assets.instances", "menu.assets.instances", "/assets/instances", "assets/instances/List", &resourceRead, 2},
		{"assets.vpcs", "menu.assets.vpcs", "/assets/vpcs", "assets/vpcs/List", &resourceRead, 3},
		{"assets.databases", "menu.assets.databases", "/assets/databases", "assets/databases/List", &resourceRead, 4},
		{"assets.sync_runs", "menu.assets.sync_runs", "/assets/sync-runs", "assets/sync-runs/List", &syncRead, 5},
	}

	var parent models.Menu
	require.NoError(t, gdb.Where("code = ?", "assets").First(&parent).Error)
	for i, expected := range want {
		var menu models.Menu
		require.NoError(t, gdb.Where("code = ?", expected.code).First(&menu).Error, "missing menu %q", expected.code)
		require.Equal(t, expected.name, menu.Name)
		require.Equal(t, expected.path, menu.Path)
		require.Equal(t, expected.component, menu.Component)
		require.Equal(t, expected.permission, menu.PermissionCode)
		require.Equal(t, expected.sortOrder, menu.SortOrder)
		if i == 0 {
			require.Nil(t, menu.ParentID)
		} else {
			require.Equal(t, &parent.ID, menu.ParentID)
		}
	}

	var assetsRows int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code = ? OR code LIKE ?", "assets", "assets.%").Count(&assetsRows).Error)
	require.Equal(t, int64(6), assetsRows)
}

func TestRun_AssetsMenusRemainIdempotent(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	options := seed.Options{AdminUsername: "admin", AdminEmail: "admin@example.com"}
	_, err = seed.Run(context.Background(), gdb, options)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, options)
	require.NoError(t, err)

	var assetsRows int64
	require.NoError(t, gdb.Model(&models.Menu{}).Where("code = ? OR code LIKE ?", "assets", "assets.%").Count(&assetsRows).Error)
	require.Equal(t, int64(6), assetsRows)
}

func TestRun_AdminAndViewerCoverAssetsPermissions(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: "admin", AdminEmail: "admin@example.com",
	})
	require.NoError(t, err)

	roleHasPermission := func(roleCode, permissionCode string) bool {
		var count int64
		require.NoError(t, gdb.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
			Joins("JOIN roles ON roles.id = role_permissions.role_id").
			Where("roles.code = ? AND permissions.code = ?", roleCode, permissionCode).
			Count(&count).Error)
		return count == 1
	}

	for _, code := range []string{
		"assets:account:read", "assets:account:write", "assets:account:delete",
		"assets:resource:read", "assets:sync:read",
	} {
		require.True(t, roleHasPermission("admin", code), "admin role missing %q", code)
	}
	for _, code := range []string{
		"assets:account:read", "assets:resource:read", "assets:sync:read",
	} {
		require.True(t, roleHasPermission("viewer", code), "viewer role missing %q", code)
	}
	require.False(t, roleHasPermission("viewer", "assets:account:write"))
	require.False(t, roleHasPermission("viewer", "assets:account:delete"))
}
