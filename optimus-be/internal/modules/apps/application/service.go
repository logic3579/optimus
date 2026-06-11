package application

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

// HelmStatusProbe is the seam release.Service implements. Service uses it to
// decorate Detail with live status on Get. It is OPTIONAL — passing nil
// returns Detail with empty status fields. Wired by main.go.
type HelmStatusProbe interface {
	StatusForApplication(ctx context.Context, app *models.AppsApplication) (status string, revision *int, chartVersion, appVersion, lastDeployedAt string, err error)
}

// HelmInstalledChecker is the seam Service.Delete uses to refuse delete when
// the helm release still exists. Wired by main.go.
type HelmInstalledChecker interface {
	IsReleaseInstalled(ctx context.Context, app *models.AppsApplication) (bool, error)
}

// Service owns audit emission and the optional helm seams.
type Service struct {
	repo    *Repo
	audit   *audit.Recorder
	probe   HelmStatusProbe
	checker HelmInstalledChecker
}

// NewService returns a Service bound to a repo + audit recorder. The helm
// seams are wired post-construction (so this package stays free of helm
// imports).
func NewService(r *Repo, rec *audit.Recorder) *Service {
	return &Service{repo: r, audit: rec}
}

// Repo exposes the underlying repo for tests + main.go wiring.
func (s *Service) Repo() *Repo { return s.repo }

// SetHelmStatusProbe wires the live-status decorator.
func (s *Service) SetHelmStatusProbe(p HelmStatusProbe) { s.probe = p }

// SetHelmInstalledChecker wires the delete pre-check.
func (s *Service) SetHelmInstalledChecker(c HelmInstalledChecker) { s.checker = c }

// --- queries ---------------------------------------------------------------

// List returns one page of applications as Summary rows.
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(rows))
	for i := range rows {
		items = append(items, toDetail(&rows[i]).Summary)
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// GetModel returns the underlying *models.AppsApplication (with Preload-d
// associations). Used by release.Service which needs the raw row to derive
// cluster/namespace/release-name for helm SDK calls. Maps gorm.ErrRecordNotFound
// to the apps.application NotFound BizError so callers don't have to.
func (s *Service) GetModel(ctx context.Context, id uint64) (*models.AppsApplication, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found", "application not found")
		}
		return nil, err
	}
	return m, nil
}

// Get returns one application as a Detail, decorated with live helm status if
// the probe seam is wired.
func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found", "application not found")
		}
		return nil, err
	}
	d := toDetail(m)
	if s.probe != nil {
		status, rev, cv, av, ldp, perr := s.probe.StatusForApplication(ctx, m)
		if perr == nil {
			d.Status = status
			d.Revision = rev
			d.ChartVersion = cv
			d.AppVersion = av
			d.LastDeployedAt = ldp
		}
	}
	return d, nil
}

// --- mutations -------------------------------------------------------------

// Create persists a new application row after enforcing name and release-tuple
// uniqueness at the service layer (cheap pre-check; the partial unique index
// is still the source of truth at the DB level).
func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	// Name uniqueness (live row).
	if _, err := s.repo.FindByName(ctx, req.Name); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "apps.application.name_taken", "application name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Release-tuple uniqueness (cluster_id, namespace, release_name).
	if _, err := s.repo.FindByReleaseTuple(ctx, req.ClusterID, req.Namespace, req.ReleaseName); err == nil {
		return nil, apperr.New(apperr.CodeAppsReleaseNameDuplicate,
			"apps.application.release_taken",
			"(cluster, namespace, release_name) tuple already in use")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	m := &models.AppsApplication{
		Name:        req.Name,
		ClusterID:   req.ClusterID,
		Namespace:   req.Namespace,
		ReleaseName: req.ReleaseName,
		ChartRepoID: req.ChartRepoID,
		ChartName:   req.ChartName,
		Description: req.Description,
		Tags:        datatypes.NewJSONSlice[string](tags),
		OwnerUserID: req.OwnerUserID,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "apps.application.create", m.ID, ip, ua, map[string]any{
		"name":          m.Name,
		"cluster_id":    m.ClusterID,
		"namespace":     m.Namespace,
		"release_name":  m.ReleaseName,
		"chart_repo_id": m.ChartRepoID,
		"chart_name":    m.ChartName,
	})
	return s.Get(ctx, m.ID)
}

