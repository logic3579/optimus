//go:build dbtest

package repo_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/repo"
)

// migrationsPath is the relative path from this test package to the
// embedded SQL migrations directory.
const migrationsPath = "../../../../migrations"

func newRepo(t *testing.T) (*repo.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join(migrationsPath))
	return repo.NewRepo(gdb), td
}

func TestRepo_CRUD(t *testing.T) {
	r, td := newRepo(t)
	defer td()

	m := &models.AppsChartRepo{
		Name:              "bitnami",
		Type:              "http",
		URL:               "https://charts.bitnami.com/bitnami",
		EncryptedPassword: []byte{0x01, 0x02, 0x03},
	}
	require.NoError(t, r.Create(context.Background(), m))
	require.NotZero(t, m.ID)

	got, err := r.Get(context.Background(), m.ID)
	require.NoError(t, err)
	require.Equal(t, "bitnami", got.Name)
	require.Equal(t, []byte{0x01, 0x02, 0x03}, got.EncryptedPassword)

	_, total, err := r.List(context.Background(), repo.ListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)

	require.NoError(t, r.Update(context.Background(), m.ID, map[string]any{
		"description": "primary repo",
	}))
	got, err = r.Get(context.Background(), m.ID)
	require.NoError(t, err)
	require.Equal(t, "primary repo", got.Description)

	require.NoError(t, r.Delete(context.Background(), m.ID))
	_, err = r.Get(context.Background(), m.ID)
	require.Error(t, err)
}

func TestRepo_NameUniquePartialIndex(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &models.AppsChartRepo{Name: "n", Type: "http", URL: "x"}))
	err := r.Create(ctx, &models.AppsChartRepo{Name: "n", Type: "http", URL: "y"})
	require.Error(t, err, "name collision while both alive must error")
}

func TestRepo_List_FiltersAndPagination(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &models.AppsChartRepo{Name: "alpha", Type: "http", URL: "u1"}))
	require.NoError(t, r.Create(ctx, &models.AppsChartRepo{Name: "alphabet", Type: "http", URL: "u2"}))
	require.NoError(t, r.Create(ctx, &models.AppsChartRepo{Name: "beta", Type: "oci", URL: "u3"}))

	rows, total, err := r.List(ctx, repo.ListQuery{Name: "alpha"})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 2)

	rows, total, err = r.List(ctx, repo.ListQuery{Type: "oci"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "beta", rows[0].Name)

	// Pagination: page_size=1 page=2 returns the second-newest row.
	rows, total, err = r.List(ctx, repo.ListQuery{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, rows, 1)
}
