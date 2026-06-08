//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/seed"
)

func TestMeService_ListMenus_ViewerSeesOnlyPermittedNodes(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(context.Background(), gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x"})
	require.NoError(t, err)

	u := &models.User{Username: "viewer1", Email: "v@x", PasswordHash: "x", Status: "enabled"}
	gdb.Create(u)
	var viewer models.Role
	gdb.Where("code = ?", "viewer").First(&viewer)
	gdb.Create(&models.UserRole{UserID: u.ID, RoleID: viewer.ID})

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute), nil)
	tree, err := svc.ListMenus(context.Background(), u.ID)
	require.NoError(t, err)

	var foundDashboard, foundSystem bool
	for _, top := range tree {
		if top.Code == "dashboard" {
			foundDashboard = true
		}
		if top.Code == "system" {
			foundSystem = true
			require.Contains(t, codes(top.Children), "system.users")
		}
	}
	require.True(t, foundDashboard)
	require.True(t, foundSystem)
}

func codes(nodes []rbac.MeMenuNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Code)
	}
	return out
}
