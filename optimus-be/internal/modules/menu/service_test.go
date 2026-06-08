//go:build dbtest

package menu_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/menu"
)

func newSvc(t *testing.T) (*menu.Service, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	rec := audit.NewRecorder(gdb)
	return menu.NewService(menu.NewRepo(gdb), rec), td
}

func TestService_Create_DuplicateCode(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	_, err := svc.Create(ctx, 1, "", "", menu.CreateRequest{Code: "x", Name: "x"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 1, "", "", menu.CreateRequest{Code: "x", Name: "x2"})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeMenuAlreadyExists, be.Code)
}

func TestService_DeleteRejectsParentWithChildren(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	parent, err := svc.Create(ctx, 1, "", "", menu.CreateRequest{Code: "p", Name: "p"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 1, "", "", menu.CreateRequest{Code: "c", Name: "c", ParentID: &parent.ID})
	require.NoError(t, err)

	err = svc.Delete(ctx, 1, "", "", parent.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_UpdateRejectsCycle(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	a, err := svc.Create(ctx, 1, "", "", menu.CreateRequest{Code: "a", Name: "a"})
	require.NoError(t, err)
	b, err := svc.Create(ctx, 1, "", "", menu.CreateRequest{Code: "b", Name: "b", ParentID: &a.ID})
	require.NoError(t, err)

	// Try to make a a child of b (cycle)
	req := menu.UpdateRequest{ParentID: &b.ID}
	_, err = svc.Update(ctx, 1, "", "", a.ID, req)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeBadRequest, be.Code)
}
