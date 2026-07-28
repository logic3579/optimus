package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/errs"
)

func TestCanonicalFingerprintUsesOnlyImmutableRunIdentity(t *testing.T) {
	retry := uint64(41)
	artifact := Artifact{RepoID: 9, ChartName: "demo", Version: "1.2.3", Digest: "sha256:aaaa"}
	got, err := canonicalFingerprint(7, 3, artifact, &retry)
	require.NoError(t, err)

	canonical, err := json.Marshal(struct {
		ProjectID       uint64  `json:"project_id"`
		PipelineVersion int     `json:"pipeline_version"`
		ChartRepoID     uint64  `json:"chart_repo_id"`
		ChartName       string  `json:"chart_name"`
		ChartVersion    string  `json:"chart_version"`
		ChartDigest     string  `json:"chart_digest"`
		RetryOfRunID    *uint64 `json:"retry_of_run_id"`
	}{7, 3, 9, "demo", "1.2.3", "sha256:aaaa", &retry})
	require.NoError(t, err)
	wantSum := sha256.Sum256(canonical)
	require.Equal(t, hex.EncodeToString(wantSum[:]), got)

	otherRetry := uint64(42)
	changed, err := canonicalFingerprint(7, 3, artifact, &otherRetry)
	require.NoError(t, err)
	require.NotEqual(t, got, changed)
}

func TestRunPackageOwnsArtifactSeam(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ImportsOnly)
	require.NoError(t, err)
	for _, parsed := range packages {
		for filename, file := range parsed.Files {
			for _, imported := range file.Imports {
				require.NotEqual(t, `"optimus-be/internal/modules/apps/repo"`, imported.Path.Value, filename)
			}
		}
	}
	var _ ArtifactResolver = (*artifactResolver)(nil)
}

func TestCreateSnapshotsImmutablePipelineAndStartsWithApproval(t *testing.T) {
	svc, repo, apps, resolver, audits := newCreateFixture(t, true)

	created, err := svc.Create(context.Background(), 23, "127.0.0.1", "test", 7, "create-1", CreateRequest{
		ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3",
	})
	require.NoError(t, err)
	require.Equal(t, models.DeliveryRunWaitingApproval, created.State)
	require.Equal(t, 3, created.PipelineVersion)
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), created.ChartDigest)
	require.Len(t, created.Stages, 2)
	require.Equal(t, models.DeliveryStageWaitingApproval, created.Stages[0].State)
	require.Equal(t, models.DeliveryStagePending, created.Stages[1].State)
	require.Equal(t, "development", created.Stages[0].EnvironmentKey)
	require.Equal(t, "Development", created.Stages[0].EnvironmentName)
	require.Equal(t, uint64(101), created.Stages[0].ApplicationID)
	require.Equal(t, uint64(201), created.Stages[0].ClusterID)
	require.Equal(t, "dev", created.Stages[0].Namespace)
	require.Equal(t, "demo-dev", created.Stages[0].ReleaseName)
	require.Equal(t, "delivery-run-1-stage-1", created.Stages[0].OperationID)
	require.Equal(t, 10*time.Minute, created.Stages[0].Timeout)
	require.Len(t, repo.approvals, 1)
	require.Equal(t, created.Stages[0].ID, repo.approvals[0].RunStageID)
	require.Equal(t, models.DeliveryApprovalPending, repo.approvals[0].Decision)
	require.Len(t, repo.events, 2)
	require.Equal(t, "run.created", repo.events[0].EventType)
	require.Equal(t, "stage.waiting_approval", repo.events[1].EventType)
	require.Equal(t, []string{"lock_project", "activity", "lock_application:101", "lock_application:102", "create_run"}, repo.calls[:5])
	require.Equal(t, 2, apps.calls)
	require.Equal(t, 1, resolver.calls)
	require.Len(t, audits.events, 1)
	require.Equal(t, "delivery.run.create", audits.events[0].Action)

	// Later source mutations cannot alter the stored run snapshot.
	repo.environments[0].DisplayName = "Changed"
	apps.rows[101].Namespace = "changed"
	require.Equal(t, "Development", created.Stages[0].EnvironmentName)
	require.Equal(t, "dev", created.Stages[0].Namespace)
}

