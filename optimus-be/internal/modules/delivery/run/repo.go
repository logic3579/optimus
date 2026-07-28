package run

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"optimus-be/internal/infra/advisorylock"
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

// LockProject joins the project-owned lifecycle protocol used by project and
// pipeline mutations. All activity checks and snapshot inserts follow it in
// the same transaction.
func (r *Repo) LockProject(ctx context.Context, projectID uint64) error {
	var project models.DeliveryProject
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(errs.CodeProjectNotFound, errs.KeyProjectNotFound, "delivery project not found")
	}
	return err
}

func (r *Repo) LockApplication(ctx context.Context, applicationID uint64) error {
	return advisorylock.LockApplication(ctx, r.db, applicationID)
}

func (r *Repo) GetCurrent(ctx context.Context, projectID uint64) (*models.DeliveryPipeline, []models.DeliveryPipelineStage, error) {
	var pipeline models.DeliveryPipeline
	err := r.db.WithContext(ctx).Where("project_id = ? AND is_current", projectID).First(&pipeline).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var stages []models.DeliveryPipelineStage
	err = r.db.WithContext(ctx).Where("pipeline_id = ?", pipeline.ID).Order("stage_order ASC, id ASC").Find(&stages).Error
	return &pipeline, stages, err
}

func (r *Repo) ListEnvironments(ctx context.Context, projectID uint64) ([]models.DeliveryEnvironment, error) {
	var rows []models.DeliveryEnvironment
	err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repo) BlockingRun(ctx context.Context, projectID uint64) (*models.DeliveryRun, error) {
	var row models.DeliveryRun
	err := r.db.WithContext(ctx).Where("project_id = ? AND state IN ?", projectID, blockingRunStates()).
		Order("id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repo) FindByIdempotency(ctx context.Context, projectID, actor uint64, key string) (*models.DeliveryRun, error) {
	var row models.DeliveryRun
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND initiator_user_id = ? AND idempotency_key = ?", projectID, actor, key).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repo) ListRunStages(ctx context.Context, runID uint64) ([]models.DeliveryRunStage, error) {
	var rows []models.DeliveryRunStage
	err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("stage_order ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repo) CreateRun(ctx context.Context, row *models.DeliveryRun) error {
	return mapRunWriteError(r.db.WithContext(ctx).Create(row).Error)
}

func (r *Repo) CreateStages(ctx context.Context, rows []models.DeliveryRunStage) error {
	if len(rows) == 0 {
		return pipelineMissingError()
	}
	return mapRunWriteError(r.db.WithContext(ctx).Create(&rows).Error)
}

func (r *Repo) CreateApproval(ctx context.Context, row *models.DeliveryApproval) error {
	return mapRunWriteError(r.db.WithContext(ctx).Create(row).Error)
}

func (r *Repo) AppendEvents(ctx context.Context, rows []models.DeliveryRunEvent) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

func blockingRunStates() []models.DeliveryRunState {
	return []models.DeliveryRunState{
		models.DeliveryRunQueued,
		models.DeliveryRunRunning,
		models.DeliveryRunWaitingApproval,
		models.DeliveryRunCancelRequested,
		models.DeliveryRunReconciling,
		models.DeliveryRunOutcomeUnknown,
	}
}

func mapRunWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "delivery_runs_active_project_unique":
		return activeRunError()
	case "delivery_runs_idempotency_unique":
		return idempotencyConflictError()
	default:
		return err
	}
}

func pipelineMissingError() error {
	return apperr.New(errs.CodePipelineMissing, errs.KeyPipelineMissing, "delivery project has no current pipeline")
}

func activeRunError() error {
	return apperr.New(errs.CodeActiveRun, errs.KeyActiveRun, "delivery project has an active run")
}

func idempotencyConflictError() error {
	return apperr.New(errs.CodeIdempotencyConflict, errs.KeyIdempotencyConflict, "idempotency key was already used for a different run request")
}
