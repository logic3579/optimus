//go:build dbtest

package permissions_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
)

func TestP5PermissionRegistry(t *testing.T) {
	want := []string{
		"credentials:http:read", "credentials:http:write",
		"credentials:http:delete", "credentials:http:use",
		"observability:datasource:read", "observability:datasource:write",
		"observability:datasource:delete", "observability:metric:read",
		"observability:dashboard:read", "observability:dashboard:write",
		"observability:dashboard:delete",
	}

	var got []string
	for _, permission := range permissions.All {
		if strings.HasPrefix(permission.Code, "credentials:http:") || strings.HasPrefix(permission.Code, "observability:") {
			got = append(got, permission.Code)
		}
	}
	require.ElementsMatch(t, want, got)
	for _, code := range got {
		require.False(t, strings.HasPrefix(code, "observability:alert:"), code)
	}
}

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
