//go:build dbtest

package sshkey_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/credentials/sshkey"
)

func newRepo(t *testing.T) (*sshkey.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return sshkey.NewRepo(gdb), td
}

func TestRepo_CreateAndGet(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialSSHKey{
		Name:          "prod-bastion",
		Description:   "bastion host key",
		Username:      "ops",
		PrivateKeyEnc: []byte{0x01, 0x02, 0x03},
	}
	require.NoError(t, r.Create(ctx, m))
	require.NotZero(t, m.ID)

	got, err := r.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "prod-bastion", got.Name)
	require.Equal(t, "ops", got.Username)
}

func TestRepo_NameUnique(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	a := &models.CredentialSSHKey{Name: "dup", Username: "u", PrivateKeyEnc: []byte{1}}
	require.NoError(t, r.Create(ctx, a))
	b := &models.CredentialSSHKey{Name: "dup", Username: "u", PrivateKeyEnc: []byte{2}}
	require.Error(t, r.Create(ctx, b))
}

func TestRepo_ListPagesAndFilters(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		require.NoError(t, r.Create(ctx, &models.CredentialSSHKey{
			Name:          "k" + string(rune('a'+i)),
			Username:      "ops",
			PrivateKeyEnc: []byte{byte(i)},
		}))
	}
	require.NoError(t, r.Create(ctx, &models.CredentialSSHKey{
		Name: "special", Username: "deploy", PrivateKeyEnc: []byte{0xff},
	}))

	rows, total, err := r.List(ctx, sshkey.ListQuery{Page: 1, PageSize: 5})
	require.NoError(t, err)
	require.Equal(t, int64(8), total)
	require.Len(t, rows, 5)

	rows, total, err = r.List(ctx, sshkey.ListQuery{Page: 1, PageSize: 10, Username: "deploy"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, "special", rows[0].Name)

	rows, total, err = r.List(ctx, sshkey.ListQuery{Page: 1, PageSize: 10, Q: "special"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
}

func TestRepo_FindByName_NotFound(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	_, err := r.FindByName(context.Background(), "missing")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "got %v", err)
}

func TestRepo_UpdateAndDelete(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialSSHKey{Name: "k1", Username: "ops", PrivateKeyEnc: []byte{1}, PassphraseEnc: []byte{2}}
	require.NoError(t, r.Create(ctx, m))

	require.NoError(t, r.Update(ctx, m.ID, map[string]any{
		"description":    "x",
		"username":       "deploy",
		"passphrase_enc": nil, // clear column
	}))
	got, err := r.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "x", got.Description)
	require.Equal(t, "deploy", got.Username)
	require.Nil(t, got.PassphraseEnc)

	require.NoError(t, r.Delete(ctx, m.ID))
	_, err = r.Get(ctx, m.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestRepo_Update_EmptyFieldsIsNoop(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()
	m := &models.CredentialSSHKey{Name: "noop", Username: "ops", PrivateKeyEnc: []byte{1}}
	require.NoError(t, r.Create(ctx, m))
	require.NoError(t, r.Update(ctx, m.ID, nil))
	require.NoError(t, r.Update(ctx, m.ID, map[string]any{}))
}
