package release

import (
	"context"
	"strconv"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"sigs.k8s.io/yaml"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps"
	"optimus-be/internal/modules/audit"
)

// Factory is the narrow seam release.Service needs to build a per-request
// *action.Configuration. Satisfied implicitly by *helmclient.Factory. Defined
// here as an interface so unit tests can swap in an in-memory helm storage +
// fake KubeClient without touching kubeconfig / credentials wiring.
type Factory interface {
	NewForCluster(ctx context.Context, clusterID uint64, namespace, purpose string) (*action.Configuration, error)
}

// AppLookup is the narrow seam release.Service needs from application.Service.
// Satisfied implicitly by *application.Service. Defined as an interface so
// unit tests can plug in a stub that doesn't require a real database.
type AppLookup interface {
	GetModel(ctx context.Context, id uint64) (*models.AppsApplication, error)
	SetChartRepo(ctx context.Context, id, newRepoID uint64) error
}

// ChartLoader is the seam that fetches a chart .tgz from a chart repo and
// parses it into a *chart.Chart. Satisfied implicitly by *apps/repo.Service
// via its LoadChart method.
type ChartLoader interface {
	LoadChart(ctx context.Context, repoID uint64, chartName, version string) (*chart.Chart, error)
}

// Recorder is the narrow audit seam. Satisfied implicitly by *audit.Recorder.
// Declared as an interface so unit tests can drop in an in-memory capture
// without spinning up a database.
type Recorder interface {
	Record(ctx context.Context, e audit.Event) error
}

// Service wires the helm SDK action layer behind audit + a per-request
// action.Configuration. Safe to share across goroutines: every helm action
// is built from a fresh per-request *action.Configuration.
type Service struct {
	factory Factory
	apps    AppLookup
	loader  ChartLoader
	rec     Recorder
}

// NewService returns a release.Service. All four seams are required.
// Production main.go passes:
//   - factory: *helmclient.Factory built in T10.
//   - apps:    *application.Service built in T7.
//   - loader:  *apps/repo.Service built in T5 (its LoadChart method).
//   - rec:     *audit.Recorder shared with the rest of the system.
func NewService(factory Factory, apps AppLookup, loader ChartLoader, rec Recorder) *Service {
	if factory == nil {
		panic("release: NewService: factory is nil")
	}
	if apps == nil {
		panic("release: NewService: apps is nil")
	}
	if loader == nil {
		panic("release: NewService: loader is nil")
	}
	if rec == nil {
		panic("release: NewService: rec is nil")
	}
	return &Service{factory: factory, apps: apps, loader: loader, rec: rec}
}

// --- queries ---------------------------------------------------------------

// Status returns the live helm status for the application's release. Returns
// 42202 CodeAppsReleaseNotFound when the helm secret is absent.
func (s *Service) Status(ctx context.Context, appID uint64) (*ReleaseStatus, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		return nil, err
	}
	act := action.NewStatus(cfg)
	rel, err := act.Run(app.ReleaseName)
	if err != nil {
		return nil, apps.MapError(err)
	}
	return statusFromRelease(rel), nil
}

// History returns helm history for the application's release. An empty
// history (release never installed) returns a 42202 BizError via MapError.
func (s *Service) History(ctx context.Context, appID uint64) ([]RevisionRow, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.history")
	if err != nil {
		return nil, err
	}
	act := action.NewHistory(cfg)
	releases, err := act.Run(app.ReleaseName)
	if err != nil {
		return nil, apps.MapError(err)
	}
	out := make([]RevisionRow, 0, len(releases))
	for _, r := range releases {
		out = append(out, revRowFromRelease(r))
	}
	return out, nil
}

// --- mutations -------------------------------------------------------------

// Install runs helm install for the application's release. Returns
// CodeAppsReleaseAlreadyExists (42201) if the release is already installed
// (helm storage driver sentinel).
func (s *Service) Install(ctx context.Context, actorID uint64, ip, ua string, appID uint64, req InstallRequest) (*InstallResult, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	vals, err := parseValues(req.ValuesYAML)
	if err != nil {
		return nil, err
	}
	ch, err := s.loader.LoadChart(ctx, app.ChartRepoID, app.ChartName, req.ChartVersion)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.install")
	if err != nil {
		return nil, err
	}
	act := action.NewInstall(cfg)
	act.ReleaseName = app.ReleaseName
	act.Namespace = app.Namespace
	act.CreateNamespace = false
	act.Wait = false
	act.Atomic = false

	rel, err := act.RunWithContext(ctx, ch, vals)
	if err != nil {
		return nil, apps.MapError(err)
	}
	s.writeAudit(ctx, actorID, ip, ua, "apps.release.install", app.ID, map[string]any{
		"cluster_id":    app.ClusterID,
		"namespace":     app.Namespace,
		"release_name":  app.ReleaseName,
		"chart_version": req.ChartVersion,
		"revision":      rel.Version,
	})
	return installResultFromRelease(rel), nil
}

