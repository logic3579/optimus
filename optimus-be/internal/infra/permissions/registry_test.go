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

func TestAssetsPermsRegistered(t *testing.T) {
	want := []string{
		permissions.PermAssetsAccountRead,
		permissions.PermAssetsAccountWrite,
		permissions.PermAssetsAccountDelete,
		permissions.PermAssetsResourceRead,
		permissions.PermAssetsSyncRead,
	}
	exact := []string{
		"assets:account:read",
		"assets:account:write",
		"assets:account:delete",
		"assets:resource:read",
		"assets:sync:read",
	}
	have := map[string]bool{}
	for _, p := range permissions.All {
		have[p.Code] = true
	}
	for i, w := range want {
		if w != exact[i] {
			t.Errorf("permission constant = %q, want %q", w, exact[i])
		}
		if !have[w] {
			t.Errorf("permission %q not registered in All", w)
		}
		if !strings.HasPrefix(w, "assets:") {
			t.Errorf("permission %q does not have assets: prefix", w)
		}
	}

	// The viewer role should match the three read permissions, but not write/delete.
	matchedByViewer := []string{}
	for _, p := range want {
		if strings.HasSuffix(p, ":read") {
			matchedByViewer = append(matchedByViewer, p)
		}
	}
	if len(matchedByViewer) != 3 {
		t.Errorf("expected 3 read perms for viewer, got %d", len(matchedByViewer))
	}
}
