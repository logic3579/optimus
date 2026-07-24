package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

type auditWriter interface {
	Record(context.Context, audit.Event) error
}
type repository interface {
	List(context.Context, ListQuery) ([]Detail, int64, error)
	GetByID(context.Context, uint64) (*Detail, error)
	Transaction(context.Context, func(*gorm.DB) error) error
	GetModelForUpdate(context.Context, *gorm.DB, uint64) (*models.ObservabilityDashboard, error)
	FindNameAliveTx(context.Context, *gorm.DB, string, uint64) (bool, error)
	LockActiveDatasourcesTx(context.Context, *gorm.DB, []uint64) error
	CreateTx(context.Context, *gorm.DB, *models.ObservabilityDashboard) error
	UpdateTx(context.Context, *gorm.DB, uint64, map[string]any) (int64, error)
	ReplacePanelsTx(context.Context, *gorm.DB, uint64, []models.ObservabilityPanel) error
	CountPanelsTx(context.Context, *gorm.DB, uint64) (int64, error)
	DeleteTx(context.Context, *gorm.DB, uint64) (int64, error)
}
type Service struct {
	repo    repository
	auditTx func(*gorm.DB) auditWriter
}

func NewService(r *Repo, a *audit.Recorder) *Service {
	return &Service{repo: r, auditTx: func(tx *gorm.DB) auditWriter { return a.WithTx(tx) }}
}
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	items, n, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	p, z, _, err := pageValues(q.Page, q.PageSize)
	return &ListResponse{items, n, p, z}, err
}
func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	return s.repo.GetByID(ctx, id)
}
func validateSave(r SaveRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len(r.Name) > 128 || len(r.Description) > 4096 {
		return invalidPanel("invalid dashboard metadata")
	}
	if r.RefreshIntervalS < 15 || r.RefreshIntervalS > 3600 {
		return invalidPanel("invalid refresh interval")
	}
	switch r.TimeRange {
	case "15m", "1h", "6h", "24h", "7d":
	default:
		return invalidPanel("invalid time range")
	}
	if len(r.Panels) > 100 {
		return invalidPanel("too many panels")
	}
	orders := map[int]bool{}
	for _, p := range r.Panels {
		if p.DatasourceID == 0 || p.DatasourceID > math.MaxInt64 || strings.TrimSpace(p.Title) == "" || len(strings.TrimSpace(p.Title)) > 128 || strings.TrimSpace(p.PromQL) == "" || len(p.PromQL) > 8192 || len(p.Legend) > 128 {
			return invalidPanel("invalid panel")
		}
		if p.PanelType != "time_series" && p.PanelType != "stat" && p.PanelType != "table" {
			return invalidPanel("invalid panel type")
		}
		if _, ok := Units[p.Unit]; !ok {
			return invalidPanel("invalid unit")
		}
		if p.Width != 6 && p.Width != 12 {
			return invalidPanel("invalid panel width")
		}
		if p.SortOrder < 0 || int64(p.SortOrder) > math.MaxInt32 || orders[p.SortOrder] {
			return invalidPanel("invalid panel order")
		}
		orders[p.SortOrder] = true
	}
	return nil
}
func datasourceIDs(ps []PanelInput) []uint64 {
	m := map[uint64]bool{}
	for _, p := range ps {
		m[p.DatasourceID] = true
	}
	ids := make([]uint64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func panelModels(id uint64, ps []PanelInput) []models.ObservabilityPanel {
	out := make([]models.ObservabilityPanel, 0, len(ps))
	for _, p := range ps {
		out = append(out, models.ObservabilityPanel{DashboardID: id, DatasourceID: p.DatasourceID, Title: strings.TrimSpace(p.Title), PanelType: p.PanelType, PromQL: strings.TrimSpace(p.PromQL), Unit: p.Unit, Legend: strings.TrimSpace(p.Legend), SortOrder: p.SortOrder, Width: p.Width})
	}
	return out
}
func panelFingerprints(ps []PanelInput) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		sum := sha256.Sum256([]byte(p.PromQL))
		out = append(out, hex.EncodeToString(sum[:]))
	}
	return out
}
func actorPtr(id uint64) *uint64 {
	if id == 0 {
		return nil
	}
	return &id
}
func (s *Service) Create(ctx context.Context, actor uint64, ip, ua string, r SaveRequest) (*Detail, error) {
	if err := validateSave(r); err != nil {
		return nil, err
	}
	d := &models.ObservabilityDashboard{Name: strings.TrimSpace(r.Name), Description: strings.TrimSpace(r.Description), RefreshIntervalS: r.RefreshIntervalS, TimeRange: r.TimeRange, CreatedByUserID: actorPtr(actor)}
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.repo.LockActiveDatasourcesTx(ctx, tx, datasourceIDs(r.Panels)); err != nil {
			return err
		}
		yes, err := s.repo.FindNameAliveTx(ctx, tx, d.Name, 0)
		if err != nil {
			return err
		}
		if yes {
			return nameTaken()
		}
		if err = s.repo.CreateTx(ctx, tx, d); err != nil {
			return err
		}
		if err = s.repo.ReplacePanelsTx(ctx, tx, d.ID, panelModels(d.ID, r.Panels)); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, "observability.dashboard.create", d.ID, ip, ua, map[string]any{"name": d.Name, "refresh_interval_s": d.RefreshIntervalS, "time_range": d.TimeRange, "panel_count": len(r.Panels), "panel_fingerprints": panelFingerprints(r.Panels)})
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, d.ID)
}
func (s *Service) Update(ctx context.Context, actor uint64, ip, ua string, id uint64, r SaveRequest) (*Detail, error) {
	if err := validateSave(r); err != nil {
		return nil, err
	}
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		old, err := s.repo.GetModelForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = s.repo.LockActiveDatasourcesTx(ctx, tx, datasourceIDs(r.Panels)); err != nil {
			return err
		}
		name := strings.TrimSpace(r.Name)
		yes, err := s.repo.FindNameAliveTx(ctx, tx, name, id)
		if err != nil {
			return err
		}
		if yes {
			return nameTaken()
		}
		fields := map[string]any{"name": name, "description": strings.TrimSpace(r.Description), "refresh_interval_s": r.RefreshIntervalS, "time_range": r.TimeRange}
		if _, err = s.repo.UpdateTx(ctx, tx, id, fields); err != nil {
			return err
		}
		if err = s.repo.ReplacePanelsTx(ctx, tx, id, panelModels(id, r.Panels)); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, "observability.dashboard.update", id, ip, ua, map[string]any{"changed_metadata": old.Name != name || old.Description != strings.TrimSpace(r.Description) || old.RefreshIntervalS != r.RefreshIntervalS || old.TimeRange != r.TimeRange, "panel_count": len(r.Panels), "panel_fingerprints": panelFingerprints(r.Panels)})
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}
func (s *Service) Delete(ctx context.Context, actor uint64, ip, ua string, id uint64) error {
	return s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		d, err := s.repo.GetModelForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		panelCount, err := s.repo.CountPanelsTx(ctx, tx, id)
		if err != nil {
			return err
		}
		n, err := s.repo.DeleteTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if n != 1 {
			return notFound()
		}
		return s.record(ctx, tx, actor, "observability.dashboard.delete", id, ip, ua, map[string]any{"name": d.Name, "panel_count": panelCount})
	})
}
func (s *Service) record(ctx context.Context, tx *gorm.DB, actor uint64, action string, id uint64, ip, ua string, p any) error {
	return s.auditTx(tx).Record(ctx, audit.Event{UserID: actorPtr(actor), Action: action, TargetType: "observability_dashboard", TargetID: strconv.FormatUint(id, 10), Payload: p, IP: ip, UserAgent: ua})
}
