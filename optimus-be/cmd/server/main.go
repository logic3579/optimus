package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/modules/health"
)

var (
	Version = "dev" // set via -ldflags at build time
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	checkPerms := flag.Bool("check-permissions", false, "register permission codes and exit")
	flag.Parse()

	abs, _ := filepath.Abs(*cfgPath)
	cfg, err := config.Load(abs)
	if err != nil {
		fail("load config", err)
	}
	if err := cfg.ValidateStrict(); err != nil {
		fail("validate config", err)
	}

	logger := log.New(log.Options{Level: cfg.Log.Level, Format: cfg.Log.Format})
	logger.Info("optimus-be starting", "version", Version)

	gdb, err := db.Open(cfg.Database)
	if err != nil {
		fail("open db", err)
	}

	if _, err := permissions.Register(context.Background(), gdb, permissions.All); err != nil {
		fail("register permissions", err)
	}
	if *checkPerms {
		logger.Info("permissions registered, exiting due to -check-permissions")
		return
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api/v1")
	(&health.Handler{DB: gdb, Version: Version}).Register(api)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "err", err)
	}

	if sqlDB, _ := gdb.DB(); sqlDB != nil {
		_ = sqlDB.Close()
	}
	logger.Info("stopped")
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", stage, err)
	os.Exit(1)
}
