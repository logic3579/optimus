//go:build dbtest

package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
)

func TestStartTestPostgres_RunsMigrations(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	require.NoError(t, db.Ping(context.Background(), gdb))

	var count int
	require.NoError(t, gdb.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'").Row().Scan(&count))
	// 8 business tables + goose_db_version = 9
	require.GreaterOrEqual(t, count, 9)
}
