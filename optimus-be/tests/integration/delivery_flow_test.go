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
	appsapplication "optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/delivery/approval"
	"optimus-be/internal/modules/delivery/orchestrator"
	"optimus-be/internal/modules/delivery/pipeline"
	"optimus-be/internal/modules/delivery/project"
	deliveryrun "optimus-be/internal/modules/delivery/run"
	"optimus-be/tests/dbtest"
)

func TestDeliveryFlowPromotesImmutableArtifactWithApproval(t *testing.T) {
	_, db := setupServer(t)
	ctx := context.Background()
	initiator := dbtest.SeedUser(t, db, "delivery-flow-initiator")
	approver := dbtest.SeedUser(t, db, "delivery-flow-approver")
	cluster := dbtest.SeedCluster(t, db, "delivery-flow")
	chartRepo := &models.AppsChartRepo{Name: "delivery-flow-repo", Type: "http", URL: "https://flow.example.test"}
	require.NoError(t, db.Create(chartRepo).Error)

	projectApps := &flowProjectApplications{applications: make(map[uint64]project.Application)}
	runApps := &flowRunApplications{applications: make(map[uint64]deliveryrun.Application)}
	for i, environment := range []string{"dev", "staging", "prod"} {
		app := &models.AppsApplication{
			Name: "delivery-flow-" + environment, ClusterID: cluster.ID, Namespace: environment,
			ReleaseName: "flow-" + environment, ChartRepoID: chartRepo.ID, ChartName: "flow-chart",
		}
		require.NoError(t, db.Create(app).Error)
		projectApps.applications[app.ID] = project.Application{
			ID: app.ID, Name: app.Name, ChartRepoID: chartRepo.ID, ChartName: app.ChartName,
			Installed: true, ClusterID: cluster.ID, Namespace: app.Namespace, ReleaseName: app.ReleaseName,
		}
		runApps.applications[app.ID] = deliveryrun.Application{
			ID: app.ID, ChartRepoID: chartRepo.ID, ChartName: app.ChartName, Installed: true,
			ClusterID: cluster.ID, Namespace: app.Namespace, ReleaseName: app.ReleaseName,
		}
		_ = i
	}

	projectRepo := project.NewRepo(db)
	projectSvc := project.NewService(projectRepo, projectApps, nil, nil)
	createdProject, err := projectSvc.CreateProject(ctx, initiator.ID, "", "", project.CreateProjectRequest{Name: "Delivery Flow"})
	require.NoError(t, err)
	environments := make([]project.Environment, 0, 3)
	for _, key := range []string{"dev", "staging", "prod"} {
		var applicationID uint64
		for id, app := range projectApps.applications {
			if app.Namespace == key {
				applicationID = id
				break
			}
		}
		bound, bindErr := projectSvc.BindEnvironment(ctx, initiator.ID, "", "", createdProject.ID, project.BindEnvironmentRequest{
			EnvironmentKey: key, DisplayName: key, ApplicationID: applicationID,
		})
		require.NoError(t, bindErr)
		environments = append(environments, *bound)
	}

	pipelineSvc := pipeline.NewService(pipeline.NewRepo(db), nil, 30*time.Minute)
	published, err := pipelineSvc.Publish(ctx, initiator.ID, "", "", createdProject.ID, pipeline.PublishRequest{Stages: []pipeline.StageInput{
		{EnvironmentID: environments[0].ID, Timeout: time.Minute},
		{EnvironmentID: environments[1].ID, ApprovalRequired: true, Timeout: time.Minute},
		{EnvironmentID: environments[2].ID, Timeout: time.Minute},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, published.Version)

	digest := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	artifacts := &flowArtifactResolver{artifact: deliveryrun.Artifact{RepoID: chartRepo.ID, ChartName: "flow-chart", Version: "1.2.3", Digest: digest}}
	runSvc := deliveryrun.NewService(deliveryrun.NewRepo(db), runApps, artifacts, nil)
	request := deliveryrun.CreateRequest{ChartRepoID: chartRepo.ID, ChartName: "flow-chart", ChartVersion: "1.2.3"}
	created := make(chan *deliveryrun.Run, 2)
	createErrors := make(chan error, 2)
	var createWG sync.WaitGroup
	for range 2 {
		createWG.Add(1)
		go func() {
			defer createWG.Done()
			result, createErr := runSvc.Create(ctx, initiator.ID, "", "", createdProject.ID, "delivery-flow", request)
			created <- result
			createErrors <- createErr
		}()
	}
	createWG.Wait()
	close(created)
	close(createErrors)
	for createErr := range createErrors {
		require.NoError(t, createErr)
	}
	var run *deliveryrun.Run
	for result := range created {
		require.NotNil(t, result)
		if run == nil {
			run = result
		}
		require.Equal(t, run.ID, result.ID)
	}
	require.Equal(t, 2, artifacts.calls)
	require.Len(t, run.Stages, 3)
	require.Equal(t, models.DeliveryStageQueued, run.Stages[0].State)
	require.Equal(t, models.DeliveryStagePending, run.Stages[1].State)

	executor := &flowExecutor{digest: digest}
	workerConfig := orchestrator.Config{Concurrency: 1, LeaseDuration: time.Minute, RenewInterval: 10 * time.Second}
	workerErrors := make(chan error, 2)
	for _, owner := range []string{"flow-worker-a", "flow-worker-b"} {
		go func(owner string) {
			workerErrors <- orchestrator.NewWorker(db, executor, workerConfig, owner).ProcessOnce(ctx)
		}(owner)
	}
	require.NoError(t, <-workerErrors)
	require.NoError(t, <-workerErrors)
	require.Len(t, executor.requests(), 1)

	var stages []models.DeliveryRunStage
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("stage_order ASC").Find(&stages).Error)
	require.Equal(t, models.DeliveryStageSucceeded, stages[0].State)
	require.Equal(t, models.DeliveryStageWaitingApproval, stages[1].State)
	var persistedRun models.DeliveryRun
	require.NoError(t, db.First(&persistedRun, run.ID).Error)
	require.Equal(t, models.DeliveryRunWaitingApproval, persistedRun.State)
	approvalSvc := approval.NewService(approval.NewRepo(db), allowApprovalPermission{}, nil)
	_, err = approvalSvc.Approve(ctx, initiator.ID, "", "", stages[1].ID, approval.DecisionRequest{Comment: "self approval must fail"})
	require.Error(t, err)
	var biz *apperr.BizError
	require.True(t, errors.As(err, &biz))
	require.Equal(t, apperr.CodeDeliveryApprovalSelfApproval, biz.Code)
	var pending models.DeliveryApproval
	require.NoError(t, db.Where("run_stage_id = ?", stages[1].ID).First(&pending).Error)
	require.Equal(t, models.DeliveryApprovalPending, pending.Decision)

	_, err = approvalSvc.Approve(ctx, approver.ID, "", "", stages[1].ID, approval.DecisionRequest{Comment: "promote immutable digest"})
	require.NoError(t, err)
	require.NoError(t, orchestrator.NewWorker(db, executor, workerConfig, "flow-worker").ProcessOnce(ctx))
	require.NoError(t, orchestrator.NewWorker(db, executor, workerConfig, "flow-worker").ProcessOnce(ctx))

	require.NoError(t, db.First(&persistedRun, run.ID).Error)
	require.Equal(t, models.DeliveryRunSucceeded, persistedRun.State)
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("stage_order ASC").Find(&stages).Error)
	for i := range stages {
		require.Equal(t, models.DeliveryStageSucceeded, stages[i].State)
		require.NotNil(t, stages[i].ResultRevision)
		require.Equal(t, int64(i+1), *stages[i].ResultRevision)
		require.NotNil(t, stages[i].ResultDigest)
		require.Equal(t, digest, *stages[i].ResultDigest)
	}
	require.Equal(t, []uint64{runApps.applicationIDs()[0], runApps.applicationIDs()[1], runApps.applicationIDs()[2]}, executor.applicationIDs())
	for _, call := range executor.requests() {
		require.Equal(t, digest, call.Digest)
		require.Equal(t, chartRepo.ID, call.RepoID)
		require.NotEmpty(t, call.OperationID)
		require.Contains(t, call.Purpose, "delivery.run.")
	}

	var events []models.DeliveryRunEvent
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("id ASC").Find(&events).Error)
	require.Equal(t, []string{
		"run.created", "stage.queued", "run.running", "stage.running", "stage.succeeded",
		"stage.waiting_approval", "run.waiting_approval", "stage.approved", "run.approved",
		"run.running", "stage.running", "stage.succeeded", "stage.queued", "stage.running",
		"stage.succeeded", "run.succeeded",
	}, eventTypes(events))
	for _, event := range events {
		require.NotContains(t, string(event.Metadata), "promote immutable digest")
		require.NotContains(t, string(event.Metadata), "self approval must fail")
	}

	appSvc := appsapplication.NewService(appsapplication.NewRepo(db), nil)
	appSvc.SetDeliveryApplicationCounter(projectRepo)
	err = appSvc.Delete(ctx, initiator.ID, "", "", environments[0].ApplicationID)
	require.Error(t, err)
	require.True(t, errors.As(err, &biz))
	require.Equal(t, apperr.CodeDeliveryEnvironmentInUse, biz.Code)
	var activeApp models.AppsApplication
	require.NoError(t, db.First(&activeApp, environments[0].ApplicationID).Error)
}

