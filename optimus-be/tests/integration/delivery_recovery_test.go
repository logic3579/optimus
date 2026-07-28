//go:build dbtest

package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
	"optimus-be/internal/modules/delivery/orchestrator"
	deliveryrun "optimus-be/internal/modules/delivery/run"
	"optimus-be/tests/dbtest"
)

func TestDeliveryReconciliationPersistsAndAdvancesAtomically(t *testing.T) {
	_, db := setupServer(t)
	run, stage, next := seedReconcilingRun(t, db, "advance", true)
	inspector := &recoveryInspector{evidence: orchestrator.Inspection{Revision: 8, Digest: run.ChartDigest}}
	require.NoError(t, orchestrator.NewReconciler(db, inspector).Reconcile(context.Background(), run.ID))

	require.NoError(t, db.First(run, run.ID).Error)
	require.Equal(t, models.DeliveryRunWaitingApproval, run.State)
	require.NoError(t, db.First(stage, stage.ID).Error)
	require.Equal(t, models.DeliveryStageSucceeded, stage.State)
	require.Equal(t, int64(8), *stage.ResultRevision)
	require.Equal(t, run.ChartDigest, *stage.ResultDigest)
	require.NoError(t, db.First(next, next.ID).Error)
	require.Equal(t, models.DeliveryStageWaitingApproval, next.State)
	var approvals []models.DeliveryApproval
	require.NoError(t, db.Where("run_stage_id = ?", next.ID).Find(&approvals).Error)
	require.Len(t, approvals, 1)
	require.Equal(t, models.DeliveryApprovalPending, approvals[0].Decision)

	var events []models.DeliveryRunEvent
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("id ASC").Find(&events).Error)
	require.Equal(t, []string{"stage.reconciling", "stage.succeeded", "stage.waiting_approval", "run.waiting_approval"}, eventTypes(events))
}

func TestDeliveryReconciliationRollsBackWhenRenewedApprovalCannotPersist(t *testing.T) {
	_, db := setupServer(t)
	run, stage, next := seedReconcilingRun(t, db, "approval-rollback", true)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.DeliveryApproval{
		RunID: run.ID, RunStageID: next.ID, RequestedAt: now,
		Decision: models.DeliveryApprovalPending, CreatedAt: now, UpdatedAt: now,
	}).Error)

	err := orchestrator.NewReconciler(db, &recoveryInspector{
		evidence: orchestrator.Inspection{Revision: 8, Digest: run.ChartDigest},
	}).Reconcile(context.Background(), run.ID)
	require.Error(t, err)

	require.NoError(t, db.First(run, run.ID).Error)
	require.Equal(t, models.DeliveryRunReconciling, run.State)
	require.NoError(t, db.First(stage, stage.ID).Error)
	require.Equal(t, models.DeliveryStageReconciling, stage.State)
	require.Nil(t, stage.ResultRevision)
	require.NoError(t, db.First(next, next.ID).Error)
	require.Equal(t, models.DeliveryStagePending, next.State)
	var succeededEvents int64
	require.NoError(t, db.Model(&models.DeliveryRunEvent{}).Where("run_id = ? AND event_type = ?", run.ID, "stage.succeeded").Count(&succeededEvents).Error)
	require.Zero(t, succeededEvents)
}

func TestDeliveryReconciliationDiscardsPartialEvidenceAndPersistsAfterCancellation(t *testing.T) {
	_, db := setupServer(t)
	run, stage, _ := seedReconcilingRun(t, db, "canceled-inspect", false)
	ctx, cancel := context.WithCancel(context.Background())
	inspector := &recoveryInspector{
		evidence: orchestrator.Inspection{Revision: 99, Digest: run.ChartDigest, PreviousDigestProven: true},
		err:      errors.New("unsafe upstream detail"),
		entered:  make(chan struct{}),
		cancel:   cancel,
	}
	require.NoError(t, orchestrator.NewReconciler(db, inspector).Reconcile(ctx, run.ID))

	require.NoError(t, db.First(run, run.ID).Error)
	require.Equal(t, models.DeliveryRunOutcomeUnknown, run.State)
	require.Equal(t, int(errs.CodeOutcomeUnknown), *run.ErrorCode)
	require.Equal(t, errs.KeyOutcomeUnknown, *run.ErrorMessageKey)
	require.NoError(t, db.First(stage, stage.ID).Error)
	require.Equal(t, models.DeliveryStageOutcomeUnknown, stage.State)
	require.Nil(t, stage.ResultRevision)
	require.Nil(t, stage.ResultDigest)
	require.Equal(t, int(errs.CodeOutcomeUnknown), *stage.ErrorCode)
	var leaked int64
	require.NoError(t, db.Model(&models.DeliveryRunEvent{}).Where("run_id = ? AND metadata::text LIKE ?", run.ID, "%unsafe upstream detail%").Count(&leaked).Error)
	require.Zero(t, leaked)
}

