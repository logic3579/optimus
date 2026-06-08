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
	"time"

	"github.com/gin-gonic/gin"

	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/rbac"
)

var Version = "dev"

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path to config")
	checkPerms := flag.Bool("check-permissions", false, "register permission codes and exit")
	flag.Parse()

	abs, err := filepath.Abs(*cfgPath)
	if err != nil {
		fail("resolve config path", err)
	}
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

	signer := crypto.NewJWTSigner(cfg.JWT.Secret)
	limiter := ratelimit.NewLoginLimiter(
		cfg.Auth.LoginRateLimit.PerIP,
		cfg.Auth.LoginRateLimit.Window,
		cfg.Auth.LoginRateLimit.Block,
	)
	authRepo := auth.NewRepo(gdb)
	authSvc := auth.NewService(authRepo, signer, limiter, auth.ServiceOptions{
		AccessTTL:  cfg.JWT.AccessTTL,
		RefreshTTL: cfg.JWT.RefreshTTL,
		BcryptCost: cfg.Auth.BcryptCost,
	})
	authHandler := auth.NewHandler(authSvc)

	// Permission cache TTL: 60s per spec §7.4.
	permCache := rbac.NewPermissionCache(gdb, 60*time.Second)
	meSvc := rbac.NewMeService(gdb, permCache)
	meHandler := rbac.NewHandler(meSvc)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recover(logger))
	r.Use(middleware.CORS(cfg.CORS))
	r.Use(middleware.I18n(cfg.I18n))

	api := r.Group("/api/v1")

	// public
	(&health.Handler{DB: gdb, Version: Version}).Register(api)
	authHandler.Register(api.Group("/auth"))

	// authenticated
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(signer))
	meHandler.RegisterMe(protected)

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
