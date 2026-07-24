package httpcredential

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type behaviorRepo struct {
	row                                                    *models.HTTPCredential
	deleted                                                bool
	createErr, getErr, findErr, listErr, updateErr, delErr error
}

func (r *behaviorRepo) Create(_ context.Context, m *models.HTTPCredential) error {
	if r.createErr != nil {
		return r.createErr
	}
	m.ID = 1
	r.row = m
	return nil
}
func (r *behaviorRepo) Get(_ context.Context, id uint64) (*models.HTTPCredential, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.deleted || r.row == nil || r.row.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	out := *r.row
	return &out, nil
}
func (r *behaviorRepo) FindByName(_ context.Context, name string) (*models.HTTPCredential, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	if r.row != nil && !r.deleted && r.row.Name == name {
		return r.row, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (r *behaviorRepo) List(context.Context, ListQuery) ([]models.HTTPCredential, int64, error) {
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	if r.row == nil || r.deleted {
		return nil, 0, nil
	}
	return []models.HTTPCredential{*r.row}, 1, nil
}
func (r *behaviorRepo) Update(_ context.Context, id uint64, fields map[string]any) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.row == nil || r.row.ID != id {
		return gorm.ErrRecordNotFound
	}
	if value, ok := fields["name"].(string); ok {
		r.row.Name = value
	}
	if value, ok := fields["username"].(string); ok {
		r.row.Username = &value
	}
	if value, ok := fields["secret_ciphertext"].([]byte); ok {
		r.row.SecretCiphertext = value
	}
	return nil
}
func (r *behaviorRepo) Delete(context.Context, uint64) error {
	if r.delErr != nil {
		return r.delErr
	}
	r.deleted = true
	return nil
}

type copyCipher struct{}

func (copyCipher) Seal(value []byte) ([]byte, error) { return append([]byte(nil), value...), nil }
func (copyCipher) Open(value []byte) ([]byte, error) { return append([]byte(nil), value...), nil }

type faultCipher struct{ sealErr, openErr error }

func (c faultCipher) Seal([]byte) ([]byte, error) { return nil, c.sealErr }
func (c faultCipher) Open([]byte) ([]byte, error) { return nil, c.openErr }

type behaviorCounter struct {
	n   int64
	err error
}

func (c behaviorCounter) CountByHTTPCredentialID(context.Context, uint64) (int64, error) {
	return c.n, c.err
}

func marshalBehaviorJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func TestServiceCRUDConsumeKeepsSecretsOutOfPublicShapes(t *testing.T) {
	repo := &behaviorRepo{}
	svc := NewService(repo, copyCipher{}, nil)
	username := " reader "
	got, err := svc.Create(t.Context(), 7, "192.0.2.1", "test", CreateRequest{
		Name: " prom ", AuthType: "basic", Username: &username, Secret: "top-secret",
	})
	require.NoError(t, err)
	require.Equal(t, "prom", got.Name)
	require.Equal(t, "reader", *got.Username)
	require.NotContains(t, marshalBehaviorJSON(t, got), "top-secret")
	require.Equal(t, []byte("top-secret"), repo.row.SecretCiphertext)

	page, err := svc.List(t.Context(), ListQuery{})
	require.NoError(t, err)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 20, page.PageSize)
	require.NotContains(t, marshalBehaviorJSON(t, page), "top-secret")

	name, replacement := "renamed", "replacement-secret"
	got, err = svc.Update(t.Context(), 7, "", "", 1, UpdateRequest{Name: &name, Secret: &replacement})
	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.NotContains(t, marshalBehaviorJSON(t, got), replacement)

	actor := uint64(7)
	consumed, err := svc.Consume(t.Context(), &actor, 1, "observability.query.instant")
	require.NoError(t, err)
	require.Equal(t, []byte(replacement), consumed.Secret)
	require.NotSame(t, &repo.row.SecretCiphertext[0], &consumed.Secret[0])

	svc.SetInUseCounter(behaviorCounter{n: 1})
	require.Error(t, svc.Delete(t.Context(), 7, "", "", 1))
	require.False(t, repo.deleted)
	svc.SetInUseCounter(behaviorCounter{err: errors.New("count failed")})
	require.ErrorContains(t, svc.Delete(t.Context(), 7, "", "", 1), "count failed")
	svc.SetInUseCounter(nil)
	require.NoError(t, svc.Delete(t.Context(), 7, "", "", 1))
	require.True(t, repo.deleted)
}

