//go:build dbtest

package permissions_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
)

func TestRegister_InsertsAllCodes(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	result, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	require.Equal(t, len(permissions.All), result.Inserted)
	require.Equal(t, 0, result.Updated)
	require.Empty(t, result.Stale)

	var count int64
	gdb.Model(&models.Permission{}).Count(&count)
	require.Equal(t, int64(len(permissions.All)), count)
}

func TestRegister_UpdatesChangedRows(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	_, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)

	modified := append([]permissions.Permission{}, permissions.All...)
	modified[0].Description = "NEW DESCRIPTION"

	result, err := permissions.Register(context.Background(), gdb, modified)
	require.NoError(t, err)
	require.Equal(t, 0, result.Inserted)
	require.Equal(t, 1, result.Updated)

	var got models.Permission
	gdb.Where("code = ?", modified[0].Code).First(&got)
	require.Equal(t, "NEW DESCRIPTION", got.Description)
}

func TestRegister_DetectsStaleRows(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	// Seed an extra permission row that's not in our registry.
	gdb.Create(&models.Permission{Code: "obsolete:thing:read", Name: "obsolete", Category: "obsolete"})

	result, err := permissions.Register(context.Background(), gdb, permissions.All)
	require.NoError(t, err)
	require.Contains(t, result.Stale, "obsolete:thing:read")
}
