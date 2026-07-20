package httpcredential

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"optimus-be/internal/models"
)

type errorRepo struct {
	findErr error
	get     *models.HTTPCredential
}

func (*errorRepo) Create(context.Context, *models.HTTPCredential) error { return nil }
func (r *errorRepo) Get(context.Context, uint64) (*models.HTTPCredential, error) {
	if r.get != nil {
		return r.get, nil
	}
	return nil, errors.New("unused")
}
func (r *errorRepo) FindByName(context.Context, string) (*models.HTTPCredential, error) {
	return nil, r.findErr
}
func (*errorRepo) List(context.Context, ListQuery) ([]models.HTTPCredential, int64, error) {
	return nil, 0, nil
}
func (*errorRepo) Update(context.Context, uint64, map[string]any) error { return nil }
func (*errorRepo) Delete(context.Context, uint64) error                 { return nil }

type unitCipher struct{ called bool }

func (c *unitCipher) Seal([]byte) ([]byte, error) { c.called = true; return nil, nil }
func (*unitCipher) Open([]byte) ([]byte, error)   { return nil, nil }

func TestCreatePropagatesFindByNameDatabaseErrorBeforeSeal(t *testing.T) {
	dbErr := errors.New("database unavailable")
	c := &unitCipher{}
	s := NewService(&errorRepo{findErr: dbErr}, c, nil)
	_, err := s.Create(t.Context(), 0, "", "", CreateRequest{Name: "name", AuthType: "bearer", Secret: "x"})
	require.ErrorIs(t, err, dbErr)
	require.False(t, c.called)
}
func TestUpdatePropagatesFindByNameDatabaseError(t *testing.T) {
	dbErr := errors.New("database unavailable")
	s := NewService(&errorRepo{findErr: dbErr, get: &models.HTTPCredential{ID: 1, Name: "old", AuthType: "bearer"}}, &unitCipher{}, nil)
	name := "new"
	_, err := s.Update(t.Context(), 0, "", "", 1, UpdateRequest{Name: &name})
	require.ErrorIs(t, err, dbErr)
}