func TestServiceValidationAndFailuresDoNotEchoAuth(t *testing.T) {
	const auth = "never-echo-this-auth"
	for _, request := range []CreateRequest{
		{Name: " ", AuthType: "bearer", Secret: auth},
		{Name: strings.Repeat("n", 129), AuthType: "bearer", Secret: auth},
		{Name: "bad-auth", AuthType: "digest", Secret: auth},
		{Name: "missing-user", AuthType: "basic", Secret: auth},
		{Name: "empty-secret", AuthType: "bearer"},
		{Name: "large-secret", AuthType: "bearer", Secret: strings.Repeat("s", 16385)},
	} {
		_, err := NewService(&behaviorRepo{}, copyCipher{}, nil).Create(t.Context(), 0, "", "", request)
		require.Error(t, err)
		require.NotContains(t, err.Error(), auth)
	}

	dbErr := errors.New("database unavailable")
	_, err := NewService(&behaviorRepo{findErr: dbErr}, copyCipher{}, nil).Create(t.Context(), 0, "", "", CreateRequest{Name: "x", AuthType: "bearer", Secret: auth})
	require.ErrorIs(t, err, dbErr)
	_, err = NewService(&behaviorRepo{}, faultCipher{sealErr: errors.New(auth)}, nil).Create(t.Context(), 0, "", "", CreateRequest{Name: "x", AuthType: "bearer", Secret: auth})
	require.Error(t, err)
	require.NotContains(t, err.Error(), auth)
	_, err = NewService(&behaviorRepo{createErr: dbErr}, copyCipher{}, nil).Create(t.Context(), 0, "", "", CreateRequest{Name: "x", AuthType: "bearer", Secret: auth})
	require.ErrorIs(t, err, dbErr)
	duplicate := &behaviorRepo{row: &models.HTTPCredential{ID: 2, Name: "taken", AuthType: "bearer"}}
	_, err = NewService(duplicate, copyCipher{}, nil).Create(t.Context(), 0, "", "", CreateRequest{Name: "taken", AuthType: "bearer", Secret: auth})
	require.Error(t, err)
	require.NotContains(t, err.Error(), auth)
}