func TestCreateQueuesApprovalFreeFirstStage(t *testing.T) {
	svc, repo, _, _, _ := newCreateFixture(t, false)
	created, err := svc.Create(context.Background(), 23, "", "", 7, "create-1", CreateRequest{
		ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3",
	})
	require.NoError(t, err)
	require.Equal(t, models.DeliveryRunQueued, created.State)
	require.Equal(t, models.DeliveryStageQueued, created.Stages[0].State)
	require.Empty(t, repo.approvals)
	require.Equal(t, "stage.queued", repo.events[1].EventType)
}

func TestCreateRejectsMissingPipelineAndBlockingRuns(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*memoryRepository)
		wantCode apperr.Code
	}{
		{name: "missing pipeline", prepare: func(r *memoryRepository) { r.pipeline = nil }, wantCode: errs.CodePipelineMissing},
		{name: "active run", prepare: func(r *memoryRepository) { r.blocking = &models.DeliveryRun{State: models.DeliveryRunRunning} }, wantCode: errs.CodeActiveRun},
		{name: "unknown outcome", prepare: func(r *memoryRepository) { r.blocking = &models.DeliveryRun{State: models.DeliveryRunOutcomeUnknown} }, wantCode: errs.CodeOutcomeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _, _, _ := newCreateFixture(t, false)
			tt.prepare(repo)
			_, err := svc.Create(context.Background(), 23, "", "", 7, "key", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
			requireBizCode(t, err, tt.wantCode)
			require.Empty(t, repo.runs)
		})
	}
}

func TestCreateRejectsArtifactIdentityMismatchAcrossEnvironments(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*applicationReader)
		code   apperr.Code
	}{
		{name: "repository", mutate: func(a *applicationReader) { a.rows[102].ChartRepoID = 10 }, code: errs.CodeChartIdentityMismatch},
		{name: "chart", mutate: func(a *applicationReader) { a.rows[102].ChartName = "other" }, code: errs.CodeChartIdentityMismatch},
		{name: "not installed", mutate: func(a *applicationReader) { a.rows[102].Installed = false }, code: errs.CodeApplicationUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, apps, _, _ := newCreateFixture(t, false)
			tt.mutate(apps)
			_, err := svc.Create(context.Background(), 23, "", "", 7, "key", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
			requireBizCode(t, err, tt.code)
			require.Empty(t, repo.runs)
		})
	}
}