// Upgrade runs helm upgrade. If req.ChartRepoID is set and differs from the
// stored value, the application row is atomically repointed to the new repo
// before the helm action runs — the chart load also goes against the new
// repo so a values.yaml drift surfaces immediately.
func (s *Service) Upgrade(ctx context.Context, actorID uint64, ip, ua string, appID uint64, req UpgradeRequest) (*UpgradeResult, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	if req.ChartRepoID != nil && *req.ChartRepoID != app.ChartRepoID {
		if err := s.apps.SetChartRepo(ctx, app.ID, *req.ChartRepoID); err != nil {
			return nil, err
		}
		app.ChartRepoID = *req.ChartRepoID
	}
	vals, err := parseValues(req.ValuesYAML)
	if err != nil {
		return nil, err
	}
	ch, err := s.loader.LoadChart(ctx, app.ChartRepoID, app.ChartName, req.ChartVersion)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.upgrade")
	if err != nil {
		return nil, err
	}
	act := action.NewUpgrade(cfg)
	act.Namespace = app.Namespace
	act.Wait = false
	act.Atomic = false

	rel, err := act.RunWithContext(ctx, app.ReleaseName, ch, vals)
	if err != nil {
		return nil, apps.MapError(err)
	}
	s.writeAudit(ctx, actorID, ip, ua, "apps.release.upgrade", app.ID, map[string]any{
		"cluster_id":    app.ClusterID,
		"namespace":     app.Namespace,
		"release_name":  app.ReleaseName,
		"chart_version": req.ChartVersion,
		"revision":      rel.Version,
	})
	return installResultFromRelease(rel), nil
}

// Rollback runs helm rollback to the given revision. Helm wraps "no such
// revision" inside a generic error; we detect it via a substring match and
// surface 42203 CodeAppsReleaseHistoryTooShort. The new revision is read
// back via Status (helm's Rollback.Run is void on success).
func (s *Service) Rollback(ctx context.Context, actorID uint64, ip, ua string, appID uint64, req RollbackRequest) (*RollbackResult, error) {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.rollback")
	if err != nil {
		return nil, err
	}
	act := action.NewRollback(cfg)
	act.Version = req.Revision
	act.Wait = false

	if rerr := act.Run(app.ReleaseName); rerr != nil {
		// helm wraps "no such revision" inside a generic error of the form
		// "release has no N version". There is no typed sentinel for it, so
		// we fall back to a substring check. Catch both that text and the
		// older "revision ... not found" variant some helm versions emit.
		msg := strings.ToLower(rerr.Error())
		switch {
		case strings.Contains(msg, "has no") && strings.Contains(msg, "version"),
			strings.Contains(msg, "revision") && strings.Contains(msg, "not found"):
			return nil, apperr.Wrap(rerr, apperr.CodeAppsReleaseHistoryTooShort,
				"apps.release.revision_missing", rerr.Error())
		}
		return nil, apps.MapError(rerr)
	}
	// Read back the new revision via Status. If Status errors we still report
	// the rollback succeeded — the helm action returned nil, the caller can
	// re-fetch status later.
	st, _ := s.Status(ctx, app.ID)
	s.writeAudit(ctx, actorID, ip, ua, "apps.release.rollback", app.ID, map[string]any{
		"cluster_id":     app.ClusterID,
		"namespace":      app.Namespace,
		"release_name":   app.ReleaseName,
		"rolled_back_to": req.Revision,
	})
	if st == nil {
		return &RollbackResult{Revision: req.Revision, Status: "unknown"}, nil
	}
	return &RollbackResult{
		Revision:       st.Revision,
		Status:         st.Status,
		ChartVersion:   st.ChartVersion,
		LastDeployedAt: st.LastDeployedAt,
	}, nil
}

