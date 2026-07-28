package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	GetRun(context.Context, uint64) (*models.DeliveryRun, error)
	LockRun(context.Context, uint64) (*models.DeliveryRun, error)
	LockRunStages(context.Context, uint64) ([]models.DeliveryRunStage, error)
	UpdateRun(context.Context, uint64, models.DeliveryRunState, models.DeliveryRunState, map[string]any) error
	UpdateStage(context.Context, uint64, models.DeliveryStageState, models.DeliveryStageState, map[string]any) error
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

// Cancel requests cooperative cancellation. Work that has not entered P3 is
// canceled immediately; running work is marked cancel_requested and is later
// resolved only through release inspection.
func (s *Service) Cancel(ctx context.Context, actor uint64, ip, userAgent string, runID uint64) (*Run, error) {
	if actor == 0 || runID == 0 {
		return nil, validationError("actor and run are required")
	}
	var result *Run
	changed := false
	err := s.repo.Transaction(ctx, func(tx repository) error {
		runRow, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		stages, err := tx.LockRunStages(ctx, runID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		switch runRow.State {
		case models.DeliveryRunCanceled, models.DeliveryRunCancelRequested:
			result = runFrom(runRow, stages)
			return nil
		case models.DeliveryRunQueued, models.DeliveryRunWaitingApproval:
			for i := range stages {
				if stages[i].State != models.DeliveryStagePending && stages[i].State != models.DeliveryStageWaitingApproval && stages[i].State != models.DeliveryStageQueued {
					continue
				}
				old := stages[i].State
				if err := tx.UpdateStage(ctx, stages[i].ID, old, models.DeliveryStageCanceled, map[string]any{"finished_at": now}); err != nil {
					return err
				}
				stages[i].State, stages[i].FinishedAt = models.DeliveryStageCanceled, &now
				if err := tx.AppendEvents(ctx, []models.DeliveryRunEvent{transitionEvent(runID, &stages[i].ID, "stage.canceled", string(old), string(models.DeliveryStageCanceled), actor, now)}); err != nil {
					return err
				}
			}
			old := runRow.State
			if err := tx.UpdateRun(ctx, runID, old, models.DeliveryRunCanceled, map[string]any{"finished_at": now}); err != nil {
				return err
			}
			runRow.State, runRow.FinishedAt = models.DeliveryRunCanceled, &now
			if err := tx.AppendEvents(ctx, []models.DeliveryRunEvent{transitionEvent(runID, nil, "run.canceled", string(old), string(models.DeliveryRunCanceled), actor, now)}); err != nil {
				return err
			}
		case models.DeliveryRunRunning:
			if err := tx.UpdateRun(ctx, runID, runRow.State, models.DeliveryRunCancelRequested, nil); err != nil {
				return err
			}
			old := runRow.State
			runRow.State = models.DeliveryRunCancelRequested
			if err := tx.AppendEvents(ctx, []models.DeliveryRunEvent{transitionEvent(runID, nil, "run.cancel_requested", string(old), string(runRow.State), actor, now)}); err != nil {
				return err
			}
		default:
			return apperr.New(errs.CodeRunCancelConflict, errs.KeyRunCancelConflict, "delivery run cannot be canceled in its current state")
		}
		runRow.UpdatedAt = now
		result = runFrom(runRow, stages)
		changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.recordAction(ctx, actor, ip, userAgent, "delivery.run.cancel", result)
	}
	return result, nil
}

// RequestReconcile reopens only an unknown result. The reconciler still needs
// definite P3 inspection evidence before it may choose a terminal state.
func (s *Service) RequestReconcile(ctx context.Context, actor uint64, ip, userAgent string, runID uint64) (*Run, error) {
	if actor == 0 || runID == 0 {
		return nil, validationError("actor and run are required")
	}
	var result *Run
	changed := false
	err := s.repo.Transaction(ctx, func(tx repository) error {
		runRow, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		stages, err := tx.LockRunStages(ctx, runID)
		if err != nil {
			return err
		}
		if runRow.State == models.DeliveryRunReconciling {
			result = runFrom(runRow, stages)
			return nil
		}
		if runRow.State != models.DeliveryRunOutcomeUnknown {
			return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "only an unknown delivery run can be reconciled")
		}
		now := s.now().UTC()
		unknownStage := -1
		for i := range stages {
			if stages[i].State != models.DeliveryStageOutcomeUnknown {
				continue
			}
			if unknownStage >= 0 {
				return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery run has multiple unknown stages")
			}
			unknownStage = i
		}
		if unknownStage < 0 {
			return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery run has no unknown stage")
		}
		stage := &stages[unknownStage]
		stageFields := map[string]any{"finished_at": nil, "error_code": nil, "error_message_key": nil}
		if err := tx.UpdateStage(ctx, stage.ID, stage.State, models.DeliveryStageReconciling, stageFields); err != nil {
			return err
		}
		oldStage := stage.State
		stage.State = models.DeliveryStageReconciling
		stage.FinishedAt, stage.ErrorCode, stage.ErrorMessageKey = nil, nil, nil
		if err := tx.AppendEvents(ctx, []models.DeliveryRunEvent{transitionEvent(runID, &stage.ID, "stage.reconciling", string(oldStage), string(stage.State), actor, now)}); err != nil {
			return err
		}
		runFields := map[string]any{"finished_at": nil, "error_code": nil, "error_message_key": nil}
		if err := tx.UpdateRun(ctx, runID, runRow.State, models.DeliveryRunReconciling, runFields); err != nil {
			return err
		}
		old := runRow.State
		runRow.State = models.DeliveryRunReconciling
		runRow.FinishedAt, runRow.ErrorCode, runRow.ErrorMessageKey = nil, nil, nil
		if err := tx.AppendEvents(ctx, []models.DeliveryRunEvent{transitionEvent(runID, nil, "run.reconciling", string(old), string(runRow.State), actor, now)}); err != nil {
			return err
		}
		result = runFrom(runRow, stages)
		changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.recordAction(ctx, actor, ip, userAgent, "delivery.run.reconcile", result)
	}
	return result, nil
}

// Retry creates a linked run from the immutable original snapshot. It never
// resolves the artifact again and never changes the old terminal run.
func (s *Service) Retry(ctx context.Context, actor uint64, ip, userAgent string, runID uint64, idempotencyKey string) (*Run, error) {
	key := strings.TrimSpace(idempotencyKey)
	if actor == 0 || runID == 0 {
		return nil, validationError("actor and run are required")
	}
	if key == "" {
		return nil, apperr.New(errs.CodeIdempotencyMissing, errs.KeyIdempotencyMissing, "Idempotency-Key is required")
	}
	if len(key) > 128 {
		return nil, validationError("Idempotency-Key must not exceed 128 bytes")
	}
	var result *Run
	created := false
	err := s.repo.Transaction(ctx, func(tx repository) error {
		originIdentity, err := tx.GetRun(ctx, runID)
		if err != nil {
			return err
		}
		if err := tx.LockProject(ctx, originIdentity.ProjectID); err != nil {
			return err
		}
		origin, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		originStages, err := tx.LockRunStages(ctx, runID)
		if err != nil {
			return err
		}
		if origin.State == models.DeliveryRunOutcomeUnknown || origin.State == models.DeliveryRunReconciling || origin.State == models.DeliveryRunQueued || origin.State == models.DeliveryRunRunning || origin.State == models.DeliveryRunWaitingApproval || origin.State == models.DeliveryRunCancelRequested || origin.State == models.DeliveryRunSucceeded {
			return apperr.New(errs.CodeRunRetryUnavailable, errs.KeyRunRetryUnavailable, "delivery run is not eligible for retry")
		}
		fingerprint, err := canonicalFingerprint(origin.ProjectID, origin.PipelineVersion, Artifact{RepoID: origin.ChartRepoID, ChartName: origin.ChartName, Version: origin.ChartVersion, Digest: origin.ChartDigest}, &origin.ID)
		if err != nil {
			return executionUnavailableError(err)
		}
		existing, err := tx.FindByIdempotency(ctx, origin.ProjectID, actor, key)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestFingerprint != fingerprint {
				return idempotencyConflictError()
			}
			rows, err := tx.ListRunStages(ctx, existing.ID)
			if err != nil {
				return err
			}
			result = runFrom(existing, rows)
			return nil
		}
		blocking, err := tx.BlockingRun(ctx, origin.ProjectID)
		if err != nil {
			return err
		}
		if blocking != nil {
			if blocking.State == models.DeliveryRunOutcomeUnknown {
				return outcomeUnknownError()
			}
			return activeRunError()
		}
		start := -1
		for i := range originStages {
			if originStages[i].State != models.DeliveryStageSucceeded {
				start = i
				break
			}
		}
		if start < 0 {
			return apperr.New(errs.CodeRunRetryUnavailable, errs.KeyRunRetryUnavailable, "delivery run has no failed environment")
		}
		applicationSet := make(map[uint64]struct{}, len(originStages)-start)
		for _, st := range originStages[start:] {
			applicationSet[st.ApplicationID] = struct{}{}
		}
		applicationIDs := make([]uint64, 0, len(applicationSet))
		for id := range applicationSet {
			applicationIDs = append(applicationIDs, id)
		}
		sort.Slice(applicationIDs, func(i, j int) bool { return applicationIDs[i] < applicationIDs[j] })
		for _, applicationID := range applicationIDs {
			if err := tx.LockApplication(ctx, applicationID); err != nil {
				return err
			}
		}
		now := s.now().UTC()
		firstState := models.DeliveryStageQueued
		runState := models.DeliveryRunQueued
		if originStages[start].ApprovalRequired {
			firstState, runState = models.DeliveryStageWaitingApproval, models.DeliveryRunWaitingApproval
		}
		row := &models.DeliveryRun{ProjectID: origin.ProjectID, PipelineID: origin.PipelineID, PipelineVersion: origin.PipelineVersion, ChartRepoID: origin.ChartRepoID, ChartName: origin.ChartName, ChartVersion: origin.ChartVersion, ChartDigest: origin.ChartDigest, InitiatorUserID: actor, IdempotencyKey: key, RequestFingerprint: fingerprint, State: runState, RetryOfRunID: &origin.ID, CreatedAt: now, UpdatedAt: now}
		if err := tx.CreateRun(ctx, row); err != nil {
			return err
		}
		rows := make([]models.DeliveryRunStage, len(originStages)-start)
		for i, source := range originStages[start:] {
			state := models.DeliveryStagePending
			if i == 0 {
				state = firstState
			}
			rows[i] = source
			rows[i].ID = 0
			rows[i].RunID = row.ID
			rows[i].StageOrder = i + 1
			rows[i].State = state
			rows[i].OperationID = fmt.Sprintf("delivery-run-%d-stage-%d", row.ID, i+1)
			rows[i].LeaseOwner = nil
			rows[i].LeaseExpiresAt = nil
			rows[i].ResultRevision = nil
			rows[i].ResultDigest = nil
			rows[i].StartedAt = nil
			rows[i].FinishedAt = nil
			rows[i].ErrorCode = nil
			rows[i].ErrorMessageKey = nil
			rows[i].CorrelationID = nil
			rows[i].CreatedAt = now
			rows[i].UpdatedAt = now
		}
		if err := tx.CreateStages(ctx, rows); err != nil {
			return err
		}
		if rows[0].ApprovalRequired {
			if err := tx.CreateApproval(ctx, &models.DeliveryApproval{RunID: row.ID, RunStageID: rows[0].ID, RequestedAt: now, Decision: models.DeliveryApprovalPending, CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		}
		if err := tx.AppendEvents(ctx, initialEvents(row, rows[0], actor, now)); err != nil {
			return err
		}
		result = runFrom(row, rows)
		created = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if created {
		s.recordAction(ctx, actor, ip, userAgent, "delivery.run.retry", result)
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

func (s *Service) recordAction(ctx context.Context, actor uint64, ip, userAgent, action string, row *Run) {
	if s.audit == nil || row == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{UserID: &actor, Action: action, TargetType: "delivery.run", TargetID: strconv.FormatUint(row.ID, 10), IP: ip, UserAgent: userAgent, Payload: map[string]any{"project_id": row.ProjectID, "state": row.State, "retry_of_run_id": row.RetryOfRunID}})
}

func transitionEvent(runID uint64, stageID *uint64, event, old, next string, actor uint64, now time.Time) models.DeliveryRunEvent {
	return models.DeliveryRunEvent{RunID: runID, RunStageID: stageID, EventType: event, OldState: &old, NewState: &next, ActorType: models.DeliveryEventActorUser, ActorID: &actor, OccurredAt: now, Metadata: datatypes.JSON([]byte(`{}`))}
}

// The recovery methods live here to keep Task 14's planned file boundary;
// they remain ordinary run-repository operations and never reach another
// module's tables.
func (r *Repo) GetRun(ctx context.Context, id uint64) (*models.DeliveryRun, error) {
	var row models.DeliveryRun
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(errs.CodeRunNotFound, errs.KeyRunNotFound, "delivery run not found")
	}
	return &row, err
}

func (r *Repo) LockRun(ctx context.Context, id uint64) (*models.DeliveryRun, error) {
	var row models.DeliveryRun
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(errs.CodeRunNotFound, errs.KeyRunNotFound, "delivery run not found")
	}
	return &row, err
}

func (r *Repo) LockRunStages(ctx context.Context, runID uint64) ([]models.DeliveryRunStage, error) {
	var rows []models.DeliveryRunStage
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).Order("stage_order ASC,id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repo) UpdateRun(ctx context.Context, id uint64, from, to models.DeliveryRunState, fields map[string]any) error {
	if !CanTransitionRun(from, to) {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "invalid delivery run transition")
	}
	updates := map[string]any{"state": to, "updated_at": time.Now().UTC()}
	for key, value := range fields {
		updates[key] = value
	}
	res := r.db.WithContext(ctx).Model(&models.DeliveryRun{}).Where("id = ? AND state = ?", id, from).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery run state changed concurrently")
	}
	return nil
}

func (r *Repo) UpdateStage(ctx context.Context, id uint64, from, to models.DeliveryStageState, fields map[string]any) error {
	if !CanTransitionStage(from, to) {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "invalid delivery stage transition")
	}
	updates := map[string]any{"state": to, "updated_at": time.Now().UTC()}
	for key, value := range fields {
		updates[key] = value
	}
	res := r.db.WithContext(ctx).Model(&models.DeliveryRunStage{}).Where("id = ? AND state = ?", id, from).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery stage state changed concurrently")
	}
	return nil
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