func TestCreateIdempotentReplayAndFingerprintConflict(t *testing.T) {
	svc, repo, _, resolver, audits := newCreateFixture(t, false)
	req := CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"}
	first, err := svc.Create(context.Background(), 23, "", "", 7, "same-key", req)
	require.NoError(t, err)
	replay, err := svc.Create(context.Background(), 23, "", "", 7, "same-key", req)
	require.NoError(t, err)
	require.Equal(t, first, replay)
	require.Len(t, repo.runs, 1)
	require.Len(t, audits.events, 1, "an idempotent replay is not a second mutation")

	resolver.artifact.Version = "1.2.4"
	resolver.artifact.Digest = "sha256:" + strings.Repeat("b", 64)
	_, err = svc.Create(context.Background(), 23, "", "", 7, "same-key", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.4"})
	requireBizCode(t, err, errs.CodeIdempotencyConflict)
}

func TestCreateValidatesIdempotencyBeforeExternalResolution(t *testing.T) {
	svc, _, _, resolver, _ := newCreateFixture(t, false)
	_, err := svc.Create(context.Background(), 23, "", "", 7, " ", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
	requireBizCode(t, err, errs.CodeIdempotencyMissing)
	require.Zero(t, resolver.calls)
}

func TestCreateRollsBackEveryPersistedSnapshotOnFailure(t *testing.T) {
	tests := []struct {
		name     string
		approval bool
		failAt   string
	}{
		{name: "approval create run", approval: true, failAt: "create_run"},
		{name: "approval create stages", approval: true, failAt: "create_stages"},
		{name: "approval create approval", approval: true, failAt: "create_approval"},
		{name: "approval append events", approval: true, failAt: "append_events"},
		{name: "queued create run", approval: false, failAt: "create_run"},
		{name: "queued create stages", approval: false, failAt: "create_stages"},
		{name: "queued append events", approval: false, failAt: "append_events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _, _, audits := newCreateFixture(t, tt.approval)
			repo.failAt = tt.failAt
			repo.failErr = errors.New("injected persistence failure")
			req := CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"}

			_, err := svc.Create(context.Background(), 23, "", "", 7, "retryable-key", req)
			require.ErrorIs(t, err, repo.failErr)
			require.Empty(t, repo.runs, "run and idempotency fingerprint must roll back")
			require.Empty(t, repo.stages)
			require.Empty(t, repo.approvals)
			require.Empty(t, repo.events)
			require.Empty(t, audits.events)

			repo.failAt = ""
			created, err := svc.Create(context.Background(), 23, "", "", 7, "retryable-key", req)
			require.NoError(t, err)
			require.NotNil(t, created)
			require.Len(t, repo.runs, 1)
			require.Len(t, repo.stages, 2)
			require.Len(t, repo.events, 2)
			if tt.approval {
				require.Len(t, repo.approvals, 1)
			} else {
				require.Empty(t, repo.approvals)
			}
		})
	}
}

func TestCancelBeforeExecutionIsImmediateAndIdempotent(t *testing.T) {
	svc, repo, _, _, audits := newCreateFixture(t, true)
	created, err := svc.Create(context.Background(), 23, "", "", 7, "create", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
	require.NoError(t, err)
	canceled, err := svc.Cancel(context.Background(), 23, "127.0.0.1", "test", created.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliveryRunCanceled, canceled.State)
	require.Equal(t, models.DeliveryStageCanceled, canceled.Stages[0].State)
	require.Equal(t, models.DeliveryStageCanceled, canceled.Stages[1].State)
	require.Equal(t, "run.canceled", repo.events[len(repo.events)-1].EventType)
	require.Len(t, audits.events, 2)
	replay, err := svc.Cancel(context.Background(), 23, "", "", created.ID)
	require.NoError(t, err)
	require.Equal(t, canceled.State, replay.State)
	require.Len(t, audits.events, 2)
}

func TestCancelRunningRequestsCooperativeCancellation(t *testing.T) {
	svc, repo, _, _, _ := newCreateFixture(t, false)
	created, err := svc.Create(context.Background(), 23, "", "", 7, "create", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
	require.NoError(t, err)
	repo.runs[0].State = models.DeliveryRunRunning
	repo.stages[0].State = models.DeliveryStageRunning
	result, err := svc.Cancel(context.Background(), 23, "", "", created.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliveryRunCancelRequested, result.State)
	require.Equal(t, models.DeliveryStageRunning, result.Stages[0].State)
	require.Equal(t, "run.cancel_requested", repo.events[len(repo.events)-1].EventType)
}

func TestRequestReconcileReopensUnknownWithoutGuessingOutcome(t *testing.T) {
	svc, repo, _, _, _ := newCreateFixture(t, false)
	created, err := svc.Create(context.Background(), 23, "", "", 7, "create", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
	require.NoError(t, err)
	repo.runs[0].State = models.DeliveryRunOutcomeUnknown
	finished := svc.now().UTC()
	repo.runs[0].FinishedAt = &finished
	errorCode := 500
	repo.runs[0].ErrorCode = &errorCode
	repo.stages[0].State = models.DeliveryStageOutcomeUnknown
	repo.stages[0].FinishedAt = &finished
	repo.stages[0].ErrorCode = &errorCode
	result, err := svc.RequestReconcile(context.Background(), 23, "", "", created.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliveryRunReconciling, result.State)
	require.Equal(t, models.DeliveryStageReconciling, result.Stages[0].State)
	require.Nil(t, repo.runs[0].FinishedAt)
	require.Nil(t, repo.runs[0].ErrorCode)
	require.Nil(t, repo.stages[0].FinishedAt)
	require.Nil(t, repo.stages[0].ErrorCode)
}

func TestRequestReconcileRejectsUnknownRunWithoutExactlyOneUnknownStage(t *testing.T) {
	svc, repo, _, _, _ := newCreateFixture(t, false)
	created, err := svc.Create(context.Background(), 23, "", "", 7, "create", CreateRequest{ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3"})
	require.NoError(t, err)
	repo.runs[0].State = models.DeliveryRunOutcomeUnknown

	_, err = svc.RequestReconcile(context.Background(), 23, "", "", created.ID)
	requireBizCode(t, err, errs.CodeRunInvalidState)
}

func TestRetryClonesImmutableOriginalFromFailedEnvironmentAndRenewsApproval(t *testing.T) {
	svc, repo, _, resolver, audits := newCreateFixture(t, false)
	digest := "sha256:" + strings.Repeat("d", 64)
	finished := svc.now().UTC()
	repo.runs = []models.DeliveryRun{{ID: 40, ProjectID: 7, PipelineID: 31, PipelineVersion: 3, ChartRepoID: 9, ChartName: "demo", ChartVersion: "1.2.3", ChartDigest: digest, InitiatorUserID: 8, IdempotencyKey: "old", RequestFingerprint: "old", State: models.DeliveryRunFailed, FinishedAt: &finished}}
	repo.stages = []models.DeliveryRunStage{
		{ID: 80, RunID: 40, EnvironmentID: 71, EnvironmentKey: "development", ApplicationID: 101, ClusterID: 201, Namespace: "dev", ReleaseName: "demo-dev", StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, State: models.DeliveryStageSucceeded, OperationID: "old-1"},
		{ID: 81, RunID: 40, EnvironmentID: 72, EnvironmentKey: "production", ApplicationID: 102, ClusterID: 202, Namespace: "prod", ReleaseName: "demo-prod", StageOrder: 2, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, ApprovalRequired: true, TimeoutSeconds: 900, State: models.DeliveryStageFailed, OperationID: "old-2"},
	}
	original := repo.runs[0]
	retried, err := svc.Retry(context.Background(), 23, "", "", 40, "retry-1")
	require.NoError(t, err)
	require.Equal(t, digest, retried.ChartDigest)
	require.Equal(t, uint64(40), *retried.RetryOfRunID)
	require.Len(t, retried.Stages, 1)
	require.Equal(t, "production", retried.Stages[0].EnvironmentKey)
	require.Equal(t, 1, retried.Stages[0].Order)
	require.Equal(t, models.DeliveryStageWaitingApproval, retried.Stages[0].State)
	require.Len(t, repo.approvals, 1)
	require.Equal(t, original, repo.runs[0], "retry must not mutate the old terminal run")
	require.Zero(t, resolver.calls, "retry must not resolve a mutable artifact")
	require.Equal(t, []string{"lock_project", "activity", "lock_application:102", "create_run"}, repo.calls[len(repo.calls)-4:])
	require.Equal(t, "delivery.run.retry", audits.events[0].Action)
	replay, err := svc.Retry(context.Background(), 23, "", "", 40, "retry-1")
	require.NoError(t, err)
	require.Equal(t, retried.ID, replay.ID)
	require.Len(t, repo.runs, 2)
}

type memoryRepository struct {
	pipeline     *models.DeliveryPipeline
	pipelineRows []models.DeliveryPipelineStage
	environments []models.DeliveryEnvironment
	blocking     *models.DeliveryRun
	runs         []models.DeliveryRun
	stages       []models.DeliveryRunStage
	approvals    []models.DeliveryApproval
	events       []models.DeliveryRunEvent
	calls        []string
	failAt       string
	failErr      error
}

func (r *memoryRepository) Transaction(_ context.Context, fn func(repository) error) error {
	runsBefore := append([]models.DeliveryRun(nil), r.runs...)
	stagesBefore := append([]models.DeliveryRunStage(nil), r.stages...)
	approvalsBefore := append([]models.DeliveryApproval(nil), r.approvals...)
	eventsBefore := append([]models.DeliveryRunEvent(nil), r.events...)
	if err := fn(r); err != nil {
		r.runs = runsBefore
		r.stages = stagesBefore
		r.approvals = approvalsBefore
		r.events = eventsBefore
		return err
	}
	return nil
}
func (r *memoryRepository) LockProject(context.Context, uint64) error {
	r.calls = append(r.calls, "lock_project")
	return nil
}
func (r *memoryRepository) LockApplication(_ context.Context, id uint64) error {
	r.calls = append(r.calls, fmt.Sprintf("lock_application:%d", id))
	return nil
}
func (r *memoryRepository) GetCurrent(context.Context, uint64) (*models.DeliveryPipeline, []models.DeliveryPipelineStage, error) {
	return clonePipeline(r.pipeline), append([]models.DeliveryPipelineStage(nil), r.pipelineRows...), nil
}
func (r *memoryRepository) ListEnvironments(context.Context, uint64) ([]models.DeliveryEnvironment, error) {
	return append([]models.DeliveryEnvironment(nil), r.environments...), nil
}
func (r *memoryRepository) BlockingRun(context.Context, uint64) (*models.DeliveryRun, error) {
	r.calls = append(r.calls, "activity")
	return cloneRun(r.blocking), nil
}
func (r *memoryRepository) FindByIdempotency(_ context.Context, projectID, actor uint64, key string) (*models.DeliveryRun, error) {
	for i := range r.runs {
		if r.runs[i].ProjectID == projectID && r.runs[i].InitiatorUserID == actor && r.runs[i].IdempotencyKey == key {
			return cloneRun(&r.runs[i]), nil
		}
	}
	return nil, nil
}
func (r *memoryRepository) ListRunStages(_ context.Context, runID uint64) ([]models.DeliveryRunStage, error) {
	var out []models.DeliveryRunStage
	for i := range r.stages {
		if r.stages[i].RunID == runID {
			out = append(out, r.stages[i])
		}
	}
	return out, nil
}
func (r *memoryRepository) CreateRun(_ context.Context, row *models.DeliveryRun) error {
	r.calls = append(r.calls, "create_run")
	row.ID = uint64(len(r.runs) + 1)
	r.runs = append(r.runs, *row)
	if r.failAt == "create_run" {
		return r.failErr
	}
	return nil
}
func (r *memoryRepository) CreateStages(_ context.Context, rows []models.DeliveryRunStage) error {
	for i := range rows {
		rows[i].ID = uint64(len(r.stages) + 1)
		r.stages = append(r.stages, rows[i])
	}
	if r.failAt == "create_stages" {
		return r.failErr
	}
	return nil
}
func (r *memoryRepository) CreateApproval(_ context.Context, row *models.DeliveryApproval) error {
	row.ID = uint64(len(r.approvals) + 1)
	r.approvals = append(r.approvals, *row)
	if r.failAt == "create_approval" {
		return r.failErr
	}
	return nil
}
func (r *memoryRepository) AppendEvents(_ context.Context, rows []models.DeliveryRunEvent) error {
	for i := range rows {
		rows[i].ID = uint64(len(r.events) + 1)
		r.events = append(r.events, rows[i])
	}
	if r.failAt == "append_events" {
		return r.failErr
	}
	return nil
}
func (r *memoryRepository) GetRun(_ context.Context, id uint64) (*models.DeliveryRun, error) {
	for i := range r.runs {
		if r.runs[i].ID == id {
			return cloneRun(&r.runs[i]), nil
		}
	}
	return nil, apperr.New(errs.CodeRunNotFound, errs.KeyRunNotFound, "not found")
}
func (r *memoryRepository) LockRun(ctx context.Context, id uint64) (*models.DeliveryRun, error) {
	return r.GetRun(ctx, id)
}
func (r *memoryRepository) LockRunStages(ctx context.Context, id uint64) ([]models.DeliveryRunStage, error) {
	return r.ListRunStages(ctx, id)
}
func (r *memoryRepository) UpdateRun(_ context.Context, id uint64, from, to models.DeliveryRunState, fields map[string]any) error {
	for i := range r.runs {
		if r.runs[i].ID == id && r.runs[i].State == from {
			r.runs[i].State = to
			if value, ok := fields["finished_at"].(time.Time); ok {
				r.runs[i].FinishedAt = &value
			}
			if value, ok := fields["finished_at"]; ok && value == nil {
				r.runs[i].FinishedAt = nil
			}
			if value, ok := fields["error_code"]; ok && value == nil {
				r.runs[i].ErrorCode = nil
			}
			return nil
		}
	}
	return errors.New("run transition conflict")
}
func (r *memoryRepository) UpdateStage(_ context.Context, id uint64, from, to models.DeliveryStageState, fields map[string]any) error {
	for i := range r.stages {
		if r.stages[i].ID == id && r.stages[i].State == from {
			r.stages[i].State = to
			if value, ok := fields["finished_at"].(time.Time); ok {
				r.stages[i].FinishedAt = &value
			}
			if value, ok := fields["finished_at"]; ok && value == nil {
				r.stages[i].FinishedAt = nil
			}
			if value, ok := fields["error_code"]; ok && value == nil {
				r.stages[i].ErrorCode = nil
			}
			return nil
		}
	}
	return errors.New("stage transition conflict")
}

type applicationReader struct {
	rows  map[uint64]*Application
	calls int
}

func (r *applicationReader) GetApplication(_ context.Context, id uint64) (*Application, error) {
	r.calls++
	row := r.rows[id]
	if row == nil {
		return nil, errors.New("missing application")
	}
	cloned := *row
	return &cloned, nil
}

type artifactResolver struct {
	artifact Artifact
	err      error
	calls    int
}

func (r *artifactResolver) ResolveArtifact(context.Context, uint64, string, string) (*Artifact, error) {
	r.calls++
	cloned := r.artifact
	return &cloned, r.err
}

type auditRecorder struct{ events []audit.Event }

func (r *auditRecorder) Record(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func newCreateFixture(t *testing.T, approval bool) (*Service, *memoryRepository, *applicationReader, *artifactResolver, *auditRecorder) {
	t.Helper()
	repo := &memoryRepository{
		pipeline: &models.DeliveryPipeline{ID: 31, ProjectID: 7, Version: 3, IsCurrent: true},
		pipelineRows: []models.DeliveryPipelineStage{
			{ID: 51, PipelineID: 31, EnvironmentID: 71, StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, ApprovalRequired: approval, TimeoutSeconds: 600},
			{ID: 52, PipelineID: 31, EnvironmentID: 72, StageOrder: 2, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 900},
		},
		environments: []models.DeliveryEnvironment{
			{ID: 71, ProjectID: 7, EnvironmentKey: "development", DisplayName: "Development", ApplicationID: 101},
			{ID: 72, ProjectID: 7, EnvironmentKey: "production", DisplayName: "Production", ApplicationID: 102},
		},
	}
	apps := &applicationReader{rows: map[uint64]*Application{
		101: {ID: 101, ChartRepoID: 9, ChartName: "demo", Installed: true, ClusterID: 201, Namespace: "dev", ReleaseName: "demo-dev"},
		102: {ID: 102, ChartRepoID: 9, ChartName: "demo", Installed: true, ClusterID: 202, Namespace: "prod", ReleaseName: "demo-prod"},
	}}
	resolver := &artifactResolver{artifact: Artifact{RepoID: 9, ChartName: "demo", Version: "1.2.3", Digest: "sha256:" + strings.Repeat("a", 64)}}
	audits := &auditRecorder{}
	svc := NewService(repo, apps, resolver, audits)
	fixed := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	return svc, repo, apps, resolver, audits
}

func clonePipeline(row *models.DeliveryPipeline) *models.DeliveryPipeline {
	if row == nil {
		return nil
	}
	cloned := *row
	return &cloned
}

func cloneRun(row *models.DeliveryRun) *models.DeliveryRun {
	if row == nil {
		return nil
	}
	cloned := *row
	return &cloned
}

func requireBizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	require.Error(t, err)
	var business *apperr.BizError
	require.ErrorAs(t, err, &business)
	require.Equal(t, code, business.Code)
}
