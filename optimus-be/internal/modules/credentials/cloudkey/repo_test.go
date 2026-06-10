//go:build dbtest

package cloudkey_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/credentials/cloudkey"
)

func newRepo(t *testing.T) (*cloudkey.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return cloudkey.NewRepo(gdb), td
}

func TestRepo_CreateAndGet(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialCloudKey{
		Name: "prod-aws", Provider: "aws", Region: "us-east-1",
		AccessKeyIDEnc: []byte{1}, SecretAccessKeyEnc: []byte{2},
	}
	require.NoError(t, r.Create(ctx, m))
	got, err := r.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "aws", got.Provider)
}

func TestRepo_NameUniqueAndProviderCheck(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	require.NoError(t, r.Create(ctx, &models.CredentialCloudKey{
		Name: "dup", Provider: "aws", AccessKeyIDEnc: []byte{1}, SecretAccessKeyEnc: []byte{2},
	}))
	require.Error(t, r.Create(ctx, &models.CredentialCloudKey{
		Name: "dup", Provider: "gcp", AccessKeyIDEnc: []byte{3}, SecretAccessKeyEnc: []byte{4},
	}))
	// CHECK constraint on provider
	require.Error(t, r.Create(ctx, &models.CredentialCloudKey{
		Name: "ibm-key", Provider: "ibm", AccessKeyIDEnc: []byte{1}, SecretAccessKeyEnc: []byte{2},
	}))
}

func TestRepo_UpdateAndDelete(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialCloudKey{
		Name: "k1", Provider: "aws", Region: "us-east-1",
		AccessKeyIDEnc: []byte{1}, SecretAccessKeyEnc: []byte{2},
	}
	require.NoError(t, r.Create(ctx, m))
	require.NoError(t, r.Update(ctx, m.ID, map[string]any{"region": "eu-west-1", "provider": "gcp"}))
	got, _ := r.Get(ctx, m.ID)
	require.Equal(t, "eu-west-1", got.Region)
	require.Equal(t, "gcp", got.Provider)
	require.NoError(t, r.Delete(ctx, m.ID))
	_, err := r.Get(ctx, m.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestRepo_List_ProviderFilter(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	names := []struct {
		name string
		prov string
	}{
		{"a1", "aws"}, {"a2", "aws"}, {"g1", "gcp"}, {"z1", "azure"},
	}
	for _, n := range names {
		require.NoError(t, r.Create(ctx, &models.CredentialCloudKey{
			Name: n.name, Provider: n.prov,
			AccessKeyIDEnc: []byte{1}, SecretAccessKeyEnc: []byte{2},
		}))
	}
	rows, total, err := r.List(ctx, cloudkey.ListQuery{Page: 1, PageSize: 10, Provider: "aws"})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
}
