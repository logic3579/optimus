package project

import (
	"context"
	"errors"
	"math"
	"strings"

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

func (r *Repo) Transaction(ctx context.Context, fn func(projectRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&Repo{db: tx}) })
}

func (r *Repo) ListProjects(ctx context.Context, q ListQuery) ([]models.DeliveryProject, int64, error) {
	_, size, offset, err := pageValues(q.Page, q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	tx := r.db.WithContext(ctx).Model(&models.DeliveryProject{})
	if search := strings.TrimSpace(q.Q); search != "" {
		tx = tx.Where("name ILIKE ?", "%"+search+"%")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.DeliveryProject
	if err := tx.Order("id DESC").Limit(size).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) GetProject(ctx context.Context, id uint64) (*models.DeliveryProject, error) {
	var row models.DeliveryProject
	return &row, mapProjectReadError(r.db.WithContext(ctx).First(&row, id).Error)
}

func (r *Repo) LockProject(ctx context.Context, id uint64) (*models.DeliveryProject, error) {
	var row models.DeliveryProject
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error
	return &row, mapProjectReadError(err)
}

func (r *Repo) LockApplication(ctx context.Context, id uint64) error {
	return advisorylock.LockApplication(ctx, r.db, id)
}

func (r *Repo) CreateProject(ctx context.Context, row *models.DeliveryProject) error {
	return mapProjectWriteError(r.db.WithContext(ctx).Create(row).Error)
}

func (r *Repo) UpdateProject(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&models.DeliveryProject{}).Where("id = ?", id).Updates(fields)
	if err := mapProjectWriteError(result.Error); err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return projectNotFoundError()
	}
	return nil
}

func (r *Repo) DeleteProject(ctx context.Context, id uint64) error {
	result := r.db.WithContext(ctx).Delete(&models.DeliveryProject{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return projectNotFoundError()
	}
	return nil
}

func (r *Repo) ListEnvironments(ctx context.Context, projectID uint64) ([]models.DeliveryEnvironment, error) {
	var rows []models.DeliveryEnvironment
	err := r.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("environment_key ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repo) GetEnvironment(ctx context.Context, projectID, id uint64) (*models.DeliveryEnvironment, error) {
	var row models.DeliveryEnvironment
	err := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, id).First(&row).Error
	return &row, mapEnvironmentReadError(err)
}

func (r *Repo) ApplicationBound(ctx context.Context, applicationID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.DeliveryEnvironment{}).
		Where("application_id = ?", applicationID).Count(&count).Error
	return count > 0, err
}

func (r *Repo) CountByApplicationID(ctx context.Context, applicationID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.DeliveryEnvironment{}).
		Where("application_id = ?", applicationID).Count(&count).Error
	return count, err
}

func (r *Repo) CreateEnvironment(ctx context.Context, row *models.DeliveryEnvironment) error {
	return mapEnvironmentWriteError(r.db.WithContext(ctx).Create(row).Error)
}

func (r *Repo) UpdateEnvironment(ctx context.Context, projectID, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&models.DeliveryEnvironment{}).
		Where("project_id = ? AND id = ?", projectID, id).Updates(fields)
	if err := mapEnvironmentWriteError(result.Error); err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return environmentNotFoundError()
	}
	return nil
}

func (r *Repo) DeleteEnvironment(ctx context.Context, projectID, id uint64) error {
	result := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, id).
		Delete(&models.DeliveryEnvironment{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return environmentNotFoundError()
	}
	return nil
}

func (r *Repo) CountActiveEnvironments(ctx context.Context, projectID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.DeliveryEnvironment{}).
		Where("project_id = ?", projectID).Count(&count).Error
	return count, err
}

func (r *Repo) CountPipelineReferences(ctx context.Context, environmentID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("delivery_pipeline_stages AS s").
		Joins("JOIN delivery_pipelines AS p ON p.id = s.pipeline_id").
		Where("s.environment_id = ? AND p.is_current", environmentID).Count(&count).Error
	return count, err
}

func pageValues(page, size int) (int, int, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	if page-1 > math.MaxInt/size {
		return 0, 0, 0, validationError("pagination is too large")
	}
	return page, size, (page - 1) * size, nil
}

func mapProjectReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return projectNotFoundError()
	}
	return err
}

func mapEnvironmentReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return environmentNotFoundError()
	}
	return err
}

func mapProjectWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "delivery_projects_active_name_unique" {
		return projectNameConflictError()
	}
	return err
}

func mapEnvironmentWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "delivery_environments_active_application_unique":
		return applicationAlreadyBoundError()
	case "delivery_environments_active_project_key_unique":
		return environmentInUseError()
	default:
		return err
	}
}

func projectNotFoundError() error {
	return apperr.New(errs.CodeProjectNotFound, errs.KeyProjectNotFound, "delivery project not found")
}

func projectNameConflictError() error {
	return apperr.New(errs.CodeProjectNameConflict, errs.KeyProjectNameConflict, "an active delivery project already uses this name")
}

func environmentNotFoundError() error {
	return apperr.New(errs.CodeEnvironmentNotFound, errs.KeyEnvironmentNotFound, "delivery environment not found")
}

func applicationAlreadyBoundError() error {
	return apperr.New(errs.CodeApplicationAlreadyBound, errs.KeyApplicationAlreadyBound, "application is already bound to an active environment")
}

func environmentInUseError() error {
	return apperr.New(errs.CodeEnvironmentInUse, errs.KeyEnvironmentInUse, "delivery environment is in use")
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}
