package migrations

import (
	"strings"
	"testing"

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
