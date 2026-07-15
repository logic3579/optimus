package migrations

import (
	"math"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestEmbedContainsAllMigrations(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)
	var sqlCount int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlCount++
		}
	}
	require.GreaterOrEqual(t, sqlCount, 11, "expected at least 11 embedded migration files")
}

func TestEmbeddedMigrationsHaveUniqueVersions(t *testing.T) {
	goose.SetBaseFS(FS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })

	migrations, err := goose.CollectMigrations(".", 0, math.MaxInt64)
	require.NoError(t, err)

	var version21Count int
	for _, migration := range migrations {
		if migration.Version == 21 {
			version21Count++
		}
	}
	require.Equal(t, 1, version21Count, "migration version 21 must be discoverable exactly once")
}
