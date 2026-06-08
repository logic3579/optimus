//go:build dbtest

package db

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// StartTestPostgres boots an ephemeral Postgres in a container,
// runs migrations under migrationsDir, returns the GORM DB and a teardown
// function. Caller must defer teardown().
func StartTestPostgres(t *testing.T, migrationsDir string) (*gorm.DB, func()) {
	t.Helper()
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("dockertest pool: %v", err)
	}

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
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	dsn := fmt.Sprintf(
		"host=localhost port=%s user=test password=test dbname=test sslmode=disable",
		res.GetPort("5432/tcp"),
	)

	pool.MaxWait = 60 * time.Second
	var gdb *gorm.DB
	if err := pool.Retry(func() error {
		var openErr error
		gdb, openErr = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if openErr != nil {
			return openErr
		}
		return Ping(context.Background(), gdb)
	}); err != nil {
		_ = pool.Purge(res)
		t.Fatalf("connect postgres: %v", err)
	}

	sqlDB, _ := gdb.DB()
	migrationsAbs, _ := filepath.Abs(migrationsDir)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, migrationsAbs); err != nil {
		log.Printf("migration up failed: %v", err)
		_ = pool.Purge(res)
		t.Fatal(err)
	}

	teardown := func() { _ = pool.Purge(res) }
	return gdb, teardown
}
