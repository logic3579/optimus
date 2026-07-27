package pipeline

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Transaction(ctx context.Context, fn func(repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repo{db: tx})
	})
}

// LockProject establishes the project-owned publication protocol. NOWAIT
// turns concurrent publishers into a stable retryable conflict instead of
// letting both requests publish successive versions unexpectedly.
func (r *Repo) LockProject(ctx context.Context, projectID uint64) error {
	var project models.DeliveryProject
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"}).
		First(&project, projectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFoundError()
	}
	return mapPipelineWriteError(err)
}

func (r *Repo) ListEnvironments(ctx context.Context, projectID uint64) ([]models.DeliveryEnvironment, error) {
	var rows []models.DeliveryEnvironment
	err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repo) GetCurrent(ctx context.Context, projectID uint64) (*models.DeliveryPipeline, []models.DeliveryPipelineStage, error) {
	var row models.DeliveryPipeline
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND is_current", projectID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var stages []models.DeliveryPipelineStage
	if err := r.db.WithContext(ctx).
		Where("pipeline_id = ?", row.ID).
		Order("stage_order ASC, id ASC").Find(&stages).Error; err != nil {
		return nil, nil, err
	}
	return &row, stages, nil
}

func (r *Repo) ClearCurrent(ctx context.Context, projectID uint64) error {
	return mapPipelineWriteError(r.db.WithContext(ctx).
		Model(&models.DeliveryPipeline{}).
		Where("project_id = ? AND is_current", projectID).
		UpdateColumn("is_current", false).Error)
}

func (r *Repo) CreatePipeline(ctx context.Context, row *models.DeliveryPipeline, stages []models.DeliveryPipelineStage) error {
	if err := validatePipelineStages(stages); err != nil {
		return err
	}
	if err := mapPipelineWriteError(r.db.WithContext(ctx).Create(row).Error); err != nil {
		return err
	}
	for i := range stages {
		stages[i].PipelineID = row.ID
	}
	return mapPipelineWriteError(r.db.WithContext(ctx).Create(&stages).Error)
}

func validatePipelineStages(stages []models.DeliveryPipelineStage) error {
	for i := range stages {
		if stages[i].Executor != models.DeliveryExecutorHelmUpgradeExistingRelease {
			return pipelineInvalidError("unsupported pipeline executor")
		}
	}
	return nil
}

func mapPipelineWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.Code == "55P03" {
		return pipelineVersionConflictError()
	}
	if pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "delivery_pipelines_project_version_unique", "delivery_pipelines_current_project_unique":
			return pipelineVersionConflictError()
		}
	}
	return err
}

func projectNotFoundError() error {
	return apperr.New(errs.CodeProjectNotFound, errs.KeyProjectNotFound, "delivery project not found")
}

func pipelineInvalidError(message string) error {
	return apperr.New(errs.CodePipelineInvalid, errs.KeyPipelineInvalid, message)
}

func pipelineVersionConflictError() error {
	return apperr.New(errs.CodePipelineVersionConflict, errs.KeyPipelineVersionConflict, "pipeline publication conflicted; retry the request")
}
