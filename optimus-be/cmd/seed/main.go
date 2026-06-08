package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/seed"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	flag.Parse()

	abs, _ := filepath.Abs(*cfgPath)
	cfg, err := config.Load(abs)
	if err != nil {
		die("load config", err)
	}
	if err := cfg.ValidateStrict(); err != nil {
		die("validate config", err)
	}
	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})

	gdb, err := db.Open(cfg.Database)
	if err != nil {
		die("open db", err)
	}

	if r, err := permissions.Register(context.Background(), gdb, permissions.All); err != nil {
		die("register permissions", err)
	} else {
		logger.Info("permissions registered", "inserted", r.Inserted, "updated", r.Updated, "stale", r.Stale)
	}

	res, err := seed.Run(context.Background(), gdb, seed.Options{
		AdminUsername: cfg.Boot.AdminUsername,
		AdminEmail:    cfg.Boot.AdminEmail,
		BcryptCost:    cfg.Auth.BcryptCost,
	})
	if err != nil {
		die("seed", err)
	}

	if res.AdminInitialPassword != "" {
		logger.Warn(
			"INITIAL ADMIN CREDENTIALS — RECORD THESE NOW (printed only once)",
			"username", cfg.Boot.AdminUsername,
			"password", res.AdminInitialPassword,
		)
	} else {
		logger.Info("admin user already exists; no password generated")
	}
}

func die(stage string, err error) {
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", stage, err)
	os.Exit(1)
}
