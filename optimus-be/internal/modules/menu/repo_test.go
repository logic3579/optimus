//go:build dbtest

package menu_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/menu"
)

func newRepo(t *testing.T) (*menu.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	return menu.NewRepo(gdb), td
}

func TestRepo_BuildTree(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()

	root := &models.Menu{Code: "sys", Name: "menu.sys", SortOrder: 0}
	require.NoError(t, r.Create(ctx, root))
	child := &models.Menu{ParentID: &root.ID, Code: "sys.users", Name: "menu.sys.users", SortOrder: 0}
	require.NoError(t, r.Create(ctx, child))

	tree, err := r.Tree(ctx)
	require.NoError(t, err)
	require.Len(t, tree, 1)
	require.Equal(t, "sys", tree[0].Code)
	require.Len(t, tree[0].Children, 1)
	require.Equal(t, "sys.users", tree[0].Children[0].Code)
}

func TestRepo_DeleteRejectsParentWithChildren(t *testing.T) {
	r, td := newRepo(t)
	defer td()
	ctx := context.Background()

	root := &models.Menu{Code: "sys", Name: "menu.sys"}
	require.NoError(t, r.Create(ctx, root))
	child := &models.Menu{ParentID: &root.ID, Code: "sys.users", Name: "menu.sys.users"}
	require.NoError(t, r.Create(ctx, child))

	hasChildren, err := r.HasChildren(ctx, root.ID)
	require.NoError(t, err)
	require.True(t, hasChildren)

	hasChildren, err = r.HasChildren(ctx, child.ID)
	require.NoError(t, err)
	require.False(t, hasChildren)
}
