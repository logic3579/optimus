//go:build dbtest

package user_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/user"
)

func newRepo(t *testing.T) (*user.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	return user.NewRepo(gdb), td
}

func TestRepo_CreateAndGet(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	u := &models.User{Username: "alice", Email: "a@x", PasswordHash: "h", Status: "enabled"}
	require.NoError(t, r.Create(ctx, u))
	require.NotZero(t, u.ID)

	got, err := r.Get(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", got.Username)
}

func TestRepo_List_PaginatesAndFilters(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		require.NoError(t, r.Create(ctx, &models.User{
			Username:     fmt.Sprintf("u%02d", i),
			Email:        fmt.Sprintf("u%02d@x", i),
			PasswordHash: "h",
			Status:       map[bool]string{true: "enabled", false: "disabled"}[i%2 == 0],
		}))
	}
	rows, total, err := r.List(ctx, user.ListQuery{}, pagination.Params{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, rows, 10)
	require.EqualValues(t, 25, total)

	// Filter by status
	rows, total, err = r.List(ctx, user.ListQuery{Status: "disabled"}, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.EqualValues(t, 12, total) // odd indices 1,3,...23 → 12 rows
	for _, u := range rows {
		require.Equal(t, "disabled", u.Status)
	}

	// Filter by search
	rows, _, err = r.List(ctx, user.ListQuery{Search: "u02"}, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "u02", rows[0].Username)
}

func TestRepo_Update(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	u := &models.User{Username: "bob", Email: "b@x", PasswordHash: "h", Status: "enabled"}
	require.NoError(t, r.Create(ctx, u))

	newEmail := "bob@new"
	newName := "Bob"
	require.NoError(t, r.Update(ctx, u.ID, map[string]any{"email": newEmail, "display_name": newName}))

	got, err := r.Get(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "bob@new", got.Email)
	require.Equal(t, "Bob", got.DisplayName)
}

func TestRepo_SoftDelete_AndUsernameReusable(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	u := &models.User{Username: "carol", Email: "c@x", PasswordHash: "h", Status: "enabled"}
	require.NoError(t, r.Create(ctx, u))
	require.NoError(t, r.SoftDelete(ctx, u.ID))

	// Should be re-creatable
	u2 := &models.User{Username: "carol", Email: "c2@x", PasswordHash: "h", Status: "enabled"}
	require.NoError(t, r.Create(ctx, u2))
	require.NotEqual(t, u.ID, u2.ID)
}

func TestRepo_SetRoles_ReplacesAtomically(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	// Need two roles in DB
	role1 := &models.Role{Code: "r1", Name: "r1"}
	role2 := &models.Role{Code: "r2", Name: "r2"}
	require.NoError(t, r.DB().Create(role1).Error)
	require.NoError(t, r.DB().Create(role2).Error)

	u := &models.User{Username: "dan", Email: "d@x", PasswordHash: "h", Status: "enabled"}
	require.NoError(t, r.Create(ctx, u))

	require.NoError(t, r.SetRoles(ctx, u.ID, []uint64{role1.ID}))
	got, err := r.ListRoleIDs(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, []uint64{role1.ID}, got)

	require.NoError(t, r.SetRoles(ctx, u.ID, []uint64{role2.ID}))
	got, err = r.ListRoleIDs(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, []uint64{role2.ID}, got)
}
