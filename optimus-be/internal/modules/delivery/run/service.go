package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/errs"
)

var chartDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type repository interface {
	Transaction(context.Context, func(repository) error) error
	LockProject(context.Context, uint64) error
	LockApplication(context.Context, uint64) error
	GetCurrent(context.Context, uint64) (*models.DeliveryPipeline, []models.DeliveryPipelineStage, error)
	ListEnvironments(context.Context, uint64) ([]models.DeliveryEnvironment, error)
	BlockingRun(context.Context, uint64) (*models.DeliveryRun, error)
	FindByIdempotency(context.Context, uint64, uint64, string) (*models.DeliveryRun, error)
	ListRunStages(context.Context, uint64) ([]models.DeliveryRunStage, error)
	CreateRun(context.Context, *models.DeliveryRun) error
	CreateStages(context.Context, []models.DeliveryRunStage) error
	CreateApproval(context.Context, *models.DeliveryApproval) error
	AppendEvents(context.Context, []models.DeliveryRunEvent) error
}

type ApplicationReader interface {
	GetApplication(context.Context, uint64) (*Application, error)
}

type ArtifactResolver interface {
	ResolveArtifact(context.Context, uint64, string, string) (*Artifact, error)
}

type Recorder interface {
	Record(context.Context, audit.Event) error
}

type Service struct {
	repo      repository
	apps      ApplicationReader
	artifacts ArtifactResolver
	audit     Recorder
	now       func() time.Time
}

func NewService(repo repository, apps ApplicationReader, artifacts ArtifactResolver, recorder Recorder) *Service {
	return &Service{repo: repo, apps: apps, artifacts: artifacts, audit: recorder, now: time.Now}
}

