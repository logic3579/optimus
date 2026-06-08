//go:build dbtest

package role_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/role"
)

func newRepo(t *testing.T) (*role.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	return role.NewRepo(gdb), td
}

func TestRepo_CreateAndList(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	require.NoError(t, r.Create(ctx, &models.Role{Code: "operator", Name: "Operator"}))
	rows, err := r.List(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	codes := []string{}
	for _, row := range rows {
		codes = append(codes, row.Code)
	}
	require.Contains(t, codes, "operator")
}

func TestRepo_SetPermissions_ReplacesAtomically(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	role1 := &models.Role{Code: "rx", Name: "rx"}
	require.NoError(t, r.Create(ctx, role1))

	require.NoError(t, r.SetPermissionsByCode(ctx, role1.ID, []string{"system:user:read", "system:role:read"}))
	codes, err := r.ListPermissionCodes(ctx, role1.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"system:user:read", "system:role:read"}, codes)

	require.NoError(t, r.SetPermissionsByCode(ctx, role1.ID, []string{"system:user:read"}))
	codes, err = r.ListPermissionCodes(ctx, role1.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"system:user:read"}, codes)
}

func TestRepo_SoftDelete_CascadesUserRoles(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	role1 := &models.Role{Code: "rx", Name: "rx"}
	require.NoError(t, r.Create(ctx, role1))
	require.NoError(t, r.DB().Create(&models.User{Username: "u", Email: "u@x", PasswordHash: "h", Status: "enabled"}).Error)
	var u models.User
	require.NoError(t, r.DB().Where("username = ?", "u").First(&u).Error)
	require.NoError(t, r.DB().Create(&models.UserRole{UserID: u.ID, RoleID: role1.ID}).Error)

	userIDs, err := r.SoftDelete(ctx, role1.ID)
	require.NoError(t, err)
	require.Contains(t, userIDs, u.ID)
	// user_roles is hard-deleted in the same tx
	var n int64
	require.NoError(t, r.DB().Model(&models.UserRole{}).Where("role_id = ?", role1.ID).Count(&n).Error)
	require.Zero(t, n)
}
