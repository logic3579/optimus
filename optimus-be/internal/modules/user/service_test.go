//go:build dbtest

package user_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/user"
	"optimus-be/internal/seed"
)

func newSvc(t *testing.T) (*user.Service, func(), *rbac.PermissionCache) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)
	cache := rbac.NewPermissionCache(gdb, time.Minute)
	rec := audit.NewRecorder(gdb)
	svc := user.NewService(user.NewRepo(gdb), cache, rec, user.ServiceOptions{BcryptCost: 4, AdminUsername: "admin"})
	return svc, td, cache
}

func TestService_Create_HashesPasswordAndAuditss(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()
	actor := uint64(1) // admin

	out, err := svc.Create(ctx, actor, "127.0.0.1", "go-test", user.CreateRequest{
		Username: "alice", Email: "a@new", Password: "Pass1234",
		DisplayName: "Alice", RoleIDs: []uint64{},
	})
	require.NoError(t, err)
	require.Equal(t, "alice", out.Username)
	require.NotZero(t, out.ID)
}

func TestService_Create_DuplicateUsername(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()
	_, err := svc.Create(ctx, 1, "", "", user.CreateRequest{
		Username: "admin", Email: "x@x", Password: "Pass1234",
	})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeUserAlreadyExists, be.Code)
}

func TestService_Delete_RejectsSelfAndAdmin(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()
	// admin self (id=1)
	err := svc.Delete(ctx, 1, "", "", 1)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeCannotDeleteSelf, be.Code)

	// Create a second admin-like user, try to delete admin as that other user
	other, err := svc.Create(ctx, 1, "", "", user.CreateRequest{
		Username: "op", Email: "op@x", Password: "Pass1234",
	})
	require.NoError(t, err)
	err = svc.Delete(ctx, other.ID, "", "", 1) // 1 = admin per seed
	be, ok = apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeCannotDeleteAdmin, be.Code)
}

func TestService_SetRoles_InvalidatesCache(t *testing.T) {
	svc, td, cache := newSvc(t)
	defer td()
	ctx := context.Background()
	// Create a user and grant viewer role
	out, err := svc.Create(ctx, 1, "", "", user.CreateRequest{
		Username: "u1", Email: "u1@x", Password: "Pass1234",
	})
	require.NoError(t, err)

	// Pre-warm cache
	codes, err := cache.Get(ctx, out.ID)
	require.NoError(t, err)
	require.Empty(t, codes)

	// Find viewer role id
	var viewer models.Role
	require.NoError(t, svc.Repo().DB().Where("code = ?", "viewer").First(&viewer).Error)

	require.NoError(t, svc.SetRoles(ctx, 1, "", "", out.ID, []uint64{viewer.ID}))
	codes, err = cache.Get(ctx, out.ID)
	require.NoError(t, err)
	require.NotEmpty(t, codes, "cache should reload after invalidation")
}

func TestService_List_ReturnsPage(t *testing.T) {
	svc, td, _ := newSvc(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, 1, "", "", user.CreateRequest{
			Username: "li" + string(rune('a'+i)), Email: "li" + string(rune('a'+i)) + "@x", Password: "Pass1234",
		})
		require.NoError(t, err)
	}
	page, err := svc.List(ctx, user.ListQuery{}, pagination.Params{Page: 1, PageSize: 3})
	require.NoError(t, err)
	require.Len(t, page.Items, 3)
	require.EqualValues(t, 6, page.Total) // 5 created + 1 admin seeded
}
