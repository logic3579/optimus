package dashboard

import (
	"context"
	"errors"
	"math"
	"sort"
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
func (r *Repo) List(ctx context.Context, q ListQuery) ([]Detail, int64, error) {
	page, size, offset, err := pageValues(q.Page, q.PageSize)
	_ = page
	if err != nil {
		return nil, 0, err
	}
	tx := r.db.WithContext(ctx).Model(&models.ObservabilityDashboard{})
	if v := strings.TrimSpace(q.Q); v != "" {
		tx = tx.Where("name ILIKE ?", "%"+v+"%")
	}
	var total int64
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.ObservabilityDashboard
	err = tx.Order("id DESC").Limit(size).Offset(offset).Find(&rows).Error
	out := make([]Detail, 0, len(rows))
	for i := range rows {
		out = append(out, *detailFromModel(&rows[i], nil))
	}
	return out, total, err
}
func (r *Repo) GetByID(ctx context.Context, id uint64) (*Detail, error) {
	var d models.ObservabilityDashboard
	if err := r.db.WithContext(ctx).First(&d, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound()
	} else if err != nil {
		return nil, err
	}
	var ps []models.ObservabilityPanel
	if err := r.db.WithContext(ctx).Where("dashboard_id=?", id).Order("sort_order ASC").Find(&ps).Error; err != nil {
		return nil, err
	}
	return detailFromModel(&d, ps), nil
}
func (r *Repo) GetModelForUpdate(ctx context.Context, tx *gorm.DB, id uint64) (*models.ObservabilityDashboard, error) {
	var d models.ObservabilityDashboard
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	return &d, err
}
func (r *Repo) FindNameAliveTx(ctx context.Context, tx *gorm.DB, name string, exclude uint64) (bool, error) {
	q := tx.WithContext(ctx).Model(&models.ObservabilityDashboard{}).Where("name=?", name)
	if exclude != 0 {
		q = q.Where("id<>?", exclude)
	}
	var n int64
	err := q.Count(&n).Error
	return n > 0, err
}
func (r *Repo) LockActiveDatasourcesTx(ctx context.Context, tx *gorm.DB, ids []uint64) error {
	ids = append([]uint64(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var rows []models.ObservabilityDatasource
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(ids) {
		return invalidPanel("data source not found")
	}
	return nil
}
func (r *Repo) CreateTx(ctx context.Context, tx *gorm.DB, d *models.ObservabilityDashboard) error {
	return mapWriteError(tx.WithContext(ctx).Create(d).Error)
}
func (r *Repo) UpdateTx(ctx context.Context, tx *gorm.DB, id uint64, fields map[string]any) (int64, error) {
	res := tx.WithContext(ctx).Model(&models.ObservabilityDashboard{}).Where("id=?", id).Updates(fields)
	return res.RowsAffected, mapWriteError(res.Error)
}
func (r *Repo) ReplacePanelsTx(ctx context.Context, tx *gorm.DB, id uint64, ps []models.ObservabilityPanel) error {
	if err := tx.WithContext(ctx).Where("dashboard_id=?", id).Delete(&models.ObservabilityPanel{}).Error; err != nil {
		return err
	}
	if len(ps) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&ps).Error
}
func (r *Repo) CountPanelsTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := tx.WithContext(ctx).Model(&models.ObservabilityPanel{}).Where("dashboard_id=?", id).Count(&n).Error
	return n, err
}
func (r *Repo) DeleteTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	if err := tx.WithContext(ctx).Where("dashboard_id=?", id).Delete(&models.ObservabilityPanel{}).Error; err != nil {
		return 0, err
	}
	res := tx.WithContext(ctx).Delete(&models.ObservabilityDashboard{}, id)
	return res.RowsAffected, res.Error
}
func (r *Repo) CountByDatasourceIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := tx.WithContext(ctx).Table("observability_panels p").Joins("JOIN observability_dashboards d ON d.id=p.dashboard_id AND d.deleted_at IS NULL").Where("p.datasource_id=?", id).Count(&n).Error
	return n, err
}
func mapWriteError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" && p.ConstraintName == "observability_dashboard_name_unique" {
		return nameTaken()
	}
	return err
}
func detailFromModel(d *models.ObservabilityDashboard, ps []models.ObservabilityPanel) *Detail {
	out := &Detail{ID: d.ID, Name: d.Name, Description: d.Description, RefreshIntervalS: d.RefreshIntervalS, TimeRange: d.TimeRange, CreatedByUserID: d.CreatedByUserID, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Panels: make([]Panel, 0, len(ps))}
	for _, p := range ps {
		out.Panels = append(out.Panels, Panel{p.ID, p.DashboardID, p.DatasourceID, p.Title, p.PanelType, p.PromQL, p.Unit, p.Legend, p.SortOrder, p.Width, p.CreatedAt, p.UpdatedAt})
	}
	return out
}
func notFound() error {
	return apperr.New(apperr.CodeObservabilityDashboardNotFound, "observability.dashboard.not_found", "dashboard not found")
}
func nameTaken() error {
	return apperr.New(apperr.CodeObservabilityDashboardNameTaken, "observability.dashboard.name_taken", "dashboard name already exists")
}
func invalidPanel(msg string) error {
	return apperr.New(apperr.CodeObservabilityDashboardInvalidPanel, "observability.dashboard.invalid_panel", msg)
}