// Uninstall runs helm uninstall. KeepHistory=true preserves the helm secrets
// so a future rollback is possible. The Optimus application row is NOT
// deleted by this call — that's a separate DELETE /apps/applications/:id.
func (s *Service) Uninstall(ctx context.Context, actorID uint64, ip, ua string, appID uint64, req UninstallRequest) error {
	app, err := s.appsGet(ctx, appID)
	if err != nil {
		return err
	}
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.uninstall")
	if err != nil {
		return err
	}
	act := action.NewUninstall(cfg)
	act.KeepHistory = req.KeepHistory
	act.Wait = false

	if _, uerr := act.Run(app.ReleaseName); uerr != nil {
		return apps.MapError(uerr)
	}
	s.writeAudit(ctx, actorID, ip, ua, "apps.release.uninstall", app.ID, map[string]any{
		"cluster_id":   app.ClusterID,
		"namespace":    app.Namespace,
		"release_name": app.ReleaseName,
		"keep_history": req.KeepHistory,
	})
	return nil
}

// --- helpers ---------------------------------------------------------------

// appsGet pulls the AppsApplication model row (with associations) via the
// application service, returning a NotFound BizError if missing.
func (s *Service) appsGet(ctx context.Context, id uint64) (*models.AppsApplication, error) {
	return s.apps.GetModel(ctx, id)
}

func (s *Service) writeAudit(ctx context.Context, actorID uint64, ip, ua, action string, appID uint64, payload map[string]any) {
	if s.rec == nil {
		return
	}
	var uid *uint64
	if actorID != 0 {
		v := actorID
		uid = &v
	}
	_ = s.rec.Record(ctx, audit.Event{
		UserID:     uid,
		Action:     action,
		TargetType: "apps_application",
		TargetID:   strconv.FormatUint(appID, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

// parseValues parses a YAML body into a map. An empty / whitespace-only body
// is normalised to an empty map. A non-map root document (scalar, list, ...)
// returns CodeAppsReleaseInvalidValues (42205) — helm requires a top-level
// map for chart values.
func parseValues(y string) (map[string]any, error) {
	if strings.TrimSpace(y) == "" {
		return map[string]any{}, nil
	}
	// Decode into any first so we can distinguish "scalar / list at root" from
	// "non-YAML". sigs.k8s.io/yaml.Unmarshal goes through json, so map keys
	// will already be strings on success.
	var root any
	if err := yaml.Unmarshal([]byte(y), &root); err != nil {
		return nil, apperr.New(apperr.CodeAppsReleaseInvalidValues, "apps.release.invalid_values", err.Error())
	}
	if root == nil {
		return map[string]any{}, nil
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, apperr.New(apperr.CodeAppsReleaseInvalidValues, "apps.release.invalid_values",
			"values document must be a YAML map at the top level")
	}
	return m, nil
}

func statusFromRelease(r *release.Release) *ReleaseStatus {
	if r == nil {
		return &ReleaseStatus{Status: "unknown"}
	}
	return &ReleaseStatus{
		Status:         string(r.Info.Status),
		Revision:       r.Version,
		ChartVersion:   chartVersion(r),
		AppVersion:     chartAppVersion(r),
		LastDeployedAt: r.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
		Notes:          r.Info.Notes,
	}
}

func revRowFromRelease(r *release.Release) RevisionRow {
	return RevisionRow{
		Revision:     r.Version,
		Status:       string(r.Info.Status),
		ChartVersion: chartVersion(r),
		AppVersion:   chartAppVersion(r),
		UpdatedAt:    r.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
		Description:  r.Info.Description,
	}
}

func installResultFromRelease(r *release.Release) *InstallResult {
	return &InstallResult{
		Revision:       r.Version,
		Status:         string(r.Info.Status),
		ChartVersion:   chartVersion(r),
		LastDeployedAt: r.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// chartVersion / chartAppVersion guard against nil metadata — helm test
// fixtures sometimes elide it and a nil-deref here would crash the handler.
func chartVersion(r *release.Release) string {
	if r == nil || r.Chart == nil || r.Chart.Metadata == nil {
		return ""
	}
	return r.Chart.Metadata.Version
}

func chartAppVersion(r *release.Release) string {
	if r == nil || r.Chart == nil || r.Chart.Metadata == nil {
		return ""
	}
	return r.Chart.Metadata.AppVersion
}