// Update mutates ONLY description / tags / owner_user_id. chart_repo_id is
// patched exclusively by SetChartRepo (used by release.Upgrade); the immutable
// fields (name, cluster_id, namespace, release_name, chart_name) cannot be
// changed through this path — UpdateRequest doesn't expose them.
func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found", "application not found")
		}
		return nil, err
	}

	fields := map[string]any{}
	var changed []string

	if req.Description != nil && *req.Description != m.Description {
		fields["description"] = *req.Description
		changed = append(changed, "description")
	}
	if req.Tags != nil {
		// Always wrap as JSONSlice so GORM goes through the Value() codec and
		// writes proper JSONB. nil is normalised to an empty slice.
		tags := req.Tags
		if tags == nil {
			tags = []string{}
		}
		fields["tags"] = datatypes.NewJSONSlice[string](tags)
		changed = append(changed, "tags")
	}
	if req.OwnerUserID != nil {
		// Pointer present always represents an intent to set (nil-pointer field
		// is "absent" thanks to omitempty).
		fields["owner_user_id"] = *req.OwnerUserID
		changed = append(changed, "owner_user_id")
	}

	if len(fields) > 0 {
		if err := s.repo.Update(ctx, id, fields); err != nil {
			return nil, err
		}
	}
	if len(changed) > 0 {
		s.writeAudit(ctx, ptrIfNonZero(actorID), "apps.application.update", id, ip, ua, map[string]any{
			"name":           m.Name,
			"changed_fields": changed,
		})
	}
	return s.Get(ctx, id)
}

// Delete soft-deletes one application. Refused with CodeAppsReleaseStillPresent
// if the HelmInstalledChecker seam reports the underlying helm release is still
// installed on the cluster.
func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "apps.application.not_found", "application not found")
		}
		return err
	}
	if s.checker != nil {
		installed, cerr := s.checker.IsReleaseInstalled(ctx, m)
		if cerr != nil {
			return cerr
		}
		if installed {
			return apperr.New(apperr.CodeAppsReleaseStillPresent,
				"apps.application.release_still_installed",
				"helm release still installed; uninstall before deleting the application record")
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "apps.application.delete", id, ip, ua, map[string]any{
		"name":         m.Name,
		"cluster_id":   m.ClusterID,
		"namespace":    m.Namespace,
		"release_name": m.ReleaseName,
	})
	return nil
}

// SetChartRepo is the narrow back-door release.Service.Upgrade uses to atomically
// repoint an application at a different chart repo. Not exposed over HTTP.
func (s *Service) SetChartRepo(ctx context.Context, id, newRepoID uint64) error {
	return s.repo.Update(ctx, id, map[string]any{"chart_repo_id": newRepoID})
}

// --- helpers ---------------------------------------------------------------

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     actor,
		Action:     action,
		TargetType: "apps.application",
		TargetID:   strconv.FormatUint(id, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

func ptrIfNonZero(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func toDetail(m *models.AppsApplication) *Detail {
	d := &Detail{
		Summary: Summary{
			ID:          m.ID,
			Name:        m.Name,
			ClusterID:   m.ClusterID,
			Namespace:   m.Namespace,
			ReleaseName: m.ReleaseName,
			ChartRepoID: m.ChartRepoID,
			ChartName:   m.ChartName,
			Description: m.Description,
			Tags:        []string(m.Tags),
			OwnerUserID: m.OwnerUserID,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
		},
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	if m.Cluster != nil {
		d.ClusterName = m.Cluster.Name
	}
	if m.OwnerUser != nil {
		d.OwnerName = m.OwnerUser.DisplayName
		if d.OwnerName == "" {
			d.OwnerName = m.OwnerUser.Username
		}
	}
	return d
}
