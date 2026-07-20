//go:build dbtest

package httpcredential_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/httpcredential"
)

type cipher struct{ seals int }

func (c *cipher) Seal(b []byte) ([]byte, error) {
	c.seals++
	return append([]byte("sealed:"), b...), nil
}
func (*cipher) Open(b []byte) ([]byte, error) {
	if len(b) < 7 {
		return nil, errors.New("bad")
	}
	return append([]byte(nil), b[7:]...), nil
}

func setup(t *testing.T) (*httpcredential.Service, *cipher, func()) {
	t.Helper()
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	c := &cipher{}
	return httpcredential.NewService(httpcredential.NewRepo(g), c, audit.NewRecorder(g)), c, td
}

func TestCreateValidatesBeforeSealAndRedactsSecret(t *testing.T) {
	s, c, td := setup(t)
	defer td()
	_, err := s.Create(context.Background(), 0, "", "", httpcredential.CreateRequest{Name: " x ", AuthType: "basic", Secret: "secret"})
	require.Error(t, err)
	require.Zero(t, c.seals)
	d, err := s.Create(context.Background(), 0, "", "", httpcredential.CreateRequest{Name: " x ", AuthType: "basic", Username: " alice ", Secret: "secret"})
	require.NoError(t, err)
	require.Equal(t, "x", d.Name)
	require.Equal(t, "alice", *d.Username)
	require.NotContains(t, requireJSON(t, d), "secret")
}
func TestBearerRejectsUsernameAndPersistsNull(t *testing.T) {
	s, _, td := setup(t)
	defer td()
	_, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "b", AuthType: "bearer", Username: "bad", Secret: "token"})
	require.Error(t, err)
	d, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "b", AuthType: "bearer", Secret: "token"})
	require.NoError(t, err)
	require.Nil(t, d.Username)
}
func TestUpdatePreservesAndReplacesSecret(t *testing.T) {
	s, _, td := setup(t)
	defer td()
	d, _ := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "b", AuthType: "bearer", Secret: "old"})
	n := "new name"
	_, err := s.Update(t.Context(), 0, "", "", d.ID, httpcredential.UpdateRequest{Name: &n})
	require.NoError(t, err)
	r, _ := s.Consume(t.Context(), nil, d.ID, "system:test")
	require.Equal(t, []byte("old"), r.Secret)
	replacement := "new"
	_, err = s.Update(t.Context(), 0, "", "", d.ID, httpcredential.UpdateRequest{Secret: &replacement})
	require.NoError(t, err)
	r, _ = s.Consume(t.Context(), nil, d.ID, "system:test")
	require.Equal(t, []byte("new"), r.Secret)
}
func TestDuplicateNotFoundAndNilCounterDelete(t *testing.T) {
	s, _, td := setup(t)
	defer td()
	req := httpcredential.CreateRequest{Name: "b", AuthType: "bearer", Secret: "t"}
	d, _ := s.Create(t.Context(), 0, "", "", req)
	_, err := s.Create(t.Context(), 0, "", "", req)
	assertCode(t, err, apperr.CodeConflict)
	_, err = s.Get(t.Context(), 9999)
	assertCode(t, err, apperr.CodeNotFound)
	s.SetInUseCounter(nil)
	require.NoError(t, s.Delete(t.Context(), 0, "", "", d.ID))
}
func TestUpdateMapsDuplicateNameToConflict(t *testing.T) {
	s, _, td := setup(t)
	defer td()
	_, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "one", AuthType: "bearer", Secret: "x"})
	require.NoError(t, err)
	two, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "two", AuthType: "bearer", Secret: "y"})
	require.NoError(t, err)
	duplicate := "one"
	_, err = s.Update(t.Context(), 0, "", "", two.ID, httpcredential.UpdateRequest{Name: &duplicate})
	assertCode(t, err, apperr.CodeConflict)
}
func TestAuditsNeverContainSecretOrCiphertextAndConsumeExactlyOnce(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	s := httpcredential.NewService(httpcredential.NewRepo(g), &cipher{}, audit.NewRecorder(g))
	d, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "audit", AuthType: "bearer", Secret: "top-secret"})
	require.NoError(t, err)
	_, err = s.Consume(t.Context(), nil, d.ID, "system:test")
	require.NoError(t, err)
	var logs []models.AuditLog
	require.NoError(t, g.Where("target_type = ?", "credentials.http_credential").Find(&logs).Error)
	require.Len(t, logs, 2)
	for _, log := range logs {
		payload := string(log.Payload)
		require.NotContains(t, payload, "top-secret")
		require.NotContains(t, payload, "ciphertext")
	}
	var consumes int64
	require.NoError(t, g.Model(&models.AuditLog{}).Where("action = ?", "credentials.consume.http_credential").Count(&consumes).Error)
	require.Equal(t, int64(1), consumes)
}
func assertCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	b, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, code, b.Code)
}
func requireJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
