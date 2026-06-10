//go:build dbtest

package sshkey_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/sshkey"
)

func genTestSSHKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	blk, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	return string(pem.EncodeToMemory(blk))
}

type passthroughCipher struct{}

func (passthroughCipher) Seal(b []byte) ([]byte, error) {
	out := make([]byte, 0, len(b)+4)
	out = append(out, []byte("SEAL")...)
	out = append(out, b...)
	return out, nil
}

func (passthroughCipher) Open(b []byte) ([]byte, error) {
	if len(b) < 4 || string(b[:4]) != "SEAL" {
		return nil, errors.New("bad ciphertext")
	}
	return b[4:], nil
}

func newSvc(t *testing.T) (*sshkey.Service, *audit.Recorder, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	rec := audit.NewRecorder(gdb)
	svc := sshkey.NewService(sshkey.NewRepo(gdb), passthroughCipher{}, rec)
	return svc, rec, td
}

func TestService_Create_RoundTrip(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)

	d, err := svc.Create(ctx, 0, "1.2.3.4", "test-ua", sshkey.CreateRequest{
		Name: "n1", Username: "ops", PrivateKey: key,
	})
	require.NoError(t, err)
	require.NotZero(t, d.ID)
	require.Equal(t, "n1", d.Name)
	require.Equal(t, "ops", d.Username)
}

func TestService_Create_InvalidKey(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	_, err := svc.Create(context.Background(), 0, "", "", sshkey.CreateRequest{
		Name: "bad", Username: "ops", PrivateKey: "not-pem",
	})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.invalid_key_format", be.MessageKey)
}

func TestService_Create_NameTaken(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	_, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "dup", Username: "u", PrivateKey: key})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "dup", Username: "u", PrivateKey: key})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	_, err := svc.Get(context.Background(), 99999)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Update_OnlyChangedFields(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "u1", Username: "ops", PrivateKey: key})
	require.NoError(t, err)

	desc := "new desc"
	_, err = svc.Update(ctx, 0, "", "", d.ID, sshkey.UpdateRequest{Description: &desc})
	require.NoError(t, err)
	got, err := svc.Get(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, "new desc", got.Description)
	require.Equal(t, "ops", got.Username) // unchanged
}

func TestService_Update_NameTaken(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	a, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "a", Username: "u", PrivateKey: key})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "b", Username: "u", PrivateKey: key})
	require.NoError(t, err)

	newName := "b"
	_, err = svc.Update(ctx, 0, "", "", a.ID, sshkey.UpdateRequest{Name: &newName})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_Update_RotateSecret(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key1 := genTestSSHKey(t)
	d, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "rot", Username: "ops", PrivateKey: key1})
	require.NoError(t, err)

	key2 := genTestSSHKey(t)
	_, err = svc.Update(ctx, 0, "", "", d.ID, sshkey.UpdateRequest{PrivateKey: &key2})
	require.NoError(t, err)

	actor := uint64(7)
	rec, err := svc.Consume(ctx, &actor, d.ID, "test.rotate")
	require.NoError(t, err)
	require.Equal(t, key2, string(rec.PrivateKey))
}

func TestService_Update_ClearPassphrase(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{
		Name: "pp", Username: "ops", PrivateKey: key, Passphrase: "secret",
	})
	require.NoError(t, err)

	empty := ""
	_, err = svc.Update(ctx, 0, "", "", d.ID, sshkey.UpdateRequest{Passphrase: &empty})
	require.NoError(t, err)

	// Internal verify: the passphrase_enc column should be NULL.
	var got models.CredentialSSHKey
	require.NoError(t, svc.Repo().DB().First(&got, d.ID).Error)
	require.Nil(t, got.PassphraseEnc)
}

func TestService_Delete(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "del", Username: "ops", PrivateKey: key})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))
	_, err = svc.Get(ctx, d.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	err := svc.Delete(context.Background(), 0, "", "", 99999)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Consume_EmptyPurpose(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	actor := uint64(1)
	_, err := svc.Consume(context.Background(), &actor, 1, "")
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.invalid_purpose", be.MessageKey)
}

func TestService_Consume_SystemRequiresPrefix(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "sp", Username: "ops", PrivateKey: key})
	require.NoError(t, err)
	_, err = svc.Consume(ctx, nil, d.ID, "not-a-system-purpose")
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.system_purpose_required", be.MessageKey)
}

func TestService_Consume_SystemAccepted(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	key := genTestSSHKey(t)
	d, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{Name: "sa", Username: "ops", PrivateKey: key})
	require.NoError(t, err)
	rec, err := svc.Consume(ctx, nil, d.ID, "system:cron.sync")
	require.NoError(t, err)
	require.Equal(t, key, string(rec.PrivateKey))
}

func TestService_Consume_NotFound(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	actor := uint64(1)
	_, err := svc.Consume(context.Background(), &actor, 99999, "test.thing")
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_List_Pagination(t *testing.T) {
	svc, _, td := newSvc(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		_, err := svc.Create(ctx, 0, "", "", sshkey.CreateRequest{
			Name:       "l" + string(rune('a'+i)),
			Username:   "ops",
			PrivateKey: genTestSSHKey(t),
		})
		require.NoError(t, err)
	}
	res, err := svc.List(ctx, sshkey.ListQuery{Page: 1, PageSize: 4})
	require.NoError(t, err)
	require.Equal(t, int64(6), res.Total)
	require.Len(t, res.Items, 4)
	require.Equal(t, 1, res.Page)
}
