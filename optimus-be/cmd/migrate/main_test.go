//go:build dbtest

package main

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

func TestRunGoose_UpAppliesAllMigrations(t *testing.T) {
	db, teardown := startRawPostgres(t)
	defer teardown()

	require.NoError(t, runGoose(db, "up"))

	var maxVersion int64
	require.NoError(t,
		db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&maxVersion))
	require.GreaterOrEqual(t, maxVersion, int64(11),
		"expected at least 11 migrations to be applied")

	var tableCount int
	require.NoError(t, db.QueryRow(`
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN (
			'users','roles','permissions','user_roles','role_permissions',
			'menus','refresh_tokens','audit_logs'
		)
	`).Scan(&tableCount))
	require.Equal(t, 8, tableCount, "all 8 business tables should exist")
}

func TestRunGoose_UpIsIdempotent(t *testing.T) {
	db, teardown := startRawPostgres(t)
	defer teardown()

	require.NoError(t, runGoose(db, "up"))
	// Second invocation must succeed without changing anything.
	require.NoError(t, runGoose(db, "up"))
}

func TestRunGoose_RejectsUnknownDirection(t *testing.T) {
	// No DB needed — runGoose returns the error before touching the DB.
	err := runGoose(nil, "sideways")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown direction")
}

// startRawPostgres boots a clean Postgres container with NO migrations
// pre-applied. Distinct from internal/infra/db.StartTestPostgres which
// auto-migrates — we explicitly want a virgin DB so runGoose has work to do.
func startRawPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err)

	dsn := fmt.Sprintf(
		"host=localhost port=%s user=test password=test dbname=test sslmode=disable",
		res.GetPort("5432/tcp"),
	)
	pool.MaxWait = 60 * time.Second

	var db *sql.DB
	require.NoError(t, pool.Retry(func() error {
		var openErr error
		db, openErr = sql.Open("pgx", dsn)
		if openErr != nil {
			return openErr
		}
		return db.Ping()
	}))

	return db, func() {
		_ = db.Close()
		_ = pool.Purge(res)
	}
}