func (s *Service) Create(
	ctx context.Context,
	actor uint64,
	ip, userAgent string,
	projectID uint64,
	idempotencyKey string,
	req CreateRequest,
) (*Run, error) {
	key, err := validateCreateRequest(actor, projectID, idempotencyKey, req)
	if err != nil {
		return nil, err
	}
	if s.repo == nil || s.apps == nil || s.artifacts == nil {
		return nil, executionUnavailableError(nil)
	}

	artifact, err := s.artifacts.ResolveArtifact(ctx, req.ChartRepoID, strings.TrimSpace(req.ChartName), strings.TrimSpace(req.ChartVersion))
	if err != nil {
		return nil, executionUnavailableError(err)
	}
	if !validResolvedArtifact(artifact, req) {
		return nil, chartIdentityMismatchError()
	}

	var result *Run
	created := false
	err = s.repo.Transaction(ctx, func(tx repository) error {
		if err := tx.LockProject(ctx, projectID); err != nil {
			return err
		}
		pipeline, pipelineStages, err := tx.GetCurrent(ctx, projectID)
		if err != nil {
			return err
		}
		if err := validateCurrentPipeline(projectID, pipeline, pipelineStages); err != nil {
			return err
		}
		fingerprint, err := canonicalFingerprint(projectID, pipeline.Version, *artifact, req.RetryOfRunID)
		if err != nil {
			return executionUnavailableError(err)
		}

		existing, err := tx.FindByIdempotency(ctx, projectID, actor, key)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestFingerprint != fingerprint {
				return idempotencyConflictError()
			}
			stages, err := tx.ListRunStages(ctx, existing.ID)
			if err != nil {
				return err
			}
			result = runFrom(existing, stages)
			return nil
		}

		blocking, err := tx.BlockingRun(ctx, projectID)
		if err != nil {
			return err
		}
		if blocking != nil {
			if blocking.State == models.DeliveryRunOutcomeUnknown {
				return outcomeUnknownError()
			}
			return activeRunError()
		}

		environments, err := tx.ListEnvironments(ctx, projectID)
		if err != nil {
			return err
		}
		environmentByID := make(map[uint64]models.DeliveryEnvironment, len(environments))
		for i := range environments {
			environmentByID[environments[i].ID] = environments[i]
		}
		applicationIDs, err := pipelineApplicationIDs(pipelineStages, environmentByID)
		if err != nil {
			return err
		}
		for _, applicationID := range applicationIDs {
			if err := tx.LockApplication(ctx, applicationID); err != nil {
				return err
			}
		}

		applications := make(map[uint64]*Application, len(applicationIDs))
		for _, applicationID := range applicationIDs {
			application, err := s.apps.GetApplication(ctx, applicationID)
			if err != nil || application == nil {
				return applicationUnavailableError(err)
			}
			if err := validateApplication(applicationID, application, *artifact); err != nil {
				return err
			}
			applications[applicationID] = application
		}

		now := s.now().UTC()
		initialRunState := models.DeliveryRunQueued
		initialStageState := models.DeliveryStageQueued
		if pipelineStages[0].ApprovalRequired {
			initialRunState = models.DeliveryRunWaitingApproval
			initialStageState = models.DeliveryStageWaitingApproval
		}
		runRow := &models.DeliveryRun{
			ProjectID: projectID, PipelineID: pipeline.ID, PipelineVersion: pipeline.Version,
			ChartRepoID: artifact.RepoID, ChartName: artifact.ChartName, ChartVersion: artifact.Version,
			ChartDigest: artifact.Digest, InitiatorUserID: actor, IdempotencyKey: key,
			RequestFingerprint: fingerprint, State: initialRunState, RetryOfRunID: cloneUint64(req.RetryOfRunID),
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.CreateRun(ctx, runRow); err != nil {
			return err
		}
		stageRows := snapshotStages(runRow.ID, pipelineStages, environmentByID, applications, initialStageState, now)
		if err := tx.CreateStages(ctx, stageRows); err != nil {
			return err
		}
		if pipelineStages[0].ApprovalRequired {
			approval := &models.DeliveryApproval{
				RunID: runRow.ID, RunStageID: stageRows[0].ID, RequestedAt: now,
				Decision: models.DeliveryApprovalPending, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.CreateApproval(ctx, approval); err != nil {
				return err
			}
		}
		if err := tx.AppendEvents(ctx, initialEvents(runRow, stageRows[0], actor, now)); err != nil {
			return err
		}
		result = runFrom(runRow, stageRows)
		created = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if created {
		s.recordCreate(ctx, actor, ip, userAgent, result)
	}
	return result, nil
}

func validateCreateRequest(actor, projectID uint64, idempotencyKey string, req CreateRequest) (string, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return "", apperr.New(errs.CodeIdempotencyMissing, errs.KeyIdempotencyMissing, "Idempotency-Key is required")
	}
	if len(key) > 128 {
		return "", validationError("Idempotency-Key must not exceed 128 bytes")
	}
	if actor == 0 || projectID == 0 || req.ChartRepoID == 0 {
		return "", validationError("actor, project, and chart repository are required")
	}
	chartName, version := strings.TrimSpace(req.ChartName), strings.TrimSpace(req.ChartVersion)
	if chartName == "" || len(chartName) > 128 || version == "" || len(version) > 128 {
		return "", validationError("chart name and version must contain between 1 and 128 bytes")
	}
	if req.RetryOfRunID != nil && *req.RetryOfRunID == 0 {
		return "", validationError("retry origin must be greater than zero")
	}
	return key, nil
}

func validResolvedArtifact(artifact *Artifact, req CreateRequest) bool {
	return artifact != nil && artifact.RepoID == req.ChartRepoID &&
		artifact.ChartName == strings.TrimSpace(req.ChartName) &&
		artifact.Version == strings.TrimSpace(req.ChartVersion) &&
		chartDigestPattern.MatchString(artifact.Digest)
}

func validateCurrentPipeline(projectID uint64, pipeline *models.DeliveryPipeline, stages []models.DeliveryPipelineStage) error {
	if pipeline == nil {
		return pipelineMissingError()
	}
	if pipeline.ProjectID != projectID || !pipeline.IsCurrent || pipeline.ID == 0 || pipeline.Version < 1 || len(stages) == 0 {
		return apperr.New(errs.CodePipelineInvalid, errs.KeyPipelineInvalid, "current delivery pipeline is invalid")
	}
	for i := range stages {
		if stages[i].PipelineID != pipeline.ID || stages[i].StageOrder != i+1 || stages[i].EnvironmentID == 0 ||
			stages[i].Executor != models.DeliveryExecutorHelmUpgradeExistingRelease || stages[i].TimeoutSeconds < 1 {
			return apperr.New(errs.CodePipelineInvalid, errs.KeyPipelineInvalid, "current delivery pipeline stages are invalid")
		}
	}
	return nil
}

func pipelineApplicationIDs(stages []models.DeliveryPipelineStage, environments map[uint64]models.DeliveryEnvironment) ([]uint64, error) {
	seen := make(map[uint64]struct{}, len(stages))
	ids := make([]uint64, 0, len(stages))
	for i := range stages {
		environment, ok := environments[stages[i].EnvironmentID]
		if !ok || environment.ApplicationID == 0 {
			return nil, apperr.New(errs.CodePipelineInvalid, errs.KeyPipelineInvalid, "pipeline environment binding is unavailable")
		}
		if _, exists := seen[environment.ApplicationID]; !exists {
			seen[environment.ApplicationID] = struct{}{}
			ids = append(ids, environment.ApplicationID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func validateApplication(id uint64, application *Application, artifact Artifact) error {
	if application.ID != id || application.ClusterID == 0 || strings.TrimSpace(application.Namespace) == "" ||
		strings.TrimSpace(application.ReleaseName) == "" {
		return applicationUnavailableError(nil)
	}
	if !application.Installed {
		return applicationUnavailableError(nil)
	}
	if application.ChartRepoID != artifact.RepoID || application.ChartName != artifact.ChartName {
		return chartIdentityMismatchError()
	}
	return nil
}

func snapshotStages(
	runID uint64,
	pipelineStages []models.DeliveryPipelineStage,
	environments map[uint64]models.DeliveryEnvironment,
	applications map[uint64]*Application,
	initial models.DeliveryStageState,
	now time.Time,
) []models.DeliveryRunStage {
	rows := make([]models.DeliveryRunStage, len(pipelineStages))
	for i := range pipelineStages {
		source := pipelineStages[i]
		environment := environments[source.EnvironmentID]
		application := applications[environment.ApplicationID]
		state := models.DeliveryStagePending
		if i == 0 {
			state = initial
		}
		rows[i] = models.DeliveryRunStage{
			RunID: runID, EnvironmentID: environment.ID, EnvironmentKey: environment.EnvironmentKey,
			EnvironmentName: environment.DisplayName, ApplicationID: application.ID, ClusterID: application.ClusterID,
			Namespace: application.Namespace, ReleaseName: application.ReleaseName, StageOrder: source.StageOrder,
			Executor: source.Executor, ApprovalRequired: source.ApprovalRequired, TimeoutSeconds: source.TimeoutSeconds,
			State: state, OperationID: fmt.Sprintf("delivery-run-%d-stage-%d", runID, source.StageOrder),
			CreatedAt: now, UpdatedAt: now,
		}
	}
	return rows
}

func initialEvents(runRow *models.DeliveryRun, first models.DeliveryRunStage, actor uint64, now time.Time) []models.DeliveryRunEvent {
	runState, pending, stageState := string(runRow.State), string(models.DeliveryStagePending), string(first.State)
	stageID := first.ID
	return []models.DeliveryRunEvent{
		{
			RunID: runRow.ID, EventType: "run.created", NewState: &runState,
			ActorType: models.DeliveryEventActorUser, ActorID: &actor, OccurredAt: now,
			Metadata: datatypes.JSON([]byte(`{}`)),
		},
		{
			RunID: runRow.ID, RunStageID: &stageID, EventType: "stage." + stageState,
			OldState: &pending, NewState: &stageState, ActorType: models.DeliveryEventActorUser,
			ActorID: &actor, OccurredAt: now, Metadata: datatypes.JSON([]byte(`{}`)),
		},
	}
}

func runFrom(row *models.DeliveryRun, stages []models.DeliveryRunStage) *Run {
	out := &Run{
		ID: row.ID, ProjectID: row.ProjectID, PipelineID: row.PipelineID, PipelineVersion: row.PipelineVersion,
		ChartRepoID: row.ChartRepoID, ChartName: row.ChartName, ChartVersion: row.ChartVersion,
		ChartDigest: row.ChartDigest, InitiatorUserID: row.InitiatorUserID,
		RequestFingerprint: row.RequestFingerprint, State: row.State, RetryOfRunID: cloneUint64(row.RetryOfRunID),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Stages: make([]Stage, len(stages)),
	}
	for i := range stages {
		out.Stages[i] = Stage{
			ID: stages[i].ID, EnvironmentID: stages[i].EnvironmentID, EnvironmentKey: stages[i].EnvironmentKey,
			EnvironmentName: stages[i].EnvironmentName, ApplicationID: stages[i].ApplicationID,
			ClusterID: stages[i].ClusterID, Namespace: stages[i].Namespace, ReleaseName: stages[i].ReleaseName,
			Order: stages[i].StageOrder, Executor: stages[i].Executor, ApprovalRequired: stages[i].ApprovalRequired,
			Timeout: time.Duration(stages[i].TimeoutSeconds) * time.Second, State: stages[i].State,
			OperationID: stages[i].OperationID,
		}
	}
	return out
}

func canonicalFingerprint(projectID uint64, pipelineVersion int, artifact Artifact, retryOfRunID *uint64) (string, error) {
	canonical, err := json.Marshal(struct {
		ProjectID       uint64  `json:"project_id"`
		PipelineVersion int     `json:"pipeline_version"`
		ChartRepoID     uint64  `json:"chart_repo_id"`
		ChartName       string  `json:"chart_name"`
		ChartVersion    string  `json:"chart_version"`
		ChartDigest     string  `json:"chart_digest"`
		RetryOfRunID    *uint64 `json:"retry_of_run_id"`
	}{
		ProjectID: projectID, PipelineVersion: pipelineVersion, ChartRepoID: artifact.RepoID,
		ChartName: artifact.ChartName, ChartVersion: artifact.Version, ChartDigest: artifact.Digest,
		RetryOfRunID: retryOfRunID,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) recordCreate(ctx context.Context, actor uint64, ip, userAgent string, created *Run) {
	if s.audit == nil || created == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actor, Action: "delivery.run.create", TargetType: "delivery.run",
		TargetID: strconv.FormatUint(created.ID, 10), IP: ip, UserAgent: userAgent,
		Payload: map[string]any{
			"project_id": created.ProjectID, "pipeline_version": created.PipelineVersion,
			"chart_repo_id": created.ChartRepoID, "chart_name": created.ChartName,
			"chart_version": created.ChartVersion, "chart_digest": created.ChartDigest,
			"retry_of_run_id": created.RetryOfRunID,
		},
	})
}

func applicationUnavailableError(cause error) error {
	if cause != nil {
		return apperr.Wrap(cause, errs.CodeApplicationUnavailable, errs.KeyApplicationUnavailable, "delivery application is unavailable")
	}
	return apperr.New(errs.CodeApplicationUnavailable, errs.KeyApplicationUnavailable, "delivery application is unavailable")
}

func chartIdentityMismatchError() error {
	return apperr.New(errs.CodeChartIdentityMismatch, errs.KeyChartIdentityMismatch, "all delivery targets must use the selected chart identity")
}

func outcomeUnknownError() error {
	return apperr.New(errs.CodeOutcomeUnknown, errs.KeyOutcomeUnknown, "delivery project has a run with an unknown outcome")
}

func executionUnavailableError(cause error) error {
	if cause != nil {
		return apperr.Wrap(cause, errs.CodeExecutionUnavailable, errs.KeyExecutionUnavailable, "delivery execution dependency is unavailable")
	}
	return apperr.New(errs.CodeExecutionUnavailable, errs.KeyExecutionUnavailable, "delivery execution dependency is unavailable")
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
