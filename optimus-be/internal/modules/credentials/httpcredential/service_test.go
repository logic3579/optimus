//go:build dbtest

package httpcredential_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

type recordingCipher struct {
	opened []byte
	cancel context.CancelFunc
}
type counter struct {
	n       int64
	err     error
	calls   int
	txCalls int
}

func (c *counter) CountByHTTPCredentialIDTx(_ context.Context, tx *gorm.DB, _ uint64) (int64, error) {
	if tx == nil {
		return 0, errors.New("nil tx")
	}
	c.txCalls++
	return c.n, c.err
}

func (c *counter) CountByHTTPCredentialID(context.Context, uint64) (int64, error) {
	c.calls++
	return c.n, c.err
}

func (*recordingCipher) Seal(b []byte) ([]byte, error) { return append([]byte(nil), b...), nil }
func (c *recordingCipher) Open([]byte) ([]byte, error) {
	c.opened = []byte("decrypted")
	if c.cancel != nil {
		c.cancel()
	}
	return c.opened, nil
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
	u := " alice "
	d, err := s.Create(context.Background(), 0, "", "", httpcredential.CreateRequest{Name: " x ", AuthType: "basic", Username: &u, Secret: "secret"})
	require.NoError(t, err)
	require.Equal(t, "x", d.Name)
	require.Equal(t, "alice", *d.Username)
	require.NotContains(t, requireJSON(t, d), "secret")
}
func TestBearerRejectsUsernameAndPersistsNull(t *testing.T) {
	s, _, td := setup(t)
	defer td()
	bad := "bad"
	_, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "b", AuthType: "bearer", Username: &bad, Secret: "token"})
	require.Error(t, err)
	empty := ""
	_, err = s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "empty", AuthType: "bearer", Username: &empty, Secret: "token"})
	require.Error(t, err)
	_, err = s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "basic", AuthType: "basic", Secret: "password"})
	require.Error(t, err)
	d, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "b", AuthType: "bearer", Secret: "token"})
	require.NoError(t, err)
	require.Nil(t, d.Username)
}
func TestCreateAndUpdateRejectSecretOver16KiB(t *testing.T) {
	s, c, td := setup(t)
	defer td()
	huge := strings.Repeat("x", 16385)
	_, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "huge", AuthType: "bearer", Secret: huge})
	require.Error(t, err)
	require.Zero(t, c.seals)
	d, _ := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "ok", AuthType: "bearer", Secret: "x"})
	_, err = s.Update(t.Context(), 0, "", "", d.ID, httpcredential.UpdateRequest{Secret: &huge})
	require.Error(t, err)
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
func TestConsumeCopiesAndWipesOpenedBuffer(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	c := &recordingCipher{}
	s := httpcredential.NewService(httpcredential.NewRepo(g), c, nil)
	d, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "copy", AuthType: "bearer", Secret: "x"})
	require.NoError(t, err)
	r, err := s.Consume(t.Context(), nil, d.ID, "system:test")
	require.NoError(t, err)
	require.Equal(t, []byte("decrypted"), r.Secret)
	require.Equal(t, make([]byte, len(c.opened)), c.opened)
	r.Secret[0] = 'X'
	require.Zero(t, c.opened[0])
}
func TestConsumeWipesOpenedBufferWhenCanceledAfterDecrypt(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	ctx, cancel := context.WithCancel(t.Context())
	c := &recordingCipher{cancel: cancel}
	s := httpcredential.NewService(httpcredential.NewRepo(g), c, nil)
	d, err := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "cancel", AuthType: "bearer", Secret: "x"})
	require.NoError(t, err)
	_, err = s.Consume(ctx, nil, d.ID, "system:test")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, make([]byte, len(c.opened)), c.opened)
}
func TestDeleteInUseCounterPositiveErrorAndNil(t *testing.T) {
	for _, tc := range []struct {
		name    string
		c       *counter
		wantErr error
	}{
		{"positive", &counter{n: 1}, nil}, {"error", &counter{err: errors.New("counter failed")}, errors.New("counter failed")}, {"nil", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, td := setup(t)
			defer td()
			d, _ := s.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "delete", AuthType: "bearer", Secret: "x"})
			s.SetInUseCounter(tc.c)
			err := s.Delete(t.Context(), 0, "1.2.3.4", "agent", d.ID)
			if tc.c == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, 1, tc.c.txCalls)
				require.Error(t, err)
				require.Equal(t, 1, tc.c.calls)
			}
		})
	}
}
func TestMutationAuditsCarryActorIPUAAndSafeFields(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	u := models.User{Username: "audit-user", Email: "audit@example.test", PasswordHash: "hash", Status: "enabled"}
	require.NoError(t, g.Create(&u).Error)
	s := httpcredential.NewService(httpcredential.NewRepo(g), &cipher{}, audit.NewRecorder(g))
	d, err := s.Create(t.Context(), u.ID, "10.0.0.1", "test-agent", httpcredential.CreateRequest{Name: "audit-fields", AuthType: "bearer", Secret: "secret"})
	require.NoError(t, err)
	n := "renamed"
	_, err = s.Update(t.Context(), u.ID, "10.0.0.2", "update-agent", d.ID, httpcredential.UpdateRequest{Name: &n})
	require.NoError(t, err)
	require.NoError(t, s.Delete(t.Context(), u.ID, "10.0.0.3", "delete-agent", d.ID))
	var logs []models.AuditLog
	require.NoError(t, g.Where("target_type = ?", "credentials.http_credential").Order("id").Find(&logs).Error)
	require.Len(t, logs, 3)
	for _, l := range logs {
		require.Equal(t, u.ID, *l.UserID)
		require.NotContains(t, string(l.Payload), "secret")
		require.NotContains(t, string(l.Payload), "ciphertext")
	}
	require.Equal(t, "10.0.0.1", logs[0].IP)
	require.Equal(t, "test-agent", logs[0].UserAgent)
	require.Equal(t, "10.0.0.2", logs[1].IP)
	require.Equal(t, "update-agent", logs[1].UserAgent)
	require.Equal(t, "10.0.0.3", logs[2].IP)
	require.Equal(t, "delete-agent", logs[2].UserAgent)
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
