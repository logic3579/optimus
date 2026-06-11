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
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	// Blank import registers the generated OpenAPI spec with swag at init time
	// so /swagger/* serves it. Regenerate via `make swag` whenever annotations
	// change — CI's `make swagger-diff` will catch drift otherwise.
	_ "optimus-be/api/docs"
	"optimus-be/internal/infra/config"
	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/log"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/apps/helmclient"
	appsmodule "optimus-be/internal/modules/apps/module"
	apprepo "optimus-be/internal/modules/apps/repo"
	"optimus-be/internal/modules/apps/release"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/auth"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/health"
	"optimus-be/internal/modules/k8s"
	"optimus-be/internal/modules/menu"
	"optimus-be/internal/modules/permission"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/role"
	"optimus-be/internal/modules/user"
)

var Version = "dev"

// @title           Optimus Admin API
// @version         1.0
// @description     P0 admin backend for Optimus — auth, RBAC, users, roles, permissions, menus, audit.
// @description     All authenticated endpoints expect `Authorization: Bearer <access_token>`.
// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http https
//
// @securityDefinitions.apikey BearerAuth
// @in   header
// @name Authorization
// @description Type "Bearer" followed by a space and the JWT access token.
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

	// Vault master key: refuse to start without one (parallels the JWT secret
	// gate above). Done BEFORE opening the DB so misconfiguration fails fast.
	masterKey, err := vault.LoadKey(vault.Source{
		Env:  cfg.Vault.MasterKey,
		File: cfg.Vault.MasterKeyFile,
	})
	if err != nil {
		fail("load vault master key", err)
	}
	cipher, err := vault.NewCipher(masterKey)
	if err != nil {
		fail("build vault cipher", err)
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

	// userSvc is constructed before MeService because MeService depends on it
	// for /me writes (UpdateProfile / ChangePassword). The audit recorder is
	// shared so /me writes and /users writes audit through the same sink.
	auditRec := audit.NewRecorder(gdb)
	userSvc := user.NewService(user.NewRepo(gdb), permCache, auditRec, user.ServiceOptions{
		BcryptCost:    cfg.Auth.BcryptCost,
		AdminUsername: cfg.Boot.AdminUsername,
	})

	meSvc := rbac.NewMeService(gdb, permCache, userSvc)
	meHandler := rbac.NewHandler(meSvc)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recover(logger))
	r.Use(middleware.CORS(cfg.CORS))
	r.Use(middleware.I18n(cfg.I18n))

	// Swagger UI: served at /swagger/index.html. The spec is bundled via the
	// blank import of optimus-be/api/docs above.
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api/v1")

	// public
	(&health.Handler{DB: gdb, Version: Version}).Register(api)
	authHandler.Register(api.Group("/auth"))

	// authenticated
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(signer))
	meHandler.RegisterMe(protected)

	// module wiring: per-route RequirePermission via nested sub-groups so the
	// middleware runs BEFORE the handler (gin only chains middlewares supplied
	// at Group/Use; passing them as variadic args to GET/POST/... does not
	// guarantee they run first when handlers are registered separately).
	userHandler := user.NewHandler(userSvc)
	mountUserRoutes(protected, userHandler, permCache)

	roleSvc := role.NewService(role.NewRepo(gdb), permCache, auditRec)
	roleHandler := role.NewHandler(roleSvc)
	mountRoleRoutes(protected, roleHandler, permCache)

	permHandler := permission.NewHandler(gdb)
	mountPermissionRoutes(protected, permHandler, permCache)

	menuSvc := menu.NewService(menu.NewRepo(gdb), auditRec)
	menuHandler := menu.NewHandler(menuSvc)
	mountMenuRoutes(protected, menuHandler, permCache)

	auditSvc := audit.NewService(audit.NewRepo(gdb))
	auditHandler := audit.NewHandler(auditSvc)
	mountAuditRoutes(protected, auditHandler, permCache)

	// P1 credentials vault: 3 CRUD surfaces under /credentials, gated by the
	// 12 credentials:* permission codes. The exposed credsModule.Consumer is
	// the Go-only seam for downstream sub-projects (P2+).
	credsModule := credentials.New(gdb, cipher, auditRec)
	credsModule.MountRoutes(protected, permCache)
	_ = credsModule.Consumer // referenced once so vet/staticcheck see it as live API

	// P2 k8s management: cluster CRUD + 13 read kinds + SSE pod logs.
	// Reuses credsModule.Consumer for kubeconfig fetch; emits audit via the
	// shared recorder.
	k8sModule := k8s.New(gdb, credsModule.Consumer, auditRec, permCache)
	k8sModule.MountRoutes(protected, permCache)

	// P3 applications: chart-repo CRUD + application CRUD + helm-driven
	// release lifecycle. Wiring order matters because three cross-package
	// seams are post-construction:
	//   1. apprepo.Service ← application.Repo as InUseCounter (refuses
	//      chart-repo delete while applications still reference it).
	//   2. k8s/cluster.Service ← application.Repo as AppsApplicationCounter
	//      (refuses cluster delete while applications still reference it).
	//   3. application.Service ← release.Service as HelmStatusProbe +
	//      HelmInstalledChecker (decorates Get with live status, refuses
	//      application delete while the helm release still exists).
	//
	// release.Service's ChartLoader seam is wired at construction time via
	// apps.HelmChartLoader (which delegates to apprepo.Service.LoadChart) —
	// no cycle since apprepo.Service has no reference back to release.
	//
	// The vault cipher is the SAME instance the credentials module owns —
	// never construct a second AEAD; the apps/repo chart-repo password
	// re-uses the P1 master key (see CLAUDE.md "Don't bypass Consumer").
	appsRepoSvc := apprepo.NewService(apprepo.NewRepo(gdb), cipher, auditRec)
	appsAppRepo := application.NewRepo(gdb)
	appsAppSvc := application.NewService(appsAppRepo, auditRec)
	appsRepoSvc.SetInUseCounter(appsAppRepo)
	k8sModule.SetAppsCounter(appsAppRepo)

	helmFactory := helmclient.NewFactory(credsModule.Consumer, k8sModule.Cluster)
	appsRelSvc := release.NewService(helmFactory, appsAppSvc, &appsmodule.HelmChartLoader{Repo: appsRepoSvc}, auditRec)
	appsAppSvc.SetHelmStatusProbe(appsRelSvc)
	appsAppSvc.SetHelmInstalledChecker(appsRelSvc)

	appsModule := appsmodule.New(appsRepoSvc, appsAppSvc, appsRelSvc)
	appsModule.MountRoutes(protected, permCache)

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

