package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

type Config struct {
	SyncCron          string
	StartupDelay      time.Duration
	RetentionDays     int
	AWSRequestTimeout time.Duration
}

func StartScheduler(ctx context.Context, cfg Config, engine *Engine, logger *slog.Logger) *cron.Cron {
	if logger == nil {
		logger = slog.Default()
	}
	scheduler := cron.New(cron.WithLocation(time.UTC))
	if _, err := scheduler.AddFunc(cfg.SyncCron, func() {
		if err := engine.RunAll(ctx, "cron"); err != nil {
			logger.Error("assets.sync.cron.failed")
		}
	}); err != nil {
		logger.Error("assets.sync.cron.invalid")
		return scheduler
	}
	_, _ = scheduler.AddFunc("0 3 * * *", func() {
		count, err := engine.PruneSyncRuns(ctx, cfg.RetentionDays)
		if err != nil {
			logger.Error("assets.sync.prune.failed")
			return
		}
		logger.Info("assets.sync.prune.completed", "deleted", count)
	})

	go runScheduler(ctx, scheduler, cfg.StartupDelay, logger)
	return scheduler
}

func runScheduler(ctx context.Context, scheduler *cron.Cron, delay time.Duration, logger *slog.Logger) {
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		scheduler.Start()
		logger.Info("assets.sync.scheduler.started")
	}

	<-ctx.Done()
	stopped := scheduler.Stop()
	shutdownTimer := time.NewTimer(30 * time.Second)
	defer shutdownTimer.Stop()
	select {
	case <-stopped.Done():
	case <-shutdownTimer.C:
		logger.Warn("assets.sync.scheduler.shutdown.timeout")
	}
}
