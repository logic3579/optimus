// Package main is the optimus-migrate binary: it applies pending Goose
// SQL migrations embedded into the binary against the Postgres instance
// configured by configs/config.yaml (overridable via OPTIMUS_* env vars).
//
// Exit codes:
//
//	0 — migrations applied successfully (including the no-op case where
//	    the DB is already at head)
//	1 — any failure (config invalid, DB unreachable, migration error)
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/log"
	"optimus-be/migrations"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	direction := flag.String("dir", "up", "up | down | status")
	flag.Parse()

	abs, err := filepath.Abs(*cfgPath)
	if err != nil {
		die("resolve config path", err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		die("load config", err)
	}
	if err := cfg.ValidateForMigrate(); err != nil {
		die("validate config", err)
	}

	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})
	logger.Info("optimus-migrate starting", "direction", *direction)

	db, err := sql.Open("pgx", cfg.Database.DSN)
	if err != nil {
		die("open db", err)
	}
	defer db.Close()

	if err := runGoose(db, *direction); err != nil {
		die("migrate "+*direction, err)
	}
	logger.Info("optimus-migrate done")
}

func runGoose(db *sql.DB, direction string) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	switch direction {
	case "up":
		return goose.Up(db, ".")
	case "down":
		return goose.Down(db, ".")
	case "status":
		return goose.Status(db, ".")
	default:
		return fmt.Errorf("unknown direction: %s", direction)
	}
}

func die(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", msg)
	}
	os.Exit(1)
}