// mountUserRoutes mounts /users with per-route RBAC gates per spec §7.2.
func mountUserRoutes(protected *gin.RouterGroup, h *user.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/users")

	rd := g.Group("", middleware.RequirePermission(cache, "system:user:read"))
	rd.GET("", h.HandleList())
	rd.GET("/:id", h.HandleGet())

	wr := g.Group("", middleware.RequirePermission(cache, "system:user:write"))
	wr.POST("", h.HandleCreate())
	wr.PUT("/:id", h.HandleUpdate())
	wr.PUT("/:id/roles", h.HandleSetRoles())
	wr.PUT("/:id/status", h.HandleSetStatus())

	rp := g.Group("", middleware.RequirePermission(cache, "system:user:reset_pass"))
	rp.PUT("/:id/password", h.HandleSetPassword())

	del := g.Group("", middleware.RequirePermission(cache, "system:user:delete"))
	del.DELETE("/:id", h.HandleDelete())
}

// mountRoleRoutes mounts /roles with per-route RBAC gates per spec §7.2.
func mountRoleRoutes(protected *gin.RouterGroup, h *role.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/roles")

	rd := g.Group("", middleware.RequirePermission(cache, "system:role:read"))
	rd.GET("", h.HandleList())
	rd.GET("/:id", h.HandleGet())

	wr := g.Group("", middleware.RequirePermission(cache, "system:role:write"))
	wr.POST("", h.HandleCreate())
	wr.PUT("/:id", h.HandleUpdate())
	wr.PUT("/:id/permissions", h.HandleSetPermissions())

	del := g.Group("", middleware.RequirePermission(cache, "system:role:delete"))
	del.DELETE("/:id", h.HandleDelete())
}

// mountPermissionRoutes mounts the read-only /permissions endpoint.
func mountPermissionRoutes(protected *gin.RouterGroup, h *permission.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/permissions")
	rd := g.Group("", middleware.RequirePermission(cache, "system:permission:read"))
	rd.GET("", h.HandleList())
}

// mountMenuRoutes mounts /menus with per-route RBAC gates per spec §7.2.
func mountMenuRoutes(protected *gin.RouterGroup, h *menu.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/menus")

	rd := g.Group("", middleware.RequirePermission(cache, "system:menu:read"))
	rd.GET("", h.HandleTree())

	wr := g.Group("", middleware.RequirePermission(cache, "system:menu:write"))
	wr.POST("", h.HandleCreate())
	wr.PUT("/:id", h.HandleUpdate())

	del := g.Group("", middleware.RequirePermission(cache, "system:menu:delete"))
	del.DELETE("/:id", h.HandleDelete())
}

// mountAuditRoutes mounts the read-only /audit-logs endpoint.
func mountAuditRoutes(protected *gin.RouterGroup, h *audit.Handler, cache *rbac.PermissionCache) {
	g := protected.Group("/audit-logs")
	rd := g.Group("", middleware.RequirePermission(cache, "system:audit:read"))
	rd.GET("", h.HandleList())
}
