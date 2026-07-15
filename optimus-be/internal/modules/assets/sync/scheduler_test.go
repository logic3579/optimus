package sync

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartSchedulerRegistersJobsAndStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := StartScheduler(ctx, Config{
		SyncCron:      "@every 1h",
		StartupDelay:  time.Hour,
		RetentionDays: 90,
	}, &Engine{}, logger)
	require.NotNil(t, scheduler)
	require.Len(t, scheduler.Entries(), 2)
	cancel()
}

func TestStartSchedulerRejectsInvalidCronWithoutStarting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := StartScheduler(ctx, Config{SyncCron: "not-a-cron"}, &Engine{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NotNil(t, scheduler)
	require.Empty(t, scheduler.Entries())
}
