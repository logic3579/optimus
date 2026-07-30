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

	var version23Count int
	for _, migration := range migrations {
		if migration.Version == 23 {
			version23Count++
		}
	}
	require.Equal(t, 1, version23Count, "migration version 23 must be discoverable exactly once")
}

func TestP4AssetsReferencesCredentialsCloudKeys(t *testing.T) {
	sql, err := FS.ReadFile("00021_p4_assets.sql")
	require.NoError(t, err)

	contents := string(sql)
	require.Contains(t, contents, "REFERENCES credentials_cloud_keys(id)")
	require.NotContains(t, contents, "REFERENCES credential_cloud_keys")
}

func TestP6DeliveryMigrationIsEmbedded(t *testing.T) {
	_, err := FS.ReadFile("00023_p6_delivery.sql")
	require.NoError(t, err)
}

func TestP6DeliveryStageTimeoutsMatchConfiguredCeiling(t *testing.T) {
	sql, err := FS.ReadFile("00023_p6_delivery.sql")
	require.NoError(t, err)

	const timeoutConstraint = "timeout_seconds BETWEEN 1 AND 86400"
	require.Equal(t, 2, strings.Count(string(sql), timeoutConstraint),
		"pipeline and run stages must both accept the configured 24-hour ceiling")
}