func TestDeliveryReconciliationRejectsStaleGeneration(t *testing.T) {
	_, db := setupServer(t)
	run, stage, _ := seedReconcilingRun(t, db, "stale", false)
	entered, release := make(chan struct{}), make(chan struct{})
	inspector := &recoveryInspector{evidence: orchestrator.Inspection{Revision: 9, Digest: run.ChartDigest}, entered: entered, release: release}
	done := make(chan error, 1)
	go func() { done <- orchestrator.NewReconciler(db, inspector).Reconcile(context.Background(), run.ID) }()
	<-entered

	now := time.Now().UTC()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var lockedRun models.DeliveryRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRun, run.ID).Error; err != nil {
			return err
		}
		var lockedStage models.DeliveryRunStage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedStage, stage.ID).Error; err != nil {
			return err
		}
		if err := tx.Model(&lockedRun).Updates(map[string]any{"state": models.DeliveryRunOutcomeUnknown, "finished_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&lockedStage).Updates(map[string]any{"state": models.DeliveryStageOutcomeUnknown, "finished_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&lockedRun).Updates(map[string]any{"state": models.DeliveryRunReconciling, "finished_at": nil}).Error; err != nil {
			return err
		}
		if err := tx.Model(&lockedStage).Updates(map[string]any{"state": models.DeliveryStageReconciling, "finished_at": nil}).Error; err != nil {
			return err
		}
		return tx.Create(&models.DeliveryRunEvent{RunID: run.ID, RunStageID: &stage.ID, EventType: "stage.reconciling", ActorType: models.DeliveryEventActorUser, OccurredAt: now, Metadata: datatypes.JSON(`{}`)}).Error
	}))
	close(release)
	require.NoError(t, <-done)

	require.NoError(t, db.First(run, run.ID).Error)
	require.Equal(t, models.DeliveryRunReconciling, run.State)
	require.NoError(t, db.First(stage, stage.ID).Error)
	require.Equal(t, models.DeliveryStageReconciling, stage.State)
	require.Nil(t, stage.ResultRevision)
	require.Nil(t, stage.ResultDigest)
}

func TestDeliveryRetryIsConcurrentAndIdempotent(t *testing.T) {
	_, db := setupServer(t)
	origin, _, next := seedRetryOrigin(t, db, "concurrent")
	svc := deliveryrun.NewService(deliveryrun.NewRepo(db), recoveryApplicationReader{}, recoveryArtifactResolver{}, nil)
	results := make(chan *deliveryrun.Run, 2)
	errsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.Retry(context.Background(), origin.InitiatorUserID, "", "", origin.ID, "same-retry")
			results <- result
			errsCh <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errsCh)
	for err := range errsCh {
		require.NoError(t, err)
	}
	var id uint64
	for result := range results {
		require.NotNil(t, result)
		if id == 0 {
			id = result.ID
		}
		require.Equal(t, id, result.ID)
		require.Equal(t, origin.ChartDigest, result.ChartDigest)
		require.Equal(t, origin.ID, *result.RetryOfRunID)
		require.Len(t, result.Stages, 1)
		require.Equal(t, next.EnvironmentKey, result.Stages[0].EnvironmentKey)
		require.Equal(t, models.DeliveryStageWaitingApproval, result.Stages[0].State)
	}
	var linked int64
	require.NoError(t, db.Model(&models.DeliveryRun{}).Where("retry_of_run_id = ?", origin.ID).Count(&linked).Error)
	require.Equal(t, int64(1), linked)
	var approvals int64
	require.NoError(t, db.Model(&models.DeliveryApproval{}).Where("run_id = ?", id).Count(&approvals).Error)
	require.Equal(t, int64(1), approvals)
	require.NoError(t, db.First(origin, origin.ID).Error)
	require.Equal(t, models.DeliveryRunFailed, origin.State)
}

type recoveryInspector struct {
	evidence orchestrator.Inspection
	err      error
	entered  chan struct{}
	release  chan struct{}
	cancel   context.CancelFunc
}

func (i *recoveryInspector) Inspect(context.Context, uint64, string) (orchestrator.Inspection, error) {
	if i.entered != nil {
		close(i.entered)
	}
	if i.cancel != nil {
		i.cancel()
	}
	if i.release != nil {
		<-i.release
	}
	return i.evidence, i.err
}

type recoveryApplicationReader struct{}

func (recoveryApplicationReader) GetApplication(context.Context, uint64) (*deliveryrun.Application, error) {
	return nil, errors.New("retry must use snapshot")
}

type recoveryArtifactResolver struct{}

