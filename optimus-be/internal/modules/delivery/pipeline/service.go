package pipeline

import (
	"context"
	"strconv"
	"time"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

type repository interface {
	Transaction(ctx context.Context, fn func(repository) error) error
	LockProject(ctx context.Context, projectID uint64) error
	ListEnvironments(ctx context.Context, projectID uint64) ([]models.DeliveryEnvironment, error)
	GetCurrent(ctx context.Context, projectID uint64) (*models.DeliveryPipeline, []models.DeliveryPipelineStage, error)
	ClearCurrent(ctx context.Context, projectID uint64) error
	CreatePipeline(ctx context.Context, row *models.DeliveryPipeline, stages []models.DeliveryPipelineStage) error
}

// Recorder deliberately exposes only the shared audit write operation.
type Recorder interface {
	Record(ctx context.Context, event audit.Event) error
}

type Service struct {
	repo            repository
	audit           Recorder
	maxStageTimeout time.Duration
	now             func() time.Time
}

func NewService(repo repository, recorder Recorder, maxStageTimeout time.Duration) *Service {
	return &Service{repo: repo, audit: recorder, maxStageTimeout: maxStageTimeout, now: time.Now}
}

func (s *Service) Publish(ctx context.Context, actor uint64, ip, userAgent string, projectID uint64, req PublishRequest) (*Pipeline, error) {
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	var row models.DeliveryPipeline
	var stages []models.DeliveryPipelineStage
	if err := s.repo.Transaction(ctx, func(repo repository) error {
		if err := repo.LockProject(ctx, projectID); err != nil {
			return err
		}
		environments, err := repo.ListEnvironments(ctx, projectID)
		if err != nil {
			return err
		}
		bound := make(map[uint64]struct{}, len(environments))
		for i := range environments {
			bound[environments[i].ID] = struct{}{}
		}
		for i := range req.Stages {
			if _, ok := bound[req.Stages[i].EnvironmentID]; !ok {
				return pipelineInvalidError("pipeline stage environment is not bound to the project")
			}
		}

		current, _, err := repo.GetCurrent(ctx, projectID)
		if err != nil {
			return err
		}
		version := 1
		if current != nil {
			version = current.Version + 1
		}
		row = models.DeliveryPipeline{
			ProjectID: projectID, Version: version, CreatedByUserID: actor,
			PublishedAt: s.now().UTC(), IsCurrent: true,
		}
		stages = make([]models.DeliveryPipelineStage, len(req.Stages))
		for i := range req.Stages {
			stages[i] = models.DeliveryPipelineStage{
				EnvironmentID:    req.Stages[i].EnvironmentID,
				StageOrder:       i + 1,
				Executor:         models.DeliveryExecutorHelmUpgradeExistingRelease,
				ApprovalRequired: req.Stages[i].ApprovalRequired,
				TimeoutSeconds:   int(req.Stages[i].Timeout / time.Second),
			}
		}
		if err := repo.ClearCurrent(ctx, projectID); err != nil {
			return err
		}
		return repo.CreatePipeline(ctx, &row, stages)
	}); err != nil {
		return nil, err
	}

	result := pipelineFrom(&row, stages)
	s.record(ctx, actor, ip, userAgent, result)
	return result, nil
}

func (s *Service) validateRequest(req PublishRequest) error {
	if len(req.Stages) < 1 || len(req.Stages) > 20 {
		return pipelineInvalidError("pipeline must contain between one and twenty stages")
	}
	seen := make(map[uint64]struct{}, len(req.Stages))
	for i := range req.Stages {
		stage := req.Stages[i]
		if stage.EnvironmentID == 0 {
			return pipelineInvalidError("pipeline stage environment is required")
		}
		if _, duplicate := seen[stage.EnvironmentID]; duplicate {
			return pipelineInvalidError("pipeline environments must be unique")
		}
		seen[stage.EnvironmentID] = struct{}{}
		if stage.Timeout <= 0 || stage.Timeout%time.Second != 0 {
			return pipelineInvalidError("pipeline stage timeout must be a positive whole number of seconds")
		}
		if s.maxStageTimeout <= 0 || stage.Timeout > s.maxStageTimeout {
			return pipelineInvalidError("pipeline stage timeout exceeds the configured maximum")
		}
	}
	return nil
}

func pipelineFrom(row *models.DeliveryPipeline, stages []models.DeliveryPipelineStage) *Pipeline {
	out := &Pipeline{
		ID: row.ID, ProjectID: row.ProjectID, Version: row.Version,
		CreatedByUserID: row.CreatedByUserID, PublishedAt: row.PublishedAt,
		IsCurrent: row.IsCurrent, Stages: make([]Stage, len(stages)),
	}
	for i := range stages {
		out.Stages[i] = Stage{
			ID: stages[i].ID, EnvironmentID: stages[i].EnvironmentID,
			Order: stages[i].StageOrder, ApprovalRequired: stages[i].ApprovalRequired,
			Timeout: time.Duration(stages[i].TimeoutSeconds) * time.Second,
		}
	}
	return out
}

func (s *Service) record(ctx context.Context, actor uint64, ip, userAgent string, published *Pipeline) {
	if s.audit == nil {
		return
	}
	stageIDs := make([]uint64, len(published.Stages))
	environmentIDs := make([]uint64, len(published.Stages))
	for i := range published.Stages {
		stageIDs[i] = published.Stages[i].ID
		environmentIDs[i] = published.Stages[i].EnvironmentID
	}
	var userID *uint64
	if actor != 0 {
		userID = &actor
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: userID, Action: "delivery.pipeline.publish", TargetType: "delivery.pipeline",
		TargetID: strconv.FormatUint(published.ID, 10), IP: ip, UserAgent: userAgent,
		Payload: map[string]any{
			"version": published.Version, "stage_ids": stageIDs, "environment_ids": environmentIDs,
		},
	})
}
