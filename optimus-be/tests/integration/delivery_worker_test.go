//go:build dbtest

package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/orchestrator"
	"optimus-be/tests/dbtest"
)

func TestDeliveryWorkersClaimOnce(t *testing.T) {
	_, db := setupServer(t)
	initiator := dbtest.SeedUser(t, db, "delivery-worker-initiator")
	cluster := dbtest.SeedCluster(t, db, "delivery-worker")
	repo := &models.AppsChartRepo{Name: "delivery-worker-repo", Type: "http", URL: "https://worker.example.test"}
	require.NoError(t, db.Create(repo).Error)
	app := &models.AppsApplication{Name: "delivery-worker-app", ClusterID: cluster.ID, Namespace: "default", ReleaseName: "worker", ChartRepoID: repo.ID, ChartName: "worker"}
	require.NoError(t, db.Create(app).Error)
	project := &models.DeliveryProject{Name: "delivery-worker-project"}
	require.NoError(t, db.Create(project).Error)
	environment := &models.DeliveryEnvironment{ProjectID: project.ID, EnvironmentKey: "prod", DisplayName: "Production", ApplicationID: app.ID}
	require.NoError(t, db.Create(environment).Error)
	pipeline := &models.DeliveryPipeline{ProjectID: project.ID, Version: 1, CreatedByUserID: initiator.ID, PublishedAt: time.Now().UTC(), IsCurrent: true}
	require.NoError(t, db.Create(pipeline).Error)
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := &models.DeliveryRun{ProjectID: project.ID, PipelineID: pipeline.ID, PipelineVersion: 1, ChartRepoID: repo.ID, ChartName: "worker", ChartVersion: "1.0.0", ChartDigest: digest, InitiatorUserID: initiator.ID, IdempotencyKey: "worker-claim", RequestFingerprint: "worker-claim", State: models.DeliveryRunQueued}
	require.NoError(t, db.Create(run).Error)
	stage := &models.DeliveryRunStage{RunID: run.ID, EnvironmentID: environment.ID, EnvironmentKey: "prod", EnvironmentName: "Production", ApplicationID: app.ID, ClusterID: cluster.ID, Namespace: "default", ReleaseName: "worker", StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 60, State: models.DeliveryStageQueued, OperationID: "delivery-worker-claim"}
	require.NoError(t, db.Create(stage).Error)

	executor := &claimOnceExecutor{started: make(chan struct{}), release: make(chan struct{}), result: orchestrator.UpgradeResult{Revision: 2, Digest: digest}}
	cfg := orchestrator.Config{Concurrency: 1, LeaseDuration: time.Minute, RenewInterval: 10 * time.Second}
	workers := []*orchestrator.Worker{orchestrator.NewWorker(db, executor, cfg, "worker-a"), orchestrator.NewWorker(db, executor, cfg, "worker-b")}
	errs := make(chan error, 2)
	go func() { errs <- workers[0].ProcessOnce(context.Background()) }()
	<-executor.started
	go func() { errs <- workers[1].ProcessOnce(context.Background()) }()
	time.Sleep(50 * time.Millisecond)
	close(executor.release)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Equal(t, 1, executor.count())
	require.NoError(t, db.First(stage, stage.ID).Error)
	require.Equal(t, models.DeliveryStageSucceeded, stage.State)
	require.NoError(t, db.First(run, run.ID).Error)
	require.Equal(t, models.DeliveryRunSucceeded, run.State)
	var events []models.DeliveryRunEvent
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("id ASC").Find(&events).Error)
	require.Equal(t, []string{"run.running", "stage.running", "stage.succeeded", "run.succeeded"}, eventTypes(events))
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(events[2].Metadata, &metadata))
	require.Equal(t, digest, metadata["chart_digest"])
	require.NotContains(t, metadata, "digest")

	t.Run("advances to approval atomically", func(t *testing.T) {
		run, _, next, digest := seedWorkerRun(t, db, "approval", true)
		executor := &claimOnceExecutor{started: make(chan struct{}), release: closedChannel(), result: orchestrator.UpgradeResult{Revision: 3, Digest: digest}}
		require.NoError(t, orchestrator.NewWorker(db, executor, cfg, "worker-approval").ProcessOnce(context.Background()))
		require.NoError(t, db.First(run, run.ID).Error)
		require.Equal(t, models.DeliveryRunWaitingApproval, run.State)
		require.NoError(t, db.First(next, next.ID).Error)
		require.Equal(t, models.DeliveryStageWaitingApproval, next.State)
		var approval models.DeliveryApproval
		require.NoError(t, db.Where("run_stage_id = ?", next.ID).First(&approval).Error)
		require.Equal(t, models.DeliveryApprovalPending, approval.Decision)
	})

	t.Run("queues next stage without approval", func(t *testing.T) {
		run, _, next, digest := seedWorkerRun(t, db, "queued", false)
		executor := &claimOnceExecutor{started: make(chan struct{}), release: closedChannel(), result: orchestrator.UpgradeResult{Revision: 4, Digest: digest}}
		require.NoError(t, orchestrator.NewWorker(db, executor, cfg, "worker-queued").ProcessOnce(context.Background()))
		require.NoError(t, db.First(run, run.ID).Error)
		require.Equal(t, models.DeliveryRunRunning, run.State)
		require.NoError(t, db.First(next, next.ID).Error)
		require.Equal(t, models.DeliveryStageQueued, next.State)
		var approvals int64
		require.NoError(t, db.Model(&models.DeliveryApproval{}).Where("run_stage_id = ?", next.ID).Count(&approvals).Error)
		require.Zero(t, approvals)
		// Keep later subtests isolated from this intentionally queued stage.
		require.NoError(t, db.Model(&models.DeliveryRunStage{}).Where("id = ?", next.ID).Update("state", models.DeliveryStageCanceled).Error)
		require.NoError(t, db.Model(&models.DeliveryRun{}).Where("id = ?", run.ID).Update("state", models.DeliveryRunCanceled).Error)
	})

	t.Run("definite failure stops run safely", func(t *testing.T) {
		run, stage, next, _ := seedWorkerRun(t, db, "failed", false)
		executor := &claimOnceExecutor{started: make(chan struct{}), release: closedChannel(), err: apperr.New(apperr.CodeDeliveryArtifactDrift, "delivery.execution.artifact_drift", "upstream raw output")}
		require.NoError(t, orchestrator.NewWorker(db, executor, cfg, "worker-failed").ProcessOnce(context.Background()))
		require.NoError(t, db.First(run, run.ID).Error)
		require.Equal(t, models.DeliveryRunFailed, run.State)
		require.NoError(t, db.First(stage, stage.ID).Error)
		require.Equal(t, models.DeliveryStageFailed, stage.State)
		require.NoError(t, db.First(next, next.ID).Error)
		require.Equal(t, models.DeliveryStagePending, next.State)
		var rawLeaks int64
		require.NoError(t, db.Model(&models.DeliveryRunEvent{}).Where("run_id = ? AND metadata::text LIKE ?", run.ID, "%upstream raw output%").Count(&rawLeaks).Error)
		require.Zero(t, rawLeaks)
	})

	t.Run("cancel requested wins completion race", func(t *testing.T) {
		run, stage, _, digest := seedWorkerRun(t, db, "cancel", false)
		executor := &claimOnceExecutor{started: make(chan struct{}), release: make(chan struct{}), result: orchestrator.UpgradeResult{Revision: 5, Digest: digest}}
		done := make(chan error, 1)
		go func() {
			done <- orchestrator.NewWorker(db, executor, cfg, "worker-cancel").ProcessOnce(context.Background())
		}()
		<-executor.started
		require.NoError(t, db.Model(&models.DeliveryRun{}).Where("id = ? AND state = ?", run.ID, models.DeliveryRunRunning).Update("state", models.DeliveryRunCancelRequested).Error)
		close(executor.release)
		require.NoError(t, <-done)
		require.NoError(t, db.First(run, run.ID).Error)
		require.Equal(t, models.DeliveryRunReconciling, run.State)
		require.NoError(t, db.First(stage, stage.ID).Error)
		require.Equal(t, models.DeliveryStageReconciling, stage.State)
	})

	t.Run("cancel requested also overrides definite failure", func(t *testing.T) {
		run, stage, _, _ := seedWorkerRun(t, db, "cancel-error", false)
		executor := &claimOnceExecutor{started: make(chan struct{}), release: make(chan struct{}), err: apperr.New(apperr.CodeDeliveryArtifactDrift, "delivery.execution.artifact_drift", "raw failure")}
		done := make(chan error, 1)
		go func() {
			done <- orchestrator.NewWorker(db, executor, cfg, "worker-cancel-error").ProcessOnce(context.Background())
		}()
		<-executor.started
		require.NoError(t, db.Model(&models.DeliveryRun{}).Where("id = ? AND state = ?", run.ID, models.DeliveryRunRunning).Update("state", models.DeliveryRunCancelRequested).Error)
		close(executor.release)
		require.NoError(t, <-done)
		require.NoError(t, db.First(run, run.ID).Error)
		require.Equal(t, models.DeliveryRunReconciling, run.State)
		require.NoError(t, db.First(stage, stage.ID).Error)
		require.Equal(t, models.DeliveryStageReconciling, stage.State)
	})
}