func (recoveryArtifactResolver) ResolveArtifact(context.Context, uint64, string, string) (*deliveryrun.Artifact, error) {
	return nil, errors.New("retry must use original digest")
}

func seedReconcilingRun(t *testing.T, db *gorm.DB, suffix string, nextApproval bool) (*models.DeliveryRun, *models.DeliveryRunStage, *models.DeliveryRunStage) {
	t.Helper()
	initiator := dbtest.SeedUser(t, db, "delivery-recovery-"+suffix)
	cluster := dbtest.SeedCluster(t, db, "dr-"+suffix)
	repo := &models.AppsChartRepo{Name: "dr-repo-" + suffix, Type: "http", URL: "https://" + suffix + ".recovery.test"}
	require.NoError(t, db.Create(repo).Error)
	app := &models.AppsApplication{Name: "dr-app-" + suffix, ClusterID: cluster.ID, Namespace: "default", ReleaseName: "dr-" + suffix, ChartRepoID: repo.ID, ChartName: "demo"}
	nextApp := &models.AppsApplication{Name: "dr-next-" + suffix, ClusterID: cluster.ID, Namespace: "default", ReleaseName: "dr-next-" + suffix, ChartRepoID: repo.ID, ChartName: "demo"}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(nextApp).Error)
	project := &models.DeliveryProject{Name: "dr-project-" + suffix}
	require.NoError(t, db.Create(project).Error)
	env := &models.DeliveryEnvironment{ProjectID: project.ID, EnvironmentKey: "first", DisplayName: "First", ApplicationID: app.ID}
	nextEnv := &models.DeliveryEnvironment{ProjectID: project.ID, EnvironmentKey: "second", DisplayName: "Second", ApplicationID: nextApp.ID}
	require.NoError(t, db.Create(env).Error)
	require.NoError(t, db.Create(nextEnv).Error)
	pipeline := &models.DeliveryPipeline{ProjectID: project.ID, Version: 1, CreatedByUserID: initiator.ID, PublishedAt: time.Now().UTC(), IsCurrent: true}
	require.NoError(t, db.Create(pipeline).Error)
	run := &models.DeliveryRun{ProjectID: project.ID, PipelineID: pipeline.ID, PipelineVersion: 1, ChartRepoID: repo.ID, ChartName: "demo", ChartVersion: "1.0.0", ChartDigest: "sha256:" + strings.Repeat("c", 64), InitiatorUserID: initiator.ID, IdempotencyKey: "dr-" + suffix, RequestFingerprint: "dr-" + suffix, State: models.DeliveryRunReconciling}
	require.NoError(t, db.Create(run).Error)
	stage := &models.DeliveryRunStage{RunID: run.ID, EnvironmentID: env.ID, EnvironmentKey: "first", EnvironmentName: "First", ApplicationID: app.ID, ClusterID: cluster.ID, Namespace: "default", ReleaseName: app.ReleaseName, StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 60, State: models.DeliveryStageReconciling, OperationID: "dr-" + suffix + "-1"}
	next := &models.DeliveryRunStage{RunID: run.ID, EnvironmentID: nextEnv.ID, EnvironmentKey: "second", EnvironmentName: "Second", ApplicationID: nextApp.ID, ClusterID: cluster.ID, Namespace: "default", ReleaseName: nextApp.ReleaseName, StageOrder: 2, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 60, State: models.DeliveryStagePending, ApprovalRequired: nextApproval, OperationID: "dr-" + suffix + "-2"}
	require.NoError(t, db.Create(stage).Error)
	require.NoError(t, db.Create(next).Error)
	require.NoError(t, db.Create(&models.DeliveryRunEvent{RunID: run.ID, RunStageID: &stage.ID, EventType: "stage.reconciling", ActorType: models.DeliveryEventActorSystem, OccurredAt: time.Now().UTC(), Metadata: datatypes.JSON(`{"recovery_intent":"failed"}`)}).Error)
	return run, stage, next
}

func seedRetryOrigin(t *testing.T, db *gorm.DB, suffix string) (*models.DeliveryRun, *models.DeliveryRunStage, *models.DeliveryRunStage) {
	t.Helper()
	run, first, next := seedReconcilingRun(t, db, "retry-"+suffix, true)
	finished := time.Now().UTC()
	require.NoError(t, db.Model(run).Updates(map[string]any{"state": models.DeliveryRunFailed, "finished_at": finished}).Error)
	require.NoError(t, db.Model(first).Updates(map[string]any{"state": models.DeliveryStageSucceeded, "finished_at": finished}).Error)
	require.NoError(t, db.Model(next).Updates(map[string]any{"state": models.DeliveryStageFailed, "finished_at": finished}).Error)
	run.State = models.DeliveryRunFailed
	first.State = models.DeliveryStageSucceeded
	next.State = models.DeliveryStageFailed
	return run, first, next
}
