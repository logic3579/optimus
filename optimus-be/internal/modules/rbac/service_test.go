//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/rbac"
)

func TestMeService_GetUser(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	u := &models.User{Username: "alice", Email: "a@x", PasswordHash: "x", Status: "enabled", DisplayName: "Alice"}
	require.NoError(t, gdb.Create(u).Error)

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute))
	dto, err := svc.GetUser(context.Background(), u.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, dto.ID)
	require.Equal(t, "alice", dto.Username)
	require.Equal(t, "Alice", dto.DisplayName)
}

func TestMeService_GetUser_NotFound(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, 0))
	_, err := svc.GetUser(context.Background(), 999999)
	require.Error(t, err)
}