type claimOnceExecutor struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
	result  orchestrator.UpgradeResult
	err     error
}

func (e *claimOnceExecutor) UpgradeExisting(ctx context.Context, _ orchestrator.UpgradeRequest) (orchestrator.UpgradeResult, error) {
	e.mu.Lock()
	e.calls++
	if e.calls == 1 {
		close(e.started)
	}
	e.mu.Unlock()
	select {
	case <-e.release:
		return e.result, e.err
	case <-ctx.Done():
		return orchestrator.UpgradeResult{}, ctx.Err()
	}
}

func (e *claimOnceExecutor) count() int { e.mu.Lock(); defer e.mu.Unlock(); return e.calls }

func eventTypes(rows []models.DeliveryRunEvent) []string {
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].EventType
	}
	return out
}

func closedChannel() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func seedWorkerRun(t *testing.T, db *gorm.DB, suffix string, approvalRequired bool) (*models.DeliveryRun, *models.DeliveryRunStage, *models.DeliveryRunStage, string) {
	t.Helper()
	initiator := dbtest.SeedUser(t, db, "delivery-worker-"+suffix)
	cluster := dbtest.SeedCluster(t, db, "dw-"+suffix)
	repo := &models.AppsChartRepo{Name: "dw-repo-" + suffix, Type: "http", URL: "https://" + suffix + ".example.test"}
	require.NoError(t, db.Create(repo).Error)
	app := &models.AppsApplication{Name: "dw-app-" + suffix, ClusterID: cluster.ID, Namespace: "default", ReleaseName: "dw-" + suffix, ChartRepoID: repo.ID, ChartName: "worker"}
	require.NoError(t, db.Create(app).Error)
	nextApp := &models.AppsApplication{Name: "dw-app-next-" + suffix, ClusterID: cluster.ID, Namespace: "default", ReleaseName: "dw-next-" + suffix, ChartRepoID: repo.ID, ChartName: "worker"}
	require.NoError(t, db.Create(nextApp).Error)
	project := &models.DeliveryProject{Name: "dw-project-" + suffix}
	require.NoError(t, db.Create(project).Error)
	env1 := &models.DeliveryEnvironment{ProjectID: project.ID, EnvironmentKey: "first", DisplayName: "First", ApplicationID: app.ID}
	require.NoError(t, db.Create(env1).Error)
	env2 := &models.DeliveryEnvironment{ProjectID: project.ID, EnvironmentKey: "second", DisplayName: "Second", ApplicationID: nextApp.ID}
	require.NoError(t, db.Create(env2).Error)
	pipeline := &models.DeliveryPipeline{ProjectID: project.ID, Version: 1, CreatedByUserID: initiator.ID, PublishedAt: time.Now().UTC(), IsCurrent: true}
	require.NoError(t, db.Create(pipeline).Error)
	digest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	run := &models.DeliveryRun{ProjectID: project.ID, PipelineID: pipeline.ID, PipelineVersion: 1, ChartRepoID: repo.ID, ChartName: "worker", ChartVersion: "1.0.0", ChartDigest: digest, InitiatorUserID: initiator.ID, IdempotencyKey: "dw-" + suffix, RequestFingerprint: "dw-" + suffix, State: models.DeliveryRunQueued}
	require.NoError(t, db.Create(run).Error)
	stage := &models.DeliveryRunStage{RunID: run.ID, EnvironmentID: env1.ID, EnvironmentKey: "first", EnvironmentName: "First", ApplicationID: app.ID, ClusterID: cluster.ID, Namespace: "default", ReleaseName: app.ReleaseName, StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 60, State: models.DeliveryStageQueued, OperationID: "dw-" + suffix + "-1"}
	next := &models.DeliveryRunStage{RunID: run.ID, EnvironmentID: env2.ID, EnvironmentKey: "second", EnvironmentName: "Second", ApplicationID: nextApp.ID, ClusterID: cluster.ID, Namespace: "default", ReleaseName: nextApp.ReleaseName, StageOrder: 2, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 60, State: models.DeliveryStagePending, ApprovalRequired: approvalRequired, OperationID: "dw-" + suffix + "-2"}
	require.NoError(t, db.Create(stage).Error)
	require.NoError(t, db.Create(next).Error)
	return run, stage, next, digest
}
