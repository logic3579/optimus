package datasource

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }
func (r *Repo) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}
func (r *Repo) List(ctx context.Context, q ListQuery) ([]Detail, int64, error) {
	page, size, offset, err := pageValues(q.Page, q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	tx := r.db.WithContext(ctx).Table("observability_datasources d").Where("d.deleted_at IS NULL")
	if v := strings.TrimSpace(q.Q); v != "" {
		tx = tx.Where("d.name ILIKE ?", "%"+v+"%")
	}
	if q.AuthType != "" {
		tx = tx.Where("d.auth_type = ?", q.AuthType)
	}
	if q.ClusterID != nil {
		tx = tx.Where("d.cluster_id = ?", *q.ClusterID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	type joined struct {
		models.ObservabilityDatasource
		CredentialName, ClusterName string
	}
	var rows []joined
	err = tx.Select("d.*, hc.name credential_name, c.name cluster_name").Joins("LEFT JOIN credentials_http_credentials hc ON hc.id=d.http_credential_id AND hc.deleted_at IS NULL").Joins("LEFT JOIN clusters c ON c.id=d.cluster_id AND c.deleted_at IS NULL").Order("d.id DESC").Limit(size).Offset(offset).Scan(&rows).Error
	items := make([]Detail, 0, len(rows))
	for i := range rows {
		items = append(items, *detailFromModel(&rows[i].ObservabilityDatasource, rows[i].CredentialName, rows[i].ClusterName))
	}
	_ = page
	return items, total, err
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
		return 0, 0, 0, apperr.New(apperr.CodeValidation, "common.validation", "pagination too large")
	}
	return page, size, (page - 1) * size, nil
}
func (r *Repo) GetByID(ctx context.Context, id uint64) (*Detail, error) {
	type joined struct {
		models.ObservabilityDatasource
		CredentialName, ClusterName string
	}
	var row joined
	err := r.db.WithContext(ctx).Table("observability_datasources d").Select("d.*, hc.name credential_name, c.name cluster_name").Joins("LEFT JOIN credentials_http_credentials hc ON hc.id=d.http_credential_id AND hc.deleted_at IS NULL").Joins("LEFT JOIN clusters c ON c.id=d.cluster_id AND c.deleted_at IS NULL").Where("d.id=? AND d.deleted_at IS NULL", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if err != nil {
		return nil, err
	}
	return detailFromModel(&row.ObservabilityDatasource, row.CredentialName, row.ClusterName), nil
}
func (r *Repo) GetModelForUpdate(ctx context.Context, tx *gorm.DB, id uint64) (*models.ObservabilityDatasource, error) {
	var row models.ObservabilityDatasource
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	return &row, err
}
func (r *Repo) FindNameAliveTx(ctx context.Context, tx *gorm.DB, name string, exclude uint64) (bool, error) {
	q := tx.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("name=?", name)
	if exclude != 0 {
		q = q.Where("id<>?", exclude)
	}
	var n int64
	err := q.Count(&n).Error
	return n > 0, err
}
func (r *Repo) Create(ctx context.Context, m *models.ObservabilityDatasource) error {
	return mapWriteError(r.db.WithContext(ctx).Create(m).Error)
}
func (r *Repo) CreateTx(ctx context.Context, tx *gorm.DB, m *models.ObservabilityDatasource) error {
	return mapWriteError(tx.WithContext(ctx).Create(m).Error)
}
func (r *Repo) Update(ctx context.Context, id uint64, f map[string]any) error {
	return mapWriteError(r.db.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("id=?", id).Updates(f).Error)
}
func (r *Repo) UpdateTx(ctx context.Context, tx *gorm.DB, id uint64, f map[string]any) (int64, error) {
	res := tx.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("id=?", id).Updates(f)
	return res.RowsAffected, mapWriteError(res.Error)
}
func (r *Repo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.ObservabilityDatasource{}, id).Error
}
func (r *Repo) SoftDeleteTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	res := tx.WithContext(ctx).Delete(&models.ObservabilityDatasource{}, id)
	return res.RowsAffected, res.Error
}
func (r *Repo) CountByHTTPCredentialID(ctx context.Context, id uint64) (int64, error) {
	return countByHTTPCredentialID(ctx, r.db, id)
}
func (r *Repo) CountByHTTPCredentialIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	return countByHTTPCredentialID(ctx, tx, id)
}
func countByHTTPCredentialID(ctx context.Context, db *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("http_credential_id=?", id).Count(&n).Error
	return n, err
}
func (r *Repo) CountByClusterID(ctx context.Context, id uint64) (int64, error) {
	return countByClusterID(ctx, r.db, id)
}
func (r *Repo) CountByClusterIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	return countByClusterID(ctx, tx, id)
}
func countByClusterID(ctx context.Context, db *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.ObservabilityDatasource{}).Where("cluster_id=?", id).Count(&n).Error
	return n, err
}
func mapWriteError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" && p.ConstraintName == "observability_datasource_name_unique" {
		return nameTaken()
	}
	return err
}
func detailFromModel(m *models.ObservabilityDatasource, credentialName, clusterName string) *Detail {
	d := &Detail{ID: m.ID, Name: m.Name, BaseURL: m.BaseURL, AuthType: m.AuthType, TLSSkipVerify: m.TLSSkipVerify, HasCustomCA: m.CustomCAPEM != nil, Description: m.Description, CreatedByUserID: m.CreatedByUserID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	if m.CustomCAPEM != nil {
		d.customCAPEM = *m.CustomCAPEM
	}
	if m.HTTPCredentialID != nil {
		d.HTTPCredential = &NamedRef{*m.HTTPCredentialID, credentialName}
	}
	if m.ClusterID != nil {
		d.Cluster = &NamedRef{*m.ClusterID, clusterName}
	}
	return d
}
func notFound() error {
	return apperr.New(apperr.CodeObservabilityDatasourceNotFound, "observability.datasource.not_found", "data source not found")
}
func nameTaken() error {
	return apperr.New(apperr.CodeObservabilityDatasourceNameTaken, "observability.datasource.name_taken", "data source name already exists")
}
