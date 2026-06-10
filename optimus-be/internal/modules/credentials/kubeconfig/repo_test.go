//go:build dbtest

package kubeconfig_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/credentials/kubeconfig"
)

func newRepo(t *testing.T) (*kubeconfig.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return kubeconfig.NewRepo(gdb), td
}

func TestRepo_CreateAndGet(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialKubeconfig{
		Name: "prod-cluster", Description: "kc", DefaultNamespace: "default",
		KubeconfigEnc: []byte{1, 2, 3},
	}
	require.NoError(t, r.Create(ctx, m))
	require.NotZero(t, m.ID)
	got, err := r.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "prod-cluster", got.Name)
}

func TestRepo_NameUnique(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	require.NoError(t, r.Create(ctx, &models.CredentialKubeconfig{Name: "dup", KubeconfigEnc: []byte{1}}))
	require.Error(t, r.Create(ctx, &models.CredentialKubeconfig{Name: "dup", KubeconfigEnc: []byte{2}}))
}

func TestRepo_ListPagesAndFilters(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, r.Create(ctx, &models.CredentialKubeconfig{
			Name:             "kc" + string(rune('a'+i)),
			DefaultNamespace: "default",
			KubeconfigEnc:    []byte{byte(i)},
		}))
	}
	require.NoError(t, r.Create(ctx, &models.CredentialKubeconfig{
		Name: "special", DefaultNamespace: "kube-system", KubeconfigEnc: []byte{0xff},
	}))

	rows, total, err := r.List(ctx, kubeconfig.ListQuery{Page: 1, PageSize: 3})
	require.NoError(t, err)
	require.Equal(t, int64(6), total)
	require.Len(t, rows, 3)

	rows, total, err = r.List(ctx, kubeconfig.ListQuery{Page: 1, PageSize: 10, DefaultNamespace: "kube-system"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "special", rows[0].Name)
}

func TestRepo_FindByName_NotFound(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	_, err := r.FindByName(context.Background(), "missing")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestRepo_UpdateAndDelete(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialKubeconfig{Name: "k1", DefaultNamespace: "default", KubeconfigEnc: []byte{1}}
	require.NoError(t, r.Create(ctx, m))
	require.NoError(t, r.Update(ctx, m.ID, map[string]any{"description": "x", "default_namespace": "ns2"}))
	got, _ := r.Get(ctx, m.ID)
	require.Equal(t, "x", got.Description)
	require.Equal(t, "ns2", got.DefaultNamespace)
	require.NoError(t, r.Delete(ctx, m.ID))
	_, err := r.Get(ctx, m.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}
