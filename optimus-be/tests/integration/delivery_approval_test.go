//go:build dbtest

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/approval"
	"optimus-be/tests/dbtest"
)

func TestDeliveryApprovalFirstDecisionWins(t *testing.T) {
	_, db := setupServer(t)
	initiator := dbtest.SeedUser(t, db, "delivery-approval-initiator")
	approverOne := dbtest.SeedUser(t, db, "delivery-approval-one")
	approverTwo := dbtest.SeedUser(t, db, "delivery-approval-two")
	cluster := dbtest.SeedCluster(t, db, "delivery-approval")
	chartRepo := &models.AppsChartRepo{Name: "delivery-approval-repo", Type: "http", URL: "https://approval.example.test"}
	require.NoError(t, db.Create(chartRepo).Error)
	application := &models.AppsApplication{Name: "delivery-approval-app", ClusterID: cluster.ID, Namespace: "default", ReleaseName: "approval", ChartRepoID: chartRepo.ID, ChartName: "approval"}
	require.NoError(t, db.Create(application).Error)
	project := &models.DeliveryProject{Name: "delivery-approval-project"}
	require.NoError(t, db.Create(project).Error)
	environment := &models.DeliveryEnvironment{ProjectID: project.ID, EnvironmentKey: "prod", DisplayName: "Production", ApplicationID: application.ID}
	require.NoError(t, db.Create(environment).Error)
	pipeline := &models.DeliveryPipeline{ProjectID: project.ID, Version: 1, CreatedByUserID: initiator.ID, PublishedAt: time.Now(), IsCurrent: true}
	require.NoError(t, db.Create(pipeline).Error)
	run := &models.DeliveryRun{ProjectID: project.ID, PipelineID: pipeline.ID, PipelineVersion: 1, ChartRepoID: chartRepo.ID, ChartName: "approval", ChartVersion: "1.0.0", ChartDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InitiatorUserID: initiator.ID, IdempotencyKey: "approval-race", RequestFingerprint: "approval-race", State: models.DeliveryRunWaitingApproval}
	require.NoError(t, db.Create(run).Error)
	stage := &models.DeliveryRunStage{RunID: run.ID, EnvironmentID: environment.ID, EnvironmentKey: "prod", EnvironmentName: "Production", ApplicationID: application.ID, ClusterID: cluster.ID, Namespace: "default", ReleaseName: "approval", StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, ApprovalRequired: true, TimeoutSeconds: 60, State: models.DeliveryStageWaitingApproval, OperationID: "delivery-approval-race"}
	require.NoError(t, db.Create(stage).Error)
	row := &models.DeliveryApproval{RunID: run.ID, RunStageID: stage.ID, RequestedAt: time.Now(), Decision: models.DeliveryApprovalPending}
	require.NoError(t, db.Create(row).Error)

	svc := approval.NewService(approval.NewRepo(db), allowApprovalPermission{}, audit.NewRecorder(db))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, command := range []struct {
		actor   uint64
		approve bool
	}{{approverOne.ID, true}, {approverTwo.ID, false}} {
		wg.Add(1)
		go func(actor uint64, approve bool) {
			defer wg.Done()
			<-start
			if approve {
				_, err := svc.Approve(context.Background(), actor, "", "", stage.ID, approval.DecisionRequest{Comment: "approve race"})
				results <- err
				return
			}
			_, err := svc.Reject(context.Background(), actor, "", "", stage.ID, approval.DecisionRequest{Comment: "reject race"})
			results <- err
		}(command.actor, command.approve)
	}
	close(start)
	wg.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var biz *apperr.BizError
		require.True(t, errors.As(err, &biz))
		require.Equal(t, apperr.CodeDeliveryApprovalDecisionConflict, biz.Code)
		conflicts++
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)

	require.NoError(t, db.First(row, row.ID).Error)
	require.NoError(t, db.First(stage, stage.ID).Error)
	require.NoError(t, db.First(run, run.ID).Error)
	if row.Decision == models.DeliveryApprovalApproved {
		require.Equal(t, models.DeliveryStageQueued, stage.State)
		require.Equal(t, models.DeliveryRunQueued, run.State)
	} else {
		require.Equal(t, models.DeliveryApprovalRejected, row.Decision)
		require.Equal(t, models.DeliveryStageRejected, stage.State)
		require.Equal(t, models.DeliveryRunRejected, run.State)
	}
	var events []models.DeliveryRunEvent
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)

	decision := string(row.Decision)
	stageEvent, runEvent := events[0], events[1]
	require.Equal(t, "stage."+decision, stageEvent.EventType)
	require.NotNil(t, stageEvent.RunStageID)
	require.Equal(t, stage.ID, *stageEvent.RunStageID)
	require.Equal(t, string(models.DeliveryStageWaitingApproval), *stageEvent.OldState)
	require.Equal(t, string(stage.State), *stageEvent.NewState)
	require.JSONEq(t, `{}`, string(stageEvent.Metadata))

	require.Equal(t, "run."+decision, runEvent.EventType)
	require.Nil(t, runEvent.RunStageID)
	require.Equal(t, string(models.DeliveryRunWaitingApproval), *runEvent.OldState)
	require.Equal(t, string(run.State), *runEvent.NewState)
	require.JSONEq(t, `{}`, string(runEvent.Metadata))
	for _, event := range events {
		require.NotContains(t, string(event.Metadata), "approve race")
		require.NotContains(t, string(event.Metadata), "reject race")
	}
}

type allowApprovalPermission struct{}

func (allowApprovalPermission) Has(context.Context, uint64, string) (bool, error) { return true, nil }
