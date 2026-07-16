package module

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"optimus-be/internal/infra/config"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/modules/assets"
	"optimus-be/internal/modules/assets/account"
	"optimus-be/internal/modules/assets/account/inuse"
	"optimus-be/internal/modules/assets/awsclient"
	assetdatabase "optimus-be/internal/modules/assets/database"
	asseterrs "optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/assets/instance"
	assetsync "optimus-be/internal/modules/assets/sync"
	"optimus-be/internal/modules/assets/sync/runs"
	"optimus-be/internal/modules/assets/vpc"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/rbac"
)

const defaultAWSRequestTimeout = 30 * time.Second

type Input struct {
	DB                  *gorm.DB
	Config              config.AssetsConfig
	Audit               *audit.Recorder
	CredentialsConsumer credentials.Consumer
	CloudKeyService     *cloudkey.Service
	Logger              *slog.Logger
}

type Module struct {
	Consumer      assets.Consumer
	InUseCounter  inuse.Counter
	Engine        *assetsync.Engine
	CronScheduler *cron.Cron

	accountHandler  *account.Handler
	instanceHandler *instance.Handler
	vpcHandler      *vpc.Handler
	databaseHandler *assetdatabase.Handler
	runsHandler     *runs.Handler
}

func Wire(rootCtx context.Context, in Input) *Module {
	accountService := account.NewService(account.NewRepo(in.DB), in.Audit, in.CloudKeyService)
	runsRepo := runs.NewRepo(in.DB)
	engine := assetsync.NewEngine(
		in.DB,
		runsRepo,
		assetsync.ClientFactoryFunc(awsclient.For),
		&assetsync.CompositeFetcher{
			Instance: assetsync.EC2Fetcher{},
			Network:  assetsync.VPCFetcher{},
			Database: assetsync.RDSFetcher{},
		},
		in.CredentialsConsumer,
		normalizeRequestTimeout(in.Config.AWSRequestTimeout),
	)

	accountHandler := account.NewHandler(accountService)
	accountHandler.SetTriggerSync(newManualSyncTrigger(accountService, engine, in.Config.AWSRequestTimeout, in.Logger))

	module := &Module{
		Consumer:        assets.NewConsumer(in.DB),
		InUseCounter:    inuse.New(in.DB),
		Engine:          engine,
		accountHandler:  accountHandler,
		instanceHandler: instance.NewHandler(instance.NewService(instance.NewRepo(in.DB))),
		vpcHandler:      vpc.NewHandler(vpc.NewService(vpc.NewRepo(in.DB))),
		databaseHandler: assetdatabase.NewHandler(assetdatabase.NewService(assetdatabase.NewRepo(in.DB))),
		runsHandler:     runs.NewHandler(runs.NewService(runsRepo)),
	}
	module.CronScheduler = assetsync.StartScheduler(rootCtx, assetsync.Config{
		SyncCron:          in.Config.SyncCron,
		StartupDelay:      in.Config.SyncStartupDelay,
		RetentionDays:     in.Config.SyncRunRetentionDays,
		AWSRequestTimeout: normalizeRequestTimeout(in.Config.AWSRequestTimeout),
	}, engine, in.Logger)
	return module
}

func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	group := protected.Group("/assets")
	requirePermission := func(code string) gin.HandlerFunc {
		return middleware.RequirePermission(cache, code)
	}
	m.accountHandler.Mount(group, requirePermission)
	m.instanceHandler.Mount(group, requirePermission)
	m.vpcHandler.Mount(group, requirePermission)
	m.databaseHandler.Mount(group, requirePermission)
	m.runsHandler.Mount(group, requirePermission)
}

type accountSyncService interface {
	Get(context.Context, uint64) (*account.Detail, error)
	RecordSyncTrigger(context.Context, *uint64, string, string, uint64)
}

type syncEngine interface {
	TryLock(uint64) bool
	Unlock(uint64)
	RunAccountLocked(context.Context, uint64, string, *uint64) error
}

func newManualSyncTrigger(service accountSyncService, engine syncEngine, requestTimeout time.Duration, logger *slog.Logger) func(*gin.Context, uint64) {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context, accountID uint64) {
		detail, err := service.Get(c.Request.Context(), accountID)
		if err != nil {
			response.Error(c, err)
			return
		}
		if !detail.Enabled {
			response.Error(c, apperr.New(asseterrs.CodeAssetsCloudAccountDisabled, asseterrs.KeyCloudAccountDisabled, "cloud account is disabled"))
			return
		}
		if !engine.TryLock(accountID) {
			response.Error(c, apperr.New(asseterrs.CodeAssetsSyncBusy, asseterrs.KeySyncBusy, "asset sync already running"))
			return
		}

		requestCtx := c.Request.Context()
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		userID := c.GetUint64(middleware.CtxKeyUserID)
		var actor *uint64
		if userID != 0 {
			actor = &userID
		}
		service.RecordSyncTrigger(requestCtx, actor, clientIP, userAgent, accountID)
		deadline := manualSyncTimeout(requestTimeout, len(detail.EnabledRegions))

		go func() {
			defer engine.Unlock(accountID)
			workerCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), deadline)
			defer cancel()
			if err := engine.RunAccountLocked(workerCtx, accountID, "manual", actor); err != nil {
				logger.Error("assets.sync.manual.failed", "account_id", accountID)
			}
		}()

		response.Success(c, gin.H{"queued": true, "started_at": time.Now().UTC()})
	}
}

func normalizeRequestTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultAWSRequestTimeout
	}
	return timeout
}

func manualSyncTimeout(requestTimeout time.Duration, regionCount int) time.Duration {
	requestTimeout = normalizeRequestTimeout(requestTimeout)
	if regionCount < 1 {
		regionCount = 1
	}
	if uint64(regionCount) > uint64(math.MaxInt64/3) {
		return time.Duration(math.MaxInt64)
	}
	units := int64(regionCount) * 3
	if int64(requestTimeout) > math.MaxInt64/units {
		return time.Duration(math.MaxInt64)
	}
	return requestTimeout * time.Duration(units)
}