type flowProjectApplications struct {
	applications map[uint64]project.Application
}

func (f *flowProjectApplications) GetApplication(_ context.Context, id uint64) (*project.Application, error) {
	app, ok := f.applications[id]
	if !ok {
		return nil, errors.New("application unavailable")
	}
	return &app, nil
}

type flowRunApplications struct {
	applications map[uint64]deliveryrun.Application
}

func (f *flowRunApplications) GetApplication(_ context.Context, id uint64) (*deliveryrun.Application, error) {
	app, ok := f.applications[id]
	if !ok {
		return nil, errors.New("application unavailable")
	}
	return &app, nil
}

func (f *flowRunApplications) applicationIDs() []uint64 {
	ids := make([]uint64, 0, len(f.applications))
	for id := range f.applications {
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	return ids
}

type flowArtifactResolver struct {
	mu       sync.Mutex
	calls    int
	artifact deliveryrun.Artifact
}

func (f *flowArtifactResolver) ResolveArtifact(context.Context, uint64, string, string) (*deliveryrun.Artifact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	artifact := f.artifact
	return &artifact, nil
}

type flowExecutor struct {
	mu     sync.Mutex
	digest string
	calls  []orchestrator.UpgradeRequest
}

func (f *flowExecutor) UpgradeExisting(_ context.Context, req orchestrator.UpgradeRequest) (orchestrator.UpgradeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	return orchestrator.UpgradeResult{Revision: int64(len(f.calls)), Digest: f.digest}, nil
}

func (f *flowExecutor) requests() []orchestrator.UpgradeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]orchestrator.UpgradeRequest(nil), f.calls...)
}

func (f *flowExecutor) applicationIDs() []uint64 {
	calls := f.requests()
	ids := make([]uint64, len(calls))
	for i := range calls {
		ids[i] = calls[i].ApplicationID
	}
	return ids
}
