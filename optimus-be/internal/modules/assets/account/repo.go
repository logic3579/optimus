package account

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/errs"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, row *models.CloudAccount) error {
	return mapWriteError(r.db.WithContext(ctx).Create(row).Error)
}

func (r *Repo) FindByID(ctx context.Context, id uint64) (*models.CloudAccount, error) {
	return findByID(ctx, r.db, id)
}

// FindByIDForUpdate locks one live account for mutation. Task 8 sweep
// persistence must take this lock and recheck both account liveness and the
// enabled region before writing discovered resources.
func (r *Repo) FindByIDForUpdate(ctx context.Context, tx *gorm.DB, id uint64) (*models.CloudAccount, error) {
	return findByID(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), id)
}

func findByID(ctx context.Context, db *gorm.DB, id uint64) (*models.CloudAccount, error) {
	var row models.CloudAccount
	if err := db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(errs.CodeAssetsCloudAccountNotFound, errs.KeyCloudAccountNotFound, "cloud account not found")
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return mapWriteError(r.db.WithContext(ctx).
		Model(&models.CloudAccount{}).
		Where("id = ?", id).
		Updates(fields).Error)
}

func (r *Repo) UpdateTx(ctx context.Context, tx *gorm.DB, id uint64, fields map[string]any) (int64, error) {
	if len(fields) == 0 {
		return 0, nil
	}
	result := tx.WithContext(ctx).
		Model(&models.CloudAccount{}).
		Where("id = ?", id).
		Updates(fields)
	return result.RowsAffected, mapWriteError(result.Error)
}

func (r *Repo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.CloudAccount{}, id).Error
}

func (r *Repo) SoftDeleteTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	result := tx.WithContext(ctx).Delete(&models.CloudAccount{}, id)
	return result.RowsAffected, result.Error
}

func (r *Repo) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *Repo) List(ctx context.Context, q ListQuery) ([]Summary, int64, error) {
	_, size, offset, err := paginationOffset(q.Page, q.Size)
	if err != nil {
		return nil, 0, err
	}
	tx := r.db.WithContext(ctx).Model(&models.CloudAccount{})
	if q.IncludeDeleted {
		tx = tx.Unscoped()
	}
	if value := strings.TrimSpace(q.Q); value != "" {
		tx = tx.Where("name ILIKE ?", "%"+value+"%")
	}
	if provider := strings.TrimSpace(q.Provider); provider != "" {
		tx = tx.Where("provider = ?", provider)
	}
	if q.Enabled != nil {
		tx = tx.Where("enabled = ?", *q.Enabled)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.CloudAccount
	if err := tx.Order("id DESC").Limit(size).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	accountIDs := make([]uint64, 0, len(rows))
	for i := range rows {
		accountIDs = append(accountIDs, rows[i].ID)
	}
	latest, err := r.latestSyncRuns(ctx, accountIDs)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Summary, 0, len(rows))
	for i := range rows {
		sync := latest[rows[i].ID]
		items = append(items, toSummary(rows[i], "", sync.startedAt, sync.status))
	}
	return items, total, nil
}

func paginationOffset(page, size int) (int, int, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if page-1 > math.MaxInt/size {
		return 0, 0, 0, apperr.New(apperr.CodeValidation, "common.validation", "pagination offset is too large")
	}
	return page, size, (page - 1) * size, nil
}

func (r *Repo) FindNameAlive(ctx context.Context, name string, excludeID uint64) (bool, error) {
	return findNameAlive(ctx, r.db, name, excludeID)
}

func (r *Repo) FindNameAliveTx(ctx context.Context, tx *gorm.DB, name string, excludeID uint64) (bool, error) {
	return findNameAlive(ctx, tx, name, excludeID)
}

func findNameAlive(ctx context.Context, db *gorm.DB, name string, excludeID uint64) (bool, error) {
	tx := db.WithContext(ctx).Model(&models.CloudAccount{}).Where("name = ?", name)
	if excludeID != 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	var count int64
	if err := tx.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repo) CountByCloudKeyID(ctx context.Context, cloudKeyID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.CloudAccount{}).
		Where("cloudkey_id = ?", cloudKeyID).
		Count(&count).Error
	return count, err
}

func (r *Repo) CascadeSoftDeleteResources(ctx context.Context, tx *gorm.DB, accountID uint64, regions []string) (int64, error) {
	if tx == nil {
		tx = r.db
	}
	tx = tx.WithContext(ctx)
	modelsToDelete := []any{
		&models.AWSInstance{},
		&models.AWSVPC{},
		&models.AWSSubnet{},
		&models.AWSDatabase{},
	}
	var total int64
	for _, model := range modelsToDelete {
		query := tx.Where("cloud_account_id = ?", accountID)
		if len(regions) > 0 {
			query = query.Where("region IN ?", regions)
		}
		result := query.Delete(model)
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
	}
	return total, nil
}

func (r *Repo) CloudKeyNames(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	names := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var rows []struct {
		ID   uint64
		Name string
	}
	if err := r.db.WithContext(ctx).
		Model(&models.CredentialCloudKey{}).
		Select("id", "name").
		Where("id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

type latestSync struct {
	startedAt *time.Time
	status    string
}

func (r *Repo) latestSyncRuns(ctx context.Context, accountIDs []uint64) (map[uint64]latestSync, error) {
	result := make(map[uint64]latestSync, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		CloudAccountID uint64
		StartedAt      time.Time
		Status         string
	}
	if err := r.db.WithContext(ctx).
		Model(&models.AssetsSyncRun{}).
		Select("DISTINCT ON (cloud_account_id) cloud_account_id, started_at, status").
		Where("cloud_account_id IN ?", accountIDs).
		Order("cloud_account_id, started_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		startedAt := row.StartedAt
		result[row.CloudAccountID] = latestSync{startedAt: &startedAt, status: row.Status}
	}
	return result, nil
}

func toSummary(row models.CloudAccount, cloudKeyName string, lastSyncAt *time.Time, lastSyncStatus string) Summary {
	return Summary{
		ID:             row.ID,
		Name:           row.Name,
		Provider:       row.Provider,
		CloudKeyID:     row.CloudKeyID,
		CloudKeyName:   cloudKeyName,
		RegionsCount:   len(row.EnabledRegions),
		Enabled:        row.Enabled,
		LastSyncAt:     lastSyncAt,
		LastSyncStatus: lastSyncStatus,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func toDetail(row models.CloudAccount, cloudKeyName string, lastSyncAt *time.Time, lastSyncStatus string) Detail {
	return Detail{
		Summary:        toSummary(row, cloudKeyName, lastSyncAt, lastSyncStatus),
		EnabledRegions: append([]string(nil), row.EnabledRegions...),
		Description:    row.Description,
	}
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_cloud_accounts_name_alive" {
		return apperr.Wrap(err, errs.CodeAssetsCloudAccountNameConflict, errs.KeyCloudAccountNameConflict, "cloud account name already exists")
	}
	return err
}
