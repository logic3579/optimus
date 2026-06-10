//go:build dbtest

package cluster_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/k8s/cluster"
)

func newRepo(t *testing.T) (*cluster.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return cluster.NewRepo(gdb), td
}

func setupKubeconfigRow(t *testing.T, repo *cluster.Repo) uint64 {
	t.Helper()
	kc := &models.CredentialKubeconfig{
		Name:          "kc-" + t.Name(),
		KubeconfigEnc: []byte{0x01, 0x02, 0x03},
	}
	require.NoError(t, repo.DB().Create(kc).Error)
	return kc.ID
}

func TestRepo_CreateAndGet(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)

	m := &models.Cluster{
		Name:         "prod",
		KubeconfigID: kcID,
		Context:      "prod-ctx",
		Tags:         []string{"prod", "us-east-1"},
	}
	require.NoError(t, repo.Create(context.Background(), m))
	require.NotZero(t, m.ID)

	got, err := repo.Get(context.Background(), m.ID)
	require.NoError(t, err)
	require.Equal(t, "prod", got.Name)
	require.Equal(t, []string{"prod", "us-east-1"}, []string(got.Tags))
	require.NotNil(t, got.Kubeconfig)
	require.Equal(t, kcID, got.Kubeconfig.ID)
}

func TestRepo_NameUniquePartial(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "n", KubeconfigID: kcID, Context: "c1",
	}))
	err := repo.Create(ctx, &models.Cluster{
		Name: "n", KubeconfigID: kcID, Context: "c2",
	})
	require.Error(t, err) // partial unique on name violates

	// soft-delete and re-create with same name should succeed
	m, err := repo.FindByName(ctx, "n")
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, m.ID))
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "n", KubeconfigID: kcID, Context: "c3",
	}))
}

func TestRepo_KubeconfigContextUnique(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "a", KubeconfigID: kcID, Context: "shared",
	}))
	err := repo.Create(ctx, &models.Cluster{
		Name: "b", KubeconfigID: kcID, Context: "shared",
	})
	require.Error(t, err) // (kubeconfig_id, context) partial unique
}

func TestRepo_FK_Restrict(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "fk", KubeconfigID: kcID, Context: "c",
	}))
	err := repo.DB().Delete(&models.CredentialKubeconfig{}, kcID).Error
	require.Error(t, err) // ON DELETE RESTRICT
}

func TestRepo_TagFilter(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "p", KubeconfigID: kcID, Context: "p", Tags: []string{"prod"},
	}))
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "s", KubeconfigID: kcID, Context: "s", Tags: []string{"staging"},
	}))
	rows, _, err := repo.List(ctx, cluster.ListQuery{Tag: "prod"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "p", rows[0].Name)
}

func TestRepo_CountByKubeconfigID(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()
	n, err := repo.CountByKubeconfigID(ctx, kcID)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "x", KubeconfigID: kcID, Context: "c",
	}))
	n, err = repo.CountByKubeconfigID(ctx, kcID)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