func TestServiceReadUpdateAndConsumeErrorBranches(t *testing.T) {
	dbErr := errors.New("database unavailable")
	_, err := NewService(&behaviorRepo{listErr: dbErr}, copyCipher{}, nil).List(t.Context(), ListQuery{})
	require.ErrorIs(t, err, dbErr)
	_, err = NewService(&behaviorRepo{getErr: dbErr}, copyCipher{}, nil).Get(t.Context(), 1)
	require.ErrorIs(t, err, dbErr)

	row := &models.HTTPCredential{ID: 1, Name: "old", AuthType: "bearer", SecretCiphertext: []byte("old-secret")}
	_, err = NewService(&behaviorRepo{}, copyCipher{}, nil).Update(t.Context(), 0, "", "", 1, UpdateRequest{})
	require.Error(t, err)
	name := "new"
	_, err = NewService(&behaviorRepo{row: row, findErr: dbErr}, copyCipher{}, nil).Update(t.Context(), 0, "", "", 1, UpdateRequest{Name: &name})
	require.ErrorIs(t, err, dbErr)
	empty := ""
	_, err = NewService(&behaviorRepo{row: row}, copyCipher{}, nil).Update(t.Context(), 0, "", "", 1, UpdateRequest{Secret: &empty})
	require.Error(t, err)
	replacement := "replacement"
	_, err = NewService(&behaviorRepo{row: row}, faultCipher{sealErr: errors.New(replacement)}, nil).Update(t.Context(), 0, "", "", 1, UpdateRequest{Secret: &replacement})
	require.Error(t, err)
	require.NotContains(t, err.Error(), replacement)
	_, err = NewService(&behaviorRepo{row: row, updateErr: dbErr}, copyCipher{}, nil).Update(t.Context(), 0, "", "", 1, UpdateRequest{})
	require.ErrorIs(t, err, dbErr)
	username := "forbidden"
	_, err = NewService(&behaviorRepo{row: row}, copyCipher{}, nil).Update(t.Context(), 0, "", "", 1, UpdateRequest{Username: &username})
	require.Error(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = NewService(&behaviorRepo{row: row}, copyCipher{}, nil).Consume(ctx, nil, 1, "system:test")
	require.ErrorIs(t, err, context.Canceled)
	_, err = NewService(&behaviorRepo{row: row}, copyCipher{}, nil).Consume(t.Context(), nil, 1, " ")
	require.Error(t, err)
	_, err = NewService(&behaviorRepo{row: row}, copyCipher{}, nil).Consume(t.Context(), nil, 1, "user-purpose")
	require.Error(t, err)
	_, err = NewService(&behaviorRepo{getErr: dbErr}, copyCipher{}, nil).Consume(t.Context(), nil, 1, "system:test")
	require.ErrorIs(t, err, dbErr)
	_, err = NewService(&behaviorRepo{row: row}, faultCipher{openErr: errors.New(replacement)}, nil).Consume(t.Context(), nil, 1, "system:test")
	require.Error(t, err)
	require.NotContains(t, err.Error(), replacement)
}

type txBehaviorRepo struct {
	*behaviorRepo
	transactionErr, lockedErr, deleteTxErr error
}

func (r *txBehaviorRepo) Transaction(_ context.Context, fn func(*gorm.DB) error) error {
	if r.transactionErr != nil {
		return r.transactionErr
	}
	return fn(nil)
}
func (r *txBehaviorRepo) GetForUpdate(context.Context, *gorm.DB, uint64) (*models.HTTPCredential, error) {
	if r.lockedErr != nil {
		return nil, r.lockedErr
	}
	if r.row == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.row, nil
}
func (r *txBehaviorRepo) DeleteTx(context.Context, *gorm.DB, uint64) error {
	if r.deleteTxErr != nil {
		return r.deleteTxErr
	}
	r.deleted = true
	return nil
}

type txBehaviorCounter struct {
	n   int64
	err error
}

func (txBehaviorCounter) CountByHTTPCredentialID(context.Context, uint64) (int64, error) {
	panic("non-transactional counter path")
}
func (c txBehaviorCounter) CountByHTTPCredentialIDTx(context.Context, *gorm.DB, uint64) (int64, error) {
	return c.n, c.err
}

func TestServiceTransactionalDeleteBranches(t *testing.T) {
	dbErr := errors.New("database unavailable")
	cases := []struct {
		name    string
		repo    *txBehaviorRepo
		counter InUseCounter
		success bool
	}{
		{"transaction error", &txBehaviorRepo{behaviorRepo: &behaviorRepo{row: &models.HTTPCredential{ID: 1}}, transactionErr: dbErr}, nil, false},
		{"not found", &txBehaviorRepo{behaviorRepo: &behaviorRepo{}}, nil, false},
		{"read error", &txBehaviorRepo{behaviorRepo: &behaviorRepo{}, lockedErr: dbErr}, nil, false},
		{"count error", &txBehaviorRepo{behaviorRepo: &behaviorRepo{row: &models.HTTPCredential{ID: 1}}}, txBehaviorCounter{err: dbErr}, false},
		{"in use", &txBehaviorRepo{behaviorRepo: &behaviorRepo{row: &models.HTTPCredential{ID: 1}}}, txBehaviorCounter{n: 1}, false},
		{"delete error", &txBehaviorRepo{behaviorRepo: &behaviorRepo{row: &models.HTTPCredential{ID: 1}}, deleteTxErr: dbErr}, nil, false},
		{"success", &txBehaviorRepo{behaviorRepo: &behaviorRepo{row: &models.HTTPCredential{ID: 1}}}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(tc.repo, copyCipher{}, nil)
			svc.SetInUseCounter(tc.counter)
			err := svc.Delete(t.Context(), 0, "", "", 1)
			if tc.success {
				require.NoError(t, err)
				require.True(t, tc.repo.deleted)
			} else {
				require.Error(t, err)
				require.False(t, tc.repo.deleted)
			}
		})
	}
}
