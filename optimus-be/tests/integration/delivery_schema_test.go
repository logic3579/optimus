//go:build dbtest

package integration_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestDeliverySchemaConstraints(t *testing.T) {
	_, db := setupServer(t)

	user := dbtest.SeedUser(t, db, "delivery-schema")
	cluster := dbtest.SeedCluster(t, db, "delivery-schema")
	chartRepo := &models.AppsChartRepo{
		Name: "delivery-schema-repo",
		Type: "http",
		URL:  "https://charts.example.test",
	}
	require.NoError(t, db.Create(chartRepo).Error)
	application := &models.AppsApplication{
		Name:        "delivery-schema-app",
		ClusterID:   cluster.ID,
		Namespace:   "default",
		ReleaseName: "delivery-schema",
		ChartRepoID: chartRepo.ID,
		ChartName:   "delivery-chart",
	}
	require.NoError(t, db.Create(application).Error)
	secondApplication := &models.AppsApplication{
		Name:        "delivery-schema-second-app",
		ClusterID:   cluster.ID,
		Namespace:   "default",
		ReleaseName: "delivery-schema-second",
		ChartRepoID: chartRepo.ID,
		ChartName:   "delivery-chart",
	}
	require.NoError(t, db.Create(secondApplication).Error)

	project := &models.DeliveryProject{Name: "delivery-schema-project", OwnerUserID: &user.ID}
	require.NoError(t, db.Create(project).Error)
	environment := &models.DeliveryEnvironment{
		ProjectID:      project.ID,
		EnvironmentKey: "staging",
		DisplayName:    "Staging",
		ApplicationID:  application.ID,
	}
	require.NoError(t, db.Create(environment).Error)
	pipeline := &models.DeliveryPipeline{
		ProjectID:       project.ID,
		Version:         1,
		CreatedByUserID: user.ID,
		PublishedAt:     time.Now(),
		IsCurrent:       true,
	}
	require.NoError(t, db.Create(pipeline).Error)
	pipelineStage := &models.DeliveryPipelineStage{
		PipelineID:       pipeline.ID,
		EnvironmentID:    environment.ID,
		StageOrder:       1,
		Executor:         models.DeliveryExecutorHelmUpgradeExistingRelease,
		ApprovalRequired: true,
		TimeoutSeconds:   24 * 60 * 60,
	}
	require.NoError(t, db.Create(pipelineStage).Error)
	run := &models.DeliveryRun{
		ProjectID:          project.ID,
		PipelineID:         pipeline.ID,
		PipelineVersion:    pipeline.Version,
		ChartRepoID:        chartRepo.ID,
		ChartName:          application.ChartName,
		ChartVersion:       "1.2.3",
		ChartDigest:        "sha256:delivery-schema",
		InitiatorUserID:    user.ID,
		IdempotencyKey:     "delivery-schema-run",
		RequestFingerprint: "delivery-schema-fingerprint",
		State:              models.DeliveryRunQueued,
	}
	require.NoError(t, db.Create(run).Error)
	runStage := &models.DeliveryRunStage{
		RunID:            run.ID,
		EnvironmentID:    environment.ID,
		EnvironmentKey:   environment.EnvironmentKey,
		EnvironmentName:  environment.DisplayName,
		ApplicationID:    application.ID,
		ClusterID:        application.ClusterID,
		Namespace:        application.Namespace,
		ReleaseName:      application.ReleaseName,
		StageOrder:       1,
		Executor:         models.DeliveryExecutorHelmUpgradeExistingRelease,
		ApprovalRequired: true,
		TimeoutSeconds:   24 * 60 * 60,
		State:            models.DeliveryStagePending,
		OperationID:      "delivery-schema-operation",
	}
	require.NoError(t, db.Create(runStage).Error)
	approval := &models.DeliveryApproval{
		RunID:       run.ID,
		RunStageID:  runStage.ID,
		RequestedAt: time.Now(),
		Decision:    models.DeliveryApprovalPending,
	}
	require.NoError(t, db.Create(approval).Error)
	stageState := string(models.DeliveryStagePending)
	event := &models.DeliveryRunEvent{
		RunID:      run.ID,
		RunStageID: &runStage.ID,
		EventType:  "stage.created",
		NewState:   &stageState,
		ActorType:  "user",
		ActorID:    &user.ID,
	}
	require.NoError(t, db.Create(event).Error)

	duplicateBinding := &models.DeliveryEnvironment{
		ProjectID:      project.ID,
		EnvironmentKey: "production",
		DisplayName:    "Production",
		ApplicationID:  application.ID,
	}
	require.Error(t, db.Create(duplicateBinding).Error, "duplicate active application binding")
	require.NoError(t, db.Delete(environment).Error)
	replacementEnvironment := &models.DeliveryEnvironment{
		ProjectID:      project.ID,
		EnvironmentKey: "staging-replacement",
		DisplayName:    "Staging Replacement",
		ApplicationID:  application.ID,
	}
	require.NoError(t, db.Create(replacementEnvironment).Error, "soft-deleted application binding is reusable")

	duplicateRun := &models.DeliveryRun{
		ProjectID:          project.ID,
		PipelineID:         pipeline.ID,
		PipelineVersion:    pipeline.Version,
		ChartRepoID:        chartRepo.ID,
		ChartName:          application.ChartName,
		ChartVersion:       "1.2.3",
		ChartDigest:        "sha256:delivery-schema",
		InitiatorUserID:    user.ID,
		IdempotencyKey:     "delivery-schema-run-duplicate",
		RequestFingerprint: "delivery-schema-fingerprint-duplicate",
		State:              models.DeliveryRunQueued,
	}
	require.Error(t, db.Create(duplicateRun).Error, "duplicate active project run")
	require.NoError(t, db.Model(run).Update("state", models.DeliveryRunSucceeded).Error)
	replacementRun := &models.DeliveryRun{
		ProjectID:          project.ID,
		PipelineID:         pipeline.ID,
		PipelineVersion:    pipeline.Version,
		ChartRepoID:        chartRepo.ID,
		ChartName:          application.ChartName,
		ChartVersion:       "1.2.3",
		ChartDigest:        "sha256:delivery-schema",
		InitiatorUserID:    user.ID,
		IdempotencyKey:     "delivery-schema-run-replacement",
		RequestFingerprint: "delivery-schema-fingerprint-replacement",
		State:              models.DeliveryRunQueued,
	}
	require.NoError(t, db.Create(replacementRun).Error, "terminal project run permits a new active run")
	replacementRunStage := &models.DeliveryRunStage{
		RunID:            replacementRun.ID,
		EnvironmentID:    replacementEnvironment.ID,
		EnvironmentKey:   replacementEnvironment.EnvironmentKey,
		EnvironmentName:  replacementEnvironment.DisplayName,
		ApplicationID:    application.ID,
		ClusterID:        application.ClusterID,
		Namespace:        application.Namespace,
		ReleaseName:      application.ReleaseName,
		StageOrder:       1,
		Executor:         models.DeliveryExecutorHelmUpgradeExistingRelease,
		ApprovalRequired: true,
		TimeoutSeconds:   60,
		State:            models.DeliveryStagePending,
		OperationID:      "delivery-schema-replacement-operation",
	}
	require.NoError(t, db.Create(replacementRunStage).Error)
	require.Error(t, db.Create(&models.DeliveryApproval{
		RunID:       run.ID,
		RunStageID:  replacementRunStage.ID,
		RequestedAt: time.Now(),
		Decision:    models.DeliveryApprovalPending,
	}).Error, "approval stage must belong to its run")
	require.Error(t, db.Create(&models.DeliveryRunEvent{
		RunID:      run.ID,
		RunStageID: &replacementRunStage.ID,
		EventType:  "stage.mismatched",
		ActorType:  models.DeliveryEventActorSystem,
	}).Error, "event stage must belong to its run")

	operation := &models.AppsReleaseOperation{
		ApplicationID: application.ID,
		OperationID:   "delivery-schema-operation-id",
		Kind:          "upgrade",
		State:         models.AppsReleaseOperationActive,
	}
	require.NoError(t, db.Create(operation).Error)
	require.Error(t, db.Create(&models.AppsReleaseOperation{
		ApplicationID: secondApplication.ID,
		OperationID:   operation.OperationID,
		Kind:          "upgrade",
		State:         models.AppsReleaseOperationActive,
	}).Error, "operation ID must be globally unique")
}
