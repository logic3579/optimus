//go:build dbtest

package role_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/role"
	"optimus-be/internal/seed"
)

func newSvc(t *testing.T) (*role.Service, func(), *rbac.PermissionCache) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	rec := audit.NewRecorder(gdb)
	svc := role.NewService(role.NewRepo(gdb), cache, rec)
	return svc, td, cache
}

func TestService_Create_DuplicateCode(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()
	_, err := svc.Create(ctx, 1, "", "", role.CreateRequest{Code: "admin", Name: "x"})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeRoleAlreadyExists, be.Code)
}

func TestService_Delete_BuiltinRejected(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()
	var admin models.Role
	require.NoError(t, svc.Repo().DB().Where("code = ?", "admin").First(&admin).Error)
	err := svc.Delete(ctx, 1, "", "", admin.ID)
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeBuiltinRoleImmutable, be.Code)
}

func TestService_Update_HappyAndNotFound(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()

	out, err := svc.Create(ctx, 1, "", "", role.CreateRequest{Code: "ops", Name: "Ops"})
	require.NoError(t, err)

	newName := "Operations"
	newDesc := "Ops crew"
	updated, err := svc.Update(ctx, 1, "1.1.1.1", "ua", out.ID, role.UpdateRequest{Name: &newName, Description: &newDesc})
	require.NoError(t, err)
	require.Equal(t, "Operations", updated.Name)
	require.Equal(t, "Ops crew", updated.Description)

	_, err = svc.Update(ctx, 1, "", "", 99999, role.UpdateRequest{Name: &newName})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Get_NotFound(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	_, err := svc.Get(context.Background(), 99999)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_SetPermissions_InvalidatesAllBoundUsers(t *testing.T) {
	svc, td, cache := newSvc(t)
	defer td()
	ctx := context.Background()
	// Create role + bind two users to it
	out, err := svc.Create(ctx, 1, "", "", role.CreateRequest{Code: "ops", Name: "Ops"})
	require.NoError(t, err)
	gdb := svc.Repo().DB()
	for _, name := range []string{"u1", "u2"} {
		u := &models.User{Username: name, Email: name + "@x", PasswordHash: "h", Status: "enabled"}
		require.NoError(t, gdb.Create(u).Error)
		require.NoError(t, gdb.Create(&models.UserRole{UserID: u.ID, RoleID: out.ID}).Error)
		// pre-warm cache
		_, err := cache.Get(ctx, u.ID)
		require.NoError(t, err)
	}
	require.NoError(t, svc.SetPermissions(ctx, 1, "", "", out.ID, []string{"system:user:read"}))
	// cache for both users must be cleared and reload non-empty with the new code
	var rows []models.User
	require.NoError(t, gdb.Where("username IN ?", []string{"u1", "u2"}).Find(&rows).Error)
	for _, u := range rows {
		codes, err := cache.Get(ctx, u.ID)
		require.NoError(t, err)
		require.Contains(t, codes, "system:user:read")
	}
}
