//go:build dbtest

package repo_test

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/apps/repo"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/vault"
)

// setupSvc returns a Service backed by a fresh dockertest Postgres and a real
// vault.Cipher with a random key. The teardown is registered on t.Cleanup.
func setupSvc(t *testing.T) (*repo.Service, *repo.Repo) {
	t.Helper()
	gdb, td := db.StartTestPostgres(t, filepath.Join(migrationsPath))
	t.Cleanup(td)
	r := repo.NewRepo(gdb)

	key := make([]byte, vault.KeyLen)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand key: %v", err)
	}
	cipher, err := vault.NewCipher(key)
	require.NoError(t, err)

	rec := audit.NewRecorder(gdb)
	return repo.NewService(r, cipher, rec), r
}

func TestService_Create_EncryptsPassword(t *testing.T) {
	s, r := setupSvc(t)
	d, err := s.Create(context.Background(), 0, "", "", repo.CreateRequest{
		Name: "private", Type: "oci", URL: "oci://x",
		Username: "u", Password: "secret",
	})
	require.NoError(t, err)
	require.True(t, d.HasPassword)

	m, err := r.Get(context.Background(), d.ID)
	require.NoError(t, err)
	require.NotEqual(t, []byte("secret"), m.EncryptedPassword)
	require.Greater(t, len(m.EncryptedPassword), len("secret"))
}

func TestService_Create_NameTaken(t *testing.T) {
	s, _ := setupSvc(t)
	_, err := s.Create(context.Background(), 0, "", "", repo.CreateRequest{Name: "n", Type: "http", URL: "x"})
	require.NoError(t, err)
	_, err = s.Create(context.Background(), 0, "", "", repo.CreateRequest{Name: "n", Type: "http", URL: "y"})
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_Update_PasswordSemantics(t *testing.T) {
	s, r := setupSvc(t)
	d, err := s.Create(context.Background(), 0, "", "", repo.CreateRequest{
		Name: "p", Type: "http", URL: "x", Password: "secret",
	})
	require.NoError(t, err)
	original, err := r.Get(context.Background(), d.ID)
	require.NoError(t, err)
	originalCt := append([]byte(nil), original.EncryptedPassword...)
	require.NotEmpty(t, originalCt)

	// password absent -> keep.
	_, err = s.Update(context.Background(), 0, "", "", d.ID, repo.UpdateRequest{})
	require.NoError(t, err)
	cur, err := r.Get(context.Background(), d.ID)
	require.NoError(t, err)
	require.Equal(t, originalCt, cur.EncryptedPassword)

	// password empty -> keep.
	empty := ""
	_, err = s.Update(context.Background(), 0, "", "", d.ID, repo.UpdateRequest{Password: &empty})
	require.NoError(t, err)
	cur, err = r.Get(context.Background(), d.ID)
	require.NoError(t, err)
	require.Equal(t, originalCt, cur.EncryptedPassword)

	// password sentinel (null at HTTP layer) -> clear.
	clear := "\x00"
	_, err = s.Update(context.Background(), 0, "", "", d.ID, repo.UpdateRequest{Password: &clear})
	require.NoError(t, err)
	cur, err = r.Get(context.Background(), d.ID)
	require.NoError(t, err)
	require.Empty(t, cur.EncryptedPassword)

	// password new value -> re-encrypt.
	np := "newsecret"
	_, err = s.Update(context.Background(), 0, "", "", d.ID, repo.UpdateRequest{Password: &np})
	require.NoError(t, err)
	cur, err = r.Get(context.Background(), d.ID)
	require.NoError(t, err)
	require.NotEmpty(t, cur.EncryptedPassword)
	require.NotEqual(t, originalCt, cur.EncryptedPassword)
}

// fakeInUse satisfies repo.InUseCounter for the delete-refused test.
type fakeInUse struct{ n int }

func (f *fakeInUse) CountByChartRepoID(context.Context, uint64) (int, error) { return f.n, nil }

func TestService_Delete_RefusedWhenInUse(t *testing.T) {
	s, _ := setupSvc(t)
	s.SetInUseCounter(&fakeInUse{n: 2})
	d, err := s.Create(context.Background(), 0, "", "", repo.CreateRequest{Name: "x", Type: "http", URL: "x"})
	require.NoError(t, err)
	err = s.Delete(context.Background(), 0, "", "", d.ID)
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeAppsChartRepoInUse, be.Code)
}

func TestService_Delete_AllowedWhenNoCounterOrZero(t *testing.T) {
	s, _ := setupSvc(t)
	// no counter set -> allowed
	d, err := s.Create(context.Background(), 0, "", "", repo.CreateRequest{Name: "x1", Type: "http", URL: "x"})
	require.NoError(t, err)
	require.NoError(t, s.Delete(context.Background(), 0, "", "", d.ID))

	// counter set, n=0 -> allowed
	s.SetInUseCounter(&fakeInUse{n: 0})
	d, err = s.Create(context.Background(), 0, "", "", repo.CreateRequest{Name: "x2", Type: "http", URL: "x"})
	require.NoError(t, err)
	require.NoError(t, s.Delete(context.Background(), 0, "", "", d.ID))
}

func TestService_Get_NotFound(t *testing.T) {
	s, _ := setupSvc(t)
	_, err := s.Get(context.Background(), 999)
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_List_NoRows(t *testing.T) {
	s, _ := setupSvc(t)
	out, err := s.List(context.Background(), repo.ListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 0, out.Total)
	require.Empty(t, out.Items)
}
